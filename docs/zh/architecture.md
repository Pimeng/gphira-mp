# 架构文档

本文档描述了 Phira MP 服务器 Go 语言重写的整体架构和核心组件。

## 项目结构

```
.
├── cmd/
│   ├── server/              # 主服务器入口点
│   └── bench/               # 基准测试工具（连接 / 房间 / 游戏）
├── internal/
│   ├── cli/                 # 交互式管理 CLI
│   ├── config/              # 服务器配置加载
│   ├── game/                # 核心游戏逻辑（房间、用户、状态机）
│   ├── l10n/                # 本地化（基于 Fluent）
│   ├── network/             # TCP 服务器、HTTP API、WebSocket、会话管理
│   ├── replay/              # 回放录制、自动上传、清理
│   ├── state/               # 全局服务器状态（房间、用户、封禁）
│   ├── utils/               # 日志记录器、速率限制器、缓存、Redis、分享站
│   └── version/             # 构建版本信息
├── pkg/
│   ├── client/              # Go 客户端 SDK（TCP + 自动重连 + 心跳）
│   ├── half/                # float16 编码/解码
│   ├── protocol/            # 二进制协议（命令、帧、编解码器）
│   ├── roomid/              # 房间 ID 验证
│   └── stream/              # 带批处理和快速路径的帧化 TCP 流
├── test/                    # 单元测试和集成测试
├── l10n/                    # 本地化文件（.ftl）
│   ├── zh-CN.ftl
│   └── en-US.ftl
├── docs/                    # 文档
├── docker/                  # Docker 支持文件
│   └── entrypoint.sh
│   └── Dockerfile
└── go.mod
```

## 核心组件

### 1. 服务器状态 (`internal/state`)

包括房间、用户、会话、封禁列表和管理数据的全局状态管理。

**主要职责：**
- 线程安全的房间、用户、会话映射
- 服务器级别和房间级别的封禁列表
- 临时管理 OTP token
- 管理数据持久化（`data/admin_data.json`）

**关键类型：**
- `ServerState` — 使用 RWMutex 保存所有全局状态
- `AdminData` — 持久化的管理记录

### 2. 网络服务 (`internal/network`)

#### TCP 游戏服务器
- 基于 TCP 的自定义二进制协议
- 使用 LEB128 长度前缀的帧化消息（最大 2 MiB 负载）
- 版本握手（协议版本 1）
- 批量发送优化（5 毫秒 / 20 帧）
- 高优先级快速路径（Pong、Authenticate、状态变更）
- HAProxy PROXY 协议支持（可选）
- 心跳：3 秒间隔，10 秒断开超时

#### HTTP API
- 公开端点：`/status`、`/room`、`/room-creation/config`、`/replay/config`、`/chart/:id`、`/ws`、`/replay/auth`、`/replay/delete`、`/replay/upload`、`/replay/auto-upload/config`、`/replay/download`（用户/管理双模式，50 KB/s 速率限制）
- 管理认证：`/admin/otp/request`、`/admin/otp/verify`（OTP / CLI 审批流程）
- 管理端点：`/admin/status`、`/admin/replay/config`、`/admin/room-creation/config`、`/admin/broadcast`、`/admin/rooms`、`/admin/rooms/:id/(disband|chat|max_users)`、`/admin/users`、`/admin/users/:id/(disconnect|move)`、`/admin/sessions`、`/admin/logs`、`/admin/log-rate`、`/admin/ban/(user|room)`、`/admin/ip-blacklist/(remove|clear)`、`/admin/contest/rooms/:id/(config|whitelist|start)`
- 通过永久 `ADMIN_TOKEN` 或 OTP / CLI 审批签发的临时 token 进行管理认证

#### WebSocket
- 实时房间状态更新
- INFO 级别日志流
- 全局管理监控（增量更新）
- 30 秒心跳

### 3. 游戏逻辑 (`internal/game`)

#### 房间状态机
```
SelectChart -> WaitingForReady -> Playing -> SelectChart
```

**状态：**
- `SelectChart`：房主选择谱面
- `WaitingForReady`：玩家发出准备信号
- `Playing`：游戏进行中；触摸和判定流

**房间属性：**
- `maxUsers`：1–64（默认 8）
- `live`：直播流房间
- `locked`：阻止新玩家加入
- `cycle`：每局游戏后轮换房主
- `replayEligible`：启用回放录制
- `contest`：比赛模式（仅白名单、手动开始、自动解散）

#### 用户管理
- `User` 保存 ID、Name、Language、Session、Room
- Dangle（断开-重连）：10 秒宽限期
  - 正常断开：保留房间关联，允许重连恢复
  - 游戏中 / 被封禁：立即移除
- `MonitorBuffer` 为观众批量处理触摸/判定事件

### 4. 认证

**Phira Token 验证**
- 客户端在连接时发送 token
- 服务器针对 Phira API `/me` 端点进行验证
- 可配置的 API 端点和出站代理

**管理 Token 认证**
- 通过 `ADMIN_TOKEN` 配置的永久 token
- 临时 OTP（4 小时有效期，IP 绑定）
- OTP 流程：请求代码 → 验证 → 接收 token

### 5. 回放系统 (`internal/replay`)

