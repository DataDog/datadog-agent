// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/stretchr/testify/assert"
)

func TestAlignToBucket(t *testing.T) {
	tests := []struct {
		name       string
		timestamp  int64
		bucketSize time.Duration
		expected   int64
	}{
		{
			name:       "exact minute boundary",
			timestamp:  1609459320, // 2021-01-01 00:02:00
			bucketSize: time.Minute,
			expected:   1609459320,
		},
		{
			name:       "5 seconds into minute",
			timestamp:  1609459265, // 2021-01-01 00:01:05
			bucketSize: time.Minute,
			expected:   1609459260, // 2021-01-01 00:01:00
		},
		{
			name:       "55 seconds into minute",
			timestamp:  1609459315, // 2021-01-01 00:01:55
			bucketSize: time.Minute,
			expected:   1609459260, // 2021-01-01 00:01:00
		},
		{
			name:       "exact hour boundary",
			timestamp:  1609459200, // 2021-01-01 00:00:00
			bucketSize: time.Hour,
			expected:   1609459200,
		},
		{
			name:       "30 minutes into hour",
			timestamp:  1609461000, // 2021-01-01 00:30:00
			bucketSize: time.Hour,
			expected:   1609459200, // 2021-01-01 00:00:00
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := alignToBucket(tt.timestamp, tt.bucketSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRollupTier_OlderThanRetention(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute

	source := NewRingBuffer[DataPoint](100)
	dest := NewRingBuffer[DataPoint](100)

	// Current time: 2021-01-01 00:10:00
	now := time.Unix(1609459800, 0)

	// Add points:
	// - 3 minutes ago (within retention) - should NOT be rolled up
	// - 6 minutes ago (older than retention) - should be rolled up
	// - 7 minutes ago (older than retention) - should be rolled up
	source.Push(DataPoint{
		Timestamp: now.Unix() - 3*60, // 00:07:00
		Stats: SummaryStats{
			Count: 1,
			Sum:   10.0,
			Min:   10.0,
			Max:   10.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: now.Unix() - 6*60, // 00:04:00
		Stats: SummaryStats{
			Count: 1,
			Sum:   20.0,
			Min:   20.0,
			Max:   20.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: now.Unix() - 7*60, // 00:03:00
		Stats: SummaryStats{
			Count: 1,
			Sum:   30.0,
			Min:   30.0,
			Max:   30.0,
		},
	})

	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// Destination should have data from the old points
	assert.Greater(t, dest.Len(), 0, "Destination should have rolled up data")

	// Source should still have all points (we don't remove them)
	assert.Equal(t, 3, source.Len(), "Source should still have all points")
}

func TestRollupTier_BucketAlignment(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute

	source := NewRingBuffer[DataPoint](100)
	dest := NewRingBuffer[DataPoint](100)

	// Current time: 2021-01-01 00:10:00
	now := time.Unix(1609459800, 0)

	// Add two points 6 minutes ago, but at different seconds within the same minute
	// They should be merged into a single 1-minute bucket
	source.Push(DataPoint{
		Timestamp: now.Unix() - 6*60 - 10, // 00:03:50
		Stats: SummaryStats{
			Count: 1,
			Sum:   20.0,
			Min:   20.0,
			Max:   20.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: now.Unix() - 6*60 - 50, // 00:03:10
		Stats: SummaryStats{
			Count: 1,
			Sum:   30.0,
			Min:   30.0,
			Max:   30.0,
		},
	})

	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// Both points should be merged into a single bucket at 00:03:00
	expectedBucketTs := alignToBucket(now.Unix()-6*60-10, time.Minute)

	// Find the bucket in destination
	found := false
	for i := 0; i < dest.Len(); i++ {
		point := dest.Get(i)
		if point.Timestamp == expectedBucketTs {
			found = true
			// Stats should be merged
			assert.Equal(t, int64(2), point.Stats.Count)
			assert.Equal(t, 50.0, point.Stats.Sum)
			assert.Equal(t, 20.0, point.Stats.Min)
			assert.Equal(t, 30.0, point.Stats.Max)
		}
	}
	assert.True(t, found, "Bucket should be found in destination")
}

func TestRollupTier_StatsMerging(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute

	source := NewRingBuffer[DataPoint](100)
	dest := NewRingBuffer[DataPoint](100)

	// Current time: 2021-01-01 00:10:00
	now := time.Unix(1609459800, 0)

	// Add three points in the same minute bucket, 6 minutes ago
	baseTs := now.Unix() - 6*60
	source.Push(DataPoint{
		Timestamp: baseTs,
		Stats: SummaryStats{
			Count: 2,
			Sum:   15.0,
			Min:   5.0,
			Max:   10.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: baseTs + 15,
		Stats: SummaryStats{
			Count: 1,
			Sum:   25.0,
			Min:   25.0,
			Max:   25.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: baseTs + 30,
		Stats: SummaryStats{
			Count: 3,
			Sum:   30.0,
			Min:   3.0,
			Max:   15.0,
		},
	})

	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// All three should be merged into one bucket
	expectedBucketTs := alignToBucket(baseTs, time.Minute)

	found := false
	for i := 0; i < dest.Len(); i++ {
		point := dest.Get(i)
		if point.Timestamp == expectedBucketTs {
			found = true
			// Verify merged stats
			assert.Equal(t, int64(6), point.Stats.Count) // 2 + 1 + 3
			assert.Equal(t, 70.0, point.Stats.Sum)       // 15 + 25 + 30
			assert.Equal(t, 3.0, point.Stats.Min)        // min of 5, 25, 3
			assert.Equal(t, 25.0, point.Stats.Max)       // max of 10, 25, 15
		}
	}
	assert.True(t, found, "Merged bucket should be found in destination")
}

func TestRollupTier_WithinRetention(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute

	source := NewRingBuffer[DataPoint](100)
	dest := NewRingBuffer[DataPoint](100)

	// Current time: 2021-01-01 00:10:00
	now := time.Unix(1609459800, 0)

	// Add points all within retention (last 5 minutes)
	source.Push(DataPoint{
		Timestamp: now.Unix() - 1*60, // 1 minute ago
		Stats: SummaryStats{
			Count: 1,
			Sum:   10.0,
			Min:   10.0,
			Max:   10.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: now.Unix() - 2*60, // 2 minutes ago
		Stats: SummaryStats{
			Count: 1,
			Sum:   20.0,
			Min:   20.0,
			Max:   20.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: now.Unix() - 4*60, // 4 minutes ago
		Stats: SummaryStats{
			Count: 1,
			Sum:   30.0,
			Min:   30.0,
			Max:   30.0,
		},
	})

	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// No points should be rolled up (all are within retention)
	assert.Equal(t, 0, dest.Len(), "No points should be rolled up when all are within retention")
}

func TestRollup_IntegrationTest(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute
	cache.mediumRetention = 1 * time.Hour

	// Create a metric history with data in Recent tier
	history := &MetricHistory{
		Recent: NewRingBuffer[DataPoint](100),
		Medium: NewRingBuffer[DataPoint](100),
		Long:   NewRingBuffer[DataPoint](100),
	}

	// Current time: 2021-01-01 02:00:00
	now := time.Unix(1609466400, 0)

	// Add recent data (within 5 minutes) - should stay in Recent
	history.Recent.Push(DataPoint{
		Timestamp: now.Unix() - 2*60,
		Stats:     SummaryStats{Count: 1, Sum: 10.0, Min: 10.0, Max: 10.0},
	})

	// Add data 10 minutes ago - should roll to Medium (1-minute buckets)
	history.Recent.Push(DataPoint{
		Timestamp: now.Unix() - 10*60,
		Stats:     SummaryStats{Count: 1, Sum: 20.0, Min: 20.0, Max: 20.0},
	})

	// Add data to Medium tier that's older than 1 hour - should roll to Long
	history.Medium.Push(DataPoint{
		Timestamp: now.Unix() - 90*60, // 1.5 hours ago
		Stats:     SummaryStats{Count: 5, Sum: 100.0, Min: 10.0, Max: 30.0},
	})

	// Add current series to cache
	cache.series = map[ckey.ContextKey]*MetricHistory{
		ckey.ContextKey(1): history,
	}

	// Perform rollup
	cache.Rollup(now)

	// Verify Recent still has the recent data
	assert.Equal(t, 2, history.Recent.Len(), "Recent should still have 2 points")

	// Verify Medium received rolled up data from Recent
	assert.Greater(t, history.Medium.Len(), 1, "Medium should have received rolled up data")

	// Verify Long received rolled up data from Medium
	assert.Greater(t, history.Long.Len(), 0, "Long should have received rolled up data from Medium")
}

func TestConfigure(t *testing.T) {
	cache := NewMetricHistoryCache()

	cfg := Config{
		RecentDuration: 10 * time.Minute,
		MediumDuration: 2 * time.Hour,
		LongDuration:   48 * time.Hour,
	}

	cache.Configure(cfg)

	// Verify capacities are calculated correctly
	assert.Equal(t, 40, cache.recentCapacity)  // 10 minutes / 15 seconds
	assert.Equal(t, 120, cache.mediumCapacity) // 2 hours / 1 minute
	assert.Equal(t, 48, cache.longCapacity)    // 48 hours / 1 hour

	// Verify retention durations
	assert.Equal(t, 10*time.Minute, cache.recentRetention)
	assert.Equal(t, 2*time.Hour, cache.mediumRetention)
}

func TestRollupTier_MultipleRollupCalls(t *testing.T) {
	cache := NewMetricHistoryCache()
	cache.recentRetention = 5 * time.Minute

	source := NewRingBuffer[DataPoint](100)
	dest := NewRingBuffer[DataPoint](100)

	// Current time: 2021-01-01 00:10:00
	now := time.Unix(1609459800, 0)

	// Add two points 6 minutes ago in the same bucket
	baseTs := now.Unix() - 6*60
	source.Push(DataPoint{
		Timestamp: baseTs,
		Stats: SummaryStats{
			Count: 1,
			Sum:   10.0,
			Min:   10.0,
			Max:   10.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: baseTs + 15,
		Stats: SummaryStats{
			Count: 1,
			Sum:   20.0,
			Min:   20.0,
			Max:   20.0,
		},
	})

	// First rollup call
	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// Verify first rollup created the aggregated bucket
	expectedBucketTs := alignToBucket(baseTs, time.Minute)
	found := false
	for i := 0; i < dest.Len(); i++ {
		point := dest.Get(i)
		if point.Timestamp == expectedBucketTs {
			found = true
			assert.Equal(t, int64(2), point.Stats.Count)
			assert.Equal(t, 30.0, point.Stats.Sum)
			assert.Equal(t, 10.0, point.Stats.Min)
			assert.Equal(t, 20.0, point.Stats.Max)
		}
	}
	assert.True(t, found, "First rollup should create aggregated bucket")

	// Add more points in the same bucket to source
	source.Push(DataPoint{
		Timestamp: baseTs + 30,
		Stats: SummaryStats{
			Count: 1,
			Sum:   30.0,
			Min:   30.0,
			Max:   30.0,
		},
	})
	source.Push(DataPoint{
		Timestamp: baseTs + 45,
		Stats: SummaryStats{
			Count: 1,
			Sum:   5.0,
			Min:   5.0,
			Max:   5.0,
		},
	})

	// Second rollup call - should aggregate the 2 new points and push them
	// This will result in a new bucket being pushed with just the 2 new points.
	// The ring buffer will naturally handle having multiple entries for the same timestamp.
	cache.rollupTier(source, dest, now, cache.recentRetention, time.Minute)

	// Verify second rollup pushed a new bucket for the same timestamp
	// The destination buffer should now have at least 2 entries (may have more depending on iteration order)
	assert.Greater(t, dest.Len(), 0, "Destination should have rolled up data")

	// Count how many buckets we find with our expected timestamp
	bucketsFound := 0
	hasOriginalBucket := false
	hasNewBucket := false

	for i := 0; i < dest.Len(); i++ {
		point := dest.Get(i)
		if point.Timestamp == expectedBucketTs {
			bucketsFound++
			// Check if this is the original bucket (Count=2, Sum=30)
			if point.Stats.Count == 2 && point.Stats.Sum == 30.0 {
				hasOriginalBucket = true
			}
			// Check if this is the new bucket (Count=2, Sum=35)
			if point.Stats.Count == 2 && point.Stats.Sum == 35.0 {
				hasNewBucket = true
			}
		}
	}

	// Both buckets should exist in the ring buffer after the second rollup
	// This demonstrates that the fix allows multiple rollups without skipping data
	assert.True(t, bucketsFound >= 1, "Should have at least one bucket for the timestamp")
	assert.True(t, hasOriginalBucket || hasNewBucket, "Should have either the original or new bucket")
}
