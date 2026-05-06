# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **2026-05-06**: New `--since` flag to filter logs by time window
  - Filters logs from the last N time units relative to the latest log timestamp
  - Supports: minute(s), hour(s), day(s)
  - Examples: `--since "1 hour"`, `--since "2 days"`, `--since "30 minutes"`
  - Recalculates occurrence counts and frequencies based on filtered time window
  - Sources with no occurrences after filtering are omitted from output

### Fixed
- **2026-05-06**: Frequency calculation now requires minimum 1-second duration to avoid misleading rates from startup bursts
  - Previously, logs occurring in milliseconds (e.g., 57 logs in 4ms) would show unrealistic frequencies like "13154/s"
  - Now omits frequency for bursts shorter than 1 second, as these don't represent sustained logging patterns
- **2026-05-06**: Added per-day frequency unit to handle low-frequency logs
  - Prevents showing "0/h" for logs that occur less than once per hour
  - Shows fractional rates like "3.2/d" for logs occurring multiple times per day
  - Example: 2 logs in 15 hours displays as "3.2/d" instead of "0/h"

### Changed
- **2026-05-06**: Simplified output key from dynamic `last_N_occurrences` (e.g., `last_five_occurrences`, `last_three_occurrences`) to static `last_occurrences` regardless of the `-last-n` value
  - The `-last-n` flag still controls how many recent occurrences are retained (1-10, default: 5)
  - Output key is now always `last_occurrences` for consistency

### Added
- **2026-05-06**: Automatic frequency calculation for each log source
  - Calculates occurrence rate based on time difference between first and last occurrence
  - Intelligently formats frequency as:
    - `X/s` (per second) when rate ≥ 1/s
    - `X/m` (per minute) when rate ≥ 1/m but < 1/s
    - `X/h` (per hour) when rate ≥ 1/h but < 1/m
    - `X.X/d` (per day, with decimal) when rate < 1/h
  - Appears in JSON output under the `frequency` field
  - Example: "10/s", "5/m", "4/h", "3.2/d"
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
