package parser

import (
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

var severityPrefix = map[Severity]string{
	SevInfo:         "I",
	SevWarning:      "W",
	SevError:        "E",
	SevFatal:        "F",
	SevUnstructured: "U",
}

// TimeseriesCSV writes timeline data as CSV rows:
//
//	interval,severity,source,count
//
// Rows are sorted by interval, then severity, then source.
func (p *Parser) TimeseriesCSV(w io.Writer) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if err := cw.Write([]string{"interval", "severity", "source", "count"}); err != nil {
		return err
	}

	type row struct {
		interval string
		severity string
		source   string
		count    int
	}

	var rows []row
	for intervalKey, sourceBucket := range p.timelineBuckets {
		for key, agg := range sourceBucket {
			rows = append(rows, row{
				interval: intervalKey,
				severity: string(key.severity),
				source:   key.source,
				count:    agg.count,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].interval != rows[j].interval {
			return rows[i].interval < rows[j].interval
		}
		if rows[i].severity != rows[j].severity {
			return rows[i].severity < rows[j].severity
		}
		return rows[i].source < rows[j].source
	})

	for _, r := range rows {
		if err := cw.Write([]string{r.interval, r.severity, r.source, strconv.Itoa(r.count)}); err != nil {
			return err
		}
	}
	return nil
}

var sparkBlocks = []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

const markerChars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

type sourceRow struct {
	label  string
	counts []int
	total  int
}

// SparklineOptions controls sparkline output behavior.
type SparklineOptions struct {
	Wrap     bool
	MaxWidth int
	Zoom     string // marker substring or "HH:MM-HH:MM" timestamp range
	Source   string // substring filter on source labels
}

// TimeseriesSparkline writes a terminal heatmap with one row per (severity, source),
// columns per time interval, and Unicode block characters showing intensity.
// Rows sorted by total count ascending (noisiest at bottom, near legend).
// Scaling is per-row so each source's peak is █.
// Column headers are shuffled alphanumeric markers; a legend at the bottom maps
// each marker to its timestamp. Markers are shuffled so any 3-4 char substring
// is a unique fingerprint that can be copied for -zoom.
func (p *Parser) TimeseriesSparkline(w io.Writer, opts SparklineOptions) error {
	intervals, intervalIdx := p.sortedIntervals()
	if len(intervals) == 0 {
		return nil
	}

	// Assign shuffled markers to all intervals.
	allMarkers := shuffleMarkers(len(intervals))

	// Apply zoom filter if specified.
	if opts.Zoom != "" {
		var zoomIndices []int
		if isTimestampRange(opts.Zoom) {
			zoomIndices = filterByTimestampRange(intervals, opts.Zoom)
		} else {
			zoomIndices = filterByMarkerSubstring(allMarkers, opts.Zoom)
		}
		if len(zoomIndices) == 0 {
			fmt.Fprintf(w, "no intervals match zoom %q\n", opts.Zoom)
			return nil
		}
		// Rebuild intervals and markers for the zoomed subset.
		newIntervals := make([]string, len(zoomIndices))
		var newMarkers strings.Builder
		for i, idx := range zoomIndices {
			newIntervals[i] = intervals[idx]
			newMarkers.WriteByte(allMarkers[idx])
		}
		intervals = newIntervals
		allMarkers = newMarkers.String()
		// Rebuild intervalIdx for the filtered set.
		intervalIdx = make(map[string]int, len(intervals))
		for i, k := range intervals {
			intervalIdx[k] = i
		}
	}

	rows := p.collectSparklineRows(intervals, intervalIdx)

	// Apply source filter.
	if opts.Source != "" {
		filtered := rows[:0]
		for _, sr := range rows {
			if strings.Contains(sr.label, opts.Source) {
				filtered = append(filtered, sr)
			}
		}
		rows = filtered
		if len(rows) == 0 {
			fmt.Fprintf(w, "no sources match %q\n", opts.Source)
			return nil
		}
	}

	maxLabel := 0
	for _, sr := range rows {
		if len(sr.label) > maxLabel {
			maxLabel = len(sr.label)
		}
	}

	labelOverhead := maxLabel + 2

	if !opts.Wrap || opts.MaxWidth <= 0 {
		return writeSparklinePage(w, rows, intervals, allMarkers, 0, len(intervals), maxLabel)
	}

	colsPerPage := opts.MaxWidth - labelOverhead
	if colsPerPage < 1 {
		colsPerPage = 1
	}

	for start := 0; start < len(intervals); start += colsPerPage {
		end := start + colsPerPage
		if end > len(intervals) {
			end = len(intervals)
		}
		if start > 0 {
			fmt.Fprintln(w)
		}
		if err := writeSparklinePage(w, rows, intervals, allMarkers, start, end, maxLabel); err != nil {
			return err
		}
	}
	return nil
}

func (p *Parser) sortedIntervals() ([]string, map[string]int) {
	intervalSet := make(map[string]struct{})
	for k := range p.timelineBuckets {
		intervalSet[k] = struct{}{}
	}
	intervals := make([]string, 0, len(intervalSet))
	for k := range intervalSet {
		intervals = append(intervals, k)
	}
	sort.Strings(intervals)

	intervalIdx := make(map[string]int, len(intervals))
	for i, k := range intervals {
		intervalIdx[k] = i
	}
	return intervals, intervalIdx
}

func (p *Parser) collectSparklineRows(intervals []string, intervalIdx map[string]int) []*sourceRow {
	rowMap := make(map[timelineBucketKey]*sourceRow)
	for intervalKey, sourceBucket := range p.timelineBuckets {
		idx, ok := intervalIdx[intervalKey]
		if !ok {
			continue
		}
		for key, agg := range sourceBucket {
			sr, ok := rowMap[key]
			if !ok {
				prefix := severityPrefix[key.severity]
				sr = &sourceRow{
					label:  fmt.Sprintf("[%s] %s", prefix, key.source),
					counts: make([]int, len(intervals)),
				}
				rowMap[key] = sr
			}
			sr.counts[idx] = agg.count
			sr.total += agg.count
		}
	}

	rows := make([]*sourceRow, 0, len(rowMap))
	for _, sr := range rowMap {
		rows = append(rows, sr)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].total != rows[j].total {
			return rows[i].total < rows[j].total
		}
		return rows[i].label < rows[j].label
	})
	return rows
}

