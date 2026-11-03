// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package orchestrator

import (
	"math/bits"
	"time"
)

// maxExponent is the largest exponent of the skipped runs backoff. Shifting by more overflows an int.
const maxExponent = bits.UintSize - 2

// retrier schedules the retries of a failing operation attempted on each check run. After each failure it skips a
// number of runs before retrying. That number starts at one and doubles on every failure in a row, up to a maximum.
type retrier struct {
	failures                  int
	lastErr                   error
	maxRunsSkippedBeforeRetry int
	runsToSkip                int
}

// newRetrier returns a retrier for an operation run every runInterval. It skips runs for at most maxDelay before
// retrying, and at least one run.
func newRetrier(runInterval, maxDelay time.Duration) *retrier {
	return &retrier{
		maxRunsSkippedBeforeRetry: runCountFromDelay(runInterval, maxDelay),
	}
}

// CanRun reports whether the retry is due on the current run.
func (r *retrier) CanRun() bool {
	return r.runsToSkip == 0
}

// Fail records a failure and schedules the next retry.
func (r *retrier) Fail(err error) {
	r.lastErr = err
	r.failures++
	r.runsToSkip = r.runsSkippedAfter(r.failures)
}

// LastError returns the error of the last failure.
func (r *retrier) LastError() error {
	return r.lastErr
}

// Reset restarts the backoff.
func (r *retrier) Reset() {
	r.failures = 0
	r.lastErr = nil
	r.runsToSkip = 0
}

// RunsToSkip returns the number of runs still skipped before the retry.
func (r *retrier) RunsToSkip() int {
	return r.runsToSkip
}

// SkipRun counts the current run as skipped.
func (r *retrier) SkipRun() {
	if r.runsToSkip > 0 {
		r.runsToSkip--
	}
}

// runsSkippedAfter returns the number of runs to skip after failures failures in a row.
func (r *retrier) runsSkippedAfter(failures int) int {
	exponent := min(max(failures-1, 0), maxExponent)
	return min(1<<exponent, r.maxRunsSkippedBeforeRetry)
}

// runCountFromDelay returns the number of runs every runInterval that fit in delay, at least one.
func runCountFromDelay(runInterval, delay time.Duration) int {
	if runInterval <= 0 || delay < runInterval {
		return 1
	}
	return int(delay / runInterval)
}
