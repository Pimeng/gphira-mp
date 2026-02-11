# Phira-MP Go 实现

这是 Phira 多人游戏服务器的 Go 语言实现版本，与 Rust 原版协议兼容。

## 快速开始

### 安装依赖

```bash
go mod tidy
```

### 编译

#### Windows

使用批处理脚本：
```batch
# 仅编译
.\build.bat

# 编译并运行
.\build.bat run

# 清理构建目录
.\build.bat clean
```

#### Linux / macOS

使用 Shell 脚本：
```bash
# 赋予执行权限
chmod +x build.sh

# 仅编译
./build.sh

# 编译并运行
./build.sh -r

# 清理构建目录
./build.sh -c

# 自定义端口运行
./build.sh -r -p 8080
```

使用 Makefile：
```bash
# 编译
make build

# 编译并运行
make run

# 清理
make clean

# 交叉编译 Linux 版本
make build-linux

# 编译所有平台
make cross-compile

# 查看所有选项
make help
```

### 运行服务器

```bash
# 直接运行（开发模式）
go run cmd/server/main.go -port 12346

# 运行编译后的版本
./build/phira-mp-server -port 12346
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

## API 支持

我们实现了很多使用的API，具体请参考：[API 文档](docs/api.md)

## 与 Rust 原版的差异

1. **并发模型**: Go 使用 goroutine + channel，Rust 使用 tokio
2. **错误处理**: Go 使用返回值，Rust 使用 Result 类型
3. **泛型**: Go 1.21+ 支持泛型，但语法与 Rust 不同
4. **宏**: Go 不使用宏生成序列化代码，而是手动实现