// writeSparklinePage writes one page of the sparkline covering intervals[start:end].
func writeSparklinePage(w io.Writer, rows []*sourceRow, intervals []string, allMarkers string, start, end, maxLabel int) error {
	pageMarkers := allMarkers[start:end]

	// Source rows.
	for _, sr := range rows {
		maxCount := 0
		for _, c := range sr.counts {
			if c > maxCount {
				maxCount = c
			}
		}

		var spark strings.Builder
		for _, c := range sr.counts[start:end] {
			spark.WriteRune(scaleBlock(c, maxCount))
		}

		fmt.Fprintf(w, "%s  %s  (%d)\n", pad(sr.label, maxLabel), spark.String(), sr.total)
	}

	// Marker row at bottom.
	fmt.Fprintf(w, "%s  %s\n", pad("", maxLabel), pageMarkers)

	// Legend.
	fmt.Fprintln(w)
	pageIntervals := intervals[start:end]
	legend := formatLegend(pageMarkers, pageIntervals)
	for _, line := range legend {
		fmt.Fprintln(w, line)
	}

	return nil
}

func scaleBlock(count, max int) rune {
	if max == 0 || count == 0 {
		return sparkBlocks[0]
	}
	idx := (count * (len(sparkBlocks) - 1)) / max
	return sparkBlocks[idx]
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// shuffleMarkers generates a deterministic pseudo-random sequence of marker
// characters for n intervals. The shuffle ensures that any 3-4 character
// substring is very likely unique, allowing users to copy a range from the
// sparkline and use it with -zoom.
func shuffleMarkers(n int) string {
	chars := []byte(markerChars)
	rng := rand.New(rand.NewSource(42))
	rng.Shuffle(len(chars), func(i, j int) {
		chars[i], chars[j] = chars[j], chars[i]
	})

	var b strings.Builder
	for i := 0; i < n; i++ {
		// Cycle through shuffled set, re-shuffle each full cycle for variety.
		idx := i % len(chars)
		if idx == 0 && i > 0 {
			rng.Shuffle(len(chars), func(a, c int) {
				chars[a], chars[c] = chars[c], chars[a]
			})
		}
		b.WriteByte(chars[idx])
	}
	return b.String()
}

// isTimestampRange checks if s looks like a timestamp range (contains ':' and '-'
// in a pattern like "HH:MM-HH:MM" or "YYYY-MM-DDTHH:MM-YYYY-MM-DDTHH:MM").
func isTimestampRange(s string) bool {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return false
	}
	return strings.Contains(parts[0], ":") && strings.Contains(parts[1], ":")
}

// filterByTimestampRange returns indices of intervals whose time portion falls
// within the given range string "start-end". Comparison is lexicographic on
// the time portion (after 'T').
func filterByTimestampRange(intervals []string, zoomRange string) []int {
	parts := strings.SplitN(zoomRange, "-", 2)
	if len(parts) != 2 {
		return nil
	}
	lo, hi := parts[0], parts[1]

	var indices []int
	for i, iv := range intervals {
		timePart := iv
		if tIdx := strings.IndexByte(iv, 'T'); tIdx >= 0 {
			timePart = iv[tIdx+1:]
		}
		if timePart >= lo && timePart <= hi {
			indices = append(indices, i)
		}
	}
	return indices
}

// filterByMarkerSubstring finds the marker substring in the full marker string
// and returns the corresponding interval indices.
func filterByMarkerSubstring(markers, sub string) []int {
	idx := strings.Index(markers, sub)
	if idx < 0 {
		return nil
	}
	indices := make([]int, len(sub))
	for i := range sub {
		indices[i] = idx + i
	}
	return indices
}

// formatLegend builds lines mapping each marker character to its interval timestamp.
// Strips common date prefix when all intervals share the same date.
func formatLegend(markers string, intervals []string) []string {
	display := make([]string, len(intervals))
	first := intervals[0]
	tIdx := strings.IndexByte(first, 'T')
	allSameDate := tIdx >= 0
	if allSameDate {
		datePrefix := first[:tIdx+1]
		for _, iv := range intervals[1:] {
			if !strings.HasPrefix(iv, datePrefix) {
				allSameDate = false
				break
			}
		}
	}
	for i, iv := range intervals {
		if allSameDate && tIdx >= 0 {
			display[i] = iv[tIdx+1:]
		} else {
			display[i] = iv
		}
	}

	maxW := 0
	for _, d := range display {
		if len(d) > maxW {
			maxW = len(d)
		}
	}

	const perLine = 5
	var lines []string
	for i := 0; i < len(intervals); i += perLine {
		end := i + perLine
		if end > len(intervals) {
			end = len(intervals)
		}
		var parts []string
		for j := i; j < end; j++ {
			parts = append(parts, fmt.Sprintf("%c: %s", markers[j], pad(display[j], maxW)))
		}
		lines = append(lines, strings.Join(parts, "  "))
	}
	return lines
}
