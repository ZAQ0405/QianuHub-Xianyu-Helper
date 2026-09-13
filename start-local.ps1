# 本地 Windows 启动脚本：使用 SQLite 和本机 Chrome，不依赖 Docker。
$root = $PSScriptRoot
if (-not $root -and $MyInvocation.MyCommand.Path) { $root = Split-Path -Parent $MyInvocation.MyCommand.Path }
if (-not $root) { $root = (Get-Location).Path }
$server = Join-Path $root 'xianyu-server.exe'
$supervisor = Join-Path $root 'run-local-supervisor.ps1'
$dataDir = Join-Path $root 'data'
$logDir = Join-Path $root 'logs'

if (!(Test-Path $server)) {
    throw "未找到 $server，请先执行 go build -o xianyu-server.exe ./cmd/server"
}

if (!(Test-Path $supervisor)) {
    throw "未找到 $supervisor"
}

$existingSupervisor = Get-CimInstance Win32_Process -Filter "Name = 'pwsh.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.CommandLine -like '*run-local-supervisor.ps1*' } |
    Select-Object -First 1
if ($existingSupervisor) {
    Write-Output "QianuHub 闲鱼助手已在运行，守护进程 PID=$($existingSupervisor.ProcessId)"
    Write-Output '管理后台：http://127.0.0.1:59188'
    exit 0
}

New-Item -ItemType Directory -Path $dataDir, $logDir -Force | Out-Null
$env:PLAYWRIGHT_DRIVER_PATH = Join-Path $dataDir 'playwright-driver'
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $dataDir 'playwright-browsers'
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = 'C:\Program Files\Google\Chrome\Application\chrome.exe'
$dbPath = Join-Path $dataDir 'xianyu_data.db'
$outLog = Join-Path $logDir 'server.out.log'
$errLog = Join-Path $logDir 'server.err.log'
$shell = (Get-Command pwsh.exe -ErrorAction SilentlyContinue | Select-Object -First 1).Source
if (-not $shell) { $shell = (Get-Command powershell.exe -ErrorAction Stop | Select-Object -First 1).Source }

$process = Start-Process -FilePath $shell `
    -ArgumentList '-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $supervisor `
    -WorkingDirectory $root `
    -WindowStyle Hidden `
    -PassThru

Write-Output "QianuHub 闲鱼助手已启动，守护进程 PID=$($process.Id)"
Write-Output '管理后台：http://127.0.0.1:59188'
