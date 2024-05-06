// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import "time"

// TestBackoffTimer is a backoff timer that ticks on-demand.
type TestBackoffTimer struct {
	tickChannel chan time.Time
}

// NewTestBackoffTimer creates a new instance of a TestBackoffTimer.
func NewTestBackoffTimer() *TestBackoffTimer {
	return &TestBackoffTimer{
		// tick channel without buffer allows us to sync with the sender during the tests by sending ticks
		tickChannel: make(chan time.Time),
	}
}

// ScheduleRetry on a TestBackoffTimer is a no-op.
func (t *TestBackoffTimer) ScheduleRetry(_ error) (int, time.Duration) {
	// Do nothing, we'll trigger whenever we want
	return 0, 0
}

// CurrentDelay in a TestBackoffTimer always returns 0.
func (t *TestBackoffTimer) CurrentDelay() time.Duration {
	// This timer doesn't have delays, it's triggered on-demand
	return 0
}

// NumRetries in a TestBackoffTimer always returns 0.
func (t *TestBackoffTimer) NumRetries() int {
	// This timer doesn't keep track of num retries
	return 0
}

// ReceiveTick returns the channel where ticks are sent.
func (t *TestBackoffTimer) ReceiveTick() <-chan time.Time {
	return t.tickChannel
}

// TriggerTick immediately sends a tick with the current timestamp through the ticking channel.
func (t *TestBackoffTimer) TriggerTick() {
	t.tickChannel <- time.Now()
}

// Reset in a TestBackoffTimer is a no-op.
func (t *TestBackoffTimer) Reset() {
	// Nothing to reset
}

// Stop in a TestBackoffTimer is a no-op.
func (t *TestBackoffTimer) Stop() {
	// Nothing to stop
}

// Close closes the ticking channel of this backoff timer.
func (t *TestBackoffTimer) Close() {
	close(t.tickChannel)
}
