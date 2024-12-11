// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package util

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
	// ThrottlingMessage is the warning message logged when the throttling is triggered.
	// An empty message won't log anything.
	ThrottlingMessage string

	execCount      uint32
	lastThrottling time.Time
}

// changed to a mocked clock in unit tests
//
//nolint:revive // TODO(AML) Fix revive linter
var timeNow func() time.Time = time.Now

// NewSimpleThrottler creates and returns a SimpleThrottler.
func NewSimpleThrottler(execLimit uint32, pauseDuration time.Duration, throttlingMessage string) SimpleThrottler {
	return SimpleThrottler{
		ExecLimit:         execLimit,
		PauseDuration:     pauseDuration,
		ThrottlingMessage: throttlingMessage,
	}
}

// ShouldThrottle returns a boolean indicating if the throttle consider the execution
// should be throttled.
// Not thread-safe.
func (t *SimpleThrottler) ShouldThrottle() bool {
	isThrottled, hasJustStartedToThrottle := t.shouldThrottle()
	// the throttled has just started to throttle, however, this last execution has to be done
	// anyway so the caller shouldn't received "throttled" just yet.
	// however, next calls (if pause duration isn't over) will indeed return "true" for throttled.
	return isThrottled && !hasJustStartedToThrottle
}

// shouldThrottle implementation, returns as the first boolean if the throttle consider the execution
// has to be throttled, as a second boolean if the throttling has just started.
func (t *SimpleThrottler) shouldThrottle() (bool, bool) {
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
			if t.ThrottlingMessage != "" {
				log.Warn(t.ThrottlingMessage)
			}
			return true, true
		}

		// no throttling
		return false, false
	}

	// throttling because lastThrottling isn't zero and has not been reset
	return true, false
}
