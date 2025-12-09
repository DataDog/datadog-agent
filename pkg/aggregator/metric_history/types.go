// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metric_history provides a multi-tier time-series cache for storing
// and querying historical metric data at different resolutions.
package metric_history

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// SeriesKey uniquely identifies a metric series using a context key hash
// along with the original name and tags for debugging/display purposes.
type SeriesKey struct {
	ContextKey ckey.ContextKey // 128-bit hash for identity
	Name       string          // stored for debugging/display
	Tags       []string        // stored for debugging/display
}

// SummaryStats captures distribution summary statistics for a metric.
type SummaryStats struct {
	Count int64
	Sum   float64
	Min   float64
	Max   float64
}

// Mean returns the average value. Returns 0 if Count is 0.
func (s *SummaryStats) Mean() float64 {
	if s.Count == 0 {
		return 0
	}
	return s.Sum / float64(s.Count)
}

// Merge combines another SummaryStats into this one.
func (s *SummaryStats) Merge(other SummaryStats) {
	s.Count += other.Count
	s.Sum += other.Sum
	if other.Min < s.Min {
		s.Min = other.Min
	}
	if other.Max > s.Max {
		s.Max = other.Max
	}
}

// DataPoint represents a timestamped snapshot of summary statistics.
type DataPoint struct {
	Timestamp int64
	Stats     SummaryStats
}

// RingBuffer is a fixed-capacity circular buffer for storing historical data.
type RingBuffer[T any] struct {
	data  []T
	head  int // next write position
	count int // number of elements (up to capacity)
}

// NewRingBuffer creates a new ring buffer with the specified capacity.
func NewRingBuffer[T any](capacity int) *RingBuffer[T] {
	return &RingBuffer[T]{
		data:  make([]T, capacity),
		head:  0,
		count: 0,
	}
}

// Push adds an item to the ring buffer, overwriting the oldest item if full.
func (r *RingBuffer[T]) Push(item T) {
	r.data[r.head] = item
	r.head = (r.head + 1) % len(r.data)
	if r.count < len(r.data) {
		r.count++
	}
}

// Len returns the number of elements currently in the buffer.
func (r *RingBuffer[T]) Len() int {
	return r.count
}

// Cap returns the capacity of the buffer.
func (r *RingBuffer[T]) Cap() int {
	return len(r.data)
}

// Get returns the element at the given index (0 = oldest).
func (r *RingBuffer[T]) Get(index int) T {
	if index < 0 || index >= r.count {
		var zero T
		return zero
	}
	// Calculate the actual position in the circular buffer
	actualIndex := (r.head - r.count + index + len(r.data)) % len(r.data)
	return r.data[actualIndex]
}

// ToSlice returns all elements as a slice, ordered from oldest to newest.
func (r *RingBuffer[T]) ToSlice() []T {
	if r.count == 0 {
		return []T{}
	}
	result := make([]T, r.count)
	for i := 0; i < r.count; i++ {
		result[i] = r.Get(i)
	}
	return result
}

// Clear removes all elements from the buffer.
func (r *RingBuffer[T]) Clear() {
	r.head = 0
	r.count = 0
}

// Tier represents a resolution tier for metric history storage.
type Tier int

const (
	// TierRecent stores high-resolution recent data
	TierRecent Tier = iota
	// TierMedium stores medium-resolution data
	TierMedium
	// TierLong stores low-resolution long-term data
	TierLong
)

// MetricHistory holds all resolution tiers for a single metric series.
type MetricHistory struct {
	Key  SeriesKey
	Type metrics.APIMetricType // from pkg/metrics

	Recent *RingBuffer[DataPoint]
	Medium *RingBuffer[DataPoint]
	Long   *RingBuffer[DataPoint]

	LastSeen int64 // timestamp of last observation (for expiration)
}

// TimestampedValue represents a single timestamped value for query results.
type TimestampedValue struct {
	Timestamp int64
	Value     float64
}
