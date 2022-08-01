// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

const DefaultDelta = 0.001

var (
	dummyPollingFuncToggle = true

	pollingFuncInvocationCount     = atomic.NewInt64(0)
	statsUpdateFuncInvocationCount = atomic.NewInt64(0)
	statsUpdateFuncValue           = atomic.NewFloat64(0)
)

// This function on average should return 0.5 value
func dummyTogglingPollingFunc() float64 {
	pollingFuncInvocationCount.Add(1)

	dummyPollingFuncToggle = !dummyPollingFuncToggle

	if dummyPollingFuncToggle {
		return 1.0
	}

	return 0.0
}

func dummyStatsUpdateFunc(value float64) {
	statsUpdateFuncInvocationCount.Add(1)
	statsUpdateFuncValue.Store(value)
}

// This function on average should return about 0.3 value
func dummyFractionalPollingFunc() float64 {
	pollingFuncInvocationCount.Add(1)
	randBusy := 0.3 + ((rand.Float64() - 0.5) * 0.001)

	return randBusy
}

func TestSlidingWindow(t *testing.T) {
	statsUpdateFuncInvocationCount.Store(0)
	statsUpdateFuncValue.Store(0.0)

	clk := clock.NewMock()
	sw, err := NewSlidingWindowWithClock(2*time.Second, 100*time.Millisecond, clk)
	require.Nil(t, err)
	require.NotNil(t, sw)

	err = sw.Start(dummyTogglingPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer func() {
		sw.Stop()
	}()

	clk.Add(2250 * time.Millisecond)

	assert.EqualValues(t, statsUpdateFuncInvocationCount.Load(), 22)
	assert.InDelta(t, statsUpdateFuncValue.Load(), 0.5, 0.05)
}

func TestSlidingWindowAccuracy(t *testing.T) {
	// Floats don't really have good atomic primitives
	var cbLock sync.RWMutex
	lastAverage := 0.0

	statsUpdateFunc := func(avg float64) {
		cbLock.Lock()
		defer cbLock.Unlock()
		lastAverage = avg
	}

	clk := clock.NewMock()
	sw, err := NewSlidingWindowWithClock(1*time.Second, 10*time.Millisecond, clk)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, statsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	clk.Add(1200 * time.Millisecond)

	assert.Equal(t, 1*time.Second, sw.WindowSize())

	cbLock.RLock()
	defer cbLock.RUnlock()
	assert.InDelta(t, lastAverage, 0.3, DefaultDelta)
}

func TestSlidingWindowAverage(t *testing.T) {
	statsUpdateFuncValue.Store(0.0)

	clk := clock.NewMock()
	sw, err := NewSlidingWindowWithClock(1*time.Second, 100*time.Millisecond, clk)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	clk.Add(50 * time.Millisecond)
	assert.InDelta(t, statsUpdateFuncValue.Load(), 0.0, DefaultDelta)

	for idx := 0; idx < 12; idx++ {
		clk.Add(100 * time.Millisecond)
		assert.InDelta(t, statsUpdateFuncValue.Load(), 0.3, DefaultDelta)
	}
}

func TestSlidingWindowCallback(t *testing.T) {
	statsUpdateFuncInvocationCount.Store(0)
	pollingFuncInvocationCount.Store(0)

	clk := clock.NewMock()
	sw, err := NewSlidingWindowWithClock(100*time.Millisecond, 10*time.Millisecond, clk)
	require.Nil(t, err)

	pollingFunc := func() float64 {
		pollingFuncInvocationCount.Store(1)

		return 0.0
	}

	err = sw.Start(pollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	clk.Add(200 * time.Millisecond)

	require.EqualValues(t, pollingFuncInvocationCount.Load(), 1)
	require.EqualValues(t, statsUpdateFuncInvocationCount.Load(), 20)
}

func TestSlidingWindowFuncInvocationCounts(t *testing.T) {
	statsUpdateFuncInvocationCount.Store(0)
	pollingFuncInvocationCount.Store(0)

	windowSize := 2000 * time.Millisecond
	pollingInterval := 100 * time.Millisecond

	clk := clock.NewMock()
	sw, err := NewSlidingWindowWithClock(windowSize, pollingInterval, clk)
	require.Nil(t, err)

	err = sw.Start(dummyFractionalPollingFunc, dummyStatsUpdateFunc)
	require.Nil(t, err)
	defer sw.Stop()

	assert.Equal(t, windowSize, sw.WindowSize())

	clk.Add(windowSize)
	assert.InDelta(t, pollingFuncInvocationCount.Load(), 19, 3)
	assert.InDelta(t, statsUpdateFuncInvocationCount.Load(), 19, 3)

	clk.Add(200 * time.Millisecond)
	assert.InDelta(t, pollingFuncInvocationCount.Load(), 21, 3)
	assert.InDelta(t, statsUpdateFuncInvocationCount.Load(), 21, 3)
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
