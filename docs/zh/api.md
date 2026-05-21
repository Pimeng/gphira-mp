# HTTP API 参考

## 基础 URL

所有 HTTP 端点均通过配置的 `HTTP_PORT` 提供服务（默认 `12347`）。

```
http://host:HTTP_PORT
```

## 认证

管理端点需要通过以下方式之一进行认证：

- Header: `X-Admin-Token: <token>`
- Header: `Authorization: Bearer <token>`
- 查询参数: `?token=<token>`

Token 通过配置文件或环境变量中的 `ADMIN_TOKEN` 进行配置。

---

## 公开端点

### `GET /status`

返回基本服务器状态。公开 — 无需认证。

**响应:**
```json
{
  "server_name": "Phira MP",
  "online": 42,
  "rooms": 5
}
```

> 详细的计数器（`sessions`、`room_ids`）仅在 `/admin/status` 上可用。

### `GET /room`

返回公开房间列表。隐藏房间（以 `_` 开头的 RoomID）被排除在外。

**响应:**
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

> 未选择谱面时 `chart` 字段被省略。`total` 是所有房间中 `players.length` 的总和。

### `GET /room-creation/config`

返回是否启用了房间创建功能。

**响应:**
```json
{"ok": true, "enabled": true}
```

### `GET /replay/config`

返回回放录制配置。

**响应:**
```json
{"ok": true, "enabled": false}
```

### `GET /chart/:id`

代理谱面请求到 Phira API，并带有本地缓存。

**响应:**
```json
{"ok": true, "id": 123, "name": "Chart Name", "cache": false}
```

### `POST /replay/auth`

将 Phira 用户 token 兑换为短期回放会话 token（有效期 30 分钟）。

**请求体:**
```json
{"token": "<phira-user-token>"}
```

**响应:**
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

两种模式：

- **用户模式**（带有 `sessionToken` 查询参数）：需要来自 `/replay/auth` 的有效回放会话 token。
  查询: `?sessionToken=<token>&chartId=<id>&timestamp=<ms>`
- **管理 mode**（无 `sessionToken`）：回退到管理路径；需要管理 token。
  查询: `?userId=<id>&chartId=<id>&timestamp=<ms>` + 管理 token（header 或 `?token=`）。

以约 50 KB/s 的速度、4 KB 分块进行流式传输。返回 `.phirarec` 文件，类型为 `application/octet-stream`。

### `POST /replay/delete`

删除用户录制或上传的回放之一。需要会话 token。

**请求体:**
```json
{"sessionToken": "...", "chartId": 123, "timestamp": 1700000000000}
```

### `POST /replay/upload`

将回放文件上传到配置的 share-station。需要 Phira 用户 token。

**请求体:**
```json
{"token": "<phira-user-token>", "chartId": 123, "timestamp": 1700000000000}
```

### `GET|POST /replay/auto-upload/config`

获取或更新每个用户的自动上传可见性标志。GET 使用 `?token=`，POST 使用请求体 `{"token","show"}`。

### `POST /admin/otp/request`

请求一次性管理员 OTP（仅在未设置 `ADMIN_TOKEN` 时可用）。返回一个 `ssid` + 输出到 stdout 的 OTP。

**请求体:** `{"mode": "otp"}` 或 `{"mode": "cli"}`（CLI 模式需要操作员通过交互式命令行批准）。

### `POST /admin/otp/verify`

验证 OTP 并获取临时管理 token（有效期 4 小时）。

**请求体:**
```json
{"ssid": "...", "otp": "...", "mode": "otp"}
```

---

## 管理端点

### `GET /admin/status`

返回包含会话和房间 ID 详细信息的服务器状态。

**响应:**
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

切换全局回放录制标志。POST 请求体: `{"enabled": true|false}`。禁用时将结束所有活动房间的录制，并将更改写回配置文件。

### `GET|POST /admin/room-creation/config`

切换客户端是否可以创建房间。POST 请求体: `{"enabled": true|false}`。

### `POST /admin/broadcast`

