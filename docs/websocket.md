# WebSocket API

The server exposes a WebSocket endpoint for real-time room state updates and admin monitoring.

## Connection

```
ws://host:HTTP_PORT/ws
```

Example:
```javascript
const ws = new WebSocket("ws://127.0.0.1:12347/ws");
```

---

## Client Messages

### `subscribe`

Subscribe to a room's real-time updates.

```json
{
  "type": "subscribe",
  "roomId": "room1",
  "userId": 123
}
```

### `unsubscribe`

Unsubscribe from the current room.

```json
{
  "type": "unsubscribe"
}
```

### `ping`

Keep-alive ping.

```json
{"type": "ping"}
```

Server responds with:

```json
{"type": "pong"}
```

### `admin_subscribe`

Subscribe to global admin updates (requires admin token).

```json
{
  "type": "admin_subscribe",
  "token": "your-admin-token"
}
```

Server responds with:

```json
{"type": "admin_subscribed"}
```

---

## Server Pushes

### `subscribed`

Sent after a successful `subscribe`.

```json
{"type": "subscribed", "roomId": "room1"}
```

### `room_update`

Sent when the subscribed room's state changes.

```json
{"type": "room_update", "data": null}
```

> The Go server sends a `room_update` notification with `null` data. Clients should fetch the latest state via their TCP connection or HTTP API if needed.

### `room_log`

Sent when a new log message is added to the room.

```json
{
  "type": "room_log",
  "data": {
    "message": "Alice joined the room",
    "timestamp": 1716000000000
  }
}
```

### `admin_update`

Sent to admin subscribers when any room state changes.

> In the current Go implementation, admin WebSocket subscriptions receive `admin_subscribed` confirmation but room-level state pushes are sent via `room_update` on a per-room basis.

### `error`

Sent when the server rejects a message or encounters an error.

```json
{"type": "error", "message": "invalid-room-id"}
```

---

## Heartbeat

The server sends WebSocket ping frames every **30 seconds**. Clients should respond with pong frames or send `ping` messages to keep the connection alive. The read timeout is **60 seconds**.

---

## Python Example

```python
import asyncio
import json
import websockets

async def monitor():
    uri = "ws://127.0.0.1:12347/ws"
    async with websockets.connect(uri) as ws:
        # Subscribe to a room
        await ws.send(json.dumps({"type": "subscribe", "roomId": "room1", "userId": 1}))

        async for message in ws:
            data = json.loads(message)
            print(f"Received: {data}")

asyncio.run(monitor())
```

---

## JavaScript Example

```javascript
const ws = new WebSocket("ws://127.0.0.1:12347/ws");

ws.onopen = () => {
  ws.send(JSON.stringify({ type: "subscribe", roomId: "room1", userId: 1 }));
};

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  console.log("Received:", msg);
};
```
