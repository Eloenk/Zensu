# Zensu Future Features & Anime Research Document

This document summarizes feature ideas, architecture designs, and streaming source research for future Zensu updates.

---

## 🚀 Planned Features & Architecture Roadmap

### 1. 🎴 Search Card Tracking Toggle
- **Description**: Add a `[ 🔔 Track ]` / `[ ✓ Tracking ]` button directly to each anime card in search results.
- **Function**: Allows one-click tracking/untracking without opening extra submenus.

### 2. 📡 Smart Airing Status (AniList GraphQL API)
- **API**: `https://graphql.anilist.co` (Free, no API key required).
- **Status Types**:
  - `RELEASING`: Currently airing -> Zensu schedules background checks.
  - `FINISHED`: Finished airing -> Zensu completes missing episodes, marks status as "Completed", and stops polling to save bandwidth.
- **Bonus Data**: `nextAiringEpisode` provides countdown timer until the next episode airs in Japan.

### 3. 📺 Background Anime Watcher & Auto-Downloader
- **System Tray**: Minimizes Zensu to the Windows System Tray (`Hide()`) to run in the background with low CPU/RAM usage.
- **Notifications**: Sends Windows Toast Notifications when a new episode drops (*"Episode 12 of Anime X is out!"*).
- **Toggles**:
  - `[x] Enable Background Monitoring (Minimize to System Tray)`
  - `[x] Automatically Track All Downloaded Anime` (Default: ON, with per-card override)
  - `[x] Auto-Check for New Episodes`
  - `[x] Auto-Download New Episodes as They Air`

### 4. 📁 Open Main Download Folder Button
- **Description**: A dedicated button in Settings or top navigation bar to launch the root download directory (`cfg.DownloadDir`) directly in Windows File Explorer via `explorer.exe`.

### 5. 🔄 App Self-Updater (Auto-Update Executable)
- **Mechanism**: Queries GitHub Releases API (`/releases/latest`) to check version tags.
- **In-Place File Swap**: Renames running `zensu.exe` -> `zensu.exe.old`, downloads new `zensu.exe`, and restarts automatically. No DLLs required.
- **Toggle**: `[x] Check for App Updates Automatically`.

### 6. 🚀 Launch at Windows Startup
- **Description**: Windows Registry `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` entry to launch Zensu minimized on system boot.

---

## 🌐 Anime Streaming & Download Sources Research

### Directory Index
- **EverythingMoe**: `https://everythingmoe.com/` (Curated Otaku toplist)

### Top Ranked Streaming Sources (via EverythingMoe)
1. **AniKoto** (`anikototv.to`) — *Ranked #1*: High quality modern scraper with multi-audio support.
2. **AnimePahe** (`animepahe.pw`) — *Ranked #2*: Highly compressed MP4/HLS streams (Zensu's primary provider).
3. **Re:Anime** (`reanime.to`) — *Ranked #3*: Sleek UI with soft-sub and dub options.
4. **Miruro** (`miruro.to`) — *Ranked #4*: Multi-source player support.
5. **MKissa / AllAnime** (`mkissa.to`) — *Ranked #5*: Extensive catalog with easy download options.
6. **AniDB** (`anidb.app`) — *Ranked #6*: Clean ad-free player.
7. **AniZone** (`anizone.to`) — *Ranked #7*: High-speed playback.
8. **AniNeko** (`anineko.to`) — *Ranked #8*: Fast multi-host options.
9. **Senshi** (`senshi.live`) — *Ranked #9*: Modern web player.
10. **KickAssAnime** (`kaa.lt`) — *Ranked #12*: Well-established provider.

### Multi-Provider Scraper API
- **Consumet API**: `github.com/consumet/consumet.ts` — Unified API wrapper for Gogoanime, AnimePahe, HiAnime, Yugen, and Nyaa.

---

## 🔑 Technical & Cookie Notes

- **Cloudflare Clearance (`cf_clearance`)**: Typically valid for 24 hours to 30 days. Invalidated instantly if user IP changes or security rules shift.
- **Zensu CF Fetcher**: "Fetch CF Clearance" in Settings opens embedded Chromium browser to refresh clearance tokens seamlessly.
