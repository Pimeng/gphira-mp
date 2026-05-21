# HTTP API Reference

## Base URL

All HTTP endpoints are served on the configured `HTTP_PORT` (default `12347`).

```
http://host:HTTP_PORT
```

## Authentication

Admin endpoints require authentication via one of:

- Header: `X-Admin-Token: <token>`
- Header: `Authorization: Bearer <token>`
- Query parameter: `?token=<token>`

The token is configured via `ADMIN_TOKEN` in the config file or environment.

---

## Public Endpoints

### `GET /status`

Returns basic server status. Public — no authentication required.

**Response:**
```json
{
  "server_name": "Phira MP",
  "online": 42,
  "rooms": 5
}
```

> The detailed counters (`sessions`, `room_ids`) are only available on `/admin/status`.

### `GET /room`

Returns the public room list. Hidden rooms (RoomID starting with `_`) are excluded.

**Response:**
```json
{
  "total": 12,
  "rooms": [
    {
      "roomid": "room1",
      "cycle": false,
      "lock": false,
      "host": {"id": "1", "name": "Alice"},
      "state": "select_chart",
      "chart": {"id": "1234", "name": "Chart Name"},
      "players": [
        {"id": 1, "name": "Alice"},
        {"id": 2, "name": "Bob"}
      ]
    }
  ]
}
```

> `chart` is omitted when no chart is selected. `total` is the sum of `players.length` across all rooms.

### `GET /room-creation/config`

Returns whether room creation is enabled.

**Response:**
```json
{"ok": true, "enabled": true}
```

### `GET /replay/config`

Returns replay recording configuration.

**Response:**
```json
{"ok": true, "enabled": false}
```

### `GET /chart/:id`

Proxies a chart request to the Phira API with local caching.

**Response:**
```json
{"ok": true, "id": 123, "name": "Chart Name", "cache": false}
```

### `POST /replay/auth`

Exchange a Phira user token for a short-lived replay session token (TTL 30 min).

**Body:**
```json
{"token": "<phira-user-token>"}
```

**Response:**
```json
{
  "ok": true,
  "userId": 12345,
  "charts": [...],
  "sessionToken": "uuid-like-string",
  "expiresAt": 1700000000000
}
```

### `GET /replay/download`

Two modes:

- **User mode** (with `sessionToken` query): requires a valid replay session token from `/replay/auth`.
  Query: `?sessionToken=<token>&chartId=<id>&timestamp=<ms>`
- **Admin mode** (no `sessionToken`): falls through to admin path; requires admin token.
  Query: `?userId=<id>&chartId=<id>&timestamp=<ms>` + admin token (header or `?token=`).

Streamed at ~50 KB/s in 4 KB chunks. Returns the `.phirarec` file as `application/octet-stream`.

### `POST /replay/delete`

Delete one of the user's recorded or uploaded replays. Requires a session token.

**Body:**
```json
{"sessionToken": "...", "chartId": 123, "timestamp": 1700000000000}
```

### `POST /replay/upload`

Upload a replay file to the configured share-station. Requires a Phira user token.

**Body:**
```json
{"token": "<phira-user-token>", "chartId": 123, "timestamp": 1700000000000}
```

### `GET|POST /replay/auto-upload/config`

Get or update the per-user auto-upload visibility flag. GET uses `?token=`, POST uses body `{"token","show"}`.

### `POST /admin/otp/request`

Request a one-time admin OTP (only when `ADMIN_TOKEN` is not set). Returns an `ssid` + OTP printed to stdout.

**Body:** `{"mode": "otp"}` or `{"mode": "cli"}` (CLI mode requires operator approval via the interactive command line).

### `POST /admin/otp/verify`

Verify an OTP and obtain a temporary admin token (TTL 4 hours).

**Body:**
```json
{"ssid": "...", "otp": "...", "mode": "otp"}
```

---

## Admin Endpoints

### `GET /admin/status`

Returns server status including session and room id details.

**Response:**
```json
{
  "ok": true,
  "server_name": "Phira MP",
  "online": 42,
  "rooms": 5,
  "sessions": 42,
  "room_ids": ["room1", "room2"]
}
```

