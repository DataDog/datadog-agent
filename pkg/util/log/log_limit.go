// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limit is a utility that can be used to avoid logging noisily
type Limit struct {
	mu sync.Mutex

	// n is the times remaining that the Limit will return true for ShouldLog.
	// we repeatedly subtract 1 from it, if it is nonzero.
	n int

	limiter *rate.Limiter
}

// NewLogLimit creates a Limit where shouldLog will return
// true the first N times it is called, and will return true once every
// interval thereafter.
func NewLogLimit(n int, interval time.Duration) *Limit {
	return &Limit{
		n:       n,
		limiter: rate.NewLimiter(rate.Every(interval), 1),
	}
}

// ShouldLog returns true if the caller should log
func (l *Limit) ShouldLog() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.n > 0 {
		l.n--

		// consume one rate limit period,
		// as if the N first events were one big event
		if l.n == 0 {
			l.limiter.Allow()
		}

		return true
	}

	return l.limiter.Allow()
}

// resetCounter is only used in tests
func (l *Limit) resetCounter() {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.n == 0 {
		l.n++
	}
}
