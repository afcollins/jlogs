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
				// Calculate the true occurrence count by filtering all timestamps
				trueOccurrences := 0
				var filteredFirstTS, filteredLastTS string
				for _, ts := range agg.allTimestamps {
					t, err := time.Parse(time.RFC3339Nano, ts)
					if err == nil && !t.Before(cutoff) {
						trueOccurrences++
						if filteredFirstTS == "" {
							filteredFirstTS = ts
						}
						filteredLastTS = ts
					}
				}

				// Skip sources with no occurrences after filtering
				if trueOccurrences == 0 {
					continue
				}

				// Combine and filter the actual occurrence data (for display)
				allOccurrences := append([]Occurrence{}, first...)
				allOccurrences = append(allOccurrences, recent...)
				allOccurrences = deduplicateOccurrences(allOccurrences)
				filtered := filterOccurrences(allOccurrences, cutoff)

				// Sort by timestamp to extract first N and last N
				sortOccurrencesByTime(filtered)

				// Extract first N occurrences from filtered set
				firstN := min(len(filtered), p.firstN)
				first = filtered[:firstN]

				// Extract last N occurrences from filtered set
				lastN := min(len(filtered), p.lastN)
				recent = filtered[len(filtered)-lastN:]

				// Use the true count from all timestamps
				occurrences = trueOccurrences
				firstTS = filteredFirstTS
				lastTS = filteredLastTS
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

// sortOccurrencesByTime sorts occurrences by timestamp in ascending order (oldest first).
func sortOccurrencesByTime(occurrences []Occurrence) {
	sort.Slice(occurrences, func(i, j int) bool {
		// Parse timestamps for comparison
		ti, erri := time.Parse(time.RFC3339Nano, occurrences[i].Time)
		tj, errj := time.Parse(time.RFC3339Nano, occurrences[j].Time)
		// If parsing fails, maintain original order
		if erri != nil || errj != nil {
			return i < j
		}
		return ti.Before(tj)
	})
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
