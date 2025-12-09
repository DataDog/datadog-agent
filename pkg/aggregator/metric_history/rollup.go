// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"time"
)

// Rollup performs time-based aggregation from fine to coarse resolution.
// Should be called after each flush cycle.
func (c *MetricHistoryCache) Rollup(now time.Time) {
	for _, history := range c.series {
		c.rollupSeries(history, now)
	}
}

func (c *MetricHistoryCache) rollupSeries(history *MetricHistory, now time.Time) {
	// Roll Recent → Medium (data older than recentRetention, aggregate to 1-minute buckets)
	c.rollupTier(history.Recent, history.Medium, now, c.recentRetention, time.Minute)

	// Roll Medium → Long (data older than mediumRetention, aggregate to 1-hour buckets)
	c.rollupTier(history.Medium, history.Long, now, c.mediumRetention, time.Hour)
}

func (c *MetricHistoryCache) rollupTier(
	source *RingBuffer[DataPoint],
	dest *RingBuffer[DataPoint],
	now time.Time,
	retention time.Duration,
	bucketSize time.Duration,
) {
	// Calculate the cutoff time - points older than this should be rolled up
	cutoffTimestamp := now.Unix() - int64(retention.Seconds())

	// Map to group points by bucket timestamp
	buckets := make(map[int64]SummaryStats)

	// Iterate through source buffer to find points older than retention
	for i := 0; i < source.Len(); i++ {
		point := source.Get(i)

		// Only process points older than retention
		if point.Timestamp <= cutoffTimestamp {
			// Align timestamp to bucket boundary
			bucketTs := alignToBucket(point.Timestamp, bucketSize)

			// Merge stats into the bucket
			if stats, exists := buckets[bucketTs]; exists {
				stats.Merge(point.Stats)
				buckets[bucketTs] = stats
			} else {
				buckets[bucketTs] = point.Stats
			}
		}
	}

	// Push aggregated buckets to destination buffer
	// Note: If a bucket with the same timestamp already exists in the destination,
	// the ring buffer will naturally handle overwrites as it cycles through.
	// Since we aggregate oldest-first, newer aggregated data will naturally
	// replace older versions of the same bucket.
	for bucketTs, stats := range buckets {
		dest.Push(DataPoint{
			Timestamp: bucketTs,
			Stats:     stats,
		})
	}

	// Note: We don't remove points from source as per the instructions.
	// They will age out naturally via ring buffer overwrite.
}

// alignToBucket floors a timestamp to the nearest bucket boundary.
// For example, with bucketSize = 1 minute:
// - 1609459265 (2021-01-01 00:01:05) → 1609459260 (2021-01-01 00:01:00)
// - 1609459320 (2021-01-01 00:02:00) → 1609459320 (2021-01-01 00:02:00)
func alignToBucket(timestamp int64, bucketSize time.Duration) int64 {
	bucketSizeSeconds := int64(bucketSize.Seconds())
	return (timestamp / bucketSizeSeconds) * bucketSizeSeconds
}
