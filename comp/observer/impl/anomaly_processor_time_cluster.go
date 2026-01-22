// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TimeClusterConfig configures the TimeClusterCorrelator.
type TimeClusterConfig struct {
	// SlackSeconds is the time slack for considering anomalies as overlapping.
	// Two anomalies overlap if their time ranges are within SlackSeconds of each other.
	// Default: 5 seconds.
	SlackSeconds int64

	// MinClusterSize is the minimum number of anomalies to form a reportable cluster.
	// Default: 2.
	MinClusterSize int

	// WindowSeconds is how long to keep anomalies before eviction.
	// Default: 60 seconds.
	WindowSeconds int64
}

// DefaultTimeClusterConfig returns a TimeClusterConfig with default values.
func DefaultTimeClusterConfig() TimeClusterConfig {
	return TimeClusterConfig{
		SlackSeconds:   1, // Only 1 second slack - anomalies must be nearly simultaneous
		MinClusterSize: 2,
		WindowSeconds:  60,
	}
}

// timeCluster represents a group of temporally-related anomalies.
type timeCluster struct {
	id          int
	anomalies   map[string]observer.AnomalyOutput // keyed by Source for dedup
	timeRange   observer.TimeRange                // union of all anomaly time ranges (for reporting)
	anchorRange observer.TimeRange                // fixed anchor range for overlap checks (doesn't expand)
}

// TimeClusterCorrelator clusters anomalies based purely on time overlap.
// Anomalies whose time ranges overlap (with configurable slack) are grouped together.
type TimeClusterCorrelator struct {
	config          TimeClusterConfig
	clusters        []*timeCluster
	nextClusterID   int
	currentDataTime int64
}

// NewTimeClusterCorrelator creates a new TimeClusterCorrelator with the given config.
func NewTimeClusterCorrelator(config TimeClusterConfig) *TimeClusterCorrelator {
	if config.SlackSeconds == 0 {
		config.SlackSeconds = 5
	}
	if config.MinClusterSize == 0 {
		config.MinClusterSize = 2
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 60
	}
	return &TimeClusterCorrelator{
		config:   config,
		clusters: nil,
	}
}

// Name returns the processor name.
func (c *TimeClusterCorrelator) Name() string {
	return "time_cluster_correlator"
}

// Process adds an anomaly, either to an existing cluster or a new one.
func (c *TimeClusterCorrelator) Process(anomaly observer.AnomalyOutput) {
	// Update current data time
	if anomaly.TimeRange.End > c.currentDataTime {
		c.currentDataTime = anomaly.TimeRange.End
	}

	// Find clusters this anomaly overlaps with (using anchor range, not expanded range)
	var overlapping []*timeCluster
	for _, cluster := range c.clusters {
		if c.timeRangesOverlap(anomaly.TimeRange, cluster.anchorRange) {
			overlapping = append(overlapping, cluster)
		}
	}

	if len(overlapping) == 0 {
		// No overlap - create new cluster
		c.nextClusterID++
		newCluster := &timeCluster{
			id:          c.nextClusterID,
			anomalies:   map[string]observer.AnomalyOutput{anomaly.Source: anomaly},
			timeRange:   anomaly.TimeRange,
			anchorRange: anomaly.TimeRange, // Fixed anchor - won't expand
		}
		c.clusters = append(c.clusters, newCluster)
	} else if len(overlapping) == 1 {
		// Single overlap - add to existing cluster
		cluster := overlapping[0]
		c.addToCluster(cluster, anomaly)
	} else {
		// Multiple overlaps - merge clusters and add anomaly
		merged := c.mergeClusters(overlapping)
		c.addToCluster(merged, anomaly)
	}
}

// timeRangesOverlap checks if two time ranges overlap, considering slack.
func (c *TimeClusterCorrelator) timeRangesOverlap(a, b observer.TimeRange) bool {
	slack := c.config.SlackSeconds
	// Ranges overlap if: a.Start <= b.End + slack AND b.Start <= a.End + slack
	return a.Start <= b.End+slack && b.Start <= a.End+slack
}

// addToCluster adds an anomaly to a cluster, updating time range (for display) and deduping by source.
// Note: anchorRange is NOT expanded - this prevents cluster creep.
func (c *TimeClusterCorrelator) addToCluster(cluster *timeCluster, anomaly observer.AnomalyOutput) {
	// Dedup by source - keep the one with later End time (more recent data)
	if existing, ok := cluster.anomalies[anomaly.Source]; ok {
		if anomaly.TimeRange.End > existing.TimeRange.End {
			cluster.anomalies[anomaly.Source] = anomaly
		}
	} else {
		cluster.anomalies[anomaly.Source] = anomaly
	}

	// Expand display time range (for UI), but NOT anchorRange
	if anomaly.TimeRange.Start < cluster.timeRange.Start {
		cluster.timeRange.Start = anomaly.TimeRange.Start
	}
	if anomaly.TimeRange.End > cluster.timeRange.End {
		cluster.timeRange.End = anomaly.TimeRange.End
	}
}

