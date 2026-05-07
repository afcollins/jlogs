package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseKlogLine(t *testing.T) {
	p, err := New(5, 1, 0)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	line := `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512       1 node_controller.go:1056] No nodes available for updates`
	p.parseLine(line)

	got := p.Summary()
	infos := got["info"]
	if len(infos) != 1 {
		t.Fatalf("expected 1 info entry, got %d", len(infos))
	}
	if infos[0].Source != "node_controller.go:1056" {
		t.Errorf("source = %q, want node_controller.go:1056", infos[0].Source)
	}
	if infos[0].Occurrences != 1 {
		t.Errorf("occurrences = %d, want 1", infos[0].Occurrences)
	}
	msg, ok := infos[0].Recent[0].Log.(string)
	if !ok || msg != "No nodes available for updates" {
		t.Errorf("recent log = %v, want klog message string", infos[0].Recent[0].Log)
	}
}

func TestParseJSONLine(t *testing.T) {
	p, _ := New(5, 1, 0)
	line := `2024-12-30T10:46:29.390512670Z {"level":"error","caller":"foo.go:42","msg":"boom","error":"bad thing"}`
	p.parseLine(line)

	errs := p.Summary()["error"]
	if len(errs) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(errs))
	}
	if errs[0].Source != "foo.go:42" {
		t.Errorf("source = %q, want foo.go:42", errs[0].Source)
	}
	full, ok := errs[0].Recent[0].Log.(map[string]any)
	if !ok {
		t.Fatalf("expected JSON payload to be map, got %T", errs[0].Recent[0].Log)
	}
	if full["error"] != "bad thing" {
		t.Errorf("full payload missing error field: %v", full)
	}
}

func TestSeverityAliases(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"info", "info"},
		{"warn", "warning"},
		{"warning", "warning"},
		{"error", "error"},
		{"fatal", "fatal"},
		{"INFO", "info"}, // case-insensitive
	}
	for _, tc := range cases {
		t.Run(tc.level, func(t *testing.T) {
			p, _ := New(5, 1, 0)
			line := `2024-12-30T10:46:29.390512670Z {"level":"` + tc.level + `","caller":"x.go:1","msg":"m"}`
			p.parseLine(line)
			if got := len(p.Summary()[tc.want]); got != 1 {
				t.Errorf("severity %q: got %d entries in bucket %q, want 1", tc.level, got, tc.want)
			}
		})
	}
}

func TestRingOverflow(t *testing.T) {
	p, _ := New(3, 1, 0)
	for i := 0; i < 10; i++ {
		line := `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512       1 a.go:1] msg`
		p.parseLine(line)
	}
	entry := p.Summary()["info"][0]
	if entry.Occurrences != 10 {
		t.Errorf("occurrences = %d, want 10", entry.Occurrences)
	}
	if len(entry.Recent) != 3 {
		t.Errorf("recent len = %d, want 3 (capped)", len(entry.Recent))
	}
}

func TestMalformedJSONCapturedAsUnstructured(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Bad JSON should not crash and should be captured as unstructured.
	bad := `2024-12-30T10:46:29.390512670Z {not valid json}`
	good := `2024-12-30T10:46:29.390512670Z {"level":"info","caller":"x.go:1","msg":"ok"}`
	r := strings.NewReader(bad + "\n" + good + "\n")
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	summary := p.Summary()

	// Good line should be in info
	if got := len(summary["info"]); got != 1 {
		t.Errorf("info entries = %d, want 1 (good line only)", got)
	}

	// Bad line should be in unstructured
	if got := len(summary["unstructured"]); got != 1 {
		t.Errorf("unstructured entries = %d, want 1 (bad JSON)", got)
	}

	// Verify the malformed JSON is preserved as-is
	msg, ok := summary["unstructured"][0].Recent[0].Log.(string)
	if !ok || msg != bad {
		t.Errorf("unstructured log = %q, want %q", msg, bad)
	}
}

func TestInvalidTimestampReturnsError(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Line without valid RFC3339Nano timestamp should return error
	r := strings.NewReader("this is not a structured log line\n")
	err := p.Consume(r)
	if err == nil {
		t.Fatal("expected error for invalid timestamp, got nil")
	}
	if !strings.Contains(err.Error(), "invalid timestamp prefix") {
		t.Errorf("error = %v, want 'invalid timestamp prefix'", err)
	}
}

