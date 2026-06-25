$ErrorActionPreference = "Stop"

Write-Host "=== Zensu Development Environment Setup ===" -ForegroundColor Cyan
Write-Host ""

# Proactively refresh PATH in case Go or other tools were just installed
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")

# Add standard Go bin directory to the session PATH if not already present
$userGoBin = "$env:USERPROFILE\go\bin"
$homeGoBin = "$env:HOME\go\bin"
if (Test-Path $userGoBin) {
    if ($env:Path -notlike "*$userGoBin*") { $env:Path += ";$userGoBin" }
}
if (Test-Path $homeGoBin) {
    if ($env:Path -notlike "*$homeGoBin*") { $env:Path += ";$homeGoBin" }
}

# 1. Check Go
$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) {
    $ver = & go version
    Write-Host "[OK] Go is installed ($ver)" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Go is NOT installed! Please download and install Go from: https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

# 2. Check Node.js
$node = Get-Command node -ErrorAction SilentlyContinue
if ($node) {
    $ver = & node -v
    Write-Host "[OK] Node.js is installed ($ver)" -ForegroundColor Green
} else {
    Write-Host "[ERROR] Node.js is NOT installed! Please download and install Node from: https://nodejs.org/" -ForegroundColor Red
    exit 1
}

# 3. Check Wails
$wails = Get-Command wails -ErrorAction SilentlyContinue
if ($wails) {
    Write-Host "[OK] Wails CLI is installed." -ForegroundColor Green
} else {
    Write-Host "[WARN] Wails CLI is NOT installed." -ForegroundColor Yellow
    Write-Host "Attempting to install Wails CLI..." -ForegroundColor Cyan
    & go install github.com/wailsapp/wails/v2/cmd/wails@latest
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[OK] Wails CLI successfully installed!" -ForegroundColor Green
        # Refresh Path again to find the newly installed wails executable
        $env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
        if (Test-Path $userGoBin) {
            if ($env:Path -notlike "*$userGoBin*") { $env:Path += ";$userGoBin" }
        }
    } else {
        Write-Host "[ERROR] Failed to install Wails CLI automatically. Please run: go install github.com/wailsapp/wails/v2/cmd/wails@latest" -ForegroundColor Red
        exit 1
    }
}

# Verify wails is accessible now
$wails = Get-Command wails -ErrorAction SilentlyContinue
if (-not $wails) {
    Write-Host "[ERROR] Wails CLI was installed but is still not in the PATH." -ForegroundColor Red
    exit 1
}

# 4. Terminate and Clean
Write-Host ""
Write-Host "=== Starting Zensu Compilation ===" -ForegroundColor Cyan
Write-Host "Stopping any running Zensu instances..." -ForegroundColor Cyan
Stop-Process -Name "zensu" -Force -ErrorAction SilentlyContinue
Stop-Process -Name "zensu-cli" -Force -ErrorAction SilentlyContinue

Write-Host "Cleaning old build directory..." -ForegroundColor Cyan
if (Test-Path "build/bin") {
    Remove-Item -Recurse -Force "build/bin" -ErrorAction SilentlyContinue
}

Write-Host "Building Zensu Desktop App via Wails..." -ForegroundColor Cyan
wails build -clean
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Wails build failed!" -ForegroundColor Red
    exit $LASTEXITCODE
}

Write-Host "Building CLI versions..." -ForegroundColor Cyan
if (-not (Test-Path "build/bin/cli")) {
    New-Item -ItemType Directory -Force -Path "build/bin/cli" | Out-Null
}

$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH

try {
    Write-Host "  -> Windows x64 CLI..." -ForegroundColor Gray
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"
    go build -ldflags="-s -w" -o build/bin/cli/zensu-cli.exe ./cmd/
    if ($LASTEXITCODE -ne 0) { throw "Windows CLI build failed" }

    Write-Host "  -> Linux x64 CLI..." -ForegroundColor Gray
    $env:GOOS = "linux"
    $env:GOARCH = "amd64"
    go build -ldflags="-s -w" -o build/bin/cli/zensu-cli ./cmd/
    if ($LASTEXITCODE -ne 0) { throw "Linux CLI build failed" }

    Write-Host "  -> Android / Termux ARM64 CLI..." -ForegroundColor Gray
    $env:GOOS = "android"
    $env:GOARCH = "arm64"
    go build -ldflags="-s -w" -o build/bin/cli/zensu-termux ./cmd/
    if ($LASTEXITCODE -ne 0) { throw "Android/Termux CLI build failed" }

    Write-Host "[OK] Build complete!" -ForegroundColor Green
}
finally {
    # Restore original environment variables
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
}

# 5. Launch CLI and show user the path
$cliPath = Resolve-Path "build/bin/cli/zensu-cli.exe"
Write-Host ""
Write-Host "=============================================" -ForegroundColor Cyan
Write-Host "Zensu CLI is located at: $cliPath" -ForegroundColor Green
Write-Host "Automatically launching CLI..." -ForegroundColor Cyan
Write-Host "=============================================" -ForegroundColor Cyan
Write-Host ""

& $cliPath
