[CmdletBinding()]
param()

Set-StrictMode -Version 2.0
$ErrorActionPreference = "Stop"

$projectRoot = $PSScriptRoot
$backendDir = Join-Path $projectRoot "backend"
$frontendDir = Join-Path $projectRoot "frontend"
$runDir = Join-Path $projectRoot ".run"
$runBin = Join-Path $runDir "fakemex.exe"
$configPath = if ($env:FAKEMEX_CONFIG) { $env:FAKEMEX_CONFIG } else { Join-Path $projectRoot "config\local.env" }
$backendHost = "127.0.0.1"
$backendPort = 8080
$frontendHost = "127.0.0.1"
$frontendPortDefault = 4200
$readyTimeout = 60

$backendLog = Join-Path $runDir "backend.log"
$backendErrorLog = Join-Path $runDir "backend-error.log"
$frontendLog = Join-Path $runDir "frontend.log"
$frontendErrorLog = Join-Path $runDir "frontend-error.log"
$backendPidFile = Join-Path $runDir "backend.pid"
$frontendPidFile = Join-Path $runDir "frontend.pid"
$backendProcess = $null
$frontendProcess = $null
$frontendPort = $frontendPortDefault

function Fail([string]$Message) {
    throw "launch.ps1: $Message"
}

function Require-Command([string]$Name) {
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($null -eq $command) {
        Fail "required command not found: $Name"
    }
    return $command
}

function Read-Config([string]$Path) {
    $values = @{}
    foreach ($rawLine in [System.IO.File]::ReadAllLines($Path)) {
        $line = ($rawLine -replace '#.*$', '').Trim()
        if (-not $line -or $line -notmatch '^([^=]+)=(.*)$') {
            continue
        }
        $key = $matches[1].Trim()
        $value = $matches[2].Trim()
        if ($value.Length -ge 2) {
            $first = $value.Substring(0, 1)
            $last = $value.Substring($value.Length - 1, 1)
            if (($first -eq '"' -and $last -eq '"') -or ($first -eq "'" -and $last -eq "'")) {
                $value = $value.Substring(1, $value.Length - 2)
            }
        }
        $values[$key] = $value
    }
    return $values
}

function Test-ProcessRunning([int]$Id) {
    return $null -ne (Get-Process -Id $Id -ErrorAction SilentlyContinue)
}

function Get-ProcessDetails([int]$Id) {
    return Get-CimInstance Win32_Process -Filter "ProcessId = $Id" -ErrorAction SilentlyContinue
}

function Test-ServiceProcess([int]$Id, [string]$Service) {
    if (-not (Test-ProcessRunning $Id)) {
        return $false
    }
    $details = Get-ProcessDetails $Id
    if ($null -eq $details) {
        return $false
    }
    if ($Service -eq "backend") {
        return $details.ExecutablePath -and [System.IO.Path]::GetFullPath($details.ExecutablePath) -eq [System.IO.Path]::GetFullPath($runBin)
    }
    if ($Service -eq "frontend") {
        $nodePath = (Get-Command "node.exe" -ErrorAction SilentlyContinue).Source
        $ngPath = Join-Path $frontendDir "node_modules\@angular\cli\bin\ng.js"
        return $details.ExecutablePath -and $nodePath -and
            ([System.IO.Path]::GetFullPath($details.ExecutablePath) -eq [System.IO.Path]::GetFullPath($nodePath)) -and
            $details.CommandLine -and $details.CommandLine.Contains($ngPath) -and
            $details.CommandLine.Contains("--port $frontendPort")
    }
    return $false
}

function Remove-PidFileIfOwned([string]$Path, [int]$Id) {
    if (Test-Path -LiteralPath $Path) {
        $recorded = ([System.IO.File]::ReadAllText($Path)).Trim()
        if ($recorded -eq $Id.ToString()) {
            Remove-Item -LiteralPath $Path -Force
        }
    }
}

function Stop-ServiceProcess([int]$Id, [string]$Service, [switch]$KnownChild) {
    if (-not (Test-ProcessRunning $Id)) {
        return
    }
    if (-not $KnownChild -and -not (Test-ServiceProcess $Id $Service)) {
        Fail "refusing to stop process $Id because it is not an owned FakeMex $Service process"
    }
    Stop-Process -Id $Id -ErrorAction SilentlyContinue
    $deadline = [DateTime]::UtcNow.AddSeconds(5)
    while ((Test-ProcessRunning $Id) -and [DateTime]::UtcNow -lt $deadline) {
        Start-Sleep -Milliseconds 200
    }
    if (Test-ProcessRunning $Id) {
        Stop-Process -Id $Id -Force -ErrorAction SilentlyContinue
    }
    $deadline = [DateTime]::UtcNow.AddSeconds(3)
    while ((Test-ProcessRunning $Id) -and [DateTime]::UtcNow -lt $deadline) {
        Start-Sleep -Milliseconds 200
    }
    if (Test-ProcessRunning $Id) {
        Fail "could not stop $Service process ($Id)"
    }
}

