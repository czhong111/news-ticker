@echo off
chcp 65001 >nul
cd /d "%~dp0"
if not exist ticker.exe (
    echo [错误] 找不到 ticker.exe，请先运行 build.bat
    pause
    exit /b 1
)
start "" "%~dp0ticker.exe"
