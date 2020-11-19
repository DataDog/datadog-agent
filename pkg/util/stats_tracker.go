package util

import (
	"sync"
	"time"
)

type timeProvider func() int64

type taggedPoint struct {
	timeStamp int64
	value     int64
}

// StatsTracker Keeps track of simple stats over its lifetime and a configurable time range
type StatsTracker struct {
	allTimeAvg   int64
	movingAvg    int64
	allTimePeak  int64
	totalPoints  int64
	timeFrame    int64
	taggedPoints []taggedPoint
	timeProvider timeProvider
	lock         *sync.Mutex
}

// NewStatsTracker Creates a new StatsTracker instance
func NewStatsTracker(timeFrame time.Duration) StatsTracker {
	return NewStatsTrackerWithTimeProvider(timeFrame, func() int64 {
		return time.Now().UnixNano()
	})
}

// NewStatsTrackerWithTimeProvider Creates a new StatsTracker instance with a time provider closure (mostly for testing)
func NewStatsTrackerWithTimeProvider(timeFrame time.Duration, timeProvider timeProvider) StatsTracker {
	return StatsTracker{
		taggedPoints: make([]taggedPoint, 0),
		timeFrame:    int64(timeFrame),
		timeProvider: timeProvider,
		lock:         &sync.Mutex{},
	}
}

// Add Records a new value
func (s *StatsTracker) Add(value int64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.allTimeAvg = (s.totalPoints*s.allTimeAvg + value) / (s.totalPoints + 1)
	s.totalPoints++

	if value > s.allTimePeak {
		s.allTimePeak = value
	}

	bufferSize := int64(len(s.taggedPoints))
	s.movingAvg = (bufferSize*s.movingAvg + value) / (bufferSize + 1)

	now := s.timeProvider()
	s.taggedPoints = append(s.taggedPoints, taggedPoint{now, value})

	s.dropPoints(now)
}

// AllTimeAvg Gets the all time average of values seen so far
func (s *StatsTracker) AllTimeAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimeAvg
}

// MovingAvg Gets the moving average of values within the time frame
func (s *StatsTracker) MovingAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.movingAvg
}

// AllTimePeak Gets the largest value seen so far
func (s *StatsTracker) AllTimePeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimePeak
}

// MovingPeak Gets the largest value seen within the time frame
func (s *StatsTracker) MovingPeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.taggedPoints) == 0 {
		return 0
	}
	largest := s.taggedPoints[0].value
	for _, v := range s.taggedPoints {
		if v.value > largest {
			largest = v.value
		}
	}
	return largest
}

func (s *StatsTracker) dropPoints(from int64) {
	dropFromIndex := 0
	for i, v := range s.taggedPoints {
		dropFromIndex = i
		if v.timeStamp > from-s.timeFrame {
			break
		}
	}

	size := int64(len(s.taggedPoints))
	if size > 1 {
		for _, droppedPoint := range s.taggedPoints[:dropFromIndex] {
			s.movingAvg = (size*s.movingAvg - droppedPoint.value) / (size - 1)
			size--
		}
	}

	s.taggedPoints = s.taggedPoints[dropFromIndex:]
}
