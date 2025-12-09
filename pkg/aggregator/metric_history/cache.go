// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metric_history

import (
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/ckey"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// MetricHistoryCache stores historical metric data for tracked series.
type MetricHistoryCache struct {
	series map[ckey.ContextKey]*MetricHistory

	// Configuration (will be set from config later, hardcode defaults for now)
	recentCapacity int // default: 20 (5 min at 15s flush)
	mediumCapacity int // default: 60 (1 hour at 1 min)
	longCapacity   int // default: 24 (24 hours at 1 hour)

	// Retention durations for rollup logic
	recentRetention time.Duration // default: 5 minutes
	mediumRetention time.Duration // default: 1 hour

	// Metric name filtering (prefix match)
	includePrefixes []string // if empty, include all metrics
}

// NewMetricHistoryCache creates a new cache with default configuration.
func NewMetricHistoryCache() *MetricHistoryCache {
	return &MetricHistoryCache{
		series:          make(map[ckey.ContextKey]*MetricHistory),
		recentCapacity:  20,
		mediumCapacity:  60,
		longCapacity:    24,
		recentRetention: 5 * time.Minute,
		mediumRetention: 1 * time.Hour,
		includePrefixes: []string{},
	}
}

// matchesPrefix checks if a metric name matches any of the configured prefixes.
// Returns true if no prefixes are configured (include all metrics).
func (c *MetricHistoryCache) matchesPrefix(name string) bool {
	if len(c.includePrefixes) == 0 {
		return true // include all if no prefixes configured
	}
	for _, prefix := range c.includePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Observe records a metric series point. Called during flush.
// serie is from pkg/metrics.Serie
func (c *MetricHistoryCache) Observe(serie *metrics.Serie) {
	// Check if metric name matches any prefix in includePrefixes
	if !c.matchesPrefix(serie.Name) {
		return
	}

	// Get or create MetricHistory for this series
	history, exists := c.series[serie.ContextKey]
	if !exists {
		// Create a new history entry
		// Convert CompositeTags to []string using UnsafeToReadOnlySliceString
		tags := serie.Tags.UnsafeToReadOnlySliceString()

		history = &MetricHistory{
			Key: SeriesKey{
				ContextKey: serie.ContextKey,
				Name:       serie.Name,
				Tags:       tags,
			},
			Type:   serie.MType,
			Recent: NewRingBuffer[DataPoint](c.recentCapacity),
			Medium: NewRingBuffer[DataPoint](c.mediumCapacity),
			Long:   NewRingBuffer[DataPoint](c.longCapacity),
		}
		c.series[serie.ContextKey] = history
	}

	// For each point in serie.Points, create a DataPoint and push to Recent buffer
	for _, point := range serie.Points {
		stats := SummaryStats{
			Count: 1,
			Sum:   point.Value,
			Min:   point.Value,
			Max:   point.Value,
		}

		dataPoint := DataPoint{
			Timestamp: int64(point.Ts),
			Stats:     stats,
		}

		history.Recent.Push(dataPoint)

		// Update LastSeen timestamp to the latest point timestamp
		if int64(point.Ts) > history.LastSeen {
			history.LastSeen = int64(point.Ts)
		}
	}
}

// SetIncludePrefixes sets the metric name prefixes to include.
// An empty list means include all metrics.
func (c *MetricHistoryCache) SetIncludePrefixes(prefixes []string) {
	c.includePrefixes = prefixes
}

// GetHistory returns the history for a series key, or nil if not found.
func (c *MetricHistoryCache) GetHistory(key ckey.ContextKey) *MetricHistory {
	return c.series[key]
}

// SeriesCount returns the number of tracked series.
func (c *MetricHistoryCache) SeriesCount() int {
	return len(c.series)
}

// Configure applies a Config to the cache, updating capacities and retention durations.
func (c *MetricHistoryCache) Configure(cfg Config) {
	c.recentCapacity = cfg.RecentCapacity()
	c.mediumCapacity = cfg.MediumCapacity()
	c.longCapacity = cfg.LongCapacity()
	c.recentRetention = cfg.RecentDuration
	c.mediumRetention = cfg.MediumDuration
}
