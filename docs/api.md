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

Returns basic server status.

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

### `GET /room`

Returns the public room list.

**Response:**
```json
{
  "ok": true,
  "rooms": [
    {"id": "room1", "host_id": 1, "users": 3, "monitors": 1, "max": 8}
  ]
}
```

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
{"ok": true, "enabled": false, "auto_upload": false}
```

### `GET /chart/:id`

Proxies a chart request to the Phira API with local caching.

**Response:**
```json
{"ok": true, "id": 123, "name": "Chart Name", "cache": false}
```

### `GET /replay/download`

Downloads a replay file (admin only, rate-limited to 50 KB/s).

Requires `X-Admin-Token` and query parameter `?file=record/user/chart/timestamp.phirarec`.

---

## Admin Endpoints

### `GET /admin/status`

Same as public `/status` but requires admin token.

### `POST /admin/broadcast`

Broadcasts a system message to all online users.

**Body:**
```json
{"message": "Hello everyone!"}
```

### `GET /admin/rooms`

Lists all rooms with detailed info.

**Response:**
```json
{
  "ok": true,
  "rooms": [
    {
      "id": "room1",
      "host_id": 1,
      "users": [1, 2, 3],
      "monitors": [4],
      "state": "select_chart",
      "locked": false,
      "cycle": false,
      "contest": {
        "whitelist_count": 4,
        "whitelist": [1, 2, 3, 4],
        "manual_start": true,
        "auto_disband": true
      }
    }
  ]
}
```

> `contest` is omitted when contest mode is disabled.

### `POST /admin/rooms/:id/disband`

Forcibly disbands a room and disconnects all participants.

**Response:**
```json
{"ok": true, "roomid": "room1"}
```

### `POST /admin/rooms/:id/chat`

Sends a chat message to all participants in a room.

**Body:**
```json
{"message": "Admin says hi"}
```

### `GET /admin/users`

Lists all online users.

### `GET /admin/users/:id`

Returns details for a specific user.

### `GET /admin/sessions`

Lists all active TCP sessions.

### `GET /admin/logs`

Returns recent server logs.

### `POST /admin/ban/user`

Bans a user server-wide.

**Body:**
```json
{"user_id": 123}
```

### `POST /admin/ban/room`

Bans a user from a specific room.

**Body:**
```json
{"user_id": 123, "room_id": "room1"}
```

### `GET /admin/ip-blacklist`

Returns the current IP blacklist.

### `POST /admin/ip-blacklist/remove`

Removes an IP from the blacklist.

**Body:**
```json
{"ip": "192.168.1.1"}
```

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
