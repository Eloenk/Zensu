# Changelog

All notable changes to this project will be documented in this file.

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