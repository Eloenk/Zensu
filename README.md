# Zensu

Zensu is a high-performance AnimePahe downloader featuring a premium desktop GUI and an interactive terminal-based CLI.

---

## 🔌 Chrome Extension Usage

The Chrome Extension allows you to easily sync clearance cookies from your browser to Zensu.

1. Open Chrome and navigate to `chrome://extensions/`.
2. Enable **Developer mode** (top-right toggle).
3. Click **Load unpacked** and select the `extension/` folder in this repository.
4. Navigate to `https://animepahe.pw`. 
5. Open the extension icon, copy the generated cookies, and paste them into Zensu settings.

---

## 💻 CLI Usage

Run the CLI for interactive terminal-based downloads:

* **Windows**: `build\bin\cli\zensu-cli.exe`
* **Linux / Termux**: `./build/bin/cli/zensu-cli` (or `./build/bin/cli/zensu-termux`)
* **How to Use**:
  1. Launch the executable.
  2. Type your search query and hit Enter.
  3. Select the anime from the list of search results.
  4. Select episodes to download (e.g., `1,2,3` or `1-5`).

---

## 🛠️ Build & Setup

1. **Initialize Environment**:
   * Windows: `.\setup.ps1`
   * Linux / Termux / macOS: `chmod +x setup.sh && ./setup.sh`
2. **Compile Targets**:
   * Run `./build.sh` to compile GUI and CLI binaries to the `build/bin/` folder.

---

## 🐧 Linux & Android/Termux Setup

On Linux and Android (Termux), install `ffmpeg` before running the CLI:

* **Linux**: `sudo apt install ffmpeg`
* **Android (Termux)**: `pkg install ffmpeg`

Run:
```bash
chmod +x zensu-cli
./zensu-cli
```