// mergeClusters merges multiple clusters into one, removing the others.
func (c *TimeClusterCorrelator) mergeClusters(clusters []*timeCluster) *timeCluster {
	if len(clusters) == 0 {
		return nil
	}

	// Use first cluster as base
	merged := clusters[0]

	// Merge others into it
	for _, other := range clusters[1:] {
		for source, anomaly := range other.anomalies {
			if existing, ok := merged.anomalies[source]; ok {
				if anomaly.TimeRange.End > existing.TimeRange.End {
					merged.anomalies[source] = anomaly
				}
			} else {
				merged.anomalies[source] = anomaly
			}
		}
		// Expand display time range
		if other.timeRange.Start < merged.timeRange.Start {
			merged.timeRange.Start = other.timeRange.Start
		}
		if other.timeRange.End > merged.timeRange.End {
			merged.timeRange.End = other.timeRange.End
		}
		// Expand anchor range (for merges only - allows merged clusters to accept wider overlap)
		if other.anchorRange.Start < merged.anchorRange.Start {
			merged.anchorRange.Start = other.anchorRange.Start
		}
		if other.anchorRange.End > merged.anchorRange.End {
			merged.anchorRange.End = other.anchorRange.End
		}
	}

	// Remove merged clusters from main list
	toRemove := make(map[int]bool)
	for _, other := range clusters[1:] {
		toRemove[other.id] = true
	}
	newClusters := c.clusters[:0]
	for _, cluster := range c.clusters {
		if !toRemove[cluster.id] {
			newClusters = append(newClusters, cluster)
		}
	}
	c.clusters = newClusters

	return merged
}

// Flush evicts old clusters and returns empty (reporters pull state via ActiveCorrelations).
func (c *TimeClusterCorrelator) Flush() []observer.ReportOutput {
	c.evictOldClusters()
	return nil
}

// evictOldClusters removes clusters whose time range is entirely outside the window.
func (c *TimeClusterCorrelator) evictOldClusters() {
	cutoff := c.currentDataTime - c.config.WindowSeconds
	newClusters := c.clusters[:0]
	for _, cluster := range c.clusters {
		if cluster.timeRange.End >= cutoff {
			newClusters = append(newClusters, cluster)
		}
	}
	c.clusters = newClusters
}

// TimeClusterInfo represents a cluster for visualization.
type TimeClusterInfo struct {
	ID           int      `json:"id"`
	Sources      []string `json:"sources"`
	StartTime    int64    `json:"start_time"`
	EndTime      int64    `json:"end_time"`
	AnomalyCount int      `json:"anomaly_count"`
}

// GetClusters returns clusters that meet the minimum size threshold for visualization.
func (c *TimeClusterCorrelator) GetClusters() []TimeClusterInfo {
	var result []TimeClusterInfo
	for _, cluster := range c.clusters {
		// Only include clusters that meet minimum size
		if len(cluster.anomalies) < c.config.MinClusterSize {
			continue
		}

		sources := make([]string, 0, len(cluster.anomalies))
		for source := range cluster.anomalies {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		result = append(result, TimeClusterInfo{
			ID:           cluster.id,
			Sources:      sources,
			StartTime:    cluster.timeRange.Start,
			EndTime:      cluster.timeRange.End,
			AnomalyCount: len(cluster.anomalies),
		})
	}
	// Sort by size (largest first), then by time
	sort.Slice(result, func(i, j int) bool {
		if result[i].AnomalyCount != result[j].AnomalyCount {
			return result[i].AnomalyCount > result[j].AnomalyCount
		}
		return result[i].StartTime > result[j].StartTime // most recent first
	})
	return result
}

// GetStats returns statistics about the correlator state.
func (c *TimeClusterCorrelator) GetStats() map[string]interface{} {
	totalAnomalies := 0
	maxClusterSize := 0
	for _, cluster := range c.clusters {
		totalAnomalies += len(cluster.anomalies)
		if len(cluster.anomalies) > maxClusterSize {
			maxClusterSize = len(cluster.anomalies)
		}
	}
	return map[string]interface{}{
		"total_clusters":       len(c.clusters),
		"total_anomalies":      totalAnomalies,
		"largest_cluster_size": maxClusterSize,
		"slack_seconds":        c.config.SlackSeconds,
		"window_seconds":       c.config.WindowSeconds,
		"min_cluster_size":     c.config.MinClusterSize,
		"current_data_time":    c.currentDataTime,
	}
}

// ActiveCorrelations returns clusters that meet the minimum size threshold.
// Implements CorrelationState interface.
func (c *TimeClusterCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	var result []observer.ActiveCorrelation

	for _, cluster := range c.clusters {
		if len(cluster.anomalies) < c.config.MinClusterSize {
			continue
		}

		// Collect anomalies and sources
		anomalies := make([]observer.AnomalyOutput, 0, len(cluster.anomalies))
		sources := make([]string, 0, len(cluster.anomalies))
		for source, anomaly := range cluster.anomalies {
			anomalies = append(anomalies, anomaly)
			sources = append(sources, source)
		}
		sort.Strings(sources)

		result = append(result, observer.ActiveCorrelation{
			Pattern:     fmt.Sprintf("time_cluster_%d", cluster.id),
			Title:       fmt.Sprintf("Correlated: %d anomalies in time window", len(cluster.anomalies)),
			Sources:     sources,
			Anomalies:   anomalies,
			FirstSeen:   cluster.timeRange.Start,
			LastUpdated: cluster.timeRange.End,
		})
	}

	// Sort by cluster size (largest first), then by time
	sort.Slice(result, func(i, j int) bool {
		if len(result[i].Anomalies) != len(result[j].Anomalies) {
			return len(result[i].Anomalies) > len(result[j].Anomalies)
		}
		return result[i].FirstSeen < result[j].FirstSeen
	})

	return result
}
