// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

type Bucket struct {
	timestamp time.Time
	sum       int64
}

type MovingSum struct {
	buckets    []Bucket
	timeWindow time.Duration
	bucketSize time.Duration
	clock      clock.Clock
	lock       sync.Mutex
}

func NewMovingSum(timeWindow time.Duration, bucketSize time.Duration, clock clock.Clock) *MovingSum {
	return &MovingSum{
		buckets:    make([]Bucket, 0),
		timeWindow: timeWindow,
		bucketSize: bucketSize,
		clock:      clock,
		lock:       sync.Mutex{},
	}
}

func (ms *MovingSum) AddBytes(byteValue int64) {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	ms.dropOldbuckets()
	now := ms.clock.Now()
	if len(ms.buckets) == 0 || ms.buckets != nil && now.Sub(ms.buckets[len(ms.buckets)-1].timestamp) >= ms.bucketSize {
		// Create a new bucket if necessary
		ms.buckets = append(ms.buckets, Bucket{
			timestamp: now,
			sum:       byteValue,
		})
	} else {
		// Add the value to the last bucket
		ms.buckets[len(ms.buckets)-1].sum += byteValue
	}
}

func (ms *MovingSum) CalculateMovingSum() int64 {
	ms.lock.Lock()
	defer ms.lock.Unlock()

	// Drop old buckets
	ms.dropOldbuckets()

	// Calculate the sum of the remaining buckets
	sum := int64(0)
	for _, bucket := range ms.buckets {
		sum += bucket.sum

	}
	return sum
}

func (ms *MovingSum) dropOldbuckets() {
	now := ms.clock.Now()
	threshold := now.Add(-ms.timeWindow)
	dropFromIndex := 0
	for _, bucket := range ms.buckets {
		if bucket.timestamp.After(threshold) {
			break
		}
		dropFromIndex++
	}
	ms.buckets = ms.buckets[dropFromIndex:]
}

func (ms *MovingSum) InfoKey() string {
	return "24h Moving Sum (bytes): "
}

func (ms *MovingSum) Info() []string {
	MovingSum := ms.CalculateMovingSum()

	return []string{
		fmt.Sprintf("%d", MovingSum),
	}
}
