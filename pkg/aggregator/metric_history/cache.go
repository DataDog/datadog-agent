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
	excludePrefixes []string // metrics matching these prefixes are excluded

	// Expiry duration for series that haven't been seen
	expiryDuration time.Duration // default: 25 minutes (100 cycles * 15s)

	// Anomaly detection
	flushCount             int
	detectionEnabled       bool
	detectionInterval      int
	registry               *DetectorRegistry
	minSeverity            float64  // minimum severity to report (0-1)
	excludeAnomalyPrefixes []string // metric prefixes excluded from anomaly detection
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
		expiryDuration:  25 * time.Minute,
	}
}

// matchesPrefix checks if a metric name matches any of the configured prefixes.
// Returns true if no prefixes are configured (include all metrics).
func (c *MetricHistoryCache) matchesPrefix(name string) bool {
	// First check excludes
	for _, prefix := range c.excludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	// Then check includes
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

// matchesAnomalyExclude checks if a metric should be excluded from anomaly detection.
func (c *MetricHistoryCache) matchesAnomalyExclude(name string) bool {
	for _, prefix := range c.excludeAnomalyPrefixes {
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
	c.expiryDuration = cfg.ExpiryDuration
	c.excludePrefixes = cfg.ExcludePrefixes
}

// Expire removes series that haven't been seen for expiryDuration.
// Should be called after each flush cycle to clean up stale series.
// nowTimestamp is the current timestamp in seconds (same units as LastSeen).
//
// Integration Note (Task 8):
// This method should be called in the demultiplexer's flush cycle, after Rollup().
// The call should be made alongside Rollup() in the flush goroutine, typically:
//
//	cache.Rollup(time.Now())
//	cache.Expire(time.Now().Unix())
func (c *MetricHistoryCache) Expire(nowTimestamp int64) int {
	expired := 0
	expiryThreshold := nowTimestamp - int64(c.expiryDuration.Seconds())

	for key, history := range c.series {
		if history.LastSeen < expiryThreshold {
			delete(c.series, key)
			expired++
		}
	}

	return expired
}

// OnFlush is called after each flush cycle to perform maintenance.
// Returns any detected anomalies.
func (c *MetricHistoryCache) OnFlush(now time.Time) []Anomaly {
	c.flushCount++

	// Always run rollup and expiration
	c.Rollup(now)
	c.Expire(now.Unix())

	// Run detection if it's time
	if c.detectionEnabled && c.detectionInterval > 0 && c.flushCount%c.detectionInterval == 0 {
		return c.runDetection()
	}
	return nil
}

// runDetection runs all registered detectors against the cache.
func (c *MetricHistoryCache) runDetection() []Anomaly {
	if c.registry == nil {
		return nil
	}

	anomalies := c.registry.RunAll(c)

	// Apply post-detection filters
	if c.minSeverity > 0 || len(c.excludeAnomalyPrefixes) > 0 {
		filtered := make([]Anomaly, 0, len(anomalies))
		for _, a := range anomalies {
			// Skip low-severity anomalies
			if a.Severity < c.minSeverity {
				continue
			}
			// Skip excluded metrics
			if c.matchesAnomalyExclude(a.SeriesKey.Name) {
				continue
			}
			filtered = append(filtered, a)
		}
		return filtered
	}

	return anomalies
}

// SetupDetectors configures anomaly detection based on the provided configuration.
// The registry parameter should be pre-configured with detectors.
func (c *MetricHistoryCache) SetupDetectors(cfg Config, registry *DetectorRegistry) {
	c.registry = registry
	c.detectionEnabled = cfg.AnomalyDetectionEnabled
	c.detectionInterval = cfg.DetectionIntervalFlushes
	c.minSeverity = cfg.MinSeverity
	c.excludeAnomalyPrefixes = cfg.ExcludeAnomalyPrefixes
}
