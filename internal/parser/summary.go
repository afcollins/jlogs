package parser

import (
	"bytes"
	"encoding/json"
	"sort"
)

// numToWord matches the Python original's mapping; preserved for output-key
// compatibility ("last_five_occurrences", etc.).
var numToWord = map[int]string{
	1:  "one",
	2:  "two",
	3:  "three",
	4:  "four",
	5:  "five",
	6:  "six",
	7:  "seven",
	8:  "eight",
	9:  "nine",
	10: "ten",
}

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
	for _, sev := range trackedSeverities {
		bucket := p.buckets[sev]
		entries := make([]SourceSummary, 0, len(bucket))
		for source, agg := range bucket {
			entries = append(entries, SourceSummary{
				Source:      source,
				Occurrences: agg.occurrences,
				Recent:      agg.recent.snapshot(),
				recentKey:   p.recentKey,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Occurrences < entries[j].Occurrences
		})
		out[string(sev)] = entries
	}
	return out
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
