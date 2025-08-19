// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccumulator(t *testing.T) {
	r := require.New(t)
	now := int64(1)
	// 2 buckets of 2 seconds.
	a, err := newTimeIntervalAccumulator(time.Duration(4)*time.Second, time.Duration(2)*time.Second)
	r.NoError(err)

	// first bucket
	sum, duration := addToAccumulator(a, now, 8)
	expectSumAndDuration(r, sum, duration, 8, 1)
	sum, duration = addToAccumulator(a, now, 2)
	expectSumAndDuration(r, sum, duration, 10, 1)

	now++
	sum, duration = addToAccumulator(a, now, 3)
	expectSumAndDuration(r, sum, duration, 13, 2)

	// second bucket
	now++
	sum, duration = addToAccumulator(a, now, 4)
	expectSumAndDuration(r, sum, duration, 17, 3)

	now++
	sum, duration = addToAccumulator(a, now, 5)
	expectSumAndDuration(r, sum, duration, 22, 4)

	// override first bucket
	now++
	sum, duration = addToAccumulator(a, now, 6)
	expectSumAndDuration(r, sum, duration, 15, 3)

	now++
	sum, duration = addToAccumulator(a, now, 7)
	expectSumAndDuration(r, sum, duration, 22, 4)

	// override the second bucket
	now++
	sum, duration = addToAccumulator(a, now, 8)
	expectSumAndDuration(r, sum, duration, 21, 3)
}

func TestAccumulatorEmptyBucket(t *testing.T) {
	r := require.New(t)
	now := int64(1)
	// 2 buckets of 3 seconds
	a, err := newTimeIntervalAccumulator(time.Duration(6)*time.Second, time.Duration(3)*time.Second)
	r.NoError(err)

	sum, duration := addToAccumulator(a, now, 10)
	expectSumAndDuration(r, sum, duration, 10, 1)

	// Skip 2 buckets, first position in the bucket
	now += 2 * 3
	sum, duration = addToAccumulator(a, now, 1)
	expectSumAndDuration(r, sum, duration, 1, 4)

	// Skip 2 buckets, second position in the bucket
	now += 2*3 + 1
	sum, duration = addToAccumulator(a, now, 2)
	expectSumAndDuration(r, sum, duration, 2, 5)

	// Skip 2 buckets, third position in the bucket
	now += 2*3 + 1
	sum, duration = addToAccumulator(a, now, 3)
	expectSumAndDuration(r, sum, duration, 3, 6)
}

func TestAccumulatorCompareToNaive(t *testing.T) {
	r := require.New(t)

	// 6 buckets of 1 second
	a, err := newTimeIntervalAccumulator(time.Duration(6)*time.Second, time.Duration(1)*time.Second)
	r.NoError(err)

	accumulator := make([]int64, 0)
	for i := int64(0); i < 100; i++ {
		sum, duration := addToAccumulator(a, i, i)

		accumulator = append(accumulator, i)
		if len(accumulator) > 6 {
			accumulator = accumulator[1:]
		}
		expectedSum := int64(0)
		for _, v := range accumulator {
			expectedSum += v
		}
		expectSumAndDuration(r, sum, duration, expectedSum, len(accumulator))
	}
}

func addToAccumulator(a *timeIntervalAccumulator, unixTime int64, value int64) (int64, time.Duration) {
	t := time.Unix(unixTime, 0)
	a.add(t, value)
	return a.getDuration(t)
}

func expectSumAndDuration(
	r *require.Assertions,
	sum int64,
	duration time.Duration,
	expectedSum int64,
	expectedDuration int) {

	r.Equal(expectedSum, sum)
	r.Equal(time.Duration(expectedDuration)*time.Second, duration)
}
