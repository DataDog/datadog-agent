// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"time"

	"go.uber.org/atomic"
)

// LogLimit is a utility that can be used to avoid logging noisily
type LogLimit struct {
	// n is the times remaining that the LogLimit will return true for ShouldLog.
	// we repeatedly subtract 1 from it, if it is nonzero.
	n *atomic.Int32

	// exit and ticker must be different channels
	// becaues Stopping a ticker will not close the ticker channel,
	// and we will otherwise leak memory
	ticker *time.Ticker
	exit   chan struct{}
}

// NewLogLimit creates a LogLimit where shouldLog will return
// true the first N times it is called, and will return true once every
// interval thereafter.
func NewLogLimit(n int, interval time.Duration) *LogLimit {
	l := &LogLimit{
		n:      atomic.NewInt32(int32(n)),
		ticker: time.NewTicker(interval),
		exit:   make(chan struct{}),
	}

	go l.resetLoop()
	return l
}

// ShouldLog returns true if the caller should log
func (l *LogLimit) ShouldLog() bool {
	n := l.n.Load()
	if n > 0 {
		// try to decrement n, doing nothing on concurrent attempts
		l.n.CompareAndSwap(n, n-1)
		return true
	}

	return false
}

// Close will stop the underlying ticker
func (l *LogLimit) Close() {
	l.ticker.Stop()
	close(l.exit)
}

func (l *LogLimit) resetLoop() {
	for {
		select {
		case <-l.ticker.C:
			l.resetCounter()
		case <-l.exit:
			return
		}
	}
}

func (l *LogLimit) resetCounter() {
	// c.n == 0, it means we have gotten through the first few logs, and after ticker.T we should
	// do another log
	l.n.CompareAndSwap(0, 1)
}
