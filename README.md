# GPhira-MP-Next

Phira MP 服务器的 **Go 重写版**，提供更小的部署体积、更高的并发性能和原生并发支持

> 本项目是 [tphira-mp](https://github.com/Pimeng/tphira-mp) 的 Go 语言重写，核心逻辑与协议格式保持一致，可互相兼容客户端。

---

## 技术栈

- **语言**: Go 1.26
- **协议**: 自定义二进制协议（LEB128 帧头，最大 2MiB 负载）
- **网络**: TCP 游戏服 + HTTP API + WebSocket 推送
- **缓存**: 内存 LRU + 可选 Redis 后端
- **构建**: Makefile / make.bat 统一构建，自动嵌入版本号，静态编译无运行时依赖

---

## 功能特性

- 🎮 **完整房间状态机**: SelectChart → WaitingForReady → Playing
- 🏆 **比赛房间 (Contest)**: 白名单准入、手动开始、自动解散
- 🔐 **双认证体系**: Phira Token (`/me` API) + Admin Token / OTP
- 📡 **WebSocket 实时推送**: 房间状态更新、INFO 日志流、管理员监控
- 📼 **回放系统**: 自动录制 `.phirarec`，支持自动上传到分享站
- 🌐 **出站代理**: 支持 HTTP/HTTPS/SOCKS4/SOCKS5 代理
- 🚀 **HAProxy PROXY Protocol**: 获取真实客户端 IP
- 🐳 **Docker 支持**: 多阶段构建，极小镜像体积
- 📊 **内置压测工具**: connect / room / gameplay 三种场景

---

## 快速开始

### 环境要求

- Go >= 1.26
- （可选）Redis >= 5（用于分布式缓存）

### 构建

```bash
# 克隆项目
git clone https://github.com/Pimeng/gphira-mp-next.git
cd gphira-mp-next

# 下载依赖
go mod download

# Linux / macOS
make server        # 编译服务端
make bench         # 编译压测工具
make build         # 同时编译 server + bench

# Windows
make.bat server
make.bat bench
make.bat build
```

编译产物输出到 `build/bin/` 目录。

### 运行

```bash
# Linux / macOS
make run           # 编译并运行（使用 server_config.example.yml）

# Windows
make.bat run

# 或直接运行已编译的二进制
./build/bin/gphira-mp
./build/bin/gphira-mp --config config.yaml
./build/bin/gphira-mp --port 12346 --httpService true --roomMaxUsers 12
```

### 配置文件

创建 `config.yaml`（简易示例，详细请看 [server_config.example.yml](server_config.example.yml) ）：

```yaml
HOST: "::"
PORT: 12346
HTTP_SERVICE: true
HTTP_PORT: 12347
LOG_LEVEL: INFO
LANG: zh-CN
ROOM_MAX_USERS: 8
CHAT_ENABLED: true
ADMIN_TOKEN: "your-secure-token"
```

完整配置文档见 [docs/configuration.md](docs/configuration.md)。

---

## CLI 参数

| 参数 | 简写 | 说明 |
|------|------|------|
| `--config` | — | 配置文件路径 |
| `--host` | — | 监听地址 |
| `--port` | `-p` | 监听端口 |
| `--httpService` | — | 启用 HTTP 服务 (`true`/`false`) |
| `--httpPort` | — | HTTP 端口 |
| `--roomMaxUsers` | — | 房间最大人数 (1-64) |
| `--serverName` | — | 服务器名称 |
| `--monitors` | — | 观战用户 ID（逗号分隔） |

配置优先级：**CLI 参数 > 环境变量 > 配置文件 > 默认值**

---

## Docker 部署

```bash
# 构建镜像
docker build -t gphira-mp .

# 运行
docker run -d \
  -p 12346:12346 \
  -p 12347:12347 \
  -e PORT=12346 \
  -e HTTP_SERVICE=true \
  -e ADMIN_TOKEN=your-token \
  phira-mp
```

---

## 项目结构

```
.
├── cmd/
│   ├── server/          # 服务端入口
│   └── bench/           # 压测工具
├── internal/
│   ├── cli/             # 交互式管理 CLI
│   ├── config/          # 配置加载与热重载
│   ├── game/            # 房间/用户/游戏状态机
│   ├── l10n/            # 本地化 (Fluent)
│   ├── network/         # TCP/HTTP/WebSocket/Session
│   ├── replay/          # 回放录制/上传/清理
│   ├── state/           # 全局状态 (rooms, users, bans)
│   ├── utils/           # 日志/缓存/限流/Redis/代理
│   └── version/         # 构建版本
├── pkg/
│   ├── client/          # Go 客户端 SDK
│   ├── half/            # float16 编解码
│   ├── protocol/        # 二进制协议
│   ├── roomid/          # 房间 ID 校验
│   └── stream/          # 带批处理的 TCP 流
├── test/                # 单元/集成测试
├── docs/                # 文档
├── l10n/                # 本地化文件 (.ftl)
└── docker/              # Docker 支持
```

---

## 测试

```bash
# Linux / macOS
make test

# Windows
make.bat test

# 或直接使用 Go
go test ./...
go test ./test -run Contest -v
```

---

## 压测

内置三种压测模式，无需真实 Phira Token：

```bash
# 连接压测：顺序连接 + 认证
go run ./cmd/bench connect --clients 100 --rate 20 --duration 30

# 房间压测：创建房间、加入玩家、准备/聊天
go run ./cmd/bench room --rooms 10 --players-per-room 4 --duration 30

# Gameplay 压测：完整对局模拟，高频 Touches/Judges
go run ./cmd/bench gameplay --rooms 5 --players-per-room 3 --hz 30 --duration 60
```

报告保存至 `bench-results/`。

---

## 文档

- [架构文档](docs/architecture.md) — 系统架构与核心组件
- [API 文档](docs/api.md) — HTTP API 完整参考
- [命令文档](docs/commands.md) — 交互式 CLI 命令手册
- [配置文档](docs/configuration.md) — 配置项详细说明
- [WebSocket 文档](docs/websocket.md) — 实时推送 API

---

## License

AGPL-3.0
