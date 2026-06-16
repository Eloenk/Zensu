
set -e


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

echo "Build complete!"
