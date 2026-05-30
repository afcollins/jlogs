// Package main is the CLI entry point for jlogs.
//
// jlogs reads log lines from stdin (or one or more files passed as arguments),
// parses klog-style and JSON-structured logs, groups entries by severity and
// source location, and emits a JSON summary to stdout.
//
// Original Python implementation: Gabriel Meghnagi <gmeghnag@redhat.com>
// Go rewrite preserves the same input format and output schema while adding
// streaming I/O, configurable last-N tracking, and resilient error handling.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gmeghnag/jlogs/cmd/jlogs/version"
	"github.com/gmeghnag/jlogs/internal/parser"
	"golang.org/x/term"
)

func main() {
	lastN := flag.Int("last-n", 5, "number of most recent occurrences to retain per source (1-10)")
	firstN := flag.Int("first-n", 1, "number of first occurrences to retain per source (1-10)")
	pretty := flag.Bool("pretty", true, "pretty-print the JSON output")
	since := flag.String("since", "", "filter logs from last N time units (e.g., \"1 hour\", \"2 days\", \"30 minutes\")")
	timeline := flag.Bool("timeline", false, "emit a timeline view grouped by time interval instead of a summary")
	interval := flag.String("interval", "minute", "timeline interval granularity: hour, minute, or second")
	format := flag.String("format", "json", "output format: json, csv, sparkline")
	wrap := flag.Bool("wrap", false, "wrap sparkline output to fit terminal width")
	width := flag.Int("w", 0, "terminal width for sparkline wrapping (0 = auto-detect)")
	v := flag.Bool("v", false, "print version information")
	flag.Parse()

	if *v {
		fmt.Printf("jlogs version %s (commit %s)\n", version.Tag, version.Hash)
		os.Exit(0)
	}

	var sinceDuration time.Duration
	if *since != "" {
		var err error
		sinceDuration, err = parseSince(*since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "jlogs: invalid --since value: %v\n", err)
			os.Exit(1)
		}
	}

	// Build the input reader: either stdin or the concatenation of file args.
	// MultiReader lets us stream without buffering everything in memory.
	reader, closers, err := openInputs(flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()

	p, err := parser.New(*lastN, *firstN, sinceDuration)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
		os.Exit(1)
	}

	if *timeline {
		p.SetTimelineInterval(*interval)
	}

	if err := p.Consume(reader); err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		if *pretty {
			enc.SetIndent("", "  ")
		}
		var output any
		if *timeline {
			output = p.Timeline()
		} else {
			output = p.Summary()
		}
		if err := enc.Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
			os.Exit(1)
		}

	case "csv", "sparkline":
		if !*timeline {
			fmt.Fprintf(os.Stderr, "jlogs: -format %s requires -timeline\n", *format)
			os.Exit(1)
		}
		var err error
		if *format == "csv" {
			err = p.TimeseriesCSV(os.Stdout)
		} else {
			w := *width
			if *wrap && w == 0 {
				w = detectTerminalWidth()
			}
			err = p.TimeseriesSparkline(os.Stdout, *wrap, w)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "jlogs: unknown format %q (valid: json, csv, sparkline)\n", *format)
		os.Exit(1)
	}
}

// openInputs returns a Reader over all of paths in order, or os.Stdin if paths
// is empty. The returned closers must be closed by the caller.
func openInputs(paths []string) (io.Reader, []io.Closer, error) {
	if len(paths) == 0 {
		return os.Stdin, nil, nil
	}
	readers := make([]io.Reader, 0, len(paths))
	closers := make([]io.Closer, 0, len(paths))
	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			// Close anything we already opened before bailing.
			for _, c := range closers {
				_ = c.Close()
			}
			return nil, nil, fmt.Errorf("opening %q: %w", path, err)
		}
		readers = append(readers, f)
		closers = append(closers, f)
	}
	return io.MultiReader(readers...), closers, nil
}

// parseSince parses a duration string like "1 hour", "2 days", "30 minutes".
// Supports: minute(s), hour(s), day(s).
func parseSince(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`^(\d+)\s+(minute|minutes|hour|hours|day|days)$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid format, expected \"N unit\" (e.g., \"1 hour\", \"2 days\")")
	}

	num, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid number: %w", err)
	}

	unit := matches[2]
	switch unit {
	case "minute", "minutes":
		return time.Duration(num) * time.Minute, nil
	case "hour", "hours":
		return time.Duration(num) * time.Hour, nil
	case "day", "days":
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", unit)
	}
}

func detectTerminalWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}
