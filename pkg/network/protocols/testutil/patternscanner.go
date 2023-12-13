// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"regexp"
	"strings"
	"sync"
	"testing"
)

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
}

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
	ps.buffers = append(ps.buffers, string(p))
	n = len(p)
	err = nil

	if !ps.stopped && ps.pattern.Match(p) {
		ps.stopOnce.Do(func() {
			ps.DoneChan <- struct{}{}
			ps.stopped = true
		})
	}

	return
}

func (ps *PatternScanner) PrintLogs(t testing.TB) {
	t.Log(strings.Join(ps.buffers, ""))
}
