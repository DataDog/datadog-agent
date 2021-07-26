// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const DefaultDelta = 0.001

var (
	dummyPollingFuncToggle = true
	invocationCount        = 0
)

// This function on average should return 0.5 value
func dummyTogglingPollingFunc() float64 {
	invocationCount += 1
	dummyPollingFuncToggle = !dummyPollingFuncToggle

	if dummyPollingFuncToggle {
		return 1.0
	}

	return 0.0
}

// This function on average should return about 0.3 value
func dummyFractionalPollingFunc() float64 {
	invocationCount += 1
	randBusy := 0.3 + ((rand.Float64() - 0.5) * 0.001)

	return randBusy
}

func TestSlidingWindow(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 50*time.Millisecond, dummyTogglingPollingFunc)
	if !assert.Nil(t, err) {
		return
	}
	defer sw.Stop()

	time.Sleep(1200 * time.Millisecond)
	utilPct := sw.Average()

	assert.InDelta(t, utilPct, 0.5, DefaultDelta)
}

func TestSlidingWindowAccuracy(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 10*time.Millisecond, dummyFractionalPollingFunc)
	if !assert.Nil(t, err) {
		return
	}
	defer sw.Stop()

	time.Sleep(1200 * time.Millisecond)
	utilPct := sw.Average()

	assert.Equal(t, 1*time.Second, sw.WindowSize)
	assert.InDelta(t, utilPct, 0.3, DefaultDelta)
}

func TestSlidingWindowAverage(t *testing.T) {
	sw, err := NewSlidingWindow(1*time.Second, 100*time.Millisecond, dummyFractionalPollingFunc)
	if !assert.Nil(t, err) {
		return
	}
	defer sw.Stop()

	time.Sleep(50 * time.Millisecond)
	assert.InDelta(t, sw.Average(), 0.0, DefaultDelta)

	for idx := 0; idx < 12; idx++ {
		time.Sleep(100 * time.Millisecond)
		assert.InDelta(t, sw.Average(), 0.3, DefaultDelta)
	}
}

func TestSlidingWindowInvocationCount(t *testing.T) {
	invocationCount = 0

	sw, err := NewSlidingWindow(900*time.Millisecond, 10*time.Millisecond, dummyFractionalPollingFunc)
	if !assert.Nil(t, err) {
		return
	}
	defer sw.Stop()

	assert.Equal(t, 900*time.Millisecond, sw.WindowSize)

	time.Sleep(900 * time.Millisecond)
	assert.InDelta(t, invocationCount, 90, 3)

	time.Sleep(100 * time.Millisecond)
	assert.InDelta(t, invocationCount, 100, 3)
}

func TestNewSlidingWindowParamValidation(t *testing.T) {
	_, err := NewSlidingWindow(0*time.Second, 50*time.Millisecond, dummyTogglingPollingFunc)
	if assert.Error(t, err) {
		assert.EqualError(t, err, "SlidingWindow windowSize cannot be 0")
	}

	_, err = NewSlidingWindow(1*time.Second, 0*time.Second, dummyTogglingPollingFunc)
	if assert.Error(t, err) {
		assert.EqualError(t, err, "SlidingWindow pollingInterval cannot be 0")
	}

	_, err = NewSlidingWindow(1*time.Second, 73*time.Millisecond, dummyTogglingPollingFunc)
	if assert.Error(t, err) {
		assert.EqualError(t, err, "SlidingWindow windowSize must be a multiple of polling interval")
	}

	_, err = NewSlidingWindow(2000*time.Millisecond, 2001*time.Millisecond, dummyTogglingPollingFunc)
	if assert.Error(t, err) {
		assert.EqualError(t, err, "SlidingWindow windowSize must be smaller than the polling interval")
	}

	_, err = NewSlidingWindow(2*time.Second, 1*time.Second, nil)
	if assert.Error(t, err) {
		assert.EqualError(t, err, "SlidingWindow pollingFunc must not be nil")
	}
}
