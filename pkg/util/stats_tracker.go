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

// StatsTracker TODO
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

// NewStatsTracker TODO
func NewStatsTracker(timeFrame time.Duration) StatsTracker {
	return NewStatsTrackerWithTimeProvider(timeFrame, func() int64 {
		return time.Now().UnixNano()
	})
}

// NewStatsTrackerWithTimeProvider TODO
func NewStatsTrackerWithTimeProvider(timeFrame time.Duration, timeProvider timeProvider) StatsTracker {
	StatsTracker := StatsTracker{}
	StatsTracker.taggedPoints = make([]taggedPoint, 0)
	StatsTracker.timeFrame = int64(timeFrame)
	StatsTracker.timeProvider = timeProvider
	StatsTracker.lock = &sync.Mutex{}
	return StatsTracker
}

// Add TODO
func (s *StatsTracker) Add(point int64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.allTimeAvg = (s.totalPoints*s.allTimeAvg + point) / (s.totalPoints + 1)
	s.totalPoints++

	if point > s.allTimePeak {
		s.allTimePeak = point
	}

	bufferSize := int64(len(s.taggedPoints))
	s.movingAvg = (bufferSize*s.movingAvg + point) / (bufferSize + 1)

	now := s.timeProvider()
	s.taggedPoints = append(s.taggedPoints, taggedPoint{now, point})

	s.dropPoints(now)
}

// AllTimeAvg TODO
func (s *StatsTracker) AllTimeAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimeAvg
}

// MovingAvg TODO
func (s *StatsTracker) MovingAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.movingAvg
}

// AllTimePeak TODO
func (s *StatsTracker) AllTimePeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimePeak
}

// MovingPeak TODO
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
