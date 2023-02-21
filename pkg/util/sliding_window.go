// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PollingFunc is polled at specified intervals to collect the value
// that should be included in the slidingWindow data
type PollingFunc func() (value float64)

// StatsUpdateFunc is invoked when the polling function finishes collecting
// the data and the internal stats are updated.
type StatsUpdateFunc func(float64)

// slidingWindow is an object that polls for a value every `pollingInterval`
// and then keeps those values for the duration of the `windowSize` that can
// then be analyzed/averaged/aggregated. This object should be instantiated
// via `NewSlidingWindow`.
type slidingWindow struct {
	bucketIdx       int
	buckets         []float64
	initialized     bool
	numBuckets      int
	numBucketsUsed  int
	pollingFunc     PollingFunc
	pollingInterval time.Duration
	stateChangeLock sync.RWMutex // Guards against async state transitions
	statsUpdateFunc StatsUpdateFunc
	stopChan        chan struct{}
	stopped         bool
	clock           clock.Clock
	ticker          *clock.Ticker
	windowSize      time.Duration
}

// SlidingWindow is the public API that we expose from the slidingWindow object
type SlidingWindow interface {
	// Start validates the parameters of a SlidingWindow object and starts the
	// ticker. Start can only be invoked once on an instance.
	Start(PollingFunc, StatsUpdateFunc) error

	// Stop stops the polling and processing of the data.
	Stop()

	// WindowSize returns the amount of time that the SlidingWindow will keep
	// the polled values before evicting them.
	WindowSize() time.Duration
}

// NewSlidingWindow creates a new instance of a slidingWindow
func NewSlidingWindow(windowSize time.Duration, pollingInterval time.Duration) (SlidingWindow, error) {
	return NewSlidingWindowWithClock(windowSize, pollingInterval, clock.New())
}

// NewSlidingWindowWithClock creates a new instance of a slidingWindow but with
// a custom clock implementation
func NewSlidingWindowWithClock(windowSize time.Duration, pollingInterval time.Duration, clock clock.Clock) (SlidingWindow, error) {
	if windowSize == 0 {
		return nil, fmt.Errorf("SlidingWindow windowSize cannot be 0")
	}

	if pollingInterval == 0 {
		return nil, fmt.Errorf("SlidingWindow pollingInterval cannot be 0")
	}

	if pollingInterval > windowSize {
		return nil, fmt.Errorf("SlidingWindow windowSize must be smaller than the polling interval")
	}

	if windowSize%pollingInterval != 0 {
		return nil, fmt.Errorf("SlidingWindow windowSize must be a multiple of polling interval")
	}

	return &slidingWindow{
		buckets:         make([]float64, windowSize/pollingInterval),
		numBuckets:      int(windowSize / pollingInterval),
		pollingInterval: pollingInterval,
		windowSize:      windowSize,
		clock:           clock,
	}, nil
}

// Start validates the parameters of a SlidingWindow object and starts the
// ticker. We use `Start` to define most of the variables instead of the
// constructor as we are likely to need the SlidingWindow instance in the
// polling/update closures, leading to a chicken/egg problem in usage.
func (sw *slidingWindow) Start(
	pollingFunc PollingFunc,
	statsUpdateFunc StatsUpdateFunc,
) error {

	if sw.isInitialized() {
		return fmt.Errorf("SlidingWindow already initialized")
	}

	// Parameter validation

	if pollingFunc == nil {
		return fmt.Errorf("SlidingWindow pollingFunc must not be nil")
	}

	// Initialize SlidingWindow and its ticker
	sw.stateChangeLock.Lock()
	defer sw.stateChangeLock.Unlock()

	sw.pollingFunc = pollingFunc
	sw.statsUpdateFunc = statsUpdateFunc
	sw.stopChan = make(chan struct{}, 1)

	sw.newTicker()

	sw.initialized = true

	return nil
}

func (sw *slidingWindow) newTicker() {
	sw.ticker = sw.clock.Ticker(sw.pollingInterval)

	go func() {
		for {
			select {
			case <-sw.stopChan:
				return
			case <-sw.ticker.C:
				// Invoke the polling function

				sw.stateChangeLock.RLock()
				if sw.stopped {
					sw.stateChangeLock.RUnlock()
					return
				}
				sw.stateChangeLock.RUnlock()

				value := sw.pollingFunc()

				// Store the data and update any needed variables

				sw.buckets[sw.bucketIdx] = value
				if sw.numBucketsUsed < sw.numBuckets {
					sw.numBucketsUsed++
				}

				sw.bucketIdx = (sw.bucketIdx + 1) % sw.numBuckets

				// If statsUpdateFunc is defined, invoke it

				sw.stateChangeLock.RLock()

				if sw.statsUpdateFunc != nil && !sw.stopped {
					sw.statsUpdateFunc(sw.average())
				}

				sw.stateChangeLock.RUnlock()
			}
		}
	}()
}

func (sw *slidingWindow) isInitialized() bool {
	sw.stateChangeLock.RLock()
	defer sw.stateChangeLock.RUnlock()

	if !sw.initialized {
		return false
	}

	return true
}

// Stop stops the polling ticker
func (sw *slidingWindow) Stop() {
	if !sw.isInitialized() {
		log.Warnf("Attempting to use SlidingWindow.Stop() without initializting it!")
		return
	}

	sw.stateChangeLock.Lock()
	defer sw.stateChangeLock.Unlock()

	if sw.stopped {
		return
	}

	sw.ticker.Stop()
	sw.stopChan <- struct{}{}

	sw.stopped = true
}

// WindowSize is the amount of time that the SlidingWindow will keep
// the polled values before evicting them.
func (sw *slidingWindow) WindowSize() time.Duration {
	return sw.windowSize
}

// average returns an average of all the polled values collected over the
// sliding window range.
func (sw *slidingWindow) average() float64 {
	totalVal := 0.0

	if sw.numBucketsUsed == 0 {
		return 0.0
	}

	for bucketIdx, bucketVal := range sw.buckets {
		if sw.numBucketsUsed < bucketIdx {
			continue
		}

		totalVal += bucketVal
	}

	return totalVal / float64(sw.numBuckets)
}
