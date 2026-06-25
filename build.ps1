$ErrorActionPreference = "Stop"

# Keep track of original env variables to restore later
$oldGoos = $env:GOOS
$oldGoarch = $env:GOARCH

Write-Host "Stopping any running Zensu instances..." -ForegroundColor Cyan
Stop-Process -Name "zensu" -Force -ErrorAction SilentlyContinue
Stop-Process -Name "zensu-cli" -Force -ErrorAction SilentlyContinue

Write-Host "Cleaning old build directory..." -ForegroundColor Cyan
if (Test-Path "build/bin") {
    Remove-Item -Recurse -Force "build/bin" -ErrorAction SilentlyContinue
}

# Determine wails executable path
$WailsCmd = "wails"
if (-not (Get-Command $WailsCmd -ErrorAction SilentlyContinue)) {
    $goBinWails = "$env:USERPROFILE\go\bin\wails.exe"
    $homeGoBinWails = "$env:HOME\go\bin\wails.exe"
    
    if (Test-Path $goBinWails) {
        $WailsCmd = $goBinWails
    } elseif (Test-Path $homeGoBinWails) {
        $WailsCmd = $homeGoBinWails
    } else {
        Write-Host "Error: wails CLI not found. Please install it by running:" -ForegroundColor Red
        Write-Host "  go install github.com/wailsapp/wails/v2/cmd/wails@latest" -ForegroundColor Red
        exit 1
    }
}

Write-Host "Building Zensu Desktop App via Wails..." -ForegroundColor Cyan
& $WailsCmd build -clean
if ($LASTEXITCODE -ne 0) {
    Write-Host "Wails build failed!" -ForegroundColor Red
    exit $LASTEXITCODE
}

Write-Host "Building CLI versions..." -ForegroundColor Cyan
if (-not (Test-Path "build/bin/cli")) {
    New-Item -ItemType Directory -Force -Path "build/bin/cli" | Out-Null
}

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

    Write-Host "Build complete!" -ForegroundColor Green
}
finally {
    # Restore original environment variables
    $env:GOOS = $oldGoos
    $env:GOARCH = $oldGoarch
}
