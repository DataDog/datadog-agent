package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func setupStatsTracker(timeFrame time.Duration, bucketSize time.Duration) (*int64, StatsTracker) {
	now := time.Now().UnixNano()
	s := NewStatsTrackerWithTimeProvider(timeFrame, bucketSize, func() int64 {
		return now
	})
	return &now, s
}

// TestMovingAvg TODO
func TestMovingAvg(t *testing.T) {

	now, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.MovingAvg())

	s.Add(10)
	*now += int64(time.Second)
	assert.Equal(t, int64(10), s.MovingAvg())

	s.Add(10)
	*now += int64(time.Second)

	assert.Equal(t, int64(10), s.MovingAvg())

	s.Add(10)
	*now += int64(time.Second)

	assert.Equal(t, int64(10), s.MovingAvg())

	s.Add(20)
	*now += int64(time.Second)

	assert.Equal(t, int64(12), s.MovingAvg())

	s.Add(40)
	*now += int64(time.Second)

	s.Add(60)
	*now += int64(time.Second)

	s.Add(80)
	*now += int64(time.Second)

	assert.Equal(t, int64(60), s.MovingAvg())
}

func TestMovingAvgBig(t *testing.T) {

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
	// actually 19.99 but check for 19 because of truncation
	assert.Equal(t, int64(19), s.MovingAvg())

	then = *now + 12*int64(time.Hour)

	for *now < then {
		s.Add(60)
		*now += int64(time.Second)
	}
	// actually 44.99 but check for 44 because of truncation
	assert.Equal(t, int64(44), s.MovingAvg())
}

// TestMovingAvg TODO
func TestMovingPeak(t *testing.T) {

	now, s := setupStatsTracker(3*time.Second, time.Second)

	assert.Equal(t, int64(0), s.MovingPeak())

	s.Add(10)
	*now += int64(time.Second)
	assert.Equal(t, int64(10), s.MovingPeak())

	s.Add(1)
	*now += int64(time.Second)
	assert.Equal(t, int64(10), s.MovingPeak())

	s.Add(2)
	*now += int64(time.Second)
	assert.Equal(t, int64(10), s.MovingPeak())

	s.Add(0)
	*now += int64(time.Second)
	assert.Equal(t, int64(10), s.MovingPeak())

	s.Add(0)
	*now += int64(time.Second)
	assert.Equal(t, int64(2), s.MovingPeak())

	s.Add(100)
	*now += int64(time.Second)
	assert.Equal(t, int64(100), s.MovingPeak())

	s.Add(99)
	*now += int64(time.Second)
	assert.Equal(t, int64(100), s.MovingPeak())
}
