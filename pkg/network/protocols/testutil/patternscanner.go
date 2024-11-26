// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testutil provides utilities for testing the network package.
package testutil

import (
	"regexp"
	"strings"
	"sync"
	"testing"
)

// PatternScanner is a helper to scan logs for a given pattern.
type PatternScanner struct {
	// The log pattern to match on
	pattern *regexp.Regexp
	// Once we've found the correct log, we should notify the caller.
	DoneChan chan struct{}
	// A sync.Once instance to ensure we notify the caller only once, and stop the operation.
	stopOnce sync.Once
	// A helper to spare redundant calls to the analyzer once we've found the relevant log.
	stopped bool

	// keep the stdout/err in case of failure
	buffers []string

	//Buffer for accumulating partial lines
	lineBuf string
}

// NewScanner returns a new instance of PatternScanner.
func NewScanner(pattern *regexp.Regexp, doneChan chan struct{}) *PatternScanner {
	return &PatternScanner{
		pattern:  pattern,
		DoneChan: doneChan,
		stopOnce: sync.Once{},
		stopped:  false,
	}
}

// Write implemented io.Writer to be used as a callback for log/string writing.
// Once we find a match in for the given pattern, we notify the caller.
func (ps *PatternScanner) Write(p []byte) (n int, err error) {
	// Ignore writes after the pattern has been matched.
	if ps.stopped {
		return len(p), nil
	}

	// Append new data to the line buffer.
	ps.lineBuf += string(p)

	// Split the buffer into lines.
	lines := strings.Split(ps.lineBuf, "\n")
	ps.lineBuf = lines[len(lines)-1] // Save the last (possibly incomplete) line.

	// Process all complete lines.
	for _, line := range lines[:len(lines)-1] {
		ps.buffers = append(ps.buffers, line) // Save the log line.
		if !ps.stopped && ps.pattern.MatchString(line) {
			ps.stopOnce.Do(func() {
				ps.stopped = true
				close(ps.DoneChan) // Notify the caller about the match.
			})
		}
	}

	return len(p), nil
}

// PrintLogs writes the captured logs into the test logger.
func (ps *PatternScanner) PrintLogs(t testing.TB) {
	t.Log(strings.Join(ps.buffers, ""))
}
