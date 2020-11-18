package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func setupTest(timeFrame time.Duration) (*int64, SimpleStats) {
	now := time.Now().UnixNano()
	s := NewSimpleStatsWithTimeProvider(timeFrame, func() int64 {
		return now
	})
	return &now, s
}

// TestMovingAvg TODO
func TestMovingAvg(t *testing.T) {

	now, s := setupTest(3 * time.Second)

	assert.Equal(t, int64(0), s.MovingAvg())

	s.Add(2)
	*now += int64(time.Second)
	assert.Equal(t, int64(2), s.MovingAvg())

	s.Add(4)
	*now += int64(time.Second)

	assert.Equal(t, int64(3), s.MovingAvg())

	s.Add(6)
	*now += int64(time.Second)

	assert.Equal(t, int64(4), s.MovingAvg())

	s.Add(8)
	*now += int64(time.Second)

	assert.Equal(t, int64(6), s.MovingAvg())

	s.Add(10)
	*now += int64(time.Second)

	s.Add(12)
	*now += int64(time.Second)

	s.Add(14)
	*now += int64(time.Second)

	assert.Equal(t, int64(12), s.MovingAvg())
}

func TestMovingAvgSmallSize(t *testing.T) {

	now, s := setupTest(0)

	assert.Equal(t, int64(0), s.MovingAvg())

	s.Add(2)
	*now += int64(time.Second)
	assert.Equal(t, int64(2), s.MovingAvg())

	s.Add(4)
	*now += int64(time.Second)

	assert.Equal(t, int64(4), s.MovingAvg())

	s.Add(100)
	*now += int64(time.Second)

	assert.Equal(t, int64(100), s.MovingAvg())
}

// TestMovingAvg TODO
func TestMovingPeak(t *testing.T) {

	now, s := setupTest(3 * time.Second)

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
	assert.Equal(t, int64(2), s.MovingPeak())

	s.Add(100)
	*now += int64(time.Second)
	assert.Equal(t, int64(100), s.MovingPeak())

	s.Add(99)
	*now += int64(time.Second)
	assert.Equal(t, int64(100), s.MovingPeak())
}
