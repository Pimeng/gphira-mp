# WebSocket API

服务器暴露一个 WebSocket 端点，用于实时房间状态更新和管理监控。

## 连接

```
ws://host:HTTP_PORT/ws
```

示例：
```javascript
const ws = new WebSocket("ws://127.0.0.1:12347/ws");
```

---

## 客户端消息

### `subscribe`

订阅房间的实时更新。

```json
{
  "type": "subscribe",
  "roomId": "room1",
  "userId": 123
}
```

### `unsubscribe`

取消订阅当前房间。

```json
{
  "type": "unsubscribe"
}
```

### `ping`

保活 ping。

```json
{"type": "ping"}
```

服务器响应：

```json
{"type": "pong"}
```

### `admin_subscribe`

订阅全局管理更新（需要管理 token）。

```json
{
  "type": "admin_subscribe",
  "token": "your-admin-token"
}
```

服务器响应：

```json
{"type": "admin_subscribed"}
```

---

## 服务器推送

### `subscribed`

成功 `subscribe` 后发送。

```json
{"type": "subscribed", "roomId": "room1"}
```

### `room_update`

订阅房间状态变更时发送。

```json
{"type": "room_update", "data": null}
```

> Go 服务器发送一个 `data` 为 `null` 的 `room_update` 通知。客户端如有需要，应通过其 TCP 连接或 HTTP API 获取最新状态。

### `room_log`

房间添加新日志消息时发送。

```json
{
  "type": "room_log",
  "data": {
    "message": "Alice 加入了房间",
    "timestamp": 1716000000000
  }
}
```

### `admin_update`

任何房间状态变更时发送给管理订阅者。

> 在当前 Go 实现中，管理 WebSocket 订阅收到 `admin_subscribed` 确认，但房间级状态推送是按房间通过 `room_update` 发送的。

### `error`

服务器拒绝消息或遇到错误时发送。

```json
{"type": "error", "message": "invalid-room-id"}
```

---

## 心跳

服务器每 **30 秒**发送 WebSocket ping 帧。客户端应响应 pong 帧或发送 `ping` 消息以保持连接活动。读取超时为 **60 秒**。

---

## Python 示例

```python
import asyncio
import json
import websockets

async def monitor():
    uri = "ws://127.0.0.1:12347/ws"
    async with websockets.connect(uri) as ws:
        # 订阅房间
        await ws.send(json.dumps({"type": "subscribe", "roomId": "room1", "userId": 1}))

        async for message in ws:
            data = json.loads(message)
            print(f"Received: {data}")

asyncio.run(monitor())
```

---

## JavaScript 示例

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
