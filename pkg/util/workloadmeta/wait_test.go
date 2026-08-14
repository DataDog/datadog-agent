// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// fakeStore reports itself as initialized after readyAfter calls to IsInitialized, or never if
// neverReady is set. waitForInitialization polls from a single goroutine, and the tests only read
// calls after that goroutine has handed its result over a channel, so a plain counter is enough.
type fakeStore struct {
	workloadmeta.Component

	readyAfter int
	neverReady bool
	calls      int
}

func (f *fakeStore) IsInitialized() bool {
	f.calls++
	return !f.neverReady && f.calls > f.readyAfter
}

// runWithMockClock calls waitForInitialization on its own goroutine and steps a mock clock forward
// one poll interval at a time until it returns. The timer and the ticker are registered on the
// mock clock by that goroutine, so time cannot be advanced in a single jump beforehand.
// It reports what waitForInitialization returned, and how much mock time it took to get there.
func runWithMockClock(t *testing.T, wmeta workloadmeta.Component, timeout time.Duration) (bool, time.Duration) {
	t.Helper()

	clk := clock.NewMock()
	start := clk.Now()
	done := make(chan bool, 1)
	go func() {
		done <- waitForInitialization(wmeta, timeout, logmock.New(t), clk)
	}()

	// Only guards against a hang: no assertion depends on how long this takes in wall-clock time.
	giveUp := time.After(30 * time.Second)
	for {
		select {
		case result := <-done:
			return result, clk.Now().Sub(start)
		case <-giveUp:
			require.FailNow(t, "waitForInitialization did not return")
			return false, 0
		default:
			clk.Add(pollInterval)
		}
	}
}

func TestWaitForInitializationAlreadyInitialized(t *testing.T) {
	wmeta := &fakeStore{}

	// The fast path is synchronous, so the clock is never used.
	assert.True(t, waitForInitialization(wmeta, time.Minute, logmock.New(t), clock.NewMock()))
	// A single call means the ticker was never involved.
	assert.Equal(t, 1, wmeta.calls)
}

func TestWaitForInitializationWhilePolling(t *testing.T) {
	wmeta := &fakeStore{readyAfter: 3}

	ready, _ := runWithMockClock(t, wmeta, time.Minute)

	assert.True(t, ready)
	// The store is polled exactly once per tick, and the wait returns on the first poll that
	// reports it ready.
	assert.Equal(t, 4, wmeta.calls)
}

func TestWaitForInitializationTimeout(t *testing.T) {
	wmeta := &fakeStore{neverReady: true}
	timeout := 5 * pollInterval

	ready, elapsed := runWithMockClock(t, wmeta, timeout)

	assert.False(t, ready)
	// The wait is bounded: it gives up instead of blocking the command forever, and it does so
	// after the timeout rather than before it.
	assert.GreaterOrEqual(t, elapsed, timeout)
}

func TestWaitForInitializationDisabled(t *testing.T) {
	wmeta := &fakeStore{neverReady: true}

	assert.False(t, waitForInitialization(wmeta, 0, logmock.New(t), clock.NewMock()))
	// A zero timeout still reports the current state, it just never polls again.
	assert.Equal(t, 1, wmeta.calls)
}
