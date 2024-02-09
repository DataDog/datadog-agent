// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

// Bucket represents a bucket of data within a time window, containing a timestamp and sum.
type bucket struct {
	timestamp time.Time
	sum       int64
}

// MovingSum is a time-based moving sum that uses buckets to calculate the sum over a specified window.
type MovingSum struct {
	buckets    []bucket
	timeWindow time.Duration
	bucketSize time.Duration
	currentSum int64
	clock      clock.Clock
	lock       sync.Mutex
}

// NewMovingSum creates a new MovingSum with the specified time window, bucket size, and clock.
func NewMovingSum(timeWindow time.Duration, bucketSize time.Duration, clock clock.Clock) *MovingSum {
	return &MovingSum{
		buckets:    make([]bucket, 0),
		timeWindow: timeWindow,
		bucketSize: bucketSize,
		clock:      clock,
		lock:       sync.Mutex{},
	}
}

// Add adds a byte value to the MovingSum, creating a new bucket if necessary.
func (ms *MovingSum) Add(byteValue int64) {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	ms.dropOldBuckets()
	now := ms.clock.Now()
	if len(ms.buckets) == 0 || now.Sub(ms.buckets[len(ms.buckets)-1].timestamp) >= ms.bucketSize {
		// Create a new bucket if necessary
		ms.buckets = append(ms.buckets, bucket{
			timestamp: now,
			sum:       byteValue,
		})
	} else {
		// Add the value to the last bucket
		ms.buckets[len(ms.buckets)-1].sum += byteValue
	}
	ms.currentSum += byteValue
}

// MovingSum returns the current sum over the specified time window.
func (ms *MovingSum) MovingSum() int64 {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	// Drop old buckets
	ms.dropOldBuckets()

	// Return current sum
	return ms.currentSum
}

// dropOldbuckets removes buckets that are outside the specified time window.
func (ms *MovingSum) dropOldBuckets() {
	now := ms.clock.Now()
	threshold := now.Add(-ms.timeWindow)
	dropFromIndex := 0
	for _, bucket := range ms.buckets {
		if bucket.timestamp.After(threshold) {
			break
		}
		ms.currentSum -= bucket.sum
		dropFromIndex++
	}
	ms.buckets = ms.buckets[dropFromIndex:]
}

// InfoKey returns a string representing the key for the moving sum.
func (ms *MovingSum) InfoKey() string {
	hours := ms.timeWindow.Hours() //timeWindow return hh:mm:ss
	return fmt.Sprintf("%.0fh Moving Sum (bytes)", hours)
}

// Info returns the moving sum as a formatted string slice.
func (ms *MovingSum) Info() []string {
	MovingSum := ms.MovingSum()

	return []string{
		fmt.Sprintf("%d", MovingSum),
	}
}