向每个房间广播系统消息（聊天）。

**请求体:**
```json
{"message": "Hello everyone!"}
```

**约束:** 非空，最多 200 个字符。

### `GET /admin/rooms`

列出所有房间及其完整的服务器端详细信息（状态机字段、准备计数、结果、监视器、比赛配置、最近日志）。字段结构与 `roomUpdate` WebSocket 负载相同 — 请参阅 `internal/network/admin_views.go` 获取权威的结构定义。

### `POST /admin/rooms/:id/max_users`

在运行时调整房间的最大容量。请求体: `{"maxUsers": 1..64}`。

### `POST /admin/rooms/:id/disband`

强制解散房间并断开所有参与者的连接。

### `POST /admin/rooms/:id/chat`

向房间中的所有参与者发送聊天消息。请求体: `{"message": "..."}`，最多 200 个字符。

### `GET /admin/users`

列出所有在线用户。

### `GET /admin/users/:id`

返回特定用户的详细信息（id、name、monitor、connected、room、banned）。

### `POST /admin/users/:id/disconnect`

强制断开用户连接；取消他们正在进行中的游戏（如果有）。

### `POST /admin/users/:id/move`

将用户从一个房间移动到另一个房间。两个房间都必须处于 `select_chart` 状态，且用户必须处于离线状态。

**请求体:**
```json
{"roomId": "target", "monitor": false}
```

### `GET /admin/sessions`

列出所有活动的 TCP 会话（id、user_id、user_name、remote_ip）。

### `GET /admin/logs`

返回特定房间的最近日志。需要 `?roomId=<id>` 查询参数。

### `GET /admin/log-rate`

返回当前的日志吞吐率（行/秒）。

### `POST /admin/ban/user`

在服务器范围内封禁（或解封）用户。字段名使用 camelCase。

**请求体:**
```json
{"userId": 123, "banned": true, "disconnect": true}
```

### `POST /admin/ban/room`

封禁（或解封）用户进入特定房间。

**请求体:**
```json
{"userId": 123, "roomId": "room1", "banned": true}
```

### `GET /admin/ip-blacklist`

返回当前带有过期时间的 IP 黑名单。

### `POST /admin/ip-blacklist/remove`

从黑名单中移除一个 IP。请求体: `{"ip": "1.2.3.4"}`。

### `POST /admin/ip-blacklist/clear`

清空整个 IP 黑名单。

---

## 比赛房间端点

### `POST /admin/contest/rooms/:id/config`

为房间启用或禁用比赛模式。

**请求体:**
```json
{"enabled": true, "whitelist": [1, 2, 3]}
```

- 如果 `enabled` 为 `true` 且 `whitelist` 为空，则自动添加所有当前参与者。
- 如果 `enabled` 为 `false`，则移除比赛模式。

**响应:**
```json
{"ok": true}
```

### `POST /admin/contest/rooms/:id/whitelist`

更新比赛白名单。

**请求体:**
```json
{"userIds": [1, 2, 100]}
```

当前参与者会自动重新添加，以防止意外踢出。

**响应:**
```json
{"ok": true}
```

### `POST /admin/contest/rooms/:id/start`

手动开始比赛游戏。

**请求体:**
```json
{"force": false}
```

- 要求房间处于 `WaitingForReady` 状态且已选择谱面。
- 如果 `force` 为 `false`，则所有参与者必须已准备就绪。
- 如果 `force` 为 `true`，则无论准备状态如何，游戏都会开始。

**响应:**
```json
{"ok": true}
```

---

## 错误响应

所有端点均按以下格式返回错误：

```json
{"ok": false, "error": "error-code-or-message"}
```

常见 HTTP 状态码：

| 状态码 | 含义 |
|--------|------|
| 400 | 错误请求（无效的 JSON 或参数） |
| 401 | 未授权（缺少或无效的管理 token） |
| 404 | 未找到（房间或用户不存在） |
| 405 | 方法不允许 |
| 502 | 错误网关（上游 Phira API 错误） |