function Stop-RecordedService([string]$Path, [string]$Service) {
    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }
    $recorded = ([System.IO.File]::ReadAllText($Path)).Trim()
    $parsedId = 0
    if (-not [int]::TryParse($recorded, [ref]$parsedId) -or -not (Test-ProcessRunning $parsedId)) {
        Remove-Item -LiteralPath $Path -Force
        return
    }
    if (-not (Test-ServiceProcess $parsedId $Service)) {
        Write-Warning "Ignoring stale $Service PID file; process $parsedId is not owned by this project."
        Remove-Item -LiteralPath $Path -Force
        return
    }
    Write-Host "Stopping existing FakeMex $Service process ($parsedId)..."
    Stop-ServiceProcess $parsedId $Service -KnownChild
    Remove-Item -LiteralPath $Path -Force -ErrorAction SilentlyContinue
}

function Get-ListenerProcessIds([int]$Port) {
    return @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
        Select-Object -ExpandProperty OwningProcess -Unique)
}

function Stop-ServiceOnPort([string]$Service, [int]$Port) {
    $owners = @(Get-ListenerProcessIds $Port)
    foreach ($ownerId in $owners) {
        if (-not (Test-ServiceProcess $ownerId $Service)) {
            Fail "$Service port $Port is used by an unrelated process ($ownerId); refusing to stop it"
        }
    }
    foreach ($ownerId in $owners) {
        Write-Host "Stopping existing FakeMex $Service process ($ownerId)..."
        Stop-ServiceProcess $ownerId $Service -KnownChild
    }
    if (@(Get-ListenerProcessIds $Port).Count -gt 0) {
        Fail "$Service port $Port is still in use"
    }
}

function Wait-ForHttp([string]$Url, [string]$Label) {
    $deadline = [DateTime]::UtcNow.AddSeconds($readyTimeout)
    while ([DateTime]::UtcNow -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 1
            if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 400) {
                Write-Host "$Label ready: $Url"
                return
            }
        } catch {
            Start-Sleep -Seconds 1
        }
    }
    Fail "timed out waiting for ${Label}: $Url"
}

function Quote-ProcessArgument([string]$Value) {
    return '"' + $Value.Replace('"', '\"') + '"'
}

