// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "time"

// SimpleThrottler is a very basic throttler counting how many times something
// has been executed and indicating that this execution should be throttled if it
// has been executed more time ExecLimit. The pause duration is configurable
// with PauseDuration.
// After the pause, the throttler resets and restarts with the exact same behavior.
// There is no automatic decrease whatsoever in the execution count in time.
// SimpleThrottler is not thread-safe.
type SimpleThrottler struct {
	// ExecLimit represents the execution count limit after which the
	// throttler will indicates it's time to throttle
	ExecLimit uint32
	// PauseDuration represents how long the SimpleThrottler consider
	// we have to throttle execution.
	PauseDuration time.Duration

	execCount      uint32
	lastThrottling time.Time
}

// changed to a mocked clock in unit tests
var timeNow func() time.Time = time.Now

// ShouldThrottle returns a first boolean indicating if the throttle consider
// the execution should be throttled.
// The second returned boolean indicates if the throttler has just switched
// to this state (for instance to trigger a log "started throttling ...")
// Not thread-safe.
func (t *SimpleThrottler) ShouldThrottle() (bool, bool) {
	t.execCount++

	// reset the throttling, we paused it long enough
	if !t.lastThrottling.IsZero() && t.lastThrottling.Add(t.PauseDuration).Before(timeNow()) {
		t.execCount = 1
		t.lastThrottling = time.Time{}
	}

	if t.lastThrottling.IsZero() {
		if t.execCount >= t.ExecLimit {
			// starts throttling
			t.lastThrottling = timeNow()
			return true, true
		}

		// no throttling
		return false, false
	}

	// throttling because lastThrottling isn't zero and has not been reset
	return true, false
}
