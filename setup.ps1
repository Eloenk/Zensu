Write-Host "=== Zensu Development Environment Setup ===" -ForegroundColor Cyan
Write-Host ""

# 1. Check Go
$go = Get-Command go -ErrorAction SilentlyContinue
if ($go) {
    $ver = & go version
    Write-Host "[✓] Go is installed ($ver)" -ForegroundColor Green
} else {
    Write-Host "[✗] Go is NOT installed! Please download and install Go from: https://go.dev/dl/" -ForegroundColor Red
    exit 1
}

# 2. Check Node.js
$node = Get-Command node -ErrorAction SilentlyContinue
if ($node) {
    $ver = & node -v
    Write-Host "[✓] Node.js is installed ($ver)" -ForegroundColor Green
} else {
    Write-Host "[✗] Node.js is NOT installed! Please download and install Node from: https://nodejs.org/" -ForegroundColor Red
    exit 1
}

# 3. Check Wails
$wails = Get-Command wails -ErrorAction SilentlyContinue
$localWails = Test-Path "$env:USERPROFILE\go\bin\wails.exe"

if ($wails -or $localWails) {
    Write-Host "[✓] Wails CLI is installed." -ForegroundColor Green
} else {
    Write-Host "[!] Wails CLI is NOT installed." -ForegroundColor Yellow
    Write-Host "Attempting to install Wails CLI..." -ForegroundColor Cyan
    & go install github.com/wailsapp/wails/v2/cmd/wails@latest
    if ($LASTEXITCODE -eq 0) {
        Write-Host "[✓] Wails CLI successfully installed!" -ForegroundColor Green
    } else {
        Write-Host "[✗] Failed to install Wails CLI automatically. Please run: go install github.com/wailsapp/wails/v2/cmd/wails@latest" -ForegroundColor Red
        exit 1
    }
}

Write-Host ""
Write-Host "=== Environment Setup Complete! ===" -ForegroundColor Green
Write-Host "You can now run '.\build.sh' to compile Zensu." -ForegroundColor Green