$exitCode = 0
try {
    $goCommand = Require-Command "go.exe"
    $npmCommand = Require-Command "npm.cmd"
    $nodeCommand = Require-Command "node.exe"
    Require-Command "Get-NetTCPConnection" | Out-Null
    Require-Command "Get-CimInstance" | Out-Null

    if ($env:FAKEMEX_READY_TIMEOUT) {
        if (-not [int]::TryParse($env:FAKEMEX_READY_TIMEOUT, [ref]$readyTimeout) -or $readyTimeout -lt 1) {
            Fail "FAKEMEX_READY_TIMEOUT must be a positive number of seconds"
        }
    }
    $configPath = [System.IO.Path]::GetFullPath($configPath)
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        Fail "missing config file: $configPath"
    }

    $config = Read-Config $configPath
    foreach ($key in @("SERVER_ADDR", "FRONTEND_ORIGIN")) {
        if (-not $config.ContainsKey($key) -or -not $config[$key]) {
            Fail "config missing required key: $key"
        }
    }
    if ($config.ContainsKey("TRADING_ENABLED")) {
        $tradingEnabled = $config["TRADING_ENABLED"].ToLowerInvariant()
        if ($tradingEnabled -ne "true" -and $tradingEnabled -ne "false") {
            Fail "TRADING_ENABLED must be true or false"
        }
    }

    if ($config["SERVER_ADDR"] -notmatch '^(.+):(\d+)$') {
        Fail "invalid SERVER_ADDR in config: $($config['SERVER_ADDR'])"
    }
    $configuredBackendHost = $matches[1].Trim('[', ']')
    $configuredBackendPort = [int]$matches[2]
    if ($configuredBackendHost -notin @("127.0.0.1", "localhost", "::1")) {
        Fail "SERVER_ADDR must bind to loopback"
    }
    if ($configuredBackendPort -ne $backendPort) {
        Fail "backend must use loopback port $backendPort to match frontend proxy; got $configuredBackendPort"
    }

    try {
        $frontendOrigin = [Uri]$config["FRONTEND_ORIGIN"]
    } catch {
        Fail "invalid FRONTEND_ORIGIN in config: $($config['FRONTEND_ORIGIN'])"
    }
    if ($frontendOrigin.Scheme -notin @("http", "https") -or $frontendOrigin.Host -notin @("127.0.0.1", "localhost", "::1")) {
        Fail "FRONTEND_ORIGIN must be an http(s) loopback origin"
    }
    if (-not $frontendOrigin.IsDefaultPort) {
        $frontendPort = $frontendOrigin.Port
    }
    if ($frontendPort -lt 1 -or $frontendPort -gt 65535) {
        Fail "invalid frontend port: $frontendPort"
    }

    New-Item -ItemType Directory -Path $runDir -Force | Out-Null
    $ngPath = Join-Path $frontendDir "node_modules\@angular\cli\bin\ng.js"
    if (-not (Test-Path -LiteralPath $ngPath -PathType Leaf)) {
        Push-Location $frontendDir
        try {
            & $npmCommand.Source ci
            if ($LASTEXITCODE -ne 0) { Fail "npm ci failed with status $LASTEXITCODE" }
        } finally {
            Pop-Location
        }
    }

    Push-Location $backendDir
    try {
        & $goCommand.Source build -o $runBin .\cmd\fakemex
        if ($LASTEXITCODE -ne 0) { Fail "backend build failed with status $LASTEXITCODE" }
    } finally {
        Pop-Location
    }
    if (-not (Test-Path -LiteralPath $runBin -PathType Leaf)) {
        Fail "backend build failed: missing executable $runBin"
    }

    Stop-RecordedService $backendPidFile "backend"
    Stop-RecordedService $frontendPidFile "frontend"
    Stop-ServiceOnPort "backend" $backendPort
    Stop-ServiceOnPort "frontend" $frontendPort

    foreach ($log in @($backendLog, $backendErrorLog, $frontendLog, $frontendErrorLog)) {
        [System.IO.File]::WriteAllText($log, "")
    }
    $backendProcess = Start-Process -FilePath $runBin -ArgumentList @("-config", (Quote-ProcessArgument $configPath)) -WorkingDirectory $backendDir -RedirectStandardOutput $backendLog -RedirectStandardError $backendErrorLog -WindowStyle Hidden -PassThru
    [System.IO.File]::WriteAllText($backendPidFile, "$($backendProcess.Id)`r`n")

    $frontendArguments = @(
        (Quote-ProcessArgument $ngPath), "serve",
        "--host", $frontendHost,
        "--port", $frontendPort,
        "--proxy-config", "proxy.conf.json"
    )
    $frontendProcess = Start-Process -FilePath $nodeCommand.Source -ArgumentList $frontendArguments -WorkingDirectory $frontendDir -RedirectStandardOutput $frontendLog -RedirectStandardError $frontendErrorLog -WindowStyle Hidden -PassThru
    [System.IO.File]::WriteAllText($frontendPidFile, "$($frontendProcess.Id)`r`n")

    $backendUrl = "http://${backendHost}:${backendPort}"
    $frontendUrl = "http://${frontendHost}:${frontendPort}/"
    Wait-ForHttp "$backendUrl/api/v1/health" "Backend health"
    Wait-ForHttp $frontendUrl "Frontend root"

    if ($backendProcess.HasExited) { Fail "backend process failed to start (check $backendLog and $backendErrorLog)" }
    if ($frontendProcess.HasExited) { Fail "frontend process failed to start (check $frontendLog and $frontendErrorLog)" }

    Write-Host "Backend : $backendUrl"
    Write-Host "Frontend: $frontendUrl"
    Write-Host "Backend logs: $backendLog, $backendErrorLog"
    Write-Host "Frontend logs: $frontendLog, $frontendErrorLog"
    Write-Host ""
    Write-Host "Startup completed. Press Ctrl+C to stop both services."

    while ($true) {
        if ($backendProcess.HasExited) {
            Fail "backend exited unexpectedly with status $($backendProcess.ExitCode) (see $backendLog and $backendErrorLog)"
        }
        if ($frontendProcess.HasExited) {
            Fail "frontend exited unexpectedly with status $($frontendProcess.ExitCode) (see $frontendLog and $frontendErrorLog)"
        }
        Start-Sleep -Seconds 1
    }
} catch {
    Write-Error $_.Exception.Message
    $exitCode = 1
} finally {
    if ($null -ne $backendProcess) {
        Stop-ServiceProcess $backendProcess.Id "backend" -KnownChild
        Remove-PidFileIfOwned $backendPidFile $backendProcess.Id
    }
    if ($null -ne $frontendProcess) {
        Stop-ServiceProcess $frontendProcess.Id "frontend" -KnownChild
        Remove-PidFileIfOwned $frontendPidFile $frontendProcess.Id
    }
}
exit $exitCode
