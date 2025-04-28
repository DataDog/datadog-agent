// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"time"

	"golang.org/x/time/rate"
)

// Limit is a utility that can be used to avoid logging noisily
type Limit struct {
	s rate.Sometimes
}

// NewLogLimit creates a Limit where shouldLog will return
// true the first N times it is called, and will return true once every
// interval thereafter.
func NewLogLimit(n int, interval time.Duration) *Limit {
	return &Limit{
		s: rate.Sometimes{
			First:    n,
			Interval: interval,
		},
	}
}

// ShouldLog returns true if the caller should log
func (l *Limit) ShouldLog() bool {
	shouldLog := false
	l.s.Do(func() {
		shouldLog = true
	})
	return shouldLog
}
