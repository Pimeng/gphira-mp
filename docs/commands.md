# Server Console Commands

The server provides an interactive CLI for real-time management. Type commands after the `>` prompt when running the server directly (not in Docker detached mode).

Commands are case-insensitive.

---

## Room Management

### `list`, `rooms`

Lists all active rooms.

```
> list
Room ID              Host       Users    Contest  State
----------------------------------------------------------------------
abc123               Alice      3        no       select_chart
xyz789               Bob        2        yes      waiting_for_ready
```

### `maxusers <room-id> <count>`

Changes the maximum number of users for a room.

```
> maxusers abc123 12
```

### `disband <room-id>`

Forcibly disbands a room.

```
> disband abc123
Room abc123 disbanded.
```

---

## User Management

### `users`

Lists all online users.

### `kick <user-id>`

Kicks a user by ID.

```
> kick 123
User 123 kicked.
```

---

## Ban Management

### `ban <user-id>`

Bans a user server-wide.

```
> ban 123
Banned user 123
```

### `unban <user-id>`

Unbans a user.

```
> unban 123
Unbanned user 123
```

### `banlist`

Lists all banned users.

```
> banlist
Banned users:
  123
  456
```

---

## Communication

### `broadcast <message>`, `say <message>`

Broadcasts a message to all online users.

```
> broadcast Server will restart in 5 minutes.
Broadcast sent.
```

---

## Contest Rooms

### `contest <room-id> enable [user-id...]`

Enables contest mode for a room. If no user IDs are provided, all current participants are whitelisted.

```
> contest abc123 enable
Contest mode enabled for room abc123
```

```
> contest abc123 enable 100 200 300
Contest mode enabled for room abc123
```

### `contest <room-id> disable`

Disables contest mode.

```
> contest abc123 disable
Contest mode disabled for room abc123
```

### `contest <room-id> whitelist <user-id>...`

Updates the contest whitelist. Current participants are automatically preserved.

```
> contest abc123 whitelist 100 200
Contest whitelist updated for room abc123
```

### `contest <room-id> start [force]`

Manually starts the contest game.

```
> contest abc123 start
Contest game started in room abc123
```

```
> contest abc123 start force
Contest game started in room abc123 (forced)
```

---

## Feature Toggles

### `replay on`, `replay off`, `replay status`

Toggles or checks replay recording status.

### `roomcreation on`, `roomcreation off`, `roomcreation status`

Toggles or checks room creation status.

---

## Server Control

### `stop`, `exit`, `quit`

Gracefully shuts down the server.

```
> stop
Stopping server...
```

---

## Notes

- **Contest room configuration is memory-only** and lost on server restart.
- Ban lists are persisted to `admin_data.json`.
- All commands that modify state are thread-safe.