**功能：**
- 自动录制游戏到 `record/{userId}/{chartId}/{timestamp}.phirarec`
- 文件头：2 字节魔术数（`0x504D`）+ chart ID + user ID + record ID
- 每日清理超过 4 天的录制文件
- 游戏结束后 30 秒自动上传到分享站
- 可配置的每用户可见性（`show`）

### 6. 日志记录 (`internal/utils/logger.go`)

- 级别：DEBUG、INFO、MARK、WARN、ERROR
- 控制台输出带颜色
- 每日文件轮转：`logs/YYYY-MM-DD.log`
- 连接日志速率限制：每个 IP 10 条/分钟；自动列入黑名单 5 分钟
- 测试账户过滤（除非 DEBUG 否则跳过文件写入）

### 7. 缓存

**谱面缓存 (`internal/utils/cache.go`)**
- 内存 LRU（最大 200 条目，1 小时 TTL）
- JSON 磁盘持久化于 `data/cache/chart_cache.json`
- 可选 Redis 后端用于分布式缓存

**速率限制器 (`internal/utils/rate_limiter.go`)**
- 每个 IP 的连接日志令牌桶
- 重复溢出时自动列入黑名单

### 8. 本地化 (`internal/l10n`)

- **格式**：简单的 `key=value` 文本文件，扩展名为 `.ftl`
- **语言**：`zh-CN`、`en-US`（可扩展）
- **加载策略**（两级回退）：
  1. **外部文件优先**：运行时从工作目录相对的 `l10n/{lang}.ftl` 读取
  2. **嵌入式回退**：如果外部文件缺失，使用通过 `//go:embed` 编译到二进制文件中的内置默认值
- 这允许在不重新编译的情况下热编辑翻译，同时保持二进制文件自包含

## 数据流

### 客户端连接流程
```
1. 建立 TCP 连接
2. 解析 HAProxy PROXY 协议（可选）
3. 版本字节交换（客户端发送 1，服务器接受 1）
4. 认证（Phira token → /me API）
5. 创建会话和用户
6. 加入或创建房间
```

### 游戏流程
```
1. 房主选择谱面（SelectChart）
2. 房主请求开始（RequestStart）
3. 状态 → WaitingForReady
4. 玩家发送准备
5. 全部准备 → Playing
6. 玩家流式传输 Touches/Judges
7. 玩家发送 Played 结果
8. 游戏结束 → 广播结果
9. 房间解散或轮换房主（如果 cycle=true）
```

### WebSocket 推送流程
```
1. 客户端订阅房间或管理频道
2. 立即推送当前状态
3. 状态变更时增量更新
4. 实时流式传输 INFO 日志
```

## 客户端 SDK (`pkg/client`)

Go 客户端 SDK 提供了一个功能齐全的 TCP 客户端：

- **连接**：TCP 拨号 + 版本握手
- **自动重连**：带抖动的指数退避，恢复认证 + 房间
- **自适应心跳**：基于测量的延迟，5 秒 / 3 秒 / 1 秒
- **RPC 模式**：所有命令阻塞直到服务器回复或超时
- **状态跟踪**：房间状态、消息、实时玩家缓冲区

```go
import "github.com/Pimeng/gphira-mp-next/pkg/client"

c, err := client.Connect("127.0.0.1", 12346, nil)
if err != nil { ... }
defer c.Close()

err = c.Authenticate("phira-token")
err = c.CreateRoom("myroom")
err = c.Ready()
```

## 基准测试 (`cmd/bench`)

三种基准测试模式：

1. **connect**：以受控速率进行顺序连接，测量连接 + 认证延迟
2. **room**：创建房间、加入玩家/监视器、执行准备/聊天操作
3. **gameplay**：以可配置 Hz 进行高频 Touches/Judges 的完整游戏模拟

用法：
```bash
# 使用 CLI 覆盖运行服务器
./gphira-mp --config config.yaml --port 12346 --httpService true --roomMaxUsers 12

# 基准测试
go run ./cmd/bench connect --clients 100 --rate 20 --duration 30
go run ./cmd/bench room --rooms 10 --players-per-room 4 --duration 30
go run ./cmd/bench gameplay --rooms 5 --players-per-room 3 --hz 30 --duration 60
```

报告以 JSON 格式保存到 `bench-results/`。

## Docker 支持

基于 `golang:1.26-alpine` → `alpine:3.21` 的多阶段构建。

```bash
docker build -t phira-mp .
docker run -p 12346:12346 -p 12347:12347 phira-mp
```

环境变量通过 `docker/entrypoint.sh` 映射到 `server_config.yml`。

## 安全机制

- **认证**：Phira token 验证、管理 token/OTP
- **速率限制**：连接日志节流、IP 自动黑名单
- **权限**：服务器封禁、房间封禁、仅限房主的操作
- **验证**：命令参数检查、状态机守卫
- **代理支持**：用于 Phira API 和分享站的 HTTP/HTTPS/SOCKS4/SOCKS5 出站代理

## 部署建议

- **反向代理**：Nginx/Caddy 用于 HTTPS；设置 `REAL_IP_HEADER` 以获取真实客户端 IP
- **TCP 代理**：启用 `HAPROXY_PROTOCOL` 的 HAProxy
- **监控**：关注日志大小、备份 `admin_data.json`、监控房间/用户数量
- **Redis**：在多实例之间启用 `REDIS_ENABLED=true` 进行分布式谱面缓存
