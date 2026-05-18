@echo off
setlocal enabledelayedexpansion

set APP_NAME=gphira-mp
set BENCH_NAME=bench
set BIN_DIR=build\bin
set DIST_DIR=build\dist

for /f "tokens=*" %%a in ('git describe --tags --always --dirty 2^>nul') do set VERSION=%%a
if "%VERSION%"=="" set VERSION=dev

set LDFLAGS=-s -w -X github.com/Pimeng/gphira-mp-next/internal/version.version=%VERSION%
set GOFLAGS=-trimpath

if "%~1"=="" goto :help

:loop
if "%~1"=="" goto :eof

if /I "%~1"=="help" goto :help
if /I "%~1"=="server" goto :server
if /I "%~1"=="bench" goto :bench
if /I "%~1"=="build" goto :build
if /I "%~1"=="run" goto :run
if /I "%~1"=="test" goto :test
if /I "%~1"=="fmt" goto :fmt
if /I "%~1"=="vet" goto :vet
if /I "%~1"=="clean" goto :clean
if /I "%~1"=="dist" goto :dist
if /I "%~1"=="version" goto :version

echo Unknown target: %~1
goto :help

:server
mkdir %BIN_DIR% 2>nul
go build %GOFLAGS% -ldflags "%LDFLAGS%" -o %BIN_DIR%\%APP_NAME%.exe ./cmd/server
if errorlevel 1 exit /b 1
echo [OK] server =^> %BIN_DIR%\%APP_NAME%.exe
shift
goto :loop

:bench
mkdir %BIN_DIR% 2>nul
go build %GOFLAGS% -ldflags "%LDFLAGS%" -o %BIN_DIR%\%BENCH_NAME%.exe ./cmd/bench
if errorlevel 1 exit /b 1
echo [OK] bench =^> %BIN_DIR%\%BENCH_NAME%.exe
shift
goto :loop

:build
call :server
call :bench
goto :eof

:run
call :server
if errorlevel 1 exit /b 1
%BIN_DIR%\%APP_NAME%.exe --config server_config.example.yml
goto :eof

:test
REM Windows race detector requires CGO (MinGW); skip by default
go test -v -count=1 ./...
goto :eof

:fmt
go fmt ./...
goto :eof

:vet
go vet ./...
goto :eof

:clean
if exist %BUILD_DIR% rmdir /s /q build
echo [OK] cleaned
goto :eof

:dist
mkdir %DIST_DIR% 2>nul
go build %GOFLAGS% -ldflags "%LDFLAGS%" -o %DIST_DIR%\%APP_NAME%-windows-amd64.exe ./cmd/server
go build %GOFLAGS% -ldflags "%LDFLAGS%" -o %DIST_DIR%\%BENCH_NAME%-windows-amd64.exe ./cmd/bench
echo [OK] dist =^> %DIST_DIR%
goto :eof

:version
echo %VERSION%
goto :eof

:help
echo Usage: make.bat [target ...]
echo.
echo Targets:
echo   help      Show this help message
echo   server    Build server binary
echo   bench     Build benchmark binary
echo   build     Build server + bench
echo   run       Build and run server with example config
echo   test      Run all tests
echo   fmt       Format Go source files
echo   vet       Run go vet
echo   clean     Remove build artifacts
echo   dist      Build release binaries for current platform
echo   version   Show current version
goto :eof
