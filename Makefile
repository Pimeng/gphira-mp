# Phira-MP 服务器 Makefile
# 适用于 Linux/macOS

# 变量定义
BINARY_NAME := phira-mp-server
OUTPUT_DIR := build
PORT := 12346

# Go 参数
GOCMD := go
GOBUILD := $(GOCMD) build
GOCLEAN := $(GOCMD) clean
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod

# 编译参数
LDFLAGS := -s -w
CGO_ENABLED := 0

# 目标平台（可交叉编译）
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# 输出路径
BINARY_PATH := $(OUTPUT_DIR)/$(BINARY_NAME)
ifeq ($(GOOS),windows)
    BINARY_PATH := $(OUTPUT_DIR)/$(BINARY_NAME).exe
endif

# 默认目标
.PHONY: all build clean run test deps help cross-compile

all: clean build

## 构建项目
build:
	@echo "========================================"
	@echo "    Phira-MP 服务器编译"
	@echo "========================================"
	@echo ""
	@echo "[INFO] 检查 Go 环境..."
	@go version > /dev/null 2>&1 || (echo "[ERROR] 未找到 Go 环境" && exit 1)
	@echo "[SUCCESS] Go 环境正常"
	@echo ""
	@echo "[INFO] 创建输出目录..."
	@mkdir -p $(OUTPUT_DIR)
	@echo "[INFO] 目标平台: $(GOOS)/$(GOARCH)"
	@echo "[INFO] 开始编译..."
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BINARY_PATH) ./cmd/server/main.go
	@echo "[SUCCESS] 编译成功!"
	@echo "[INFO] 复制配置文件..."
	@cp -f server_config.yml $(OUTPUT_DIR)/ 2>/dev/null || echo "[WARNING] 未找到 server_config.yml"
	@echo ""
	@echo "========================================"
	@echo "[SUCCESS] 构建完成!"
	@echo "输出文件: $(BINARY_PATH)"
	@echo "========================================"

## 清理构建目录
clean:
	@echo "[INFO] 清理构建目录..."
	@rm -rf $(OUTPUT_DIR)
	@$(GOCLEAN)
	@echo "[SUCCESS] 清理完成"

## 构建并运行
run: build
	@echo ""
	@echo "[INFO] 启动服务器 (端口: $(PORT))..."
	@echo "========================================"
	@echo ""
	@./$(BINARY_PATH) -port $(PORT)

## 运行测试
test:
	@echo "[INFO] 运行测试..."
	@$(GOTEST) -v ./...

## 下载依赖
deps:
	@echo "[INFO] 下载依赖..."
	@$(GOMOD) download
	@$(GOMOD) tidy
	@echo "[SUCCESS] 依赖更新完成"

## 交叉编译 - Linux AMD64
build-linux:
	@echo "[INFO] 交叉编译 Linux AMD64..."
	@GOOS=linux GOARCH=amd64 $(MAKE) build

## 交叉编译 - Linux ARM64
build-linux-arm64:
	@echo "[INFO] 交叉编译 Linux ARM64..."
	@GOOS=linux GOARCH=arm64 $(MAKE) build

## 交叉编译 - Windows AMD64
build-windows:
	@echo "[INFO] 交叉编译 Windows AMD64..."
	@GOOS=windows GOARCH=amd64 $(MAKE) build

## 交叉编译 - macOS AMD64
build-darwin:
	@echo "[INFO] 交叉编译 macOS AMD64..."
	@GOOS=darwin GOARCH=amd64 $(MAKE) build

## 交叉编译 - macOS ARM64 (M1/M2)
build-darwin-arm64:
	@echo "[INFO] 交叉编译 macOS ARM64..."
	@GOOS=darwin GOARCH=arm64 $(MAKE) build

## 编译所有平台
cross-compile: clean
	@echo "[INFO] 开始交叉编译所有平台..."
	@mkdir -p $(OUTPUT_DIR)
	@# Linux AMD64
	@echo "[INFO] 编译 Linux AMD64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/server/main.go
	@# Linux ARM64
	@echo "[INFO] 编译 Linux ARM64..."
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/server/main.go
	@# Windows AMD64
	@echo "[INFO] 编译 Windows AMD64..."
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/server/main.go
	@# macOS AMD64
	@echo "[INFO] 编译 macOS AMD64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/server/main.go
	@# macOS ARM64
	@echo "[INFO] 编译 macOS ARM64..."
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" \
		-o $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/server/main.go
	@cp -f server_config.yml $(OUTPUT_DIR)/ 2>/dev/null || true
	@echo "[SUCCESS] 交叉编译完成!"
	@ls -lh $(OUTPUT_DIR)/

## 显示帮助信息
help:
	@echo "Phira-MP 服务器 Makefile"
	@echo ""
	@echo "用法: make [目标]"
	@echo ""
	@echo "目标:"
	@echo "  make build          - 编译项目 (默认)"
	@echo "  make clean          - 清理构建目录"
	@echo "  make run            - 编译并运行服务器"
	@echo "  make test           - 运行测试"
	@echo "  make deps           - 下载并整理依赖"
	@echo "  make build-linux    - 交叉编译 Linux AMD64"
	@echo "  make build-linux-arm64 - 交叉编译 Linux ARM64"
	@echo "  make build-windows  - 交叉编译 Windows AMD64"
	@echo "  make build-darwin   - 交叉编译 macOS AMD64"
	@echo "  make build-darwin-arm64 - 交叉编译 macOS ARM64"
	@echo "  make cross-compile  - 编译所有平台"
	@echo "  make help           - 显示此帮助信息"
	@echo ""
	@echo "环境变量:"
	@echo "  GOOS               - 目标操作系统 (linux, windows, darwin)"
	@echo "  GOARCH             - 目标架构 (amd64, arm64)"
	@echo "  PORT               - 服务器端口 (默认: 12346)"
	@echo ""
	@echo "示例:"
	@echo "  make build                    # 编译当前平台"
	@echo "  make run PORT=8080            # 编译并运行在 8080 端口"
	@echo "  make build-linux              # 交叉编译 Linux 版本"
	@echo "  GOOS=linux make build         # 同上"
