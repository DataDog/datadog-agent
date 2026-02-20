// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && nvml

package gpu

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeIntervals(t *testing.T) {
	tests := []struct {
		name      string
		intervals [][2]uint64
		expected  uint64
	}{
		{
			name:      "empty intervals",
			intervals: [][2]uint64{},
			expected:  0,
		},
		{
			name:      "single interval",
			intervals: [][2]uint64{{100, 200}},
			expected:  100,
		},
		{
			name:      "non-overlapping intervals",
			intervals: [][2]uint64{{100, 200}, {300, 400}, {500, 600}},
			expected:  300, // (200-100) + (400-300) + (600-500) = 100 + 100 + 100
		},
		{
			name:      "overlapping intervals",
			intervals: [][2]uint64{{100, 300}, {200, 400}},
			expected:  300, // Merged to [100, 400] = 300
		},
		{
			name:      "multiple overlapping intervals",
			intervals: [][2]uint64{{100, 200}, {150, 250}, {225, 300}},
			expected:  200, // Merged to [100, 300] = 200
		},
		{
			name:      "adjacent intervals",
			intervals: [][2]uint64{{100, 200}, {200, 300}},
			expected:  200, // Merged to [100, 300] = 200
		},
		{
			name:      "fully contained interval",
			intervals: [][2]uint64{{100, 500}, {200, 300}},
			expected:  400, // [200, 300] is fully contained in [100, 500]
		},
		{
			name:      "unsorted intervals in reverse order",
			intervals: [][2]uint64{{900, 1000}, {600, 800}, {300, 500}, {100, 200}},
			expected:  600, // Should sort to [[100, 200], [300, 500], [600, 800], [900, 1000]] = 100 + 200 + 200 + 100
		},
		{
			name:      "identical intervals",
			intervals: [][2]uint64{{100, 200}, {100, 200}},
			expected:  100, // Should merge to single interval
		},
		{
			name:      "one interval contains all others",
			intervals: [][2]uint64{{0, 1000}, {100, 200}, {300, 400}, {500, 600}},
			expected:  1000, // All contained in [0, 1000]
		},
		{
			name:      "zero duration interval mixed with valid",
			intervals: [][2]uint64{{100, 100}, {200, 300}},
			expected:  100, // Zero-duration interval contributes 0
		},
		{
			name:      "all intervals have zero duration",
			intervals: [][2]uint64{{100, 100}, {200, 200}, {300, 300}},
			expected:  0, // All zero-duration intervals result in 0
		},
		{
			name: "many small overlapping intervals",
			intervals: [][2]uint64{
				{100, 110}, {101, 111}, {102, 112}, {103, 113}, {104, 114},
				{105, 115}, {106, 116}, {107, 117}, {108, 118}, {109, 119},
			},
			expected: 19, // Should merge to [100, 119] = 19
		},
		{
			name: "alternating gaps and overlaps",
			intervals: [][2]uint64{
				{100, 200},             // Gap after
				{300, 400}, {350, 450}, // Overlap -> [300, 450]
				{500, 600},                         // Gap after
				{700, 800}, {750, 850}, {800, 900}, // Multiple overlaps -> [700, 900]
			},
			expected: 550, // 100 + 150 + 100 + 200 = 550
		},
		{
			name: "multiple processes with overlaps",
			intervals: [][2]uint64{
				{1000000000, 1005000000}, // Process 1: 5ms
				{1003000000, 1008000000}, // Process 2: 5ms, overlaps 2ms with P1
				{1010000000, 1015000000}, // Process 3: 5ms, no overlap
			},
			expected: 13000000, // [1000000000, 1008000000] + [1010000000, 1015000000] = 8ms + 5ms = 13ms
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeIntervals(tt.intervals)
			require.Equal(t, tt.expected, result, "incorrect merged duration for %s", tt.name)
		})
	}
}

func TestAggregatorProcessKernelSpan(t *testing.T) {
	const deviceMaxThreads = uint64(1000)

	t.Run("span within measurement window", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 1000
		agg.measuredIntervalNs = 1000 // Window: [1000, 2000]

		span := &kernelSpan{
			startKtime:     1200,
			endKtime:       1800,
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		require.Len(t, agg.activeIntervals, 1)
		require.Equal(t, uint64(1200), agg.activeIntervals[0][0])
		require.Equal(t, uint64(1800), agg.activeIntervals[0][1])
	})

	t.Run("span start before measurement window is clamped", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 1000
		agg.measuredIntervalNs = 1000 // Window: [1000, 2000]

		span := &kernelSpan{
			startKtime:     500, // Before window start
			endKtime:       1500,
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		require.Len(t, agg.activeIntervals, 1)
		require.Equal(t, uint64(1000), agg.activeIntervals[0][0], "start should be clamped to lastCheckKtime")
		require.Equal(t, uint64(1500), agg.activeIntervals[0][1])
	})

	t.Run("span end after measurement window is clamped", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 1000
		agg.measuredIntervalNs = 1000 // Window: [1000, 2000]

		span := &kernelSpan{
			startKtime:     1500,
			endKtime:       2500, // After window end
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		require.Len(t, agg.activeIntervals, 1)
		require.Equal(t, uint64(1500), agg.activeIntervals[0][0])
		require.Equal(t, uint64(2000), agg.activeIntervals[0][1], "end should be clamped to window end")
	})

	t.Run("span completely outside measurement window (before)", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 1000
		agg.measuredIntervalNs = 1000 // Window: [1000, 2000]

		span := &kernelSpan{
			startKtime:     100,
			endKtime:       500, // Completely before window
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		// The span is still recorded but with clamped values
		// After clamping: start=1000, end=500, but the interval tracking happens
		// before the start > end check in the thread calculation
		require.Len(t, agg.activeIntervals, 1)
	})

	t.Run("multiple spans accumulate intervals", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 1000
		agg.measuredIntervalNs = 2000 // Window: [1000, 3000]

		spans := []*kernelSpan{
			{startKtime: 1100, endKtime: 1400, avgThreadCount: 100, numKernels: 1},
			{startKtime: 1500, endKtime: 1800, avgThreadCount: 100, numKernels: 1},
			{startKtime: 2000, endKtime: 2500, avgThreadCount: 100, numKernels: 1},
		}

		for _, span := range spans {
			agg.processKernelSpan(span)
		}

		require.Len(t, agg.activeIntervals, 3)
	})
}

