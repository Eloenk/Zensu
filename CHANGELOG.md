# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Fixed
- **HLS Segment Downloader:** Enclosed both the HTTP request (`Do`) and stream copying (`io.Copy`) operations inside the 5-attempt segment retry loop.
- **Progress Rate Inflation:** Separated progress tracking into `completedSegmentsBytes` and `currentSegmentBytes` to prevent byte counting duplication during segment retry attempts.
### Added
- **Browser Abstraction:** Added support for other chromium based browsers (Chrome, Edge, Brave, Vivaldi, Opera)