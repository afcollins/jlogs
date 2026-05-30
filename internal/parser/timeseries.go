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

// TimeseriesSparkline writes a terminal heatmap with one row per (severity, source),
// columns per time interval, and Unicode block characters showing intensity.
// Rows are sorted by total count descending (noisiest first).
// Scaling is per-row so each source's peak is █.
func (p *Parser) TimeseriesSparkline(w io.Writer) error {
	// Collect sorted interval keys (columns).
	intervalSet := make(map[string]struct{})
	for k := range p.timelineBuckets {
		intervalSet[k] = struct{}{}
	}
	intervals := make([]string, 0, len(intervalSet))
	for k := range intervalSet {
		intervals = append(intervals, k)
	}
	sort.Strings(intervals)

	if len(intervals) == 0 {
		return nil
	}

	intervalIdx := make(map[string]int, len(intervals))
	for i, k := range intervals {
		intervalIdx[k] = i
	}

	// Collect per-(severity, source) counts across all intervals.
	type sourceRow struct {
		label  string
		counts []int
		total  int
	}

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
			return rows[i].total > rows[j].total
		}
		return rows[i].label < rows[j].label
	})

	// Compute label padding width.
	maxLabel := 0
	for _, sr := range rows {
		if len(sr.label) > maxLabel {
			maxLabel = len(sr.label)
		}
	}

	// Format interval headers — strip common date prefix if all intervals share it.
	headers := formatIntervalHeaders(intervals)

	// Print header row.
	fmt.Fprintf(w, "%s  %s\n", pad("", maxLabel), strings.Join(headers, ""))

	// Print each source row.
	for _, sr := range rows {
		maxCount := 0
		for _, c := range sr.counts {
			if c > maxCount {
				maxCount = c
			}
		}

		var spark strings.Builder
		for _, c := range sr.counts {
			spark.WriteRune(scaleBlock(c, maxCount))
		}

		fmt.Fprintf(w, "%s  %s  (%d)\n", pad(sr.label, maxLabel), spark.String(), sr.total)
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

// formatIntervalHeaders shortens interval labels when all share a common date prefix.
// Each label is right-padded to a uniform width for column alignment.
func formatIntervalHeaders(intervals []string) []string {
	if len(intervals) == 0 {
		return nil
	}

	// Find common prefix up to 'T' (date portion).
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

	headers := make([]string, len(intervals))
	for i, iv := range intervals {
		if allSameDate && tIdx >= 0 {
			headers[i] = iv[tIdx+1:]
		} else {
			headers[i] = iv
		}
	}

	// Pad all headers to same width for alignment.
	maxW := 0
	for _, h := range headers {
		if len(h) > maxW {
			maxW = len(h)
		}
	}
	colWidth := maxW + 1
	for i, h := range headers {
		headers[i] = pad(h, colWidth)
	}

	return headers
}
