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
// that should be included in the SlidingWindow data
type PollingFunc func() (value float64)

// SlidingWindow is an object that polls for a value every `pollingInterval`
// and then keeps those values for the duration of the `WindowSize` that can
// then be analyzed/averaged/aggregated. This object should be instantiated
// via `NewSlidingWindow`.
type SlidingWindow struct {
	// WindowSize is the amount of time that the SlidingWindow will keep
	// the polled values before evicting them.
	WindowSize time.Duration

	bucketIdx       int
	buckets         []float64
	bucketsLock     sync.Mutex
	numBuckets      int
	numBucketsUsed  int
	pollingFunc     PollingFunc
	pollingInterval time.Duration
	ticker          *time.Ticker
}

// NewSlidingWindow creates a new sliding window object, validates the parameters,
// and starts the ticker.
func NewSlidingWindow(
	windowSize time.Duration,
	pollingInterval time.Duration,
	pollingFunc PollingFunc,
) (*SlidingWindow, error) {

	// Parameter validation

	if windowSize == 0 {
		return nil, fmt.Errorf("SlidingWindow windowSize cannot be 0")
	}

	if pollingInterval == 0 {
		return nil, fmt.Errorf("SlidingWindow pollingInterval cannot be 0")
	}

	if pollingInterval > windowSize {
		return nil, fmt.Errorf("SlidingWindow windowSize must be smaller than the polling interval")
	}

	if pollingFunc == nil {
		return nil, fmt.Errorf("SlidingWindow pollingFunc must not be nil")
	}

	if windowSize%pollingInterval != 0 {
		return nil, fmt.Errorf("SlidingWindow windowSize must be a multiple of polling interval")
	}

	// Initialize SlidingWindow and its ticker

	sw := SlidingWindow{
		WindowSize:      windowSize,
		buckets:         make([]float64, windowSize/pollingInterval),
		numBuckets:      int(windowSize / pollingInterval),
		pollingInterval: pollingInterval,
		pollingFunc:     pollingFunc,
	}
	sw.newTicker()

	return &sw, nil
}

func (sw *SlidingWindow) newTicker() {
	sw.ticker = time.NewTicker(sw.pollingInterval)
	go func() {
		for {
			<-sw.ticker.C
			value := sw.pollingFunc()
			// TODO: REMOVE ME
			log.Warnf("value: %f\n", value)

			sw.bucketsLock.Lock()

			sw.buckets[sw.bucketIdx] = value
			if sw.numBucketsUsed < sw.numBuckets {
				sw.numBucketsUsed += 1
			}

			sw.bucketsLock.Unlock()

			sw.bucketIdx = (sw.bucketIdx + 1) % sw.numBuckets
			// TODO: REMOVE ME
			log.Warnf("Bucket idx now: %d\n", sw.bucketIdx)
		}
	}()
}

// Stop stops the polling ticker
func (sw *SlidingWindow) Stop() {
	sw.ticker.Stop()
}

// Average returns an average of all the polled values collected over the
// sliding window range.
func (sw *SlidingWindow) Average() float64 {
	// TODO: REMOVE ME
	log.Warnf("%+v\n", sw)

	totalVal := 0.0

	sw.bucketsLock.Lock()
	defer sw.bucketsLock.Unlock()

	if sw.numBucketsUsed == 0 {
		return 0.0
	}

	for bucketIdx, bucketVal := range sw.buckets {
		if sw.numBucketsUsed < bucketIdx {
			continue
		}

		// TODO: REMOVE ME
		log.Warnf("tot: %f, add: %f\n", totalVal, bucketVal)
		totalVal += bucketVal
	}

	return totalVal / float64(sw.numBucketsUsed)
}
