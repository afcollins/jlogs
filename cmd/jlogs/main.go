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

	"github.com/gmeghnag/jlogs/internal/parser"
)

func main() {
	lastN := flag.Int("last-n", 5, "number of most recent occurrences to retain per source (1-10)")
	firstN := flag.Int("first-n", 1, "number of first occurrences to retain per source (1-10)")
	pretty := flag.Bool("pretty", true, "pretty-print the JSON output")
	flag.Parse()

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

	p, err := parser.New(*lastN, *firstN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
		os.Exit(1)
	}

	if err := p.Consume(reader); err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
		os.Exit(1)
	}

	summary := p.Summary()

	enc := json.NewEncoder(os.Stdout)
	if *pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(summary); err != nil {
		fmt.Fprintf(os.Stderr, "jlogs: %v\n", err)
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
