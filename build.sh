#!/bin/bash

# Phira-MP 服务器编译脚本
# 适用于 Linux/macOS

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# 默认配置
OUTPUT_DIR="build"
OUTPUT_NAME="phira-mp-server"
PORT=12346

# 帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  -r, --run         编译并运行服务器"
    echo "  -c, --clean       清理构建目录"
    echo "  -o, --output      指定输出目录 (默认: build)"
    echo "  -n, --name        指定输出文件名 (默认: phira-mp-server)"
    echo "  -p, --port        指定服务器端口 (默认: 12346)"
    echo "  -h, --help        显示此帮助信息"
    echo ""
    echo "示例:"
    echo "  $0                # 仅编译"
    echo "  $0 -r             # 编译并运行"
    echo "  $0 -c             # 清理构建目录"
    echo "  $0 -o dist -n myserver  # 自定义输出目录和文件名"
}

# 日志函数
log_info() {
    echo -e "${CYAN}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 解析参数
RUN_AFTER_BUILD=false
CLEAN_MODE=false

while [[ $# -gt 0 ]]; do
    case $1 in
        -r|--run)
            RUN_AFTER_BUILD=true
            shift
            ;;
        -c|--clean)
            CLEAN_MODE=true
            shift
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -n|--name)
            OUTPUT_NAME="$2"
            shift 2
            ;;
        -p|--port)
            PORT="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            log_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
done

# 清理模式
if [ "$CLEAN_MODE" = true ]; then
    log_info "清理构建目录..."
    if [ -d "$OUTPUT_DIR" ]; then
        rm -rf "$OUTPUT_DIR"
        log_success "已清理构建目录"
    else
        log_warning "构建目录不存在"
    fi
    exit 0
fi

# 检查 Go 环境
log_info "检查 Go 环境..."
if ! command -v go &> /dev/null; then
    log_error "未找到 Go 环境，请确保 Go 已正确安装并添加到 PATH"
    exit 1
fi

GO_VERSION=$(go version)
log_success "Go 环境正常: $GO_VERSION"

# 创建输出目录
if [ ! -d "$OUTPUT_DIR" ]; then
    mkdir -p "$OUTPUT_DIR"
    log_info "创建输出目录: $OUTPUT_DIR"
fi

OUTPUT_PATH="$OUTPUT_DIR/$OUTPUT_NAME"

# 获取版本信息
GIT_COMMIT="unknown"
GIT_BRANCH="unknown"
if command -v git &> /dev/null && [ -d ".git" ]; then
    GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    GIT_BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
fi

BUILD_TIME=$(date '+%Y-%m-%d %H:%M:%S')

# 编译参数
LDFLAGS="-s -w"

log_info "开始编译..."
log_info "输出文件: $OUTPUT_PATH"
log_info "Git 分支: $GIT_BRANCH"
log_info "Git 提交: $GIT_COMMIT"
log_info "构建时间: $BUILD_TIME"

# 执行编译
export CGO_ENABLED=0

# 检测操作系统和架构
GOOS=${GOOS:-$(go env GOOS)}
GOARCH=${GOARCH:-$(go env GOARCH)}

log_info "目标平台: $GOOS/$GOARCH"

if ! go build -ldflags "$LDFLAGS" -o "$OUTPUT_PATH" ./cmd/server/main.go; then
    log_error "编译失败"
    exit 1
fi

log_success "编译成功!"

# 复制配置文件
log_info "复制配置文件..."
if [ -f "server_config.yml" ]; then
    cp -f "server_config.yml" "$OUTPUT_DIR/"
    log_success "已复制 server_config.yml"
else
    log_warning "未找到 server_config.yml"
fi

# 显示文件信息
if [ -f "$OUTPUT_PATH" ]; then
    FILE_SIZE=$(stat -c%s "$OUTPUT_PATH" 2>/dev/null || stat -f%z "$OUTPUT_PATH" 2>/dev/null || echo "0")
    SIZE_MB=$(echo "scale=2; $FILE_SIZE / 1024 / 1024" | bc 2>/dev/null || echo "0")
    log_info "输出文件大小: $FILE_SIZE 字节 (${SIZE_MB} MB)"
fi

echo ""
echo "========================================"
log_success "构建完成! 输出目录: $(cd "$OUTPUT_DIR" && pwd)"
echo "========================================"
echo ""

# 运行服务器
if [ "$RUN_AFTER_BUILD" = true ]; then
    log_info "启动服务器 (端口: $PORT)..."
    echo "========================================"
    echo ""
    "$OUTPUT_PATH" -port "$PORT"
fi
