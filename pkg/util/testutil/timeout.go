// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// AssertTrueBeforeTimeout regularly checks whether a condition is met. It
// does so until a timeout is reached, in which case it makes the test fail.
// Condition is evaluated in a goroutine to avoid tests hanging if a system
// is deadlocked.
func AssertTrueBeforeTimeout(t *testing.T, frequency, timeout time.Duration, condition func() bool) {
	t.Helper()
	assertTrueBeforeTimeout(t, frequency, timeout, condition)
}

// RequireTrueBeforeTimeout is the same as AssertTrueBeforeTimeout, but it calls
// t.failNow() if the condition function times out.
func RequireTrueBeforeTimeout(t *testing.T, frequency, timeout time.Duration, condition func() bool) {
	t.Helper()
	if !assertTrueBeforeTimeout(t, frequency, timeout, condition) {
		t.FailNow()
	}
}

func assertTrueBeforeTimeout(t *testing.T, frequency, timeout time.Duration, condition func() bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	r := make(chan bool, 1)

	go func() {
		// Try once immediately
		r <- condition()

		// Retry until timeout
		checkTicker := time.NewTicker(frequency)
		defer checkTicker.Stop()
		for {
			select {
			case <-checkTicker.C:
				ok := condition()
				r <- ok
				if ok {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	var ranOnce bool
	for {
		select {
		case ok := <-r:
			if ok {
				return ok
			}
			ranOnce = true
		case <-ctx.Done():
			if ranOnce {
				assert.Fail(t, "Timeout waiting for condition to happen, function returned false")
			} else {
				assert.Fail(t, "Timeout waiting for condition to happen, function never returned")
			}
			return false
		}
	}
}
