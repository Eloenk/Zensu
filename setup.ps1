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

# 4. Run Build Script
Write-Host ""
Write-Host "=== Starting Zensu Compilation ===" -ForegroundColor Cyan
& .\build.ps1

Write-Host ""
Write-Host "=== Environment Setup Complete! ===" -ForegroundColor Green
Write-Host "Zensu has been successfully built. The binaries are located in the build/bin/ folder." -ForegroundColor Green
