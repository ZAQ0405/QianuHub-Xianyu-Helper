@echo off
setlocal

rem 切换到脚本目录，确保从桌面双击时仍能找到项目文件。
set "ROOT=%~dp0"
pushd "%ROOT%"

where pwsh >nul 2>nul
if not errorlevel 1 (
    pwsh -NoProfile -ExecutionPolicy Bypass -File "%ROOT%start-local.ps1"
) else (
    powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%ROOT%start-local.ps1"
)

if errorlevel 1 (
    echo.
    echo 启动失败，请检查 PowerShell、xianyu-server.exe 和 logs 目录。
    pause
    popd
    exit /b 1
)

echo.
echo 正在打开管理后台：http://127.0.0.1:59188
start "" "http://127.0.0.1:59188/"
popd
exit /b 0