func TestNewClampsLastN(t *testing.T) {
	for _, n := range []int{0, -1, 11, 999} {
		p, err := New(n, 1, 0)
		if err != nil {
			t.Fatalf("New(%d, 1): %v", n, err)
		}
		if p.lastN != 5 {
			t.Errorf("New(%d, 1) lastN = %d, want 5 fallback", n, p.lastN)
		}
	}
}

func TestRecentKeyMatchesLastN(t *testing.T) {
	cases := []int{1, 3, 5, 10}
	for _, n := range cases {
		p, _ := New(n, 1, 0)
		if p.recentKey != "last_occurrences" {
			t.Errorf("New(%d, 1) recentKey = %q, want %q", n, p.recentKey, "last_occurrences")
		}
	}
}

func TestNewClampsFirstN(t *testing.T) {
	for _, n := range []int{0, -1, 11, 999} {
		p, err := New(5, n, 0)
		if err != nil {
			t.Fatalf("New(5, %d): %v", n, err)
		}
		if p.firstN != 1 {
			t.Errorf("New(5, %d) firstN = %d, want 1 fallback", n, p.firstN)
		}
	}
}

func TestFirstOccurrences(t *testing.T) {
	p, _ := New(3, 2, 0)
	// Add 5 occurrences of the same log line
	for i := 0; i < 5; i++ {
		line := `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512       1 test.go:10] msg`
		p.parseLine(line)
	}

	entry := p.Summary()["info"][0]
	if entry.Occurrences != 5 {
		t.Errorf("occurrences = %d, want 5", entry.Occurrences)
	}
	// Should only keep first 2
	if len(entry.First) != 2 {
		t.Errorf("first occurrences len = %d, want 2", len(entry.First))
	}
	// Should keep last 3
	if len(entry.Recent) != 3 {
		t.Errorf("recent occurrences len = %d, want 3", len(entry.Recent))
	}
}

func TestFrequencyCalculation(t *testing.T) {
	tests := []struct {
		name        string
		occurrences int
		firstTS     string
		lastTS      string
		want        string
	}{
		{
			name:        "per second - 10 logs in 1 second",
			occurrences: 10,
			firstTS:     "2024-12-30T10:46:29.000000000Z",
			lastTS:      "2024-12-30T10:46:30.000000000Z",
			want:        "10/s",
		},
		{
			name:        "per minute - 120 logs in 60 seconds (exactly 1 minute)",
			occurrences: 120,
			firstTS:     "2024-12-30T10:46:00.000000000Z",
			lastTS:      "2024-12-30T10:47:00.000000000Z",
			want:        "120/m",
		},
		{
			name:        "per minute - 5 logs in 10 minutes",
			occurrences: 5,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:10:00.000000000Z",
			want:        "0.5/m",
		},
		{
			name:        "per minute - 10 logs in 5 minutes",
			occurrences: 10,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:05:00.000000000Z",
			want:        "2.0/m",
		},
		{
			name:        "per minute - 30 logs in 15 minutes",
			occurrences: 30,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:15:00.000000000Z",
			want:        "2.0/m",
		},
		{
			name:        "per minute - 5 logs in 7 minutes (actual case from user)",
			occurrences: 5,
			firstTS:     "2026-05-04T07:50:45.581233383Z",
			lastTS:      "2026-05-04T07:57:45.637694164Z",
			want:        "0.7/m",
		},
		{
			name:        "per hour - 4 logs in 2 hours",
			occurrences: 4,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T12:00:00.000000000Z",
			want:        "2.0/h",
		},
		{
			name:        "per hour - 10 logs in 5 hours",
			occurrences: 10,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T15:00:00.000000000Z",
			want:        "2.0/h",
		},
		{
			name:        "per hour - 2 logs in 15 hours",
			occurrences: 2,
			firstTS:     "2026-04-29T16:51:29.280896980Z",
			lastTS:      "2026-04-30T07:53:21.563792653Z",
			want:        "0.1/h",
		},
		{
			name:        "per day - 2 logs in 4 days",
			occurrences: 2,
			firstTS:     "2024-12-26T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:00:00.000000000Z",
			want:        "0.5/d",
		},
		{
			name:        "per day - 5 logs in 10 days",
			occurrences: 5,
			firstTS:     "2024-12-20T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:00:00.000000000Z",
			want:        "0.5/d",
		},
		{
			name:        "single occurrence",
			occurrences: 1,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:00:00.000000000Z",
			want:        "",
		},
		{
			name:        "same timestamp - duration too short",
			occurrences: 5,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "2024-12-30T10:00:00.000000000Z",
			want:        "",
		},
		{
			name:        "burst in milliseconds - duration too short",
			occurrences: 57,
			firstTS:     "2026-04-29T16:51:23.030739787Z",
			lastTS:      "2026-04-29T16:51:23.035073027Z",
			want:        "",
		},
		{
			name:        "invalid first timestamp",
			occurrences: 10,
			firstTS:     "invalid",
			lastTS:      "2024-12-30T10:00:00.000000000Z",
			want:        "",
		},
		{
			name:        "invalid last timestamp",
			occurrences: 10,
			firstTS:     "2024-12-30T10:00:00.000000000Z",
			lastTS:      "invalid",
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateFrequency(tt.occurrences, tt.firstTS, tt.lastTS)
			if got != tt.want {
				t.Errorf("calculateFrequency(%d, %q, %q) = %q, want %q",
					tt.occurrences, tt.firstTS, tt.lastTS, got, tt.want)
			}
		})
	}
}

