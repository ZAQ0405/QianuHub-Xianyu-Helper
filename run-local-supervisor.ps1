# 本地 Windows 守护进程：服务退出后自动重启，不依赖 Docker。
$root = $PSScriptRoot
if (-not $root -and $MyInvocation.MyCommand.Path) { $root = Split-Path -Parent $MyInvocation.MyCommand.Path }
if (-not $root) { $root = (Get-Location).Path }
$server = Join-Path $root 'xianyu-server.exe'
$dataDir = Join-Path $root 'data'
$logDir = Join-Path $root 'logs'
$outLog = Join-Path $logDir 'server.out.log'
$errLog = Join-Path $logDir 'server.err.log'
$supervisorLog = Join-Path $logDir 'supervisor.log'

New-Item -ItemType Directory -Path $dataDir, $logDir -Force | Out-Null
$env:PLAYWRIGHT_DRIVER_PATH = Join-Path $dataDir 'playwright-driver'
$env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $dataDir 'playwright-browsers'
$env:PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH = 'C:\Program Files\Google\Chrome\Application\chrome.exe'

while ($true) {
    $startedAt = Get-Date -Format 'yyyy-MM-dd HH:mm:ss'
    try {
        $child = Start-Process -FilePath $server `
            -ArgumentList '-workdir', $dataDir, '-data-key-file', (Join-Path $dataDir 'data-key'), '-addr', '127.0.0.1:59188' `
            -WorkingDirectory $root `
            -RedirectStandardOutput $outLog `
            -RedirectStandardError $errLog `
            -WindowStyle Hidden `
            -PassThru
        Add-Content -LiteralPath $supervisorLog -Value "$startedAt server started pid=$($child.Id)"
        Wait-Process -Id $child.Id -ErrorAction SilentlyContinue
        $exitCode = $null
        try { $exitCode = $child.ExitCode } catch { $exitCode = 'unknown' }
        Add-Content -LiteralPath $supervisorLog -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') server exited pid=$($child.Id) code=$exitCode; restarting in 3s"
    } catch {
        Add-Content -LiteralPath $supervisorLog -Value "$(Get-Date -Format 'yyyy-MM-dd HH:mm:ss') supervisor error: $($_.Exception.Message); retrying in 3s"
    }
    Start-Sleep -Seconds 3
}
