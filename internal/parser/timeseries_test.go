package parser

import (
	"bytes"
	"strings"
	"testing"
)

func TestTimeseriesCSVBasic(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg1`,
		`2024-12-30T10:46:02.000000000Z I1230 10:46:02.000000  1 a.go:1] msg2`,
		`2024-12-30T10:47:01.000000000Z {"level":"error","caller":"b.go:5","msg":"fail"}`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	if err := p.TimeseriesCSV(&buf); err != nil {
		t.Fatalf("TimeseriesCSV error: %v", err)
	}

	out := buf.String()
	csvLines := strings.Split(strings.TrimSpace(out), "\n")

	if len(csvLines) != 3 {
		t.Fatalf("expected 3 lines (header + 2 data), got %d:\n%s", len(csvLines), out)
	}

	if csvLines[0] != "interval,severity,source,count" {
		t.Errorf("header = %q, want interval,severity,source,count", csvLines[0])
	}

	// Rows sorted by interval, severity, source
	if csvLines[1] != "2024-12-30T10:46,info,a.go:1,2" {
		t.Errorf("row[0] = %q, want 2024-12-30T10:46,info,a.go:1,2", csvLines[1])
	}
	if csvLines[2] != "2024-12-30T10:47,error,b.go:5,1" {
		t.Errorf("row[1] = %q, want 2024-12-30T10:47,error,b.go:5,1", csvLines[2])
	}
}

func TestTimeseriesCSVEmpty(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	var buf bytes.Buffer
	if err := p.TimeseriesCSV(&buf); err != nil {
		t.Fatalf("TimeseriesCSV error: %v", err)
	}

	out := buf.String()
	if strings.TrimSpace(out) != "interval,severity,source,count" {
		t.Errorf("empty CSV should have header only, got:\n%s", out)
	}
}

func TestTimeseriesCSVSorted(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 z.go:1] msg`,
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:02.000000000Z W1230 10:46:02.000000  1 a.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesCSV(&buf)

	csvLines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Should be sorted: 10:46/info/a.go, 10:46/warning/a.go, 10:47/info/z.go
	if len(csvLines) != 4 {
		t.Fatalf("expected 4 lines, got %d", len(csvLines))
	}
	if !strings.HasPrefix(csvLines[1], "2024-12-30T10:46,info,") {
		t.Errorf("row[0] should start with 10:46,info, got %q", csvLines[1])
	}
	if !strings.HasPrefix(csvLines[2], "2024-12-30T10:46,warning,") {
		t.Errorf("row[1] should start with 10:46,warning, got %q", csvLines[2])
	}
	if !strings.HasPrefix(csvLines[3], "2024-12-30T10:47,info,") {
		t.Errorf("row[2] should start with 10:47,info, got %q", csvLines[3])
	}
}

func TestTimeseriesSparklineBasic(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:02.000000000Z I1230 10:46:02.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:03.000000000Z I1230 10:46:03.000000  1 a.go:1] msg`,
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 a.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	if err := p.TimeseriesSparkline(&buf); err != nil {
		t.Fatalf("TimeseriesSparkline error: %v", err)
	}

	out := buf.String()
	outLines := strings.Split(strings.TrimSpace(out), "\n")

	if len(outLines) != 2 {
		t.Fatalf("expected 2 lines (header + 1 source), got %d:\n%s", len(outLines), out)
	}

	// Header should contain time portions (same date → stripped)
	if !strings.Contains(outLines[0], "10:46") {
		t.Errorf("header missing 10:46: %q", outLines[0])
	}

	// Source row should contain label, sparkline chars, and total
	if !strings.Contains(outLines[1], "[I] a.go:1") {
		t.Errorf("source row missing label: %q", outLines[1])
	}
	if !strings.Contains(outLines[1], "(4)") {
		t.Errorf("source row missing total (4): %q", outLines[1])
	}
}

func TestTimeseriesSparklineEmpty(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	var buf bytes.Buffer
	if err := p.TimeseriesSparkline(&buf); err != nil {
		t.Fatalf("TimeseriesSparkline error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("empty sparkline should produce no output, got %q", buf.String())
	}
}

func TestTimeseriesSparklineOrdering(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	// b.go has more total logs than a.go — should appear first
	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:02.000000000Z I1230 10:46:02.000000  1 b.go:1] msg`,
		`2024-12-30T10:46:03.000000000Z I1230 10:46:03.000000  1 b.go:1] msg`,
		`2024-12-30T10:46:04.000000000Z I1230 10:46:04.000000  1 b.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf)

	outLines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Skip header (line 0), first source row should be b.go (higher count)
	if !strings.Contains(outLines[1], "b.go:1") {
		t.Errorf("first source row should be b.go:1 (highest count), got %q", outLines[1])
	}
	if !strings.Contains(outLines[2], "a.go:1") {
		t.Errorf("second source row should be a.go:1, got %q", outLines[2])
	}
}

func TestTimeseriesSparklineMixedSeverities(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] info msg`,
		`2024-12-30T10:46:02.000000000Z E1230 10:46:02.000000  1 a.go:1] error msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf)

	out := buf.String()
	// Same source at different severities → separate rows
	if !strings.Contains(out, "[I] a.go:1") {
		t.Errorf("missing [I] a.go:1 row:\n%s", out)
	}
	if !strings.Contains(out, "[E] a.go:1") {
		t.Errorf("missing [E] a.go:1 row:\n%s", out)
	}
}

func TestTimeseriesSparklineScaling(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	// a.go: 8 in first bin, 0 in second → should be █▁
	// b.go: 1 in first, 1 in second → should be ██ (both are max)
	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:02.000000000Z I1230 10:46:02.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:03.000000000Z I1230 10:46:03.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:04.000000000Z I1230 10:46:04.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:05.000000000Z I1230 10:46:05.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:06.000000000Z I1230 10:46:06.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:07.000000000Z I1230 10:46:07.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:08.000000000Z I1230 10:46:08.000000  1 a.go:1] msg`,
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 b.go:1] msg`,
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 b.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf)

	outLines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	// a.go should be first (8 total > 2 total)
	if !strings.Contains(outLines[1], "[I] a.go:1") {
		t.Errorf("first row should be a.go:1, got %q", outLines[1])
	}
	// a.go sparkline: max=8 in first bin, 0 in second → █▁
	if !strings.Contains(outLines[1], "█▁") {
		t.Errorf("a.go sparkline should contain █▁, got %q", outLines[1])
	}
	// b.go sparkline: 1 and 1, both equal max → ██
	if !strings.Contains(outLines[2], "██") {
		t.Errorf("b.go sparkline should contain ██, got %q", outLines[2])
	}
}

func TestTimelineSeverityTracking(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] info msg`,
		`2024-12-30T10:46:02.000000000Z E1230 10:46:02.000000  1 a.go:1] error msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	tl := p.Timeline()

	if len(tl) != 1 {
		t.Fatalf("expected 1 bin, got %d", len(tl))
	}

	// Same source at different severities → 2 entries
	if len(tl[0].Sources) != 2 {
		t.Fatalf("expected 2 sources (same file, different severity), got %d", len(tl[0].Sources))
	}

	sevSet := map[string]bool{}
	for _, s := range tl[0].Sources {
		sevSet[s.Severity] = true
		if s.Source != "a.go:1" {
			t.Errorf("source = %q, want a.go:1", s.Source)
		}
	}
	if !sevSet["info"] || !sevSet["error"] {
		t.Errorf("expected info and error severities, got %v", sevSet)
	}
}