func TestFrequencyInSummary(t *testing.T) {
	p, _ := New(5, 1, 0)

	// Add logs with different timestamps
	lines := []string{
		`2024-12-30T10:00:00.000000000Z I1230 10:00:00.000000       1 freq.go:1] msg1`,
		`2024-12-30T10:00:01.000000000Z I1230 10:00:01.000000       1 freq.go:1] msg2`,
		`2024-12-30T10:00:02.000000000Z I1230 10:00:02.000000       1 freq.go:1] msg3`,
		`2024-12-30T10:00:03.000000000Z I1230 10:00:03.000000       1 freq.go:1] msg4`,
		`2024-12-30T10:00:04.000000000Z I1230 10:00:04.000000       1 freq.go:1] msg5`,
	}

	for _, line := range lines {
		p.parseLine(line)
	}

	summary := p.Summary()
	entry := summary["info"][0]

	if entry.Occurrences != 5 {
		t.Errorf("occurrences = %d, want 5", entry.Occurrences)
	}

	// 5 occurrences over 4 seconds = 1.25/s, shown as 1.2/s (one decimal)
	if entry.Frequency != "1.2/s" {
		t.Errorf("frequency = %q, want %q", entry.Frequency, "1.2/s")
	}
}

func TestSinceFiltering(t *testing.T) {
	// Create parser with 1 hour since filter
	p, _ := New(5, 1, 1*time.Hour)

	lines := []string{
		// Old logs (> 1 hour before latest)
		`2024-12-30T10:00:00.000000000Z I1230 10:00:00.000000       1 test.go:1] old1`,
		`2024-12-30T10:30:00.000000000Z I1230 10:30:00.000000       1 test.go:1] old2`,
		// Recent logs (within 1 hour of latest)
		`2024-12-30T11:30:00.000000000Z I1230 11:30:00.000000       1 test.go:1] recent1`,
		`2024-12-30T12:00:00.000000000Z I1230 12:00:00.000000       1 test.go:1] recent2`,
		`2024-12-30T12:15:00.000000000Z I1230 12:15:00.000000       1 test.go:1] recent3`,
	}

	for _, line := range lines {
		p.parseLine(line)
	}

	summary := p.Summary()
	entries := summary["info"]

	if len(entries) != 1 {
		t.Fatalf("expected 1 source, got %d", len(entries))
	}

	entry := entries[0]
	// Should only have 3 recent logs (within 1 hour of 12:15:00)
	if entry.Occurrences != 3 {
		t.Errorf("occurrences = %d, want 3", entry.Occurrences)
	}

	// Check that we only have recent logs
	allLogs := append(entry.First, entry.Recent...)
	for _, occ := range allLogs {
		if strings.Contains(occ.Log.(string), "old") {
			t.Errorf("found old log that should be filtered: %v", occ.Log)
		}
	}
}

