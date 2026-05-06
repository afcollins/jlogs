package parser

import (
	"strings"
	"testing"
)

func TestParseKlogLine(t *testing.T) {
	p, err := New(5, 1)
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
	p, _ := New(5, 1)
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
			p, _ := New(5, 1)
			line := `2024-12-30T10:46:29.390512670Z {"level":"` + tc.level + `","caller":"x.go:1","msg":"m"}`
			p.parseLine(line)
			if got := len(p.Summary()[tc.want]); got != 1 {
				t.Errorf("severity %q: got %d entries in bucket %q, want 1", tc.level, got, tc.want)
			}
		})
	}
}

func TestRingOverflow(t *testing.T) {
	p, _ := New(3, 1)
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

func TestMalformedJSONIsSkipped(t *testing.T) {
	p, _ := New(5, 1)
	// Bad JSON should not crash or pollute output.
	bad := `2024-12-30T10:46:29.390512670Z {not valid json}`
	good := `2024-12-30T10:46:29.390512670Z {"level":"info","caller":"x.go:1","msg":"ok"}`
	r := strings.NewReader(bad + "\n" + good + "\n")
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if got := len(p.Summary()["info"]); got != 1 {
		t.Errorf("info entries = %d, want 1 (good line only)", got)
	}
}

func TestUnrecognizedLineIsSkipped(t *testing.T) {
	p, _ := New(5, 1)
	r := strings.NewReader("this is not a structured log line\n")
	if err := p.Consume(r); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	for sev, entries := range p.Summary() {
		if len(entries) != 0 {
			t.Errorf("bucket %s should be empty, got %d", sev, len(entries))
		}
	}
}

func TestNewClampsLastN(t *testing.T) {
	for _, n := range []int{0, -1, 11, 999} {
		p, err := New(n, 1)
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
		p, _ := New(n, 1)
		if p.recentKey != "last_occurrences" {
			t.Errorf("New(%d, 1) recentKey = %q, want %q", n, p.recentKey, "last_occurrences")
		}
	}
}

func TestNewClampsFirstN(t *testing.T) {
	for _, n := range []int{0, -1, 11, 999} {
		p, err := New(5, n)
		if err != nil {
			t.Fatalf("New(5, %d): %v", n, err)
		}
		if p.firstN != 1 {
			t.Errorf("New(5, %d) firstN = %d, want 1 fallback", n, p.firstN)
		}
	}
}

func TestFirstOccurrences(t *testing.T) {
	p, _ := New(3, 2)
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
