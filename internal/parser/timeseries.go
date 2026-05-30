package parser

import (
	"encoding/csv"
	"fmt"
	"io"
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

// TimeseriesSparkline writes a terminal heatmap with one row per (severity, source),
// columns per time interval, and Unicode block characters showing intensity.
// Rows sorted by total count ascending (noisiest at bottom, near legend).
// Scaling is per-row so each source's peak is █.
// Column headers are single alphanumeric markers; a legend at the bottom maps
// each marker to its timestamp.
//
// When wrap is true, output is chunked into pages of maxWidth columns so
// the display fits in a terminal. Each page repeats the source labels.
// maxWidth of 0 means no wrapping (same as wrap=false).
func (p *Parser) TimeseriesSparkline(w io.Writer, wrap bool, maxWidth int) error {
	intervals, intervalIdx := p.sortedIntervals()
	if len(intervals) == 0 {
		return nil
	}

	rows := p.collectSparklineRows(intervals, intervalIdx)

	maxLabel := 0
	for _, sr := range rows {
		if len(sr.label) > maxLabel {
			maxLabel = len(sr.label)
		}
	}

	// labelOverhead: label + "  " gap + "  (N)\n" suffix.
	// We only need the label + gap for computing available columns.
	labelOverhead := maxLabel + 2

	if !wrap || maxWidth <= 0 {
		return p.writeSparklinePage(w, rows, intervals, 0, len(intervals), maxLabel)
	}

	colsPerPage := maxWidth - labelOverhead
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
		if err := p.writeSparklinePage(w, rows, intervals, start, end, maxLabel); err != nil {
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
		idx := intervalIdx[intervalKey]
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
func (p *Parser) writeSparklinePage(w io.Writer, rows []*sourceRow, intervals []string, start, end, maxLabel int) error {
	pageIntervals := intervals[start:end]
	markers := assignMarkers(len(pageIntervals))

	// Header row.
	fmt.Fprintf(w, "%s  %s\n", pad("", maxLabel), markers)

	// Source rows — sparkline uses only the page's column slice.
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

	// Legend.
	fmt.Fprintln(w)
	legend := formatLegend(markers, pageIntervals)
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

func assignMarkers(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(markerChars[i%len(markerChars)])
	}
	return b.String()
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
