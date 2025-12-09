// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBufferPush(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Test initial state
	assert.Equal(t, 0, rb.Len())
	assert.Equal(t, 3, rb.Cap())

	// Push first element
	rb.Push(1)
	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, 1, rb.Get(0))

	// Push second element
	rb.Push(2)
	assert.Equal(t, 2, rb.Len())
	assert.Equal(t, 1, rb.Get(0))
	assert.Equal(t, 2, rb.Get(1))

	// Push third element (full)
	rb.Push(3)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, 1, rb.Get(0))
	assert.Equal(t, 2, rb.Get(1))
	assert.Equal(t, 3, rb.Get(2))
}

func TestRingBufferWrapAround(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Fill the buffer
	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	// Push more elements to test wrap-around
	rb.Push(4)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, 2, rb.Get(0)) // oldest is now 2
	assert.Equal(t, 3, rb.Get(1))
	assert.Equal(t, 4, rb.Get(2)) // newest is 4

	rb.Push(5)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, 3, rb.Get(0)) // oldest is now 3
	assert.Equal(t, 4, rb.Get(1))
	assert.Equal(t, 5, rb.Get(2)) // newest is 5

	rb.Push(6)
	assert.Equal(t, 3, rb.Len())
	assert.Equal(t, 4, rb.Get(0)) // oldest is now 4
	assert.Equal(t, 5, rb.Get(1))
	assert.Equal(t, 6, rb.Get(2)) // newest is 6
}

func TestRingBufferToSlice(t *testing.T) {
	rb := NewRingBuffer[int](3)

	// Empty buffer
	slice := rb.ToSlice()
	assert.Empty(t, slice)

	// Partially filled buffer
	rb.Push(1)
	rb.Push(2)
	slice = rb.ToSlice()
	assert.Equal(t, []int{1, 2}, slice)

	// Full buffer
	rb.Push(3)
	slice = rb.ToSlice()
	assert.Equal(t, []int{1, 2, 3}, slice)

	// After wrap-around
	rb.Push(4)
	slice = rb.ToSlice()
	assert.Equal(t, []int{2, 3, 4}, slice)

	rb.Push(5)
	rb.Push(6)
	slice = rb.ToSlice()
	assert.Equal(t, []int{4, 5, 6}, slice)
}

func TestRingBufferClear(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)
	rb.Push(3)

	rb.Clear()
	assert.Equal(t, 0, rb.Len())
	assert.Equal(t, 3, rb.Cap())

	// Verify we can push after clear
	rb.Push(10)
	assert.Equal(t, 1, rb.Len())
	assert.Equal(t, 10, rb.Get(0))
}

func TestRingBufferGetOutOfBounds(t *testing.T) {
	rb := NewRingBuffer[int](3)

	rb.Push(1)
	rb.Push(2)

	// Test negative index
	val := rb.Get(-1)
	assert.Equal(t, 0, val) // zero value for int

	// Test index >= count
	val = rb.Get(2)
	assert.Equal(t, 0, val) // zero value for int

	val = rb.Get(10)
	assert.Equal(t, 0, val) // zero value for int
}

func TestSummaryStatsMerge(t *testing.T) {
	s1 := SummaryStats{
		Count: 5,
		Sum:   100.0,
		Min:   10.0,
		Max:   30.0,
	}

	s2 := SummaryStats{
		Count: 3,
		Sum:   60.0,
		Min:   5.0,
		Max:   25.0,
	}

	s1.Merge(s2)

	assert.Equal(t, int64(8), s1.Count)
	assert.Equal(t, 160.0, s1.Sum)
	assert.Equal(t, 5.0, s1.Min)   // min of 10 and 5
	assert.Equal(t, 30.0, s1.Max)  // max of 30 and 25
}

func TestSummaryStatsMergeWithNegativeValues(t *testing.T) {
	s1 := SummaryStats{
		Count: 2,
		Sum:   10.0,
		Min:   -5.0,
		Max:   15.0,
	}

	s2 := SummaryStats{
		Count: 3,
		Sum:   -15.0,
		Min:   -10.0,
		Max:   5.0,
	}

	s1.Merge(s2)

	assert.Equal(t, int64(5), s1.Count)
	assert.Equal(t, -5.0, s1.Sum)
	assert.Equal(t, -10.0, s1.Min)
	assert.Equal(t, 15.0, s1.Max)
}

func TestSummaryStatsMean(t *testing.T) {
	// Normal case
	s := SummaryStats{
		Count: 4,
		Sum:   100.0,
		Min:   10.0,
		Max:   40.0,
	}
	assert.Equal(t, 25.0, s.Mean())

	// Negative sum
	s = SummaryStats{
		Count: 2,
		Sum:   -50.0,
		Min:   -30.0,
		Max:   -20.0,
	}
	assert.Equal(t, -25.0, s.Mean())

	// Fractional result
	s = SummaryStats{
		Count: 3,
		Sum:   10.0,
		Min:   1.0,
		Max:   5.0,
	}
	assert.InDelta(t, 3.333333, s.Mean(), 0.00001)
}

func TestSummaryStatsMeanZeroCount(t *testing.T) {
	// Zero count should return 0, not panic or return NaN/Inf
	s := SummaryStats{
		Count: 0,
		Sum:   0.0,
		Min:   0.0,
		Max:   0.0,
	}
	mean := s.Mean()
	assert.Equal(t, 0.0, mean)
	assert.False(t, math.IsNaN(mean))
	assert.False(t, math.IsInf(mean, 0))

	// Zero count with non-zero sum (shouldn't happen but test defensively)
	s = SummaryStats{
		Count: 0,
		Sum:   100.0,
		Min:   10.0,
		Max:   40.0,
	}
	mean = s.Mean()
	assert.Equal(t, 0.0, mean)
	assert.False(t, math.IsNaN(mean))
	assert.False(t, math.IsInf(mean, 0))
}
