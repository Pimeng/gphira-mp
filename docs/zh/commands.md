# 服务器控制台命令

服务器提供一个交互式 CLI 用于实时管理。直接在服务器上运行时（不在 Docker 分离模式），在 `>` 提示符后输入命令。

命令不区分大小写。

---

## 房间管理

### `list`, `rooms`

列出所有活动房间。

```
> list
Room ID              Host       Users    Contest  State
----------------------------------------------------------------------
abc123               Alice      3        no       select_chart
xyz789               Bob        2        yes      waiting_for_ready
```

### `maxusers <room-id> <count>`

更改房间的最大用户数量。

```
> maxusers abc123 12
```

### `disband <room-id>`

强制解散房间。

```
> disband abc123
Room abc123 disbanded.
```

---

## 用户管理

### `users`

列出所有在线用户。

### `kick <user-id>`

按 ID 踢出用户。

```
> kick 123
User 123 kicked.
```

---

## 封禁管理

### `ban <user-id>`

在服务器范围内封禁用户。

```
> ban 123
Banned user 123
```

### `unban <user-id>`

解封用户。

```
> unban 123
Unbanned user 123
```

### `banlist`

列出所有被封禁的用户。

```
> banlist
Banned users:
  123
  456
```

---

## 通信

### `broadcast <message>`, `say <message>`

向所有在线用户广播消息。

```
> broadcast 服务器将在 5 分钟后重启。
Broadcast sent.
```

---

## 比赛房间

### `contest <room-id> enable [user-id...]`

为房间启用比赛模式。如果未提供用户 ID，则自动将所有当前参与者加入白名单。

```
> contest abc123 enable
Contest mode enabled for room abc123
```

```
> contest abc123 enable 100 200 300
Contest mode enabled for room abc123
```

### `contest <room-id> disable`

禁用比赛模式。

```
> contest abc123 disable
Contest mode disabled for room abc123
```

### `contest <room-id> whitelist <user-id>...`

更新比赛白名单。当前参与者会自动保留。

```
> contest abc123 whitelist 100 200
Contest whitelist updated for room abc123
```

### `contest <room-id> start [force]`

手动开始比赛游戏。

```
> contest abc123 start
Contest game started in room abc123
```

```
> contest abc123 start force
Contest game started in room abc123 (forced)
```

---

## 功能开关

### `replay on`, `replay off`, `replay status`

切换或检查回放录制状态。

### `roomcreation on`, `roomcreation off`, `roomcreation status`

切换或检查房间创建状态。

---

## 服务器控制

### `stop`, `exit`, `quit`

优雅地关闭服务器。

```
> stop
Stopping server...
```

---

## 注意事项

- **比赛房间配置仅保存在内存中**，服务器重启后会丢失。
- 封禁列表会持久化到 `admin_data.json`。
- 所有修改状态的命令都是线程安全的。
