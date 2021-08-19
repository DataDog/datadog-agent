// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"math/rand"
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

	callbackFuncInvocationCount int64
	pollingFuncInvocationCount  int64
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

func dummyCallbackFunc(sw SlidingWindow) {
	atomic.AddInt64(&callbackFuncInvocationCount, 1)
}

// This function on average should return about 0.3 value
func dummyFractionalPollingFunc() float64 {
	atomic.AddInt64(&pollingFuncInvocationCount, 1)
	randBusy := 0.3 + ((rand.Float64() - 0.5) * 0.001)

	return randBusy
}

func TestSlidingWindow(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 50*time.Millisecond)
	require.Nil(t, err)
	require.NotNil(t, sw)

	err = sw.Start(dummyTogglingPollingFunc, dummyCallbackFunc)
	require.Nil(t, err)
	defer func() {
		sw.Stop()
	}()

	time.Sleep(1200 * time.Millisecond)
	utilPct := sw.Average()

	assert.InDelta(t, utilPct, 0.5, DefaultDelta)
}

func TestSlidingWindowAccuracy(t *testing.T) {
	// Floats don't really have good atomic primitives
	var cbLock sync.Mutex
	lastAverage := 0.0

	callbackFunc := func(sw SlidingWindow) {
		cbLock.Lock()
		defer cbLock.Unlock()

		lastAverage = sw.Average()
	}

	sw, err := NewSlidingWindow(1*time.Second, 10*time.Millisecond)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, callbackFunc)
	require.Nil(t, err)
	defer sw.Stop()

	time.Sleep(1200 * time.Millisecond)
	utilPct := sw.Average()

	assert.Equal(t, 1*time.Second, sw.WindowSize())
	assert.InDelta(t, utilPct, 0.3, DefaultDelta)

	cbLock.Lock()
	defer cbLock.Unlock()
	assert.InDelta(t, utilPct, 0.3, lastAverage)
}

func TestSlidingWindowAverage(t *testing.T) {
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
	atomic.StoreInt64(&callbackFuncInvocationCount, 0)
	atomic.StoreInt64(&pollingFuncInvocationCount, 0)

	sw, err := NewSlidingWindow(100*time.Millisecond, 10*time.Millisecond)
	require.Nil(t, err)

	pollingFunc := func() float64 {
		atomic.StoreInt64(&pollingFuncInvocationCount, 1)

		return 0.0
	}

	callbackFunc := func(cbSlidingWindow SlidingWindow) {
		require.NotNil(t, cbSlidingWindow)
		require.Equal(t, sw, cbSlidingWindow)

		require.True(
			t,
			atomic.LoadInt64(&pollingFuncInvocationCount) > atomic.LoadInt64(&callbackFuncInvocationCount),
		)

		atomic.StoreInt64(&pollingFuncInvocationCount, 1)
	}

	err = sw.Start(pollingFunc, callbackFunc)
	require.Nil(t, err)
	defer sw.Stop()

	time.Sleep(200 * time.Millisecond)
}

func TestSlidingWindowFuncInvocationCounts(t *testing.T) {
	atomic.StoreInt64(&callbackFuncInvocationCount, 0)
	atomic.StoreInt64(&pollingFuncInvocationCount, 0)

	sw, err := NewSlidingWindow(900*time.Millisecond, 50*time.Millisecond)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, dummyCallbackFunc)
	require.Nil(t, err)
	defer sw.Stop()

	assert.Equal(t, 900*time.Millisecond, sw.WindowSize())

	time.Sleep(900 * time.Millisecond)
	assert.InDelta(t, atomic.LoadInt64(&pollingFuncInvocationCount), 18, 1)
	assert.InDelta(t, atomic.LoadInt64(&callbackFuncInvocationCount), 18, 1)

	time.Sleep(100 * time.Millisecond)
	assert.InDelta(t, atomic.LoadInt64(&pollingFuncInvocationCount), 20, 1)
	assert.InDelta(t, atomic.LoadInt64(&callbackFuncInvocationCount), 20, 1)
}

func TestNewSlidingWindowStop(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 50*time.Millisecond)
	require.Nil(t, err)

	// Implicit check - should not panic
	sw.Stop()

	err = sw.Start(dummyTogglingPollingFunc, dummyCallbackFunc)
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
	err = sw.Start(nil, dummyCallbackFunc)
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
