#!/bin/bash
set -e

echo "=== Zensu Development Environment Setup & Build ==="
echo ""

# Ensure user's Go bin directory is in the path
export PATH="$PATH:$HOME/go/bin"

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

# 4. Run Build Script
echo ""
echo "=== Starting Zensu Compilation ==="
chmod +x ./build.sh
./build.sh

echo ""
echo "=== Setup Complete! ==="
echo "Zensu has been successfully built. The binaries are located in the build/bin/ folder."
