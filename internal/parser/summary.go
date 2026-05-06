package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Summary is the top-level shape returned to callers. Map keys are severity
// names; values are the per-source aggregations. Severities with zero entries
// are still present (as empty slices) so downstream consumers can rely on the
// shape — this is a deliberate change from the Python version, which omitted
// empty severities only sometimes.
type Summary map[string][]SourceSummary

// Summary builds the final report from accumulated state. Sources within each
// severity are sorted by occurrence count (ascending), with the highest
// occurrences appearing at the bottom.
func (p *Parser) Summary() Summary {
	out := make(Summary, len(trackedSeverities))

	// Calculate cutoff time if --since is specified
	var cutoff time.Time
	applySinceFilter := p.since > 0 && p.hasLatestTime
	if applySinceFilter {
		cutoff = p.latestTime.Add(-p.since)
	}

	for _, sev := range trackedSeverities {
		bucket := p.buckets[sev]
		entries := make([]SourceSummary, 0, len(bucket))
		for source, agg := range bucket {
			recent := agg.recent.snapshot()
			first := agg.first
			occurrences := agg.occurrences
			firstTS := agg.firstTimestamp
			lastTS := agg.lastTimestamp

			// Apply time filter if --since is specified
			if applySinceFilter {
				recent = filterOccurrences(recent, cutoff)
				first = filterOccurrences(first, cutoff)
				occurrences = len(recent) + len(first)

				// Recalculate unique count (recent + first might overlap)
				allFiltered := append([]Occurrence{}, first...)
				allFiltered = append(allFiltered, recent...)
				occurrences = len(deduplicateOccurrences(allFiltered))

				// Skip sources with no occurrences after filtering
				if occurrences == 0 {
					continue
				}

				// Update first/last timestamps for frequency calculation
				if len(first) > 0 {
					firstTS = first[0].Time
				} else if len(recent) > 0 {
					firstTS = recent[0].Time
				}
				if len(recent) > 0 {
					lastTS = recent[len(recent)-1].Time
				} else if len(first) > 0 {
					lastTS = first[len(first)-1].Time
				}
			}

			entries = append(entries, SourceSummary{
				Source:      source,
				Occurrences: occurrences,
				Frequency:   calculateFrequency(occurrences, firstTS, lastTS),
				Recent:      recent,
				First:       first,
				recentKey:   p.recentKey,
				firstKey:    p.firstKey,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Occurrences < entries[j].Occurrences
		})
		out[string(sev)] = entries
	}
	return out
}

// filterOccurrences returns only occurrences at or after the cutoff time.
func filterOccurrences(occurrences []Occurrence, cutoff time.Time) []Occurrence {
	filtered := make([]Occurrence, 0, len(occurrences))
	for _, occ := range occurrences {
		t, err := time.Parse(time.RFC3339Nano, occ.Time)
		if err == nil && !t.Before(cutoff) {
			filtered = append(filtered, occ)
		}
	}
	return filtered
}

// deduplicateOccurrences removes duplicate occurrences based on timestamp+log.
func deduplicateOccurrences(occurrences []Occurrence) []Occurrence {
	seen := make(map[string]bool)
	result := make([]Occurrence, 0, len(occurrences))
	for _, occ := range occurrences {
		key := occ.Time + fmt.Sprint(occ.Log)
		if !seen[key] {
			seen[key] = true
			result = append(result, occ)
		}
	}
	return result
}

// jsonMarshal is a thin wrapper that disables HTML escaping so log messages
// containing characters like "<" or "&" round-trip cleanly.
func jsonMarshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encoder appends a trailing newline; trim it for consistency with
	// json.Marshal's behavior, since callers concatenate this output.
	b := buf.Bytes()
	if n := len(b); n > 0 && b[n-1] == '\n' {
		b = b[:n-1]
	}
	return b, nil
}
