# Phira-MP Go 实现

这是 Phira 多人游戏服务器的 Go 语言实现版本，与 Rust 原版协议兼容。

## 功能特性

- **协议兼容**: 与 Rust 原版 Phira-MP 完全协议兼容
- **二进制协议**: 使用 ULEB128 变长编码的高效二进制协议
- **房间系统**: 支持创建/加入/离开房间，房主转移
- **游戏状态机**: SelectChart -> WaitForReady -> Playing -> GameEnd
- **实时数据**: 支持触摸帧和判定事件的实时传输（直播模式）
- **心跳机制**: 自动心跳检测和连接管理
- **重连支持**: 支持断线重连恢复房间状态

## 快速开始

### 安装依赖

```bash
cd go-src
go mod tidy
```

### 运行服务器

```bash
go run cmd/server/main.go --port 12346
```

或使用配置文件：

```bash
# 创建 server_config.yml
echo "monitors:
  - 2" > server_config.yml

go run cmd/server/main.go
```

## 配置说明

### server_config.yml

```yaml
# 直播模式开关
live_mode: false

# 允许观察的用户ID列表（仅在直播模式启用时生效）
monitors:
  - 2

# 日志级别
log_level: info
```

## 与 Rust 原版的差异

1. **并发模型**: Go 使用 goroutine + channel，Rust 使用 tokio
2. **错误处理**: Go 使用返回值，Rust 使用 Result 类型
3. **泛型**: Go 1.21+ 支持泛型，但语法与 Rust 不同
4. **宏**: Go 不使用宏生成序列化代码，而是手动实现