package parser

import "sort"

// TimelineEntry is one time interval in the timeline output.
type TimelineEntry struct {
	Interval string           `json:"interval"`
	Count    int              `json:"count"`
	Sources  []TimelineSource `json:"sources"`
}

// TimelineSource is a single source's activity within a time interval.
type TimelineSource struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
	Sample any    `json:"sample"`
}

// Timeline builds the timeline report from accumulated state.
// Returns intervals sorted chronologically; within each interval, sources sorted by
// count ascending (highest count at bottom, consistent with Summary ordering).
func (p *Parser) Timeline() []TimelineEntry {
	entries := make([]TimelineEntry, 0, len(p.timelineBuckets))

	for intervalKey, sourceBucket := range p.timelineBuckets {
		sources := make([]TimelineSource, 0, len(sourceBucket))
		total := 0
		for source, agg := range sourceBucket {
			total += agg.count
			sources = append(sources, TimelineSource{
				Source: source,
				Count:  agg.count,
				Sample: agg.sample,
			})
		}
		sort.Slice(sources, func(i, j int) bool {
			return sources[i].Count < sources[j].Count
		})
		entries = append(entries, TimelineEntry{Interval: intervalKey, Count: total, Sources: sources})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Interval < entries[j].Interval
	})
	return entries
}
