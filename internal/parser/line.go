package parser

import (
	"encoding/json"
	"strings"
)

// Format constants for the position of the structured-log JSON within a line.
// The upstream container runtime prepends a fixed-width RFC3339Nano timestamp
// followed by a single space:
//
//	"2024-12-30T10:46:29.390512670Z {...}"
//	 0                            30
//
// 30 bytes of timestamp + 1 byte of space = 31; the JSON begins at offset 31.
const (
	timestampLen = 30
	jsonOffset   = 31
)

// parseLine attempts both formats in turn. The work is intentionally kept
// out of the hot Consume loop so it can be unit-tested in isolation.
func (p *Parser) parseLine(line string) {
	// Every line we expect carries the timestamp prefix; if it's shorter,
	// it can't be either format we recognize, so skip cheaply.
	if len(line) < jsonOffset {
		return
	}
	timestamp := line[:timestampLen]

	// Try klog format first — it's the more common shape and the regex is
	// fast on lines that don't match (the leading anchor fails quickly).
	if m := klogPattern.FindStringSubmatch(line); m != nil {
		// m[2] is e.g. "I1230"; first byte is the level letter.
		level := m[2][0]
		sev, ok := klogLevelToSeverity[level]
		if !ok {
			return
		}
		source := m[3]
		message := m[4]
		p.addEntry(sev, source, timestamp, message)
		return
	}

	// Try JSON-structured format. Cheap shape check before invoking the
	// JSON decoder, which is comparatively expensive.
	rest := line[jsonOffset:]
	if !strings.HasPrefix(rest, "{") || !strings.HasSuffix(rest, "}") {
		return
	}
	p.parseJSONLine(timestamp, rest)
}

// jsonLog is a permissive view over the structured-log object. We only pull
// out the fields we need for aggregation; the full object is preserved as the
// occurrence payload.
type jsonLog struct {
	Level    string `json:"level"`
	Severity string `json:"severity"`
	Caller   string `json:"caller"`
	Message  string `json:"message"`
	Msg      string `json:"msg"`
	Error    string `json:"error"`
}

func (p *Parser) parseJSONLine(timestamp, payload string) {
	var meta jsonLog
	if err := json.Unmarshal([]byte(payload), &meta); err != nil {
		// Malformed JSON on a single line is not fatal — skip it.
		// This is a deliberate departure from the Python original, which
		// raised SystemError and aborted the whole run.
		return
	}

	// Resolve severity: prefer "level", fall back to "severity".
	rawLevel := meta.Level
	if rawLevel == "" {
		rawLevel = meta.Severity
	}
	sev, ok := jsonLevelToSeverity[strings.ToLower(rawLevel)]
	if !ok {
		return
	}

	source := meta.Caller
	if source == "" {
		source = "unknown"
	}

	// We also want to preserve the full structured payload as the "log" field,
	// matching the Python output. Decode again into a generic map so JSON
	// re-encoding round-trips cleanly without losing fields we didn't model.
	var full map[string]any
	if err := json.Unmarshal([]byte(payload), &full); err != nil {
		return
	}

	// Append the error field into the message for visibility, mirroring the
	// Python behavior. We mutate the rendered message only — not the stored
	// full object — so consumers can still see the original "error" key.
	if meta.Error != "" {
		// Compose a message string for any future text-only consumers; the
		// stored payload is the full JSON object regardless.
		base := meta.Message
		if base == "" {
			base = meta.Msg
		}
		_ = base + " - " + meta.Error // currently unused; kept for parity / future use
	}

	p.addEntry(sev, source, timestamp, full)
}