func TestSinceFilteringMultipleSources(t *testing.T) {
	p, _ := New(5, 1, 30*time.Minute)

	lines := []string{
		// source1: has both old and recent logs
		`2024-12-30T10:00:00.000000000Z I1230 10:00:00.000000       1 source1.go:1] msg`,
		`2024-12-30T10:45:00.000000000Z I1230 10:45:00.000000       1 source1.go:1] msg`,
		// source2: only old logs (should be filtered out)
		`2024-12-30T10:00:00.000000000Z W1230 10:00:00.000000       1 source2.go:1] msg`,
		`2024-12-30T10:10:00.000000000Z W1230 10:10:00.000000       1 source2.go:1] msg`,
		// Latest log (defines cutoff point)
		`2024-12-30T11:00:00.000000000Z E1230 11:00:00.000000       1 source3.go:1] msg`,
	}

	for _, line := range lines {
		p.parseLine(line)
	}

	summary := p.Summary()

	// source1 should have 1 occurrence (only the 10:45 log is within 30 min of 11:00)
	if len(summary["info"]) != 1 {
		t.Errorf("info sources = %d, want 1", len(summary["info"]))
	}
	if summary["info"][0].Occurrences != 1 {
		t.Errorf("source1 occurrences = %d, want 1", summary["info"][0].Occurrences)
	}

	// source2 should be completely filtered out (all logs > 30 min old)
	if len(summary["warning"]) != 0 {
		t.Errorf("warning sources = %d, want 0 (should be filtered)", len(summary["warning"]))
	}

	// source3 should have 1 occurrence
	if len(summary["error"]) != 1 {
		t.Errorf("error sources = %d, want 1", len(summary["error"]))
	}
	if summary["error"][0].Occurrences != 1 {
		t.Errorf("source3 occurrences = %d, want 1", summary["error"][0].Occurrences)
	}
}

func TestSinceNoFilter(t *testing.T) {
	// Parser with no since filter (0 duration)
	p, _ := New(5, 1, 0)

	lines := []string{
		`2024-12-30T10:00:00.000000000Z I1230 10:00:00.000000       1 test.go:1] msg1`,
		`2024-12-30T12:00:00.000000000Z I1230 12:00:00.000000       1 test.go:1] msg2`,
	}

	for _, line := range lines {
		p.parseLine(line)
	}

	summary := p.Summary()
	entry := summary["info"][0]

	// Should have both occurrences (no filtering)
	if entry.Occurrences != 2 {
		t.Errorf("occurrences = %d, want 2 (no filter)", entry.Occurrences)
	}
}

func TestSinceWithOutOfOrderTimestamps(t *testing.T) {
	// Parser with 2 hour since filter
	p, _ := New(5, 1, 2*time.Hour)

	// Timestamps are out of order: latest (10:10) appears before 10:05
	lines := []string{
		`2024-12-30T09:00:00.000000000Z I1230 09:00:00.000000       1 test.go:1] oldest`,
		`2024-12-30T10:00:00.000000000Z I1230 10:00:00.000000       1 test.go:1] log 2`,
		`2024-12-30T10:10:00.000000000Z I1230 10:10:00.000000       1 test.go:1] latest`,
		`2024-12-30T10:05:00.000000000Z I1230 10:05:00.000000       1 test.go:1] log 4 (out of order)`,
	}

	for _, line := range lines {
		p.parseLine(line)
	}

	summary := p.Summary()
	entry := summary["info"][0]

	// All 4 logs are within 2 hours of latest (10:10)
	if entry.Occurrences != 4 {
		t.Errorf("occurrences = %d, want 4", entry.Occurrences)
	}

	// Frequency should be based on actual time range: 09:00 to 10:10 = 70 minutes
	// 4 logs / 70 minutes ≈ 3.4/h (duration is > 1 hour, so uses /h unit)
	if entry.Frequency != "3.4/h" {
		t.Errorf("frequency = %q, want \"3.4/h\" (based on chronological min/max, not array order)", entry.Frequency)
	}
}

func TestShortLineReturnsError(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Line shorter than 31 chars should return error
	r := strings.NewReader("short\n")
	err := p.Consume(r)
	if err == nil {
		t.Fatal("expected error for short line, got nil")
	}
	if !strings.Contains(err.Error(), "line too short") {
		t.Errorf("error = %v, want 'line too short'", err)
	}
}

func TestUnstructuredMultipleLinesWithTimestamp(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Unstructured lines with valid timestamps
	input := `2024-12-30T10:46:29.390512670Z Random line 1
2024-12-30T10:46:30.390512670Z Random line 2
2024-12-30T10:46:31.390512670Z Random line 3
`
	r := strings.NewReader(input)
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	unstructured := p.Summary()["unstructured"]
	if len(unstructured) != 1 {
		t.Fatalf("expected 1 unstructured source, got %d", len(unstructured))
	}

	// Should aggregate all 3 lines under same source
	if unstructured[0].Occurrences != 3 {
		t.Errorf("occurrences = %d, want 3", unstructured[0].Occurrences)
	}

	// Check recent contains all 3 lines
	if len(unstructured[0].Recent) != 3 {
		t.Errorf("recent len = %d, want 3", len(unstructured[0].Recent))
	}
}

