// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const DefaultDelta = 0.001

var (
	dummyPollingFuncToggle = true

	pollingFuncInvocationCount     int64
	statsUpdateFuncInvocationCount int64
)

// This function on average should return 0.5 value
func dummyTogglingPollingFunc() float64 {
	atomic.AddInt64(&pollingFuncInvocationCount, 1)

	dummyPollingFuncToggle = !dummyPollingFuncToggle

	if dummyPollingFuncToggle {
		return 1.0
	}

	return 0.0
}

func dummyStatsUpdateFunc(sw SlidingWindow) {
	atomic.AddInt64(&statsUpdateFuncInvocationCount, 1)
}

// This function on average should return about 0.3 value
func dummyFractionalPollingFunc() float64 {
	atomic.AddInt64(&pollingFuncInvocationCount, 1)
	randBusy := 0.3 + ((rand.Float64() - 0.5) * 0.001)

	return randBusy
}

func TestSlidingWindow(t *testing.T) {
	sw, err := NewSlidingWindow(2*time.Second, 100*time.Millisecond)
	require.Nil(t, err)
	require.NotNil(t, sw)

	err = sw.Start(dummyTogglingPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer func() {
		sw.Stop()
	}()

	time.Sleep(2250 * time.Millisecond)
	utilPct := sw.Average()

	assert.InDelta(t, utilPct, 0.5, 0.05)
}

func TestSlidingWindowAccuracy(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("Skipping flaky test on Darwin")
	}

	// Floats don't really have good atomic primitives
	var cbLock sync.RWMutex
	lastAverage := 0.0

	statsUpdateFunc := func(sw SlidingWindow) {
		cbLock.Lock()
		defer cbLock.Unlock()
		lastAverage = sw.Average()
	}

	sw, err := NewSlidingWindow(1*time.Second, 10*time.Millisecond)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, statsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	time.Sleep(1200 * time.Millisecond)
	utilPct := sw.Average()

	assert.Equal(t, 1*time.Second, sw.WindowSize())
	assert.InDelta(t, utilPct, 0.3, DefaultDelta)

	cbLock.RLock()
	defer cbLock.RUnlock()
	assert.InDelta(t, utilPct, 0.3, lastAverage)
}

func TestSlidingWindowAverage(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping flaky test on Windows")
	}

	sw, err := NewSlidingWindow(1*time.Second, 100*time.Millisecond)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, nil)
	require.Nil(t, err)
	defer sw.Stop()

	time.Sleep(50 * time.Millisecond)
	assert.InDelta(t, sw.Average(), 0.0, DefaultDelta)

	for idx := 0; idx < 12; idx++ {
		time.Sleep(100 * time.Millisecond)
		assert.InDelta(t, sw.Average(), 0.3, DefaultDelta)
	}
}

func TestSlidingWindowCallback(t *testing.T) {
	atomic.StoreInt64(&statsUpdateFuncInvocationCount, 0)
	atomic.StoreInt64(&pollingFuncInvocationCount, 0)

	sw, err := NewSlidingWindow(100*time.Millisecond, 10*time.Millisecond)
	require.Nil(t, err)

	pollingFunc := func() float64 {
		atomic.StoreInt64(&pollingFuncInvocationCount, 1)

		return 0.0
	}

	statsUpdateFunc := func(cbSlidingWindow SlidingWindow) {
		require.NotNil(t, cbSlidingWindow)
		require.Equal(t, sw, cbSlidingWindow)

		require.True(
			t,
			atomic.LoadInt64(&pollingFuncInvocationCount) > atomic.LoadInt64(&statsUpdateFuncInvocationCount),
		)

		atomic.StoreInt64(&pollingFuncInvocationCount, 1)
	}

	err = sw.Start(pollingFunc, statsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	time.Sleep(200 * time.Millisecond)
}

func TestSlidingWindowFuncInvocationCounts(t *testing.T) {
	atomic.StoreInt64(&statsUpdateFuncInvocationCount, 0)
	atomic.StoreInt64(&pollingFuncInvocationCount, 0)

	windowSize := 2000 * time.Millisecond
	pollingInterval := 100 * time.Millisecond

	sw, err := NewSlidingWindow(windowSize, pollingInterval)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	assert.Equal(t, windowSize, sw.WindowSize())

	time.Sleep(windowSize)
	assert.InDelta(t, atomic.LoadInt64(&pollingFuncInvocationCount), 19, 3)
	assert.InDelta(t, atomic.LoadInt64(&statsUpdateFuncInvocationCount), 19, 3)

	time.Sleep(200 * time.Millisecond)
	assert.InDelta(t, atomic.LoadInt64(&pollingFuncInvocationCount), 21, 3)
	assert.InDelta(t, atomic.LoadInt64(&statsUpdateFuncInvocationCount), 21, 3)
}

func TestNewSlidingWindowStop(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 50*time.Millisecond)
	require.Nil(t, err)

	// Implicit check - should not panic
	sw.Stop()

	err = sw.Start(dummyTogglingPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)

	// None of these invocations should throw an error
	sw.Stop()
	sw.Stop()
	sw.Stop()
}

func TestSlidingWindowParamValidation(t *testing.T) {
	_, err := NewSlidingWindow(0*time.Second, 50*time.Millisecond)
	require.Error(t, err)
	require.EqualError(t, err, "SlidingWindow windowSize cannot be 0")

	_, err = NewSlidingWindow(1*time.Second, 0*time.Second)
	require.Error(t, err)
	require.EqualError(t, err, "SlidingWindow pollingInterval cannot be 0")

	_, err = NewSlidingWindow(1*time.Second, 73*time.Millisecond)
	require.Error(t, err)
	require.EqualError(t, err, "SlidingWindow windowSize must be a multiple of polling interval")

	_, err = NewSlidingWindow(2000*time.Millisecond, 2001*time.Millisecond)
	require.Error(t, err)
	require.EqualError(t, err, "SlidingWindow windowSize must be smaller than the polling interval")

	sw, err := NewSlidingWindow(2*time.Second, 1*time.Second)
	require.Nil(t, err)
	err = sw.Start(nil, dummyStatsUpdateFunc)
	require.Error(t, err)
	require.EqualError(t, err, "SlidingWindow pollingFunc must not be nil")

	// Test consecutive initialization attempts

	sw, err = NewSlidingWindow(2*time.Second, 1*time.Second)
	require.Nil(t, err)
	err = sw.Start(dummyTogglingPollingFunc, nil)
	require.Nil(t, err)

	err = sw.Start(dummyTogglingPollingFunc, nil)
	require.NotNil(t, err)
	require.EqualError(t, err, "SlidingWindow already initialized")

	// Test ability to not provide a callback function

	sw, err = NewSlidingWindow(2*time.Second, 1*time.Second)
	require.Nil(t, err)
	defer sw.Stop()

	err = sw.Start(dummyTogglingPollingFunc, nil)
	require.Nil(t, err)
}
