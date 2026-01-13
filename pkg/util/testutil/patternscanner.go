// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package testutil

import (
	"errors"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// NoPattern is a sugar syntax for empty pattern
var NoPattern *regexp.Regexp

// PatternScanner is a helper to scan logs for a given pattern.
type PatternScanner struct {
	// The log pattern to match on validating successful start
	startPattern *regexp.Regexp
	// The log pattern to match on validating successful finish. This is optional
	finishPattern *regexp.Regexp
	// Once we've found the correct log, we should notify the caller.
	DoneChan chan struct{}
	// A sync.Once instance to ensure we notify the caller only once, and stop the operation.
	stopOnce sync.Once

	// flag to indicate that start was found, and we should look for finishPattern now
	startPatternFound bool
	// A helper to spare redundant calls to the analyzer once we've found both start and finish
	stopped bool

	// keep the stdout/err in case of failure
	buffers []string
	//Buffer for accumulating partial lines
	lineBuf string
}

// NewScanner returns a new instance of PatternScanner.
// at least one of the startPattern/finishPattern should be provided.
func NewScanner(startPattern, finishPattern *regexp.Regexp) (*PatternScanner, error) {
	if startPattern == nil && finishPattern == nil {
		return nil, errors.New("at least one pattern should be provided")
	}
	return &PatternScanner{
		startPattern:  startPattern,
		finishPattern: finishPattern,
		DoneChan:      make(chan struct{}, 1),
		stopOnce:      sync.Once{},
		// skip looking for start pattern if not provided
		startPatternFound: startPattern == nil,
		stopped:           false,
	}, nil
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

		// Check if we've met the scanning criteria
		ps.matchPatterns(line)
	}

	return len(p), nil
}

// matchPatterns checks if the current line matches the scanning requirements
func (ps *PatternScanner) matchPatterns(line string) {
	switch {
	// startPatternFound pattern not found yet, look for it
	case !ps.startPatternFound:
		if ps.startPattern.MatchString(line) {
			// found start pattern, flip the flag to start looking for finish pattern for following iterations
			ps.startPatternFound = true

			// no finishPattern provided, we can stop here
			if ps.finishPattern == nil {
				ps.notifyAndStop()
			}
		}
	// startPatternFound pattern found, look for finish pattern if provided
	case ps.finishPattern != nil && ps.finishPattern.MatchString(line):
		ps.notifyAndStop()
	}
}

func (ps *PatternScanner) notifyAndStop() {
	ps.stopOnce.Do(func() {
		ps.buffers = append(ps.buffers, ps.lineBuf) // flush the last line
		ps.stopped = true
		close(ps.DoneChan)
	})
}

func (ps *PatternScanner) Lines() []string {
	return ps.buffers
}

// PrintLogs writes the captured logs into the test logger.
func (ps *PatternScanner) PrintLogs(t testing.TB) {
	t.Log(strings.Join(ps.Lines(), "\n"))
}
