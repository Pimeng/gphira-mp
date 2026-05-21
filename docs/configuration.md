# Configuration Reference

Configuration can be provided via:

1. **Command-line arguments** (highest priority)
2. **Environment variables**
3. **Configuration file** (`config.yaml` by default)
4. **Built-in defaults** (lowest priority)

---

## Configuration File

Default path: `config.yaml`

All keys are **uppercase** in the YAML file:

```yaml
HOST: "::"
PORT: 12346
HTTP_SERVICE: false
HTTP_PORT: 12347
LOG_LEVEL: INFO
LANG: zh-CN
ROOM_MAX_USERS: 8
CHAT_ENABLED: true
REPLAY_ENABLED: false
ADMIN_TOKEN: "your-secure-token"
```

---

## Command-Line Arguments

```bash
./gphira-mp \
  --config config.yaml \
  --host 0.0.0.0 \
  --port 12346 \
  --httpService true \
  --httpPort 12347 \
  --roomMaxUsers 12 \
  --serverName "My Server" \
  --monitors "2,100"
```

| Flag | Short | Description |
|------|-------|-------------|
| `--config` | — | Path to configuration file |
| `--host` | — | Listen host address |
| `--port` | `-p` | Listen port (1-65535) |
| `--httpService` | — | Enable HTTP service (`true`/`false`) |
| `--httpPort` | — | HTTP service port (1-65535) |
| `--roomMaxUsers` | — | Max users per room (1-64) |
| `--serverName` | — | Server display name |
| `--monitors` | — | Monitor user IDs (comma-separated) |

---

## Environment Variables

Every config option has a corresponding environment variable with the same name:

| Variable | Type | Default |
|----------|------|---------|
| `HOST` | string | `::` |
| `PORT` | int | `12346` |
| `HTTP_SERVICE` | bool | `false` |
| `HTTP_PORT` | int | `12347` |
| `ROOM_MAX_USERS` | int | `8` |
| `ROOM_CREATION_ENABLED` | bool | `true` |
| `CHAT_ENABLED` | bool | `true` |
| `REPLAY_ENABLED` | bool | `false` |
| `REPLAY_BASE_DIR` | string | `./record` |
| `REPLAY_AUTO_UPLOAD` | bool | `false` |
| `MONITORS` | int[] | `[2]` |
| `TEST_ACCOUNT_IDS` | int[] | `[1739989]` |
| `SERVER_NAME` | string | `Phira MP` |
| `ADMIN_TOKEN` | string | *(empty)* |
| `ADMIN_DATA_PATH` | string | `./admin_data.json` |
| `ROOM_LIST_TIP` | string | *(empty)* |
| `LOG_LEVEL` | string | `INFO` |
| `REAL_IP_HEADER` | string | `X-Forwarded-For` |
| `HAPROXY_PROTOCOL` | bool | `false` |
| `LANG` | string | `zh-CN` |
| `PHIRA_API_ENDPOINT` | string | `https://phira.5wyxi.com` |
| `OUTBOUND_PROXY` | string \| false | *(empty)* |
| `HITOKOTO_API_URL` | string | `https://v1.hitokoto.cn/` |
| `SHARE_STATION_URL` | string | *(empty)* |
| `SHARE_STATION_TOKEN` | string | *(empty)* |
| `REDIS_ENABLED` | bool | `false` |
| `REDIS_HOST` | string | `127.0.0.1` |
| `REDIS_PORT` | int | `6379` |
| `REDIS_PASSWORD` | string | *(empty)* |
| `REDIS_DB` | int | `0` |

### Language & Localization

The `LANG` setting controls the server-side language for logs, CLI output, and system messages.

**Supported values:** `zh-CN`, `en-US`

**How it works:**
1. At startup, the server looks for an external language file at `l10n/{LANG}.ftl` (relative to the working directory).
2. If the file exists, translations are loaded from it — you can edit these files directly without recompiling.
3. If the file is missing, the server falls back to built-in defaults embedded in the binary.

**Customizing translations:**
```bash
# Edit or add translation keys
cat l10n/zh-CN.ftl
# log-server-starting=正在启动 GPhira MP 服务端
# cli-welcome=欢迎使用 GPhira MP CLI
```

To add a new language, create `l10n/xx-XX.ftl` and set `LANG: xx-XX` in your config.

### Outbound Proxy

The `OUTBOUND_PROXY` setting accepts:

- **Empty / unset**: Uses system default (may honor `HTTP_PROXY` env).
- **`false`**: Forces direct connection, bypassing any proxy.
- **`http://host:port`**: HTTP proxy.
- **`https://host:port`**: HTTPS proxy.
- **`socks://host:port`**: SOCKS5 proxy.
- **`socks4://host:port`**: SOCKS4 proxy.
- **`socks5://host:port`**: SOCKS5 proxy.

Authentication is supported via URL: `http://user:pass@host:port`.

---

## Example Configurations

### Development

```yaml
HOST: "127.0.0.1"
PORT: 12346
HTTP_SERVICE: true
HTTP_PORT: 12347
LOG_LEVEL: DEBUG
REPLAY_ENABLED: true
ADMIN_TOKEN: "dev-token"
```

### Production

```yaml
HOST: "0.0.0.0"
PORT: 12346
HTTP_SERVICE: true
HTTP_PORT: 12347
LOG_LEVEL: INFO
REAL_IP_HEADER: "X-Forwarded-For"
HAPROXY_PROTOCOL: true
REDIS_ENABLED: true
REDIS_HOST: "redis.internal"
```

### Docker

When running in Docker, use environment variables or mount a `config.yaml`:

```bash
docker run -e PORT=12346 -e HTTP_SERVICE=true -e ADMIN_TOKEN=secret phira-mp
```