func TestUnstructuredWithStructured(t *testing.T) {
	p, _ := New(5, 1, 0)
	input := `2024-12-30T10:46:28.390512670Z Random line 1
2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512  1 test.go:1] Info message
2024-12-30T10:46:30.390512670Z Random line 2
2024-12-30T10:46:31.390512670Z {"level":"error","caller":"foo.go:42","msg":"Error message"}
2024-12-30T10:46:32.390512670Z Random line 3
`
	r := strings.NewReader(input)
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	summary := p.Summary()

	// Should have info entry
	if len(summary["info"]) != 1 {
		t.Errorf("info entries = %d, want 1", len(summary["info"]))
	}

	// Should have error entry
	if len(summary["error"]) != 1 {
		t.Errorf("error entries = %d, want 1", len(summary["error"]))
	}

	// Should have unstructured entries
	if len(summary["unstructured"]) != 1 {
		t.Errorf("unstructured entries = %d, want 1", len(summary["unstructured"]))
	}
	if summary["unstructured"][0].Occurrences != 3 {
		t.Errorf("unstructured occurrences = %d, want 3", summary["unstructured"][0].Occurrences)
	}
}

func TestUnstructuredJSONWithUnknownSeverity(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Valid JSON structure but unrecognized severity level
	line := `2024-12-30T10:46:29.390512670Z {"level":"debug","caller":"test.go:1","msg":"debug msg"}`
	r := strings.NewReader(line + "\n")
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	summary := p.Summary()

	// Should be in unstructured, not skipped
	if len(summary["unstructured"]) != 1 {
		t.Fatalf("expected 1 unstructured entry, got %d", len(summary["unstructured"]))
	}

	// Full line should be preserved
	msg, ok := summary["unstructured"][0].Recent[0].Log.(string)
	if !ok || msg != line {
		t.Errorf("log = %q, want full line", msg)
	}
}

func TestUnstructuredEmptyOutput(t *testing.T) {
	p, _ := New(5, 1, 0)
	// Only valid structured logs
	input := `2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512  1 test.go:1] Info
2024-12-30T10:46:29.390512670Z {"level":"error","caller":"foo.go:42","msg":"Error"}
`
	r := strings.NewReader(input)
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	summary := p.Summary()

	// Unstructured should exist but be empty
	unstructured, exists := summary["unstructured"]
	if !exists {
		t.Errorf("unstructured key missing from summary")
	}
	if len(unstructured) != 0 {
		t.Errorf("unstructured entries = %d, want 0 (empty slice)", len(unstructured))
	}
}

func TestUnstructuredKlogUnknownSeverity(t *testing.T) {
	p, _ := New(5, 1, 0)
	// klog pattern matches but severity letter is unknown (X instead of I/W/E/F)
	line := `2024-12-30T10:46:29.390512670Z X1230 10:46:29.390512  1 test.go:1] Unknown severity`
	r := strings.NewReader(line + "\n")
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	summary := p.Summary()

	// Should be captured as unstructured
	if len(summary["unstructured"]) != 1 {
		t.Fatalf("expected 1 unstructured entry, got %d", len(summary["unstructured"]))
	}

	// Full line should be preserved
	msg, ok := summary["unstructured"][0].Recent[0].Log.(string)
	if !ok || msg != line {
		t.Errorf("log = %q, want full line", msg)
	}
}

func TestJSONWithoutTimestampReturnsError(t *testing.T) {
	p, _ := New(5, 1, 0)
	// JSON log without timestamp prefix (like when using --timestamps=false)
	line := `{"level":"info","ts":"2026-05-07T08:12:52.589790Z","caller":"mvcc/hash.go:151","msg":"storing new hash"}`
	r := strings.NewReader(line + "\n")
	err := p.Consume(r)
	if err == nil {
		t.Fatal("expected error for JSON without timestamp prefix, got nil")
	}
	if !strings.Contains(err.Error(), "invalid timestamp prefix") {
		t.Errorf("error = %v, want 'invalid timestamp prefix'", err)
	}
	if !strings.Contains(err.Error(), "did you forget --timestamps=true") {
		t.Errorf("error = %v, should mention --timestamps=true", err)
	}
}
