// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package util

import (
	"time"
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
	panic("not called")
}

// ShouldThrottle returns a boolean indicating if the throttle consider the execution
// should be throttled.
// Not thread-safe.
func (t *SimpleThrottler) ShouldThrottle() bool {
	panic("not called")
}

// shouldThrottle implementation, returns as the first boolean if the throttle consider the execution
// has to be throttled, as a second boolean if the throttling has just started.
func (t *SimpleThrottler) shouldThrottle() (bool, bool) {
	panic("not called")
}
