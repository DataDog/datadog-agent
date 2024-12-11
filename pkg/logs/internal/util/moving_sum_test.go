// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package util

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
)

func TestMovingSum(t *testing.T) {
	mockClock := clock.NewMock()

	timeWindow := 24 * time.Hour
	totalBuckets := 24
	bucketSize := timeWindow / time.Duration(totalBuckets)

	ms := NewMovingSum(timeWindow, bucketSize, mockClock)
	sum := ms.MovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")

	ms.Add(5)
	ms.Add(10)
	ms.Add(15)
	sum = ms.MovingSum()
	assert.Equal(t, int64(30), sum, "Expected sum to be 30")

	// Advance the clock by 30 hours
	mockClock.Add(30 * time.Hour)
	sum = ms.MovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")

	//Advance the clock by 24 hour
	mockClock.Add(24 * time.Hour)
	ms.Add(20)
	sum = ms.MovingSum()
	assert.Equal(t, int64(20), sum, "Expected sum to be 20")
}

func TestMovingSumOverloadedBuckets(t *testing.T) {
	mockClock := clock.NewMock()

	timeWindow := 10 * time.Hour
	totalBuckets := 10
	bucketSize := timeWindow / time.Duration(totalBuckets)

	// initializing a moving sum with maximum 10 buckets with each bucket being 1 hour long
	ms := NewMovingSum(timeWindow, bucketSize, mockClock)
	sum := ms.MovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")

	//Reset time
	mockClock.Add(24 * time.Hour)

	// Creating 100 buckets with a for loop that adds 2 on even iterations and add 1 on odd iterations. After the MovingSum reaches maximum bucket, the moving sum should remains the same number.
	// At 10th loop the sum will remain 15
	for i := 0; i < 100; i++ {
		mockClock.Add(1 * time.Hour)
		evenNum := 2
		oddNum := 1
		if i%2 == 0 {
			ms.Add(int64(evenNum))
		} else {
			ms.Add(int64(oddNum))
		}
	}
	sum = ms.MovingSum()
	assert.Equal(t, int64(15), sum, "Expected sum to be 15")

	// Clear buckets
	mockClock.Add(24 * time.Hour)
	sum = ms.MovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")
}