func TestAggregatorGetRawStats(t *testing.T) {
	const deviceMaxThreads = uint64(1000)
	const intervalNs = int64(10_000_000_000) // 10 seconds in nanoseconds

	t.Run("computes ActiveTimePct correctly", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = intervalNs

		// Add a span that covers 50% of the interval (5 seconds)
		span := &kernelSpan{
			startKtime:     0,
			endKtime:       5_000_000_000,
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		stats := agg.getRawStats()
		require.InDelta(t, 50.0, stats.ActiveTimePct, 0.1, "ActiveTimePct should be 50%")
	})

	t.Run("merges overlapping intervals for ActiveTimePct", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = intervalNs

		// Two overlapping spans: [0, 6s] and [4s, 8s] -> merged to [0, 8s] = 80%
		agg.processKernelSpan(&kernelSpan{
			startKtime:     0,
			endKtime:       6_000_000_000,
			avgThreadCount: 100,
			numKernels:     1,
		})
		agg.processKernelSpan(&kernelSpan{
			startKtime:     4_000_000_000,
			endKtime:       8_000_000_000,
			avgThreadCount: 100,
			numKernels:     1,
		})

		stats := agg.getRawStats()
		require.InDelta(t, 80.0, stats.ActiveTimePct, 0.1, "ActiveTimePct should be 80% after merging overlaps")
	})

	t.Run("caps ActiveTimePct at 100%", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = intervalNs

		// Span that exceeds the interval (should be capped at 100%)
		span := &kernelSpan{
			startKtime:     0,
			endKtime:       uint64(intervalNs), // Exactly 100%
			avgThreadCount: 100,
			numKernels:     1,
		}
		agg.processKernelSpan(span)

		stats := agg.getRawStats()
		require.LessOrEqual(t, stats.ActiveTimePct, 100.0, "ActiveTimePct should not exceed 100%")
	})

	t.Run("returns zero ActiveTimePct when no intervals", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = intervalNs

		stats := agg.getRawStats()
		require.Equal(t, 0.0, stats.ActiveTimePct, "ActiveTimePct should be 0 when no intervals")
	})

	t.Run("returns zero ActiveTimePct when measuredIntervalNs is zero", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = 0 // Zero interval

		agg.processKernelSpan(&kernelSpan{
			startKtime:     0,
			endKtime:       1000,
			avgThreadCount: 100,
			numKernels:     1,
		})

		stats := agg.getRawStats()
		require.Equal(t, 0.0, stats.ActiveTimePct, "ActiveTimePct should be 0 when interval is zero")
	})

	t.Run("flush clears activeIntervals", func(t *testing.T) {
		agg := newAggregator(deviceMaxThreads)
		agg.lastCheckKtime = 0
		agg.measuredIntervalNs = intervalNs

		agg.processKernelSpan(&kernelSpan{
			startKtime:     0,
			endKtime:       1000,
			avgThreadCount: 100,
			numKernels:     1,
		})

		require.Len(t, agg.activeIntervals, 1)

		// getRawStats calls flush internally
		_ = agg.getRawStats()

		require.Len(t, agg.activeIntervals, 0, "activeIntervals should be cleared after flush")
	})
}

func TestAggregatorFlush(t *testing.T) {
	agg := newAggregator(1000)
	agg.lastCheckKtime = 0
	agg.measuredIntervalNs = 10000

	// Add some data
	agg.processKernelSpan(&kernelSpan{
		startKtime:     100,
		endKtime:       200,
		avgThreadCount: 50,
		numKernels:     1,
	})
	agg.currentAllocs = append(agg.currentAllocs, &memorySpan{size: 1024})
	agg.pastAllocs = append(agg.pastAllocs, &memorySpan{size: 2048})

	require.NotEmpty(t, agg.activeIntervals)
	require.NotEmpty(t, agg.currentAllocs)
	require.NotEmpty(t, agg.pastAllocs)
	require.NotZero(t, agg.totalThreadSecondsUsed)

	agg.flush()

	require.Empty(t, agg.activeIntervals, "activeIntervals should be empty after flush")
	require.Empty(t, agg.currentAllocs, "currentAllocs should be empty after flush")
	require.Empty(t, agg.pastAllocs, "pastAllocs should be empty after flush")
	require.Zero(t, agg.totalThreadSecondsUsed, "totalThreadSecondsUsed should be zero after flush")
}
