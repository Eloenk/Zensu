#!/bin/bash
set -e

echo "=== Zensu Development Environment Setup ==="
echo ""

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
WAILS_CMD="wails"
wails_installed=false

if command -v wails &> /dev/null; then
    wails_installed=true
else
    if [ -f "$HOME/go/bin/wails" ] || [ -f "$HOME/go/bin/wails.exe" ] || [ -f "$USERPROFILE/go/bin/wails.exe" ]; then
        wails_installed=true
        echo "[✓] Wails is installed locally in the Go bin directory."
    fi
fi

if [ "$wails_installed" = true ]; then
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

echo ""
echo "=== Environment Setup Complete! ==="
echo "You can now run './build.sh' to compile Zensu."
