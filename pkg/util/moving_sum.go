// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"fmt"
	"sync"
	"time"
)

type Bucket struct {
	Timestamp time.Time
	Sum       int64
}

type MovingSum struct {
	Buckets     []*Bucket
	TimeWindow  time.Duration
	BucketSize  time.Duration
	GetTimeFunc func() time.Time
	Lock        *sync.Mutex
}

func NewMovingSum(timeWindow time.Duration, bucketSize time.Duration, getTimeFunc func() time.Time) *MovingSum {
	return &MovingSum{
		Buckets:     make([]*Bucket, 0),
		TimeWindow:  timeWindow,
		BucketSize:  bucketSize,
		GetTimeFunc: getTimeFunc,
		Lock:        &sync.Mutex{},
	}
}

func (ms *MovingSum) AddBytes(byteValue int64) {
	ms.Lock.Lock()
	defer ms.Lock.Unlock()

	now := ms.GetTimeFunc()
	if len(ms.Buckets) == 0 || ms.Buckets != nil && now.Sub(ms.Buckets[len(ms.Buckets)-1].Timestamp) >= ms.BucketSize {
		// Create a new bucket if necessary
		ms.Buckets = append(ms.Buckets, &Bucket{
			Timestamp: now,
			Sum:       byteValue,
		})
		// Drop old buckets if necessary
		if len(ms.Buckets) > 24 {
			ms.Buckets = ms.Buckets[1:]
		}
	} else {
		// Add the value to the last bucket
		ms.Buckets[len(ms.Buckets)-1].Sum += byteValue
	}
}

func (ms *MovingSum) CalculateMovingSum() int64 {
	ms.Lock.Lock()
	defer ms.Lock.Unlock()

	// Drop old buckets
	ms.dropOldBuckets()

	// Calculate the sum of the remaining buckets
	sum := int64(0)
	for _, bucket := range ms.Buckets {
		if bucket != nil {
			sum += bucket.Sum
		}
	}
	return sum
}

func (ms *MovingSum) dropOldBuckets() {
	now := ms.GetTimeFunc()
	threshold := now.Add(-ms.TimeWindow)
	dropFromIndex := 0
	for _, bucket := range ms.Buckets {
		if bucket.Timestamp.After(threshold) {
			break
		}
		dropFromIndex++
	}
	ms.Buckets = ms.Buckets[dropFromIndex:]
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
