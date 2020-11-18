package config

import (
	"sync"
	"time"
)

type timeProvider func() int64

type taggedPoint struct {
	timeStamp int64
	point     int64
}

// SimpleStats TODO
type SimpleStats struct {
	allTimeAvg   int64
	movingAvg    int64
	allTimePeak  int64
	totalPoints  int64
	timeFrame    int64
	taggedPoints []taggedPoint
	timeProvider timeProvider
	lock         *sync.Mutex
}

// NewSimpleStats TODO
func NewSimpleStats(timeFrame time.Duration) SimpleStats {
	return NewSimpleStatsWithTimeProvider(timeFrame, func() int64 {
		return time.Now().UnixNano()
	})
}

// NewSimpleStatsWithTimeProvider TODO
func NewSimpleStatsWithTimeProvider(timeFrame time.Duration, timeProvider timeProvider) SimpleStats {
	simpleStats := SimpleStats{}
	simpleStats.taggedPoints = make([]taggedPoint, 0)
	simpleStats.timeFrame = int64(timeFrame)
	simpleStats.timeProvider = timeProvider
	simpleStats.lock = &sync.Mutex{}
	return simpleStats
}

// Add TODO
func (s *SimpleStats) Add(point int64) {
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
func (s *SimpleStats) AllTimeAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimeAvg
}

// MovingAvg TODO
func (s *SimpleStats) MovingAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.movingAvg
}

// AllTimePeak TODO
func (s *SimpleStats) AllTimePeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimePeak
}

// MovingPeak TODO
func (s *SimpleStats) MovingPeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	if len(s.taggedPoints) == 0 {
		return 0
	}
	largest := s.taggedPoints[0].point
	for _, v := range s.taggedPoints {
		if v.point > largest {
			largest = v.point
		}
	}
	return largest
}

func (s *SimpleStats) dropPoints(from int64) {
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
			s.movingAvg = (size*s.movingAvg - droppedPoint.point) / (size - 1)
			size--
		}
	}

	s.taggedPoints = s.taggedPoints[dropFromIndex:]
}