### `GET|POST /admin/replay/config`

Toggle the global replay recording flag. POST body: `{"enabled": true|false}`. Disabling ends recording on all live rooms and writes the change back to the config file.

### `GET|POST /admin/room-creation/config`

Toggle whether clients can create rooms. POST body: `{"enabled": true|false}`.

### `POST /admin/broadcast`

Broadcasts a system message (chat) to every room.

**Body:**
```json
{"message": "Hello everyone!"}
```

**Constraints:** non-empty, max 200 chars.

### `GET /admin/rooms`

Lists all rooms with full server-side detail (state machine fields, ready counts, results, monitors, contest config, recent logs). Field shape mirrors the `roomUpdate` WebSocket payload — see `internal/network/admin_views.go` for the authoritative struct.

### `POST /admin/rooms/:id/max_users`

Adjust a room's max capacity at runtime. Body: `{"maxUsers": 1..64}`.

### `POST /admin/rooms/:id/disband`

Forcibly disbands a room and disconnects all participants.

### `POST /admin/rooms/:id/chat`

Sends a chat message to all participants in a room. Body: `{"message": "..."}`, max 200 chars.

### `GET /admin/users`

Lists all online users.

### `GET /admin/users/:id`

Returns details for a specific user (id, name, monitor, connected, room, banned).

### `POST /admin/users/:id/disconnect`

Force-disconnects a user; cancels their in-progress play if any.

### `POST /admin/users/:id/move`

Move a user from one room to another. Both rooms must be in `select_chart` state and the user must be offline.

**Body:**
```json
{"roomId": "target", "monitor": false}
```

### `GET /admin/sessions`

Lists all active TCP sessions (id, user_id, user_name, remote_ip).

### `GET /admin/logs`

Returns recent logs for a specific room. Requires `?roomId=<id>` query parameter.

### `GET /admin/log-rate`

Returns the current log-throughput rate (lines/sec).

### `POST /admin/ban/user`

Bans (or unbans) a user server-wide. Field names use camelCase.

**Body:**
```json
{"userId": 123, "banned": true, "disconnect": true}
```

### `POST /admin/ban/room`

Bans (or unbans) a user from a specific room.

**Body:**
```json
{"userId": 123, "roomId": "room1", "banned": true}
```

### `GET /admin/ip-blacklist`

Returns the current IP blacklist with expiry times.

### `POST /admin/ip-blacklist/remove`

Removes an IP from the blacklist. Body: `{"ip": "1.2.3.4"}`.

### `POST /admin/ip-blacklist/clear`

Clears the entire IP blacklist.

---

## Contest Room Endpoints

### `POST /admin/contest/rooms/:id/config`

Enables or disables contest mode for a room.

**Body:**
```json
{"enabled": true, "whitelist": [1, 2, 3]}
```

- If `enabled` is `true` and `whitelist` is empty, all current participants are auto-added.
- If `enabled` is `false`, contest mode is removed.

**Response:**
```json
{"ok": true}
```

### `POST /admin/contest/rooms/:id/whitelist`

Updates the contest whitelist.

**Body:**
```json
{"userIds": [1, 2, 100]}
```

Current participants are automatically re-added to prevent accidental kicks.

**Response:**
```json
{"ok": true}
```

### `POST /admin/contest/rooms/:id/start`

Manually starts a contest game.

**Body:**
```json
{"force": false}
```

- Requires the room to be in `WaitingForReady` state with a chart selected.
- If `force` is `false`, all participants must have readied up.
- If `force` is `true`, the game starts regardless of ready status.

**Response:**
```json
{"ok": true}
```

---

## Error Responses

All endpoints return errors in this format:

```json
{"ok": false, "error": "error-code-or-message"}
```

Common HTTP status codes:

| Status | Meaning |
|--------|---------|
| 400 | Bad Request (invalid JSON or parameters) |
| 401 | Unauthorized (missing or invalid admin token) |
| 404 | Not Found (room or user does not exist) |
| 405 | Method Not Allowed |
| 502 | Bad Gateway (upstream Phira API error) |
