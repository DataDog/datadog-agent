// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// PollingFunc is polled at specified intervals to collect the value
// that should be included in the slidingWindow data
type PollingFunc func() (value float64)

// CallbackFunc is invoked when the polling function collects data and
// we need to use the newly-calculated values
type CallbackFunc func(SlidingWindow)

// slidingWindow is an object that polls for a value every `pollingInterval`
// and then keeps those values for the duration of the `windowSize` that can
// then be analyzed/averaged/aggregated. This object should be instantiated
// via `NewSlidingWindow`.
type slidingWindow struct {
	bucketIdx       int
	buckets         []float64
	bucketsLock     sync.RWMutex
	callbackFunc    CallbackFunc
	initialized     bool
	numBuckets      int
	numBucketsUsed  int
	pollingFunc     PollingFunc
	pollingInterval time.Duration
	stopChan        chan struct{}
	stopped         bool
	ticker          *time.Ticker
	windowLock      sync.RWMutex
	windowSize      time.Duration
}

// SlidingWindow is the public API that we expose from the slidingWindow object
type SlidingWindow interface {
	Start(PollingFunc, CallbackFunc) error
	Stop()

	Average() float64
	WindowSize() time.Duration
}

// NewSlidingWindow creates a new instance of a slidingWindow
func NewSlidingWindow(windowSize time.Duration, pollingInterval time.Duration) (SlidingWindow, error) {
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
		pollingInterval: pollingInterval,
		windowSize:      windowSize,
	}, nil
}

// Start creates a new sliding window object, validates the parameters,
// and starts the ticker. We use `Start` to define most of the variables
// instead of the constructor as we are likely to need the SlidingWindow
// instance in the polling/callback closures, leading to a chicken/egg
// problem in usage.
func (sw *slidingWindow) Start(
	pollingFunc PollingFunc,
	callbackFunc CallbackFunc,
) error {
	if sw.isInitialized() {
		return fmt.Errorf("SlidingWindow already initialized")
	}

	// Parameter validation

	if pollingFunc == nil {
		return fmt.Errorf("SlidingWindow pollingFunc must not be nil")
	}

	// Initialize SlidingWindow and its ticker
	sw.windowLock.Lock()
	defer sw.windowLock.Unlock()

	sw.buckets = make([]float64, sw.windowSize/sw.pollingInterval)
	sw.numBuckets = int(sw.windowSize / sw.pollingInterval)

	sw.pollingFunc = pollingFunc

	sw.callbackFunc = callbackFunc
	sw.stopChan = make(chan struct{}, 1)

	sw.newTicker()

	sw.initialized = true

	return nil
}

func (sw *slidingWindow) newTicker() {
	sw.ticker = time.NewTicker(sw.pollingInterval)

	go func() {
		for {

			select {
			case <-sw.stopChan:
				return
			case <-sw.ticker.C:
				sw.windowLock.RLock()

				value := sw.pollingFunc()

				sw.bucketsLock.Lock()

				sw.buckets[sw.bucketIdx] = value
				if sw.numBucketsUsed < sw.numBuckets {
					sw.numBucketsUsed++
				}

				sw.bucketsLock.Unlock()

				sw.bucketIdx = (sw.bucketIdx + 1) % sw.numBuckets

				if sw.callbackFunc != nil {
					sw.callbackFunc(sw)
				}

				sw.windowLock.RUnlock()
			}
		}
	}()
}

func (sw *slidingWindow) isInitialized() bool {
	sw.windowLock.RLock()
	defer sw.windowLock.RUnlock()

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

	sw.windowLock.Lock()
	defer sw.windowLock.Unlock()

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
	if !sw.isInitialized() {
		log.Warnf("Attempting to use SlidingWindow.WindowSize() without initializting it!")
		return time.Duration(0)
	}

	return sw.windowSize
}

// Average returns an average of all the polled values collected over the
// sliding window range.
func (sw *slidingWindow) Average() float64 {
	if !sw.isInitialized() {
		log.Warnf("Attempting to use SlidingWindow.Average() without initializting it!")
		return 0
	}

	totalVal := 0.0

	sw.bucketsLock.RLock()
	defer sw.bucketsLock.RUnlock()

	if sw.numBucketsUsed == 0 {
		return 0.0
	}

	for bucketIdx, bucketVal := range sw.buckets {
		if sw.numBucketsUsed < bucketIdx {
			continue
		}

		totalVal += bucketVal
	}

	return totalVal / float64(sw.numBucketsUsed)
}
