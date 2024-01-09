// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package statstracker

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func setupStatsTracker(timeFrame time.Duration, bucketSize time.Duration) (*int64, *Tracker) {
	now := time.Now().UnixNano()
	s := NewTrackerWithTimeProvider(timeFrame, bucketSize, func() int64 {
		return now
	})
	return &now, s
}

func TestMovingAvg(t *testing.T) {

	now, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.MovingAvg())

	s.Add(10)
	assert.Equal(t, int64(10), s.MovingAvg())

	*now += int64(time.Second)
	s.Add(10)

	assert.Equal(t, int64(10), s.MovingAvg())

	*now += int64(time.Second)
	s.Add(10)

	assert.Equal(t, int64(10), s.MovingAvg())

	*now += int64(time.Second)
	s.Add(20)

	assert.Equal(t, int64(12), s.MovingAvg())

	*now += int64(time.Second)
	s.Add(40)

	*now += int64(time.Second)
	s.Add(60)

	*now += int64(time.Second)
	s.Add(80)

	*now += int64(time.Second)
	assert.Equal(t, int64(60), s.MovingAvg())

	// clear out all data
	*now += int64(100 * time.Second)
	assert.Equal(t, int64(0), s.MovingAvg())

}

func TestMovingAvgBigWindow(t *testing.T) {

	now, s := setupStatsTracker(24*time.Hour, time.Hour)
	then := *now + 12*int64(time.Hour)

	for *now < then {
		s.Add(10)
		*now += int64(time.Second)
	}
	assert.Equal(t, int64(10), s.MovingAvg())

	then = *now + 12*int64(time.Hour)

	for *now < then {
		s.Add(30)
		*now += int64(time.Second)
	}
	// Internally the value is 19.99 but check for 19 because of integer truncation
	assert.Equal(t, int64(19), s.MovingAvg())

	then = *now + 12*int64(time.Hour)

	for *now < then {
		s.Add(60)
		*now += int64(time.Second)
	}
	// Internally the value is 44.99 but check for 44 because of integer truncation
	assert.Equal(t, int64(44), s.MovingAvg())
}

func TestMovingPeak(t *testing.T) {

	now, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.MovingPeak())

	s.Add(10)

	assert.Equal(t, int64(10), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(1)

	assert.Equal(t, int64(10), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(2)

	assert.Equal(t, int64(10), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(0)

	assert.Equal(t, int64(10), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(0)

	assert.Equal(t, int64(2), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(100)

	assert.Equal(t, int64(100), s.MovingPeak())

	*now += int64(time.Second)
	s.Add(99)

	assert.Equal(t, int64(100), s.MovingPeak())

	// clear out all data
	*now += int64(100 * time.Second)
	assert.Equal(t, int64(0), s.MovingPeak())
}

func TestAllTimePeak(t *testing.T) {
	_, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.AllTimePeak())

	s.Add(10)
	s.Add(20)

	assert.Equal(t, int64(20), s.AllTimePeak())

	s.Add(5)

	assert.Equal(t, int64(20), s.AllTimePeak())

}

func TestAllTimeAvg(t *testing.T) {

	_, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.AllTimeAvg())

	s.Add(10)

	assert.Equal(t, int64(10), s.AllTimeAvg())

	s.Add(20)

	assert.Equal(t, int64(15), s.AllTimeAvg())

	s.Add(100)

	assert.Equal(t, int64(43), s.AllTimeAvg())
}
