# Changelog

All notable changes to this project will be documented in this file.

## [1.3.2] - 2026-07-30

### Added
- **Episode Range Selection:** Added From and To range input boxes to the episode modal for real-time range filtering. Range inputs automatically clamp to available episode bounds and prevent backward ranges.

## [Unreleased]

### Fixed
- **HLS Segment Downloader:** Enclosed both the HTTP request (`Do`) and stream copying (`io.Copy`) operations inside the 5-attempt segment retry loop.
- **Progress Rate Inflation:** Separated progress tracking into `completedSegmentsBytes` and `currentSegmentBytes` to prevent byte counting duplication during segment retry attempts.
### Added
- **Browser Abstraction:** Added support for other chromium based browsers (Chrome, Edge, Brave, Vivaldi, Opera)