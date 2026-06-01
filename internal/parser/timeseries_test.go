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
	if err := p.TimeseriesSparkline(&buf, SparklineOptions{}); err != nil {
		t.Fatalf("TimeseriesSparkline error: %v", err)
	}

	out := buf.String()
	outLines := strings.Split(strings.TrimSpace(out), "\n")

	// source row + marker row + blank + legend line(s)
	if len(outLines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d:\n%s", len(outLines), out)
	}

	// Source row (first data line) should contain label and total
	if !strings.Contains(outLines[0], "[I] a.go:1") {
		t.Errorf("source row missing label: %q", outLines[0])
	}
	if !strings.Contains(outLines[0], "(4)") {
		t.Errorf("source row missing total (4): %q", outLines[0])
	}

	// Marker row should have exactly 2 characters (2 intervals)
	// Markers are shuffled so we just check the row has 2 non-space marker chars
	markerLine := strings.TrimSpace(outLines[1])
	if len(markerLine) != 2 {
		t.Errorf("marker row should have 2 chars, got %d: %q", len(markerLine), markerLine)
	}

	// Legend should contain timestamps
	if !strings.Contains(out, "10:46") || !strings.Contains(out, "10:47") {
		t.Errorf("legend missing timestamps:\n%s", out)
	}
}

func TestTimeseriesSparklineEmpty(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	var buf bytes.Buffer
	if err := p.TimeseriesSparkline(&buf, SparklineOptions{}); err != nil {
		t.Fatalf("TimeseriesSparkline error: %v", err)
	}

	if buf.Len() != 0 {
		t.Errorf("empty sparkline should produce no output, got %q", buf.String())
	}
}

func TestTimeseriesSparklineOrdering(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	// b.go has more total logs than a.go — should appear last (ascending sort)
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
	p.TimeseriesSparkline(&buf, SparklineOptions{})

	outLines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	// Source rows come first; ascending sort: a.go (1) first, b.go (3) last
	if !strings.Contains(outLines[0], "a.go:1") {
		t.Errorf("first source row should be a.go:1 (lowest count), got %q", outLines[0])
	}
	if !strings.Contains(outLines[1], "b.go:1") {
		t.Errorf("second source row should be b.go:1 (highest count), got %q", outLines[1])
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
	p.TimeseriesSparkline(&buf, SparklineOptions{})

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
	p.TimeseriesSparkline(&buf, SparklineOptions{})

	outLines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	// ascending sort: b.go (2 total) first, a.go (8 total) last
	// source rows come before marker row
	if !strings.Contains(outLines[0], "[I] b.go:1") {
		t.Errorf("first row should be b.go:1 (lowest count), got %q", outLines[0])
	}
	if !strings.Contains(outLines[0], "██") {
		t.Errorf("b.go sparkline should contain ██, got %q", outLines[0])
	}
	if !strings.Contains(outLines[1], "[I] a.go:1") {
		t.Errorf("second row should be a.go:1 (highest count), got %q", outLines[1])
	}
	if !strings.Contains(outLines[1], "█▁") {
		t.Errorf("a.go sparkline should contain █▁, got %q", outLines[1])
	}
}

func TestTimeseriesSparklineWrap(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	// 3 intervals, wrap at width that fits only 2 columns per page
	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:48:01.000000000Z I1230 10:48:01.000000  1 a.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	// label "[I] a.go:1" = 10 chars + 2 gap = 12 overhead. Width 14 → 2 cols per page.
	if err := p.TimeseriesSparkline(&buf, SparklineOptions{Wrap: true, MaxWidth: 14}); err != nil {
		t.Fatalf("TimeseriesSparkline wrap error: %v", err)
	}

	out := buf.String()

	// Should have 2 pages with legends
	if !strings.Contains(out, "10:46") || !strings.Contains(out, "10:48") {
		t.Errorf("legend missing timestamps:\n%s", out)
	}
}

func TestTimeseriesSparklineSourceFilter(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:46:02.000000000Z I1230 10:46:02.000000  1 b.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf, SparklineOptions{Source: "a.go"})

	out := buf.String()
	if !strings.Contains(out, "a.go:1") {
		t.Errorf("output should contain a.go:1:\n%s", out)
	}
	if strings.Contains(out, "b.go:1") {
		t.Errorf("output should not contain b.go:1:\n%s", out)
	}
}

func TestTimeseriesSparklineZoomTimestamp(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:48:01.000000000Z I1230 10:48:01.000000  1 a.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf, SparklineOptions{Zoom: "10:46-10:47"})

	out := buf.String()
	if !strings.Contains(out, "10:46") || !strings.Contains(out, "10:47") {
		t.Errorf("zoomed output should contain 10:46 and 10:47:\n%s", out)
	}
	if strings.Contains(out, "10:48") {
		t.Errorf("zoomed output should not contain 10:48:\n%s", out)
	}
}

func TestTimeseriesSparklineZoomMarkers(t *testing.T) {
	p, _ := New(5, 1, 0)
	p.SetTimelineInterval("minute")

	lines := []string{
		`2024-12-30T10:46:01.000000000Z I1230 10:46:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:47:01.000000000Z I1230 10:47:01.000000  1 a.go:1] msg`,
		`2024-12-30T10:48:01.000000000Z I1230 10:48:01.000000  1 a.go:1] msg`,
	}
	for _, l := range lines {
		p.parseLine(l)
	}

	// Get the full markers first.
	markers := shuffleMarkers(3)

	// Zoom to first 2 markers.
	sub := markers[:2]

	var buf bytes.Buffer
	p.TimeseriesSparkline(&buf, SparklineOptions{Zoom: sub})

	out := buf.String()
	// Should show exactly 2 intervals.
	if !strings.Contains(out, "10:46") || !strings.Contains(out, "10:47") {
		t.Errorf("zoomed output should contain first 2 intervals:\n%s", out)
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
