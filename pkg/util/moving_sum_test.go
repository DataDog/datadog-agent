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

	getTimeFunc := func() time.Time {
		return mockClock.Now()
	}

	timeWindow := 24 * time.Hour
	totalBuckets := 24
	bucketSize := timeWindow / time.Duration(totalBuckets)

	ms := NewMovingSum(timeWindow, bucketSize, getTimeFunc)
	sum := ms.CalculateMovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")

	ms.AddBytes(5)
	ms.AddBytes(10)
	ms.AddBytes(15)
	sum = ms.CalculateMovingSum()
	assert.Equal(t, int64(30), sum, "Expected sum to be 30")

	// Advance the clock by 30 hours
	mockClock.Add(30 * time.Hour)
	sum = ms.CalculateMovingSum()
	assert.Equal(t, int64(0), sum, "Expected sum to be 0")

	//Advance the clock by 24 hour
	mockClock.Add(24 * time.Hour)
	ms.AddBytes(20)
	sum = ms.CalculateMovingSum()
	assert.Equal(t, int64(20), sum, "Expected sum to be 20")
}
