# Architecture Documentation

This document describes the overall architecture and core components of the Phira MP server Go rewrite.

## Project Structure

```
.
├── cmd/
│   ├── server/              # Main server entry point
│   └── bench/               # Benchmark tooling (connect / room / gameplay)
├── internal/
│   ├── cli/                 # Interactive admin CLI
│   ├── config/              # Server configuration loading
│   ├── game/                # Core game logic (Room, User, state machine)
│   ├── l10n/                # Localization (Fluent-based)
│   ├── network/             # TCP server, HTTP API, WebSocket, session management
│   ├── replay/              # Replay recording, auto-upload, cleanup
│   ├── state/               # Global server state (rooms, users, bans)
│   ├── utils/               # Logger, rate limiter, cache, Redis, share station
│   └── version/             # Build version info
├── pkg/
│   ├── client/              # Go client SDK (TCP + auto-reconnect + heartbeat)
│   ├── half/                # float16 encoding/decoding
│   ├── protocol/            # Binary protocol (commands, framing, codec)
│   ├── roomid/              # Room ID validation
│   └── stream/              # Framed TCP stream with batching and fast path
├── test/                    # Unit and integration tests
├── l10n/                    # Localization files (.ftl)
│   ├── zh-CN.ftl
│   └── en-US.ftl
├── docs/                    # Documentation
├── docker/                  # Docker support files
│   └── entrypoint.sh
│   └── Dockerfile
└── go.mod
```

## Core Components

### 1. Server State (`internal/state`)

Global state management including rooms, users, sessions, ban lists, and admin data.

**Key responsibilities:**
- Thread-safe maps for rooms, users, sessions
- Server-level and room-level ban lists
- Temporary admin OTP tokens
- Admin data persistence (`data/admin_data.json`)

**Key types:**
- `ServerState` — holds all global state with RWMutex
- `AdminData` — persisted admin records

### 2. Network Services (`internal/network`)

#### TCP Game Server
- Custom binary protocol over TCP
- Framed messages with LEB128 length prefix (max 2 MiB payload)
- Version handshake (protocol version 1)
- Batch send optimization (5 ms / 20 frames)
- High-priority fast path (Pong, Authenticate, state changes)
- HAProxy PROXY Protocol support (optional)
- Heartbeat: 3 s interval, 10 s disconnect timeout

#### HTTP API
- Public endpoints: `/status`, `/room`, `/room-creation/config`, `/replay/config`, `/chart/:id`, `/ws`, `/replay/auth`, `/replay/delete`, `/replay/upload`, `/replay/auto-upload/config`, `/replay/download` (user/admin dual-mode, 50 KB/s rate limit)
- Admin auth: `/admin/otp/request`, `/admin/otp/verify` (OTP / CLI approval flow)
- Admin endpoints: `/admin/status`, `/admin/replay/config`, `/admin/room-creation/config`, `/admin/broadcast`, `/admin/rooms`, `/admin/rooms/:id/(disband|chat|max_users)`, `/admin/users`, `/admin/users/:id/(disconnect|move)`, `/admin/sessions`, `/admin/logs`, `/admin/log-rate`, `/admin/ban/(user|room)`, `/admin/ip-blacklist/(remove|clear)`, `/admin/contest/rooms/:id/(config|whitelist|start)`
- Admin auth via permanent `ADMIN_TOKEN` or temporary token issued through OTP / CLI approval

#### WebSocket
- Real-time room state updates
- INFO-level log streaming
- Global admin monitoring (incremental updates)
- 30 s heartbeat

### 3. Game Logic (`internal/game`)

#### Room State Machine
```
SelectChart -> WaitingForReady -> Playing -> SelectChart
```

**States:**
- `SelectChart`: Host chooses a chart
- `WaitingForReady`: Players signal ready
- `Playing`: Game in progress; touches and judges streaming

**Room properties:**
- `maxUsers`: 1–64 (default 8)
- `live`: Live streaming room
- `locked`: Prevents new joins
- `cycle`: Rotate host after each game
- `replayEligible`: Enables replay recording
- `contest`: Contest mode (whitelist-only, manual start, auto-disband)

#### User Management
- `User` holds ID, Name, Language, Session, Room
- Dangle (disconnect-reconnect): 10 s grace period
  - Normal disconnect: retains room association, allows reconnect recovery
  - Playing / banned: immediate removal
- `MonitorBuffer` batches touch/judge events for spectators

### 4. Authentication

**Phira Token Verification**
- Client sends token on connect
- Server verifies against Phira API `/me` endpoint
- Configurable API endpoint and outbound proxy

**Admin Token Authentication**
- Permanent token via `ADMIN_TOKEN` config
- Temporary OTP (4-hour expiry, IP-bound)
- OTP flow: request code → verify → receive token

### 5. Replay System (`internal/replay`)

