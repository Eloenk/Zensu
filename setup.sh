#!/bin/bash
set -e

echo "=== Zensu Development Environment Setup & Build ==="
echo ""

# Ensure user's Go bin directory is in the path
export PATH="$PATH:$HOME/go/bin:$USERPROFILE/go/bin"

# 1. Check Go
if command -v go &> /dev/null; then
    go_version=$(go version | awk '{print $3}')
    echo "[✓] Go is installed ($go_version)"
else
    echo "[✗] Go is NOT installed! Please download and install Go from: https://go.dev/dl/"
    exit 1
fi

# 2. Check Node.js
if command -v node &> /dev/null; then
    node_version=$(node -v)
    echo "[✓] Node.js is installed ($node_version)"
else
    echo "[✗] Node.js is NOT installed! Please download and install Node from: https://nodejs.org/"
    exit 1
fi

# 3. Check Wails
if command -v wails &> /dev/null; then
    echo "[✓] Wails CLI is installed."
else
    echo "[!] Wails CLI is NOT installed."
    echo "Attempting to install Wails CLI via 'go install'..."
    if go install github.com/wailsapp/wails/v2/cmd/wails@latest; then
        echo "[✓] Wails CLI successfully installed!"
    else
        echo "[✗] Failed to install Wails CLI automatically. Please run manually: go install github.com/wailsapp/wails/v2/cmd/wails@latest"
        exit 1
    fi
fi

# Double check if wails is now available
if ! command -v wails &> /dev/null; then
    echo "[✗] Wails CLI was installed but is still not in the PATH."
    exit 1
fi

# 4. Terminate and Clean
echo ""
echo "=== Starting Zensu Compilation ==="
echo "Stopping any running Zensu instances..."
if command -v taskkill &> /dev/null; then
    taskkill //F //IM zensu.exe &> /dev/null || true
    taskkill //F //IM zensu-cli.exe &> /dev/null || true
fi
if command -v killall &> /dev/null; then
    killall zensu &> /dev/null || true
    killall zensu-cli &> /dev/null || true
fi

echo "Cleaning old build directory..."
rm -rf build/bin/ || true

# 5. Build Desktop & CLIs
echo "Building Zensu Desktop App via Wails..."
wails build -clean

echo "Building CLI versions..."
mkdir -p build/bin/cli

echo "  -> Windows x64 CLI..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build/bin/cli/zensu-cli.exe ./cmd/

echo "  -> Linux x64 CLI..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o build/bin/cli/zensu-cli ./cmd/

echo "  -> Android / Termux ARM64 CLI..."
GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o build/bin/cli/zensu-termux ./cmd/

echo "[✓] Build complete!"

# 6. Launch CLI and show user the path
OS_TYPE="$(uname -s)"
if [[ "$OS_TYPE" == *"MINGW"* || "$OS_TYPE" == *"MSYS"* || "$OS_TYPE" == *"CYGWIN"* ]]; then
    CLI_PATH="build/bin/cli/zensu-cli.exe"
else
    CLI_PATH="build/bin/cli/zensu-cli"
fi

ABS_CLI_PATH="$(cd "$(dirname "$CLI_PATH")" && pwd)/$(basename "$CLI_PATH")"

echo ""
echo "============================================="
echo "Zensu CLI is located at: $ABS_CLI_PATH"
echo "Automatically launching CLI..."
echo "============================================="
echo ""

# Run the CLI
"$ABS_CLI_PATH"
