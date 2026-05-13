# jlogs

A fast Go CLI tool that parses and aggregates log lines into a structured JSON summary.

## Purpose

`jlogs` reads log files (or stdin), recognizes klog-style and JSON-structured log formats, groups entries by severity and source location, and outputs a JSON summary with occurrence counts and recent examples.

**IMPORTANT**: `jlogs` requires that **every log line** starts with an RFC3339Nano timestamp (30 characters). This is the format produced by `oc logs --timestamps=true` or `kubectl logs --timestamps=true`. Without the `--timestamps=true` flag, `jlogs` will exit with an error.

Supported log formats:
- **klog-style**: `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512  1 node_controller.go:1056] No nodes available`
- **JSON-structured**: `2024-12-30T10:46:29.390512670Z {"level":"info","caller":"foo.go:42","msg":"..."}`
- **unstructured**: Any line with a valid timestamp that doesn't match klog or JSON formats

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

**Kubernetes/OpenShift logs (recommended):**
```bash
# CORRECT: With --timestamps=true
oc logs -n openshift-etcd pod-name --timestamps=true | jlogs
kubectl logs pod-name --timestamps=true | jlogs

# WRONG: Without timestamps will fail
oc logs pod-name | jlogs
# Error: invalid timestamp prefix - did you forget --timestamps=true?
```

**Local files:**
```bash
# Files must have RFC3339Nano timestamp prefix on each line
cat app.log | jlogs
jlogs app.log error.log

# Customize output
jlogs -first-n 2 -last-n 10 -pretty=false app.log
```

### Options

#### `-v`
Print version information and exit.
- Displays the git tag and commit hash set at build time
- Example: `jlogs -v` outputs `jlogs version v1.0.0 (commit a1b2c3d)`

#### `-last-n` (default: 5)
Number of most recent occurrences to retain per source in the output.
- Valid range: 1-10
- Values outside this range fall back to default (5)
- Example: `-last-n 3` keeps the 3 most recent log entries for each source

#### `-first-n` (default: 1)
Number of first occurrences to retain per source in the output.
- Valid range: 1-10
- Values outside this range fall back to default (1)
- Useful for capturing initial log entries along with recent ones
- Example: `-first-n 2` keeps the 2 earliest log entries for each source

#### `-since`
Filter logs to include only those from the last N time units, relative to the latest timestamp in the log stream.
- Supported units: `minute(s)`, `hour(s)`, `day(s)`
- Format: `"N unit"` (quotes recommended)
- When applied, recalculates occurrence counts and frequencies based on the filtered time window
- Sources with no occurrences in the time window are omitted from output
- Examples:
  - `-since "1 hour"` - logs from the last hour
  - `-since "30 minutes"` - logs from the last 30 minutes
  - `-since "2 days"` - logs from the last 2 days

#### `-pretty` (default: true)
Enable or disable pretty-printed JSON output.
- `true`: indented, human-readable JSON
- `false`: compact, single-line JSON
- Example: `-pretty=false` for compact output

## Output

Produces a JSON summary grouped by severity (info, warning, error, fatal, unstructured) with:
- Source file and line number
- Total occurrence count
- Frequency (occurrences per time period: X/s, X/m, X/h, or X.X/d)
- First N log entries
- Most recent N log entries

**Severity categories:**
- **info, warning, error, fatal**: Recognized klog and JSON log levels
- **unstructured**: Lines with valid timestamp that don't match klog or JSON formats, or have unrecognized severity levels

---

*AI-Assisted Go rewrite of the original Python implementation by Gabriel Meghnagi*