**Features:**
- Auto-records gameplay to `record/{userId}/{chartId}/{timestamp}.phirarec`
- File header: 2-byte magic (`0x504D`) + chart ID + user ID + record ID
- Daily cleanup of recordings older than 4 days
- Auto-upload to Share Station 30 s after game end
- Configurable per-user visibility (`show`)

### 6. Logging (`internal/utils/logger.go`)

- Levels: DEBUG, INFO, MARK, WARN, ERROR
- Console output with color
- Daily file rotation: `logs/YYYY-MM-DD.log`
- Connection log rate limiting: 10/min per IP; auto-blacklist for 5 min
- Test account filtering (skip file write unless DEBUG)

### 7. Caching

**Chart Cache (`internal/utils/cache.go`)**
- In-memory LRU (max 200 entries, 1 h TTL)
- JSON disk persistence at `data/cache/chart_cache.json`
- Optional Redis backend for distributed caching

**Rate Limiter (`internal/utils/rate_limiter.go`)**
- Token bucket per IP for connection logs
- Automatic blacklist on repeated overflow

### 8. Localization (`internal/l10n`)

- **Format**: Simple `key=value` text files with `.ftl` extension
- **Languages**: `zh-CN`, `en-US` (extensible)
- **Loading strategy** (two-tier fallback):
  1. **External file first**: Runtime reads from `l10n/{lang}.ftl` relative to working directory
  2. **Embedded fallback**: If external file is missing, uses built-in defaults compiled into the binary via `//go:embed`
- This allows hot-editing translations without recompiling, while keeping the binary self-contained

## Data Flow

### Client Connection Flow
```
1. TCP connection established
2. HAProxy PROXY Protocol parsed (optional)
3. Version byte exchanged (client sends 1, server accepts 1)
4. Authentication (Phira token → /me API)
5. Session and User created
6. Join or create room
```

### Game Flow
```
1. Host selects chart (SelectChart)
2. Host requests start (RequestStart)
3. State → WaitingForReady
4. Players send Ready
5. All ready → Playing
6. Players stream Touches/Judges
7. Players send Played result
8. Game ends → results broadcasted
9. Room disbands or cycles host (if cycle=true)
```

### WebSocket Push Flow
```
1. Client subscribes to room or admin channel
2. Current state pushed immediately
3. Incremental updates on state changes
4. INFO logs streamed in real time
```

## Client SDK (`pkg/client`)

The Go client SDK provides a full-featured TCP client:

- **Connection**: TCP dial + version handshake
- **Auto-reconnect**: Exponential backoff with jitter, restores auth + room
- **Adaptive heartbeat**: 5 s / 3 s / 1 s based on measured latency
- **RPC pattern**: All commands block until server reply or timeout
- **State tracking**: Room state, messages, live player buffers

```go
import "github.com/Pimeng/gphira-mp-next/pkg/client"

c, err := client.Connect("127.0.0.1", 12346, nil)
if err != nil { ... }
defer c.Close()

err = c.Authenticate("phira-token")
err = c.CreateRoom("myroom")
err = c.Ready()
```

## Benchmarking (`cmd/bench`)

Three benchmark modes:

1. **connect**: Sequential connections at controlled rate, measures connect + auth latency
2. **room**: Creates rooms, joins players/monitors, performs ready/chat actions
3. **gameplay**: Full gameplay simulation with high-frequency Touches/Judges at configurable Hz

Usage:
```bash
# Run server with CLI overrides
./gphira-mp --config config.yaml --port 12346 --httpService true --roomMaxUsers 12

# Benchmarks
go run ./cmd/bench connect --clients 100 --rate 20 --duration 30
go run ./cmd/bench room --rooms 10 --players-per-room 4 --duration 30
go run ./cmd/bench gameplay --rooms 5 --players-per-room 3 --hz 30 --duration 60
```

Reports are saved as JSON to `bench-results/`.

## Docker Support

Multi-stage build based on `golang:1.26-alpine` → `alpine:3.21`.

```bash
docker build -t phira-mp .
docker run -p 12346:12346 -p 12347:12347 phira-mp
```

Environment variables are mapped to `server_config.yml` via `docker/entrypoint.sh`.

## Security Mechanisms

- **Auth**: Phira token verification, admin token/OTP
- **Rate limiting**: Connection log throttling, IP auto-blacklist
- **Permissions**: Server bans, room bans, host-only actions
- **Validation**: Command param checking, state machine guards
- **Proxy support**: HTTP/HTTPS/SOCKS4/SOCKS5 outbound proxy for Phira API and Share Station

## Deployment Recommendations

- **Reverse proxy**: Nginx/Caddy for HTTPS; set `REAL_IP_HEADER` for real client IPs
- **TCP proxy**: HAProxy with `HAPROXY_PROTOCOL` enabled
- **Monitoring**: Watch log sizes, backup `admin_data.json`, monitor room/user counts
- **Redis**: Enable `REDIS_ENABLED=true` for distributed chart caching across multiple instances
