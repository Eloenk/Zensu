set -e


echo "Stopping any running Zensu instances..."
if command -v taskkill &> /dev/null; then
    taskkill //F //IM zensu.exe &> /dev/null || true
    taskkill //F //IM zensu-cli.exe &> /dev/null || true
    taskkill //F //IM ffmpeg.exe &> /dev/null || true
fi
if command -v killall &> /dev/null; then
    killall zensu &> /dev/null || true
    killall zensu-cli &> /dev/null || true
    killall ffmpeg &> /dev/null || true
fi


echo "Cleaning old build directory and root binary artifacts..."
rm -rf build/bin/ || true
rm -f zensu.exe zensu-cli.exe 2>/dev/null || true

WAILS_CMD="wails"
if ! command -v wails &> /dev/null; then
    if [ -f "$HOME/go/bin/wails" ]; then
        WAILS_CMD="$HOME/go/bin/wails"
    elif [ -f "$HOME/go/bin/wails.exe" ]; then
        WAILS_CMD="$HOME/go/bin/wails.exe"
    elif [ -f "$USERPROFILE/go/bin/wails.exe" ]; then
        WAILS_CMD="$USERPROFILE/go/bin/wails.exe"
    else
        echo "Error: wails CLI not found. Please install it by running:"
        echo "  go install github.com/wailsapp/wails/v2/cmd/wails@latest"
        exit 1
    fi
fi

echo "Building Zensu Desktop App via Wails..."
$WAILS_CMD build -clean

echo "Building CLI versions..."
mkdir -p build/bin/cli

echo "  -> Windows x64 CLI..."
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o build/bin/cli/zensu-cli.exe ./cmd/

echo "  -> Linux x64 CLI..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o build/bin/cli/zensu-cli ./cmd/

echo "  -> Android / Termux ARM64 CLI..."
GOOS=android GOARCH=arm64 go build -ldflags="-s -w" -o build/bin/cli/zensu-termux ./cmd/

# Clean root zensu binaries
rm -f zensu.exe zensu-cli.exe 2>/dev/null || true

echo "Build complete!"

# Copy complete binary if running on Windows environment
INSTALL_DIR="$USERPROFILE/AppData/Local/Programs/Zensu"
if [ -f "build/bin/zensu.exe" ] && [ -d "$USERPROFILE" ]; then
    mkdir -p "$INSTALL_DIR" || true
    cp -f "build/bin/zensu.exe" "$INSTALL_DIR/zensu.exe" 2>/dev/null || true
    echo "Copied build/bin/zensu.exe to $INSTALL_DIR/zensu.exe"
fi

rm -f zensu.exe zensu-cli.exe 2>/dev/null || true
