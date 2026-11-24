@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo ========================================
echo  新闻滚动条构建脚本
echo ========================================
echo.

REM 检查 Go 是否安装
go version >nul 2>&1
if errorlevel 1 (
    echo [错误] 未检测到 Go，请先安装 Go
    echo 下载地址: https://go.dev/dl/
    pause
    exit /b 1
)

echo [1/2] 运行测试...
go test ./...
if errorlevel 1 exit /b 1

echo.
echo [2/2] 编译 Windows 图形程序...
go build -ldflags="-H windowsgui" -o ticker.exe ticker.go
if errorlevel 1 exit /b 1

echo.
echo 构建完成: %~dp0ticker.exe
pause
