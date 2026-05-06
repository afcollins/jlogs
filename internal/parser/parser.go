// Package parser ingests log lines and aggregates them into a per-severity,
// per-source summary suitable for JSON serialization.
//
// Two log formats are recognized:
//
//  1. klog-style lines, e.g.:
//     2024-12-30T10:46:29.390512670Z I1230 10:46:29.390512  1 node_controller.go:1056] No nodes available
//
//  2. JSON-structured lines (the JSON object follows a 30-byte timestamp +
//     space prefix), e.g.:
//     2024-12-30T10:46:29.390512670Z {"level":"info","caller":"foo.go:42","msg":"..."}
//
// Lines that match neither format, or that contain malformed JSON, are
// silently skipped — log parsers must not crash on a single bad line.
package parser

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Severity is the canonical severity bucket for an entry. Using a typed string
// instead of a bare string prevents typos at compile time and gives us a
// single source of truth for valid values.
type Severity string

const (
	SevInfo    Severity = "info"
	SevWarning Severity = "warning"
	SevError   Severity = "error"
	SevFatal   Severity = "fatal"
)

// trackedSeverities is the ordered set of severities we aggregate. Order is
// preserved in the output for stable, human-friendly diffs.
var trackedSeverities = []Severity{SevInfo, SevWarning, SevError, SevFatal}

// Occurrence is a single retained log event for a given source.
type Occurrence struct {
	Time string `json:"time"`
	// Log holds either a string (klog message) or a parsed JSON object
	// (structured log). interface{} is appropriate here because the original
	// Python output mixed both shapes; downstream consumers already handle it.
	Log any `json:"log"`
}

// SourceSummary is the per-source aggregation emitted in the final report.
type SourceSummary struct {
	Source      string       `json:"source_filename_linenumber"`
	Occurrences int          `json:"occurrences"`
	Recent      []Occurrence `json:"-"` // rendered under "last_occurrences" key; see MarshalJSON
	First       []Occurrence `json:"-"` // rendered under "first_occurrences" key; see MarshalJSON
	recentKey   string       // "last_occurrences"
	firstKey    string       // "first_occurrences"
}

// MarshalJSON renders SourceSummary with the "last_occurrences" and "first_occurrences" keys.
func (s SourceSummary) MarshalJSON() ([]byte, error) {
	// Build a map so we can place the dynamic keys alongside the static fields.
	out := map[string]any{
		"source_filename_linenumber": s.Source,
		"occurrences":                s.Occurrences,
		s.firstKey:                   s.First,
		s.recentKey:                  s.Recent,
	}
	return jsonMarshal(out)
}

// klogPattern matches a klog-formatted log line. The leading (\S+) captures
// the RFC3339-ish timestamp prefix the upstream logging pipeline prepends.
//
//	1: timestamp prefix (e.g. 2024-12-30T10:46:29.390512670Z)
//	2: klog level + date (e.g. I1230)
//	3: source file:line   (e.g. node_controller.go:1056)
//	4: message            (rest of line)
var klogPattern = regexp.MustCompile(`(\S+)\s+([IWEF]\d{4})\s+\S+\s+\d+\s+(\S+\.go:\d+)\]\s+(.*)`)

// klogLevelToSeverity maps the single-letter klog prefix to our canonical
// severity. Anything not in this map is unknown and dropped.
var klogLevelToSeverity = map[byte]Severity{
	'I': SevInfo,
	'W': SevWarning,
	'E': SevError,
	'F': SevFatal,
}

// jsonLevelToSeverity maps the strings that may appear in a structured log's
// "level" or "severity" field to our canonical severity.
var jsonLevelToSeverity = map[string]Severity{
	"info":    SevInfo,
	"warn":    SevWarning,
	"warning": SevWarning,
	"error":   SevError,
	"fatal":   SevFatal,
}

// Parser accumulates parsed log entries. Construct with New, feed lines via
// Consume, then read the result with Summary. A Parser is not safe for
// concurrent use; create one per goroutine if you need parallelism.
type Parser struct {
	lastN     int
	firstN    int
	recentKey string
	firstKey  string
	// buckets[severity][source] -> aggregator
	buckets map[Severity]map[string]*sourceAggregator
}

// sourceAggregator tracks running counts, a fixed-size ring of recent
// occurrences, and the first N occurrences for a single (severity, source) pair.
type sourceAggregator struct {
	occurrences int
	recent      *ring
	first       []Occurrence
	firstCap    int
}

// New constructs a Parser. lastN and firstN must be in the range [1, 10];
// values outside that range fall back to 5 and 1 respectively.
func New(lastN, firstN int) (*Parser, error) {
	if lastN < 1 || lastN > 10 {
		lastN = 5
	}
	if firstN < 1 || firstN > 10 {
		firstN = 1
	}
	p := &Parser{
		lastN:     lastN,
		firstN:    firstN,
		recentKey: "last_occurrences",
		firstKey:  "first_occurrences",
		buckets:   make(map[Severity]map[string]*sourceAggregator, len(trackedSeverities)),
	}
	for _, sev := range trackedSeverities {
		p.buckets[sev] = make(map[string]*sourceAggregator)
	}
	return p, nil
}

// Consume reads r line-by-line until EOF, parsing and aggregating each line.
// Malformed individual lines are skipped silently; only I/O errors are
// returned. We use a Scanner with a generous buffer because real-world logs
// occasionally contain very long lines (stack traces, JSON blobs).
func (p *Parser) Consume(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Default Scanner buffer is 64KB which is too small for long stack traces.
	// 1MB handles the vast majority of real log lines without unbounded growth.
	const maxLine = 1024 * 1024
	scanner.Buffer(make([]byte, 0, 64*1024), maxLine)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if line == "" {
			continue
		}
		p.parseLine(line)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading input: %w", err)
	}
	return nil
}

// addEntry records one parsed log event under the appropriate severity bucket.
func (p *Parser) addEntry(sev Severity, source, timestamp string, message any) {
	bucket, ok := p.buckets[sev]
	if !ok {
		// Severity isn't tracked; drop. This happens for "unknown" levels.
		return
	}
	agg, ok := bucket[source]
	if !ok {
		agg = &sourceAggregator{
			recent:   newRing(p.lastN),
			first:    make([]Occurrence, 0, p.firstN),
			firstCap: p.firstN,
		}
		bucket[source] = agg
	}
	agg.occurrences++
	agg.recent.push(Occurrence{Time: timestamp, Log: message})
	// Only collect first N occurrences
	if len(agg.first) < agg.firstCap {
		agg.first = append(agg.first, Occurrence{Time: timestamp, Log: message})
	}
}
