# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- **2026-05-06**: Simplified output key from dynamic `last_N_occurrences` (e.g., `last_five_occurrences`, `last_three_occurrences`) to static `last_occurrences` regardless of the `-last-n` value
  - The `-last-n` flag still controls how many recent occurrences are retained (1-10, default: 5)
  - Output key is now always `last_occurrences` for consistency

### Added
- **2026-05-06**: New `-first-n` flag to track the first N occurrences per source
  - Default value: 1
  - Valid range: 1-10
  - Output appears under the `first_occurrences` key in the JSON summary
  - Useful for capturing initial log entries while also tracking recent ones
  - Example: `-first-n 3 -last-n 2` captures the first 3 and last 2 occurrences

## [0.1.0] - Initial Release

### Added
- Go implementation of jlogs log parser
- Support for klog-style and JSON-structured log formats
- Streaming input processing from stdin or files
- Severity-based grouping (info, warning, error, fatal)
- Source location tracking (filename:line)
- Occurrence counting per source
- Configurable retention of recent occurrences via `-last-n` flag
- Pretty-print JSON output option via `-pretty` flag
- Comprehensive test coverage
