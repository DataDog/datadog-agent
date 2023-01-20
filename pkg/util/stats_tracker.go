// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"fmt"
	"sync"
	"time"
)

type timeProvider func() int64

type taggedPoint struct {
	timeStamp int64
	value     int64
	count     int64
}

// StatsTracker Keeps track of simple stats over its lifetime and a configurable time range.
// StatsTracker is designed to be memory efficient by aggregating data into buckets. For example
// a time frame of 24 hours with a bucketFrame of 1 hour will ensure that only 24 points are ever
// kept in memory. New data is considered in the stats immediately while old data is removed by
// dropping expired aggregated buckets.
type StatsTracker struct {
	allTimeAvg           int64
	allTimePeak          int64
	totalPoints          int64
	timeFrame            int64
	bucketFrame          int64
	avgPointsHead        *taggedPoint
	peakPointsHead       *taggedPoint
	aggregatedAvgPoints  []*taggedPoint
	aggregatedPeakPoints []*taggedPoint
	timeProvider         timeProvider
	lock                 *sync.Mutex
}

// NewStatsTracker Creates a new StatsTracker instance
func NewStatsTracker(timeFrame time.Duration, bucketSize time.Duration) *StatsTracker {
	return NewStatsTrackerWithTimeProvider(timeFrame, bucketSize, func() int64 {
		return time.Now().UnixNano()
	})
}

// NewStatsTrackerWithTimeProvider Creates a new StatsTracker instance with a time provider closure (mostly for testing)
func NewStatsTrackerWithTimeProvider(timeFrame time.Duration, bucketSize time.Duration, timeProvider timeProvider) *StatsTracker {
	return &StatsTracker{
		aggregatedAvgPoints:  make([]*taggedPoint, 0),
		aggregatedPeakPoints: make([]*taggedPoint, 0),
		timeFrame:            int64(timeFrame),
		bucketFrame:          int64(bucketSize),
		timeProvider:         timeProvider,
		lock:                 &sync.Mutex{},
	}
}

// Add Records a new value to the stats tracker
func (s *StatsTracker) Add(value int64) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.allTimeAvg = (s.totalPoints*s.allTimeAvg + value) / (s.totalPoints + 1)
	s.totalPoints++

	if value > s.allTimePeak {
		s.allTimePeak = value
	}

	now := s.timeProvider()

	s.dropOldPoints(now)

	if s.avgPointsHead == nil {
		s.avgPointsHead = &taggedPoint{now, value, 0}
		s.peakPointsHead = &taggedPoint{now, value, 0}
	} else if s.peakPointsHead.value < value {
		s.peakPointsHead.value = value
	}

	// We initialized avgPointsHead with the first value, don't count it twice
	if s.avgPointsHead.count > 0 {
		s.avgPointsHead.value = (s.avgPointsHead.count*s.avgPointsHead.value + value) / (s.avgPointsHead.count + 1)
	}
	s.avgPointsHead.count++
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

	s.dropOldPoints(s.timeProvider())

	if s.avgPointsHead == nil {
		return 0
	}
	sum := s.avgPointsHead.value * s.avgPointsHead.count
	count := s.avgPointsHead.count
	for _, v := range s.aggregatedAvgPoints {
		sum += v.value * v.count
		count += v.count
	}
	return sum / count
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

	s.dropOldPoints(s.timeProvider())

	if s.peakPointsHead == nil {
		return 0
	}
	largest := s.peakPointsHead.value
	for _, v := range s.aggregatedPeakPoints {
		if v.value > largest {
			largest = v.value
		}
	}
	return largest
}

func (s *StatsTracker) dropOldPoints(now int64) {
	if s.avgPointsHead != nil && s.avgPointsHead.timeStamp < now-s.bucketFrame {
		// Pop off the oldest values
		if len(s.aggregatedAvgPoints) > 0 {
			dropFromIndex := 0
			for _, v := range s.aggregatedAvgPoints {
				if v.timeStamp > now-s.timeFrame {
					break
				}
				dropFromIndex++
			}

			s.aggregatedAvgPoints = s.aggregatedAvgPoints[dropFromIndex:]
			s.aggregatedPeakPoints = s.aggregatedPeakPoints[dropFromIndex:]
		}

		// Add the new aggregated point to the slice
		s.aggregatedAvgPoints = append(s.aggregatedAvgPoints, s.avgPointsHead)
		s.aggregatedPeakPoints = append(s.aggregatedPeakPoints, s.peakPointsHead)
		s.avgPointsHead = nil
		s.peakPointsHead = nil
	}
}

func (s *StatsTracker) InfoKey() string {
	return "Pipeline Latency"
}

func (s *StatsTracker) Info() []string {
	AllTimeAvgLatency := s.AllTimeAvg() / int64(time.Millisecond)
	AllTimePeakLatency := s.AllTimePeak() / int64(time.Millisecond)
	RecentAvgLatency := s.MovingAvg() / int64(time.Millisecond)
	RecentPeakLatency := s.MovingPeak() / int64(time.Millisecond)

	return []string{
		fmt.Sprintf("Average Latency (ms): %d", RecentAvgLatency),
		fmt.Sprintf("24h Average Latency (ms): %d", AllTimeAvgLatency),
		fmt.Sprintf("Peak Latency (ms): %d", RecentPeakLatency),
		fmt.Sprintf("24h Peak Latency (ms): %d", AllTimePeakLatency),
	}
}
