# jlogs

A fast Go CLI tool that parses and aggregates log lines into a structured JSON summary.

## Purpose

`jlogs` reads log files (or stdin), recognizes klog-style and JSON-structured log formats, groups entries by severity and source location, and outputs a JSON summary with occurrence counts and recent examples.

Supported log formats:
- **klog-style**: `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512  1 node_controller.go:1056] No nodes available`
- **JSON-structured**: `2024-12-30T10:46:29.390512670Z {"level":"info","caller":"foo.go:42","msg":"..."}`

## Installation

### Using go install
```bash
go install github.com/gmeghnag/jlogs/cmd/jlogs@latest
```

### Build from source
```bash
git clone https://github.com/gmeghnag/jlogs.git
cd jlogs
go build -o jlogs ./cmd/jlogs
```

## Usage

```bash
# Read from stdin
cat app.log | jlogs

# Read from files
jlogs app.log error.log

# Customize output
jlogs -first-n 2 -last-n 10 -pretty=false app.log
```

### Options
- `-last-n`: Number of recent occurrences to retain per source (1-10, default: 5)
- `-first-n`: Number of first occurrences to retain per source (1-10, default: 1)
- `-pretty`: Pretty-print JSON output (default: true)

## Output

Produces a JSON summary grouped by severity (info, warning, error, fatal) with:
- Source file and line number
- Total occurrence count
- First N log entries
- Most recent N log entries

---

*AI-Assisted Go rewrite of the original Python implementation by Gabriel Meghnagi*
