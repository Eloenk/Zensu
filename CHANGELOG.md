# Changelog

All notable changes to this project will be documented in this file.

## [1.5.0] - 2026-10-09

### Added
- **Netflix & Prime Video Design System:** Redesigned UI into a modern dark streaming interface with sleek poster cards, brand red accents, and responsive catalog layout.
- **Resizable Window Support:** Enabled resizable main window with expanded default resolution (1240x780) for wide monitors.
- **Per-Provider Tracked Storage:** Tracked anime now permanently stores its source provider for accurate multi-provider episode fetching.
- **Auto-Retry Cookie Resolution:** Added automatic Cloudflare clearance cookie auto-solve and transparent retry on episode fetch failures.
- **Bleach & Zoro Banner:** Updated project header graphic with high-resolution Bleach Ichigo and Zoro anime streaming banner.

### Removed
- **Cyberpunk UI Theme & Emojis:** Purged all neon glow styling, scanlines, and emojis across the entire UI, alerts, logs, and documentation.

## [1.4.4] - 2026-09-20

### Added
- **Card-Based Settings UI:** Redesigned Settings panel into 4 clean visual card groups (Download & Media, Anime Tracking, System & Behavior, Provider Domains & Solvers).
- **Multi-Provider Domain Overrides:** Independent domain configuration inputs for AnimePahe (`https://animepahe.pw`), Anikoto TV (`https://anikototv.to`), and AnimeHeaven (`https://animeheaven.me`).
- **Glassmorphic Toast Notifications:** Added custom in-app floating toast notifications to replace browser `alert()` dialogs and eliminate `"wails.localhost says"` headers.

### Fixed
- **AnimeHeaven Download Referer:** Corrected `downloadDirect` to send `Referer: https://animeheaven.me/gate.php` instead of hardcoded `kwik.cx`, fixing direct `.mp4` downloads.
- **Dynamic Search Status Provider Names:** Updated search status header to display accurate provider names (`Anikoto TV`, `AnimeHeaven`, `AnimePahe`).

## [1.3.2] - 2026-07-30

### Added
- **Episode Range Selection:** Added From and To range input boxes to the episode modal for real-time range filtering. Range inputs automatically clamp to available episode bounds and prevent backward ranges.
- **Open Download Directory:** Added 1-click "Open Folder" buttons in the Downloads and Settings panels to open the root download directory in Windows File Explorer.
- **Dual Metadata API Resolution:** Integrated metadata resolution for anime airing status using AniList GraphQL API with automatic fallback to Jikan (MyAnimeList) API.

## [1.4.0] - 2026-08-13

### Added
- **Provider Button Toggle UI:** Converted provider dropdown selection into button toggles (AnimePahe / Anikoto TV) in search bar with automatic search result clearing when switching providers.
- **Browser Abstraction:** Added support for other chromium based browsers (Chrome, Edge, Brave, Vivaldi, Opera).

### Fixed
- **HLS Segment Downloader:** Enclosed both the HTTP request (`Do`) and stream copying (`io.Copy`) operations inside the 5-attempt segment retry loop.
- **Progress Rate Inflation:** Separated progress tracking into `completedSegmentsBytes` and `currentSegmentBytes` to prevent byte counting duplication during segment retry attempts.

## [1.4.1] - 2026-09-04

### Fixed
- **Settings Save Signature Mismatch:** Aligned frontend `SaveConfig` arguments with backend Go parameters to prevent type unmarshaling errors.

## [1.4.2] - 2026-09-16

### Added
- **AnimeHeaven (`gate.php`) Subsystem Integration:** Full fast search, episode gate key extraction, direct `.mp4` HTTP file downloading, and multi-stage mirror CDN failover (`cw` -> `ct` -> `ck`).
- **Rectangular Search Provider Buttons:** Updated search provider selector toggle buttons to clean, crisp rectangular box styling.

### Fixed
- **Tracked Anime Provider Propagation:** Fixed `openEpisodeModal()` call when clicking tracked cards to explicitly pass stored `item.provider`, preventing provider/slug mismatches and `"Failed to fetch episodes"` errors.

## [1.4.3] - 2026-09-19

### Fixed
- **AnimePahe Pagination Rate Limit:** Added 300ms inter-page pacing and exponential backoff retry loop (`2.5s -> 5s -> 10s`) for HTTP 429 rate limit resilience, resolving episode pagination failures for large series (*One Piece*, etc.).