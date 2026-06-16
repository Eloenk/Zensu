# Zensu

Zensu is a high-performance desktop downloader for AnimePahe, featuring a premium glassmorphic dark user interface (powered by Wails + Vite/Vanilla JS) alongside a modular command-line interface (CLI) for advanced usage and terminal environments (including Android Termux).

## Features

- **Chrome TLS Fingerprinting:** Powered by `bogdanfinn/tls-client` to seamlessly bypass Cloudflare security layers.
- **Pure-Go Dean Edwards Deobfuscator:** 100% native Javascript packer decompression inside Go, removing runtime dependencies on Node.js or external script interpreters.
- **Self-Healing FFmpeg Engine:** If `ffmpeg` is not found on your system, Zensu automatically downloads and extracts the correct architecture-specific binary (stored in a local `./bin/` subfolder next to the binary) at runtime.
- **Network Glitch Resilience:**
  - **Direct Downloads:** Supports network-failure retries (up to 5 times) and connection resuming using HTTP `Range` headers.
  - **HLS Downloads:** Configures robust HLS segment reconnect options dynamically within FFmpeg.
- **AppData Configuration Migration:** Settings are saved directly to the user's OS-native configuration folders (`%APPDATA%/zensu/config.json` on Windows, and `~/.config/zensu/config.json` on Linux/Android) to protect config data across installations.
- **Dynamic Mirror/Domain Resolution:** Modify mirror domains directly in Settings to support AnimePahe domain changes without updating the app.
- **Intelligent Local Directory Scanning:** Checks your download directory and automatically flags completed/saved episodes in selection menus.

---

## Deliverables & Output Binaries

After building, Zensu generates compile assets under the `build/bin/` output directories:
- **`build/bin/zensu.exe`:** Premium fixed-size desktop application (Windows).
- **`build/bin/cli/zensu-cli.exe`:** Windows CLI application.
- **`build/bin/cli/zensu-cli`:** Linux AMD64 CLI application.
- **`build/bin/cli/zensu-termux`:** Android/Termux ARM64 CLI application.

---

## Setup & Compilation

### Requirements
1. **Go 1.22+** (https://go.dev/dl/)
2. **Node.js** (for building Vite frontend assets)
3. **Wails CLI v2** (Installed via: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Compile All Targets
Simply run the compilation script:
```bash
# On Linux/macOS
./build.sh

# On Windows (PowerShell)
.\build.sh
```

---

## Configuration Settings
Configuration is initialized automatically. To bypass Cloudflare blocks, you must update settings via the Settings Tab (GUI) or your system's `config.json` file:
* **Windows path:** `%APPDATA%\zensu\config.json`
* **Linux/Android path:** `~/.config/zensu/config.json`

```json
{
  "ua": "Your browser User-Agent",
  "cf": "cf_clearance cookie value",
  "cookies": "Full cookie string from browser devtools",
  "downloadDir": "C:\\Users\\Username\\Videos\\Anime",
  "maxParallel": 3,
  "quality": "1080",
  "audio": "jpn",
  "domain": "https://animepahe.pw"
}
```

### Fetching Cookie Credentials:
1. Open your browser and navigate to `animepahe.pw` (or your configured domain).
2. Press `F12` to open DevTools, select the **Network** tab, and reload the page.
3. Click any document request to the domain.
4. Copy the full `cookie:` request header and paste it as `"cookies"`.
5. Copy the individual `cf_clearance=...` cookie value and paste it as `"cf"`.
6. Copy the browser's User-Agent string and paste it as `"ua"`.

---

## Termux / Android Installation (CLI)

You can run the terminal-friendly client (`zensu-termux`) on Android devices:

1. **Install Termux** from F-Droid.
2. **Install FFmpeg and dependencies:**
   ```bash
   pkg update && pkg upgrade
   pkg install ffmpeg
   ```
3. **Download or push `zensu-termux`:** Place the compiled `zensu-termux` binary onto your device.
4. **Grant permissions & Run:**
   ```bash
   chmod +x zensu-termux
   ./zensu-termux
   ```
