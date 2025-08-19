// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package jmxfetch

import "time"

// restartLimiter checks if too many restarts happened within a time period
type restartLimiter struct {
	maxRestarts int
	interval    float64 // in seconds
	stopTimes   []time.Time
	idx         int
}

func newRestartLimiter(maxRestarts int, interval float64) restartLimiter {
	var stopTimes []time.Time
	if maxRestarts > 0 {
		stopTimes = make([]time.Time, maxRestarts)
	}
	return restartLimiter{
		maxRestarts: maxRestarts,
		interval:    interval,
		stopTimes:   stopTimes,
		idx:         0,
	}
}

func (r *restartLimiter) canRestart(now time.Time) bool {
	if r.maxRestarts < 1 {
		return false
	}

	r.stopTimes[r.idx] = now
	oldestIdx := (r.idx + 1) % r.maxRestarts

	// Please note that the zero value for `time.Time` is `0001-01-01 00:00:00+0000 UTC`
	// The sub operation with stopTimes here will only start yielding values potentially
	// <= ival _after_ the first maxRestarts-1 attempts.
	canRestart := r.stopTimes[r.idx].Sub(r.stopTimes[oldestIdx]).Seconds() > r.interval

	r.idx = oldestIdx

	return canRestart
}
