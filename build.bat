@echo off
chcp 65001 >nul
title Phira-MP 编译脚本

echo ========================================
echo    Phira-MP 服务器编译脚本
echo ========================================
echo.

:: 检查 Go 环境
echo [INFO] 检查 Go 环境...
go version >nul 2>&1
if errorlevel 1 (
    echo [ERROR] 未找到 Go 环境，请确保 Go 已正确安装并添加到 PATH
    pause
    exit /b 1
)
echo [SUCCESS] Go 环境正常
echo.

:: 设置变量
set "OUTPUT_DIR=build"
set "OUTPUT_NAME=phira-mp-server.exe"
set "PORT=12346"

:: 清理操作
if "%~1"=="clean" (
    echo [INFO] 清理构建目录...
    if exist "%OUTPUT_DIR%" (
        rmdir /s /q "%OUTPUT_DIR%"
        echo [SUCCESS] 已清理构建目录
    )
    pause
    exit /b 0
)

:: 创建输出目录
if not exist "%OUTPUT_DIR%" (
    mkdir "%OUTPUT_DIR%"
    echo [INFO] 创建输出目录: %OUTPUT_DIR%
)

echo [INFO] 开始编译...
echo [INFO] 输出文件: %OUTPUT_DIR%\%OUTPUT_NAME%

:: 执行编译
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64

go build -ldflags "-s -w" -o "%OUTPUT_DIR%\%OUTPUT_NAME%" .\cmd\server\main.go
if errorlevel 1 (
    echo [ERROR] 编译失败
    pause
    exit /b 1
)
echo [SUCCESS] 编译成功!

:: 复制配置文件
echo [INFO] 复制配置文件...
if exist "server_config.yml" (
    copy /Y "server_config.yml" "%OUTPUT_DIR%\" >nul
    echo [SUCCESS] 已复制 server_config.yml
) else (
    echo [WARNING] 未找到 server_config.yml
)

:: 显示文件信息
for %%I in ("%OUTPUT_DIR%\%OUTPUT_NAME%") do (
    echo [INFO] 输出文件大小: %%~zI 字节
)

echo.
echo ========================================
echo [SUCCESS] 构建完成! 输出目录: %CD%\%OUTPUT_DIR%
echo ========================================
echo.

:: 运行服务器
if "%~1"=="run" (
    echo [INFO] 启动服务器 (端口: %PORT%)...
    echo ========================================
    echo.
    "%OUTPUT_DIR%\%OUTPUT_NAME%" -port %PORT%
    pause
)
