// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package events holds events related files
package events

import (
	"go.uber.org/atomic"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// StdLimiter describes an object that applies limits on
// the rate of triggering of a rule to ensure we don't overflow
// with too permissive rules
type StdLimiter struct {
	rateLimiter *rate.Limiter

	// stats
	dropped *atomic.Uint64
	allowed *atomic.Uint64
}

// NewStdLimiter returns a new rule limiter
func NewStdLimiter(limit rate.Limit, burst int) *StdLimiter {
	return &StdLimiter{
		rateLimiter: rate.NewLimiter(limit, burst),
		dropped:     atomic.NewUint64(0),
		allowed:     atomic.NewUint64(0),
	}
}

// Allow returns whether the event is allowed
func (l *StdLimiter) Allow(_ Event) bool {
	if l.rateLimiter.Allow() {
		l.allowed.Inc()
		return true
	}
	l.dropped.Inc()

	return false
}

// SwapStats returns the dropped and allowed stats, and zeros the stats
func (l *StdLimiter) SwapStats() []utils.LimiterStat {
	return []utils.LimiterStat{
		{
			Dropped: l.dropped.Swap(0),
			Allowed: l.allowed.Swap(0),
		},
	}
}
