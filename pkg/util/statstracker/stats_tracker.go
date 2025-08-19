// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statstracker keeps track of simple stats in the Agent.
package statstracker

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

// Tracker Keeps track of simple stats over its lifetime and a configurable time range.
// Tracker is designed to be memory efficient by aggregating data into buckets. For example
// a time frame of 24 hours with a bucketFrame of 1 hour will ensure that only 24 points are ever
// kept in memory. New data is considered in the stats immediately while old data is removed by
// dropping expired aggregated buckets.
type Tracker struct {
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

// NewTracker Creates a new Tracker instance
func NewTracker(timeFrame time.Duration, bucketSize time.Duration) *Tracker {
	return NewTrackerWithTimeProvider(timeFrame, bucketSize, func() int64 {
		return time.Now().UnixNano()
	})
}

// NewTrackerWithTimeProvider Creates a new Tracker instance with a time provider closure (mostly for testing)
func NewTrackerWithTimeProvider(timeFrame time.Duration, bucketSize time.Duration, timeProvider timeProvider) *Tracker {
	return &Tracker{
		aggregatedAvgPoints:  make([]*taggedPoint, 0),
		aggregatedPeakPoints: make([]*taggedPoint, 0),
		timeFrame:            int64(timeFrame),
		bucketFrame:          int64(bucketSize),
		timeProvider:         timeProvider,
		lock:                 &sync.Mutex{},
	}
}

// Add Records a new value to the stats tracker
func (s *Tracker) Add(value int64) {
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
func (s *Tracker) AllTimeAvg() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimeAvg
}

// MovingAvg Gets the moving average of values within the time frame
func (s *Tracker) MovingAvg() int64 {
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
func (s *Tracker) AllTimePeak() int64 {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.allTimePeak
}

// MovingPeak Gets the largest value seen within the time frame
func (s *Tracker) MovingPeak() int64 {
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

func (s *Tracker) dropOldPoints(now int64) {
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

// InfoKey returns the key
func (s *Tracker) InfoKey() string {
	return "Pipeline Latency"
}

// Info returns the Tracker as a formatted string slice.
func (s *Tracker) Info() []string {
	return []string{
		fmt.Sprintf("Average Latency: %s", time.Duration(s.AllTimeAvg())),
		fmt.Sprintf("24h Average Latency: %s", time.Duration(s.MovingAvg())),
		fmt.Sprintf("Peak Latency: %s", time.Duration(s.AllTimePeak())),
		fmt.Sprintf("24h Peak Latency: %s", time.Duration(s.MovingPeak())),
	}
}
