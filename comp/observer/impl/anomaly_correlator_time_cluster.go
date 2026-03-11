// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TimeClusterConfig configures the TimeClusterCorrelator.
type TimeClusterConfig struct {
	// ProximitySeconds is the maximum time difference between anomaly timestamps
	// for them to be considered part of the same cluster.
	// Default: 10 seconds.
	ProximitySeconds int64

	// WindowSeconds is how long to keep anomalies before eviction.
	// Default: 60 seconds.
	WindowSeconds int64
}

// DefaultTimeClusterConfig returns a TimeClusterConfig with default values.
func DefaultTimeClusterConfig() TimeClusterConfig {
	return TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	}
}

// timeCluster represents a group of temporally-related anomalies.
type timeCluster struct {
	id           int
	anomalies    []observer.Anomaly
	minTimestamp int64 // earliest anomaly timestamp
	maxTimestamp int64 // latest anomaly timestamp
}

// TimeClusterCorrelator clusters anomalies based on timestamp proximity.
// Anomalies whose timestamps are within ProximitySeconds of each other are grouped together.
type TimeClusterCorrelator struct {
	config          TimeClusterConfig
	clusters        []*timeCluster
	nextClusterID   int
	currentDataTime int64
	mu              sync.RWMutex
}

// NewTimeClusterCorrelator creates a new TimeClusterCorrelator with the given config.
func NewTimeClusterCorrelator(config TimeClusterConfig) *TimeClusterCorrelator {
	if config.ProximitySeconds == 0 {
		config.ProximitySeconds = 10
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 60
	}
	return &TimeClusterCorrelator{
		config:   config,
		clusters: nil,
	}
}

// Name returns the correlator name.
func (c *TimeClusterCorrelator) Name() string {
	return "time_cluster_correlator"
}

// Process adds an anomaly, either to an existing cluster or a new one.
func (c *TimeClusterCorrelator) ProcessAnomaly(anomaly observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update current data time
	if anomaly.Timestamp > c.currentDataTime {
		c.currentDataTime = anomaly.Timestamp
	}

	// Find clusters this anomaly is within proximity of
	var nearby []*timeCluster
	for _, cluster := range c.clusters {
		if c.isNearCluster(anomaly.Timestamp, cluster) {
			nearby = append(nearby, cluster)
		}
	}

	if len(nearby) == 0 {
		// No nearby cluster - create new cluster
		c.nextClusterID++
		newCluster := &timeCluster{
			id:           c.nextClusterID,
			anomalies:    []observer.Anomaly{anomaly},
			minTimestamp: anomaly.Timestamp,
			maxTimestamp: anomaly.Timestamp,
		}
		c.clusters = append(c.clusters, newCluster)
	} else if len(nearby) == 1 {
		// Single nearby cluster - add to it
		cluster := nearby[0]
		c.addToCluster(cluster, anomaly)
	} else {
		// Multiple nearby clusters - merge them and add anomaly
		merged := c.mergeClusters(nearby)
		c.addToCluster(merged, anomaly)
	}
}

// isNearCluster checks if a timestamp is within proximity of any anomaly in the cluster.
func (c *TimeClusterCorrelator) isNearCluster(ts int64, cluster *timeCluster) bool {
	proximity := c.config.ProximitySeconds
	// Check if timestamp is within proximity of the cluster's time range
	return ts >= cluster.minTimestamp-proximity && ts <= cluster.maxTimestamp+proximity
}

// addToCluster adds an anomaly to a cluster, updating timestamps.
func (c *TimeClusterCorrelator) addToCluster(cluster *timeCluster, anomaly observer.Anomaly) {
	cluster.anomalies = append(cluster.anomalies, anomaly)

	if anomaly.Timestamp < cluster.minTimestamp {
		cluster.minTimestamp = anomaly.Timestamp
	}
	if anomaly.Timestamp > cluster.maxTimestamp {
		cluster.maxTimestamp = anomaly.Timestamp
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
		merged.anomalies = append(merged.anomalies, other.anomalies...)
		if other.minTimestamp < merged.minTimestamp {
			merged.minTimestamp = other.minTimestamp
		}
		if other.maxTimestamp > merged.maxTimestamp {
			merged.maxTimestamp = other.maxTimestamp
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
func (c *TimeClusterCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}
	c.evictOldClustersLocked()
}

// Reset clears all internal state for reanalysis.
func (c *TimeClusterCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.clusters = c.clusters[:0]
	c.nextClusterID = 0
	c.currentDataTime = 0
}

// evictOldClustersLocked removes clusters whose latest timestamp is outside the window.
// Caller must hold c.mu.
func (c *TimeClusterCorrelator) evictOldClustersLocked() {
	cutoff := c.currentDataTime - c.config.WindowSeconds
	newClusters := c.clusters[:0]
	for _, cluster := range c.clusters {
		if cluster.maxTimestamp >= cutoff {
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

// GetClusters returns all clusters for visualization.
func (c *TimeClusterCorrelator) GetClusters() []TimeClusterInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []TimeClusterInfo
	for _, cluster := range c.clusters {
		seen := make(map[observer.SeriesID]bool)
		for _, a := range cluster.anomalies {
			seen[a.SourceSeriesID] = true
		}
		sources := make([]string, 0, len(seen))
		for sid := range seen {
			sources = append(sources, string(sid))
		}
		sort.Strings(sources)
		result = append(result, TimeClusterInfo{
			ID:           cluster.id,
			Sources:      sources,
			StartTime:    cluster.minTimestamp,
			EndTime:      cluster.maxTimestamp,
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
	c.mu.RLock()
	defer c.mu.RUnlock()

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
		"proximity_seconds":    c.config.ProximitySeconds,
		"window_seconds":       c.config.WindowSeconds,
		"current_data_time":    c.currentDataTime,
	}
}

// GetExtraData implements ComponentDataProvider.
func (c *TimeClusterCorrelator) GetExtraData() interface{} {
	return c.GetClusters()
}

// ActiveCorrelations returns clusters as active correlation patterns.
func (c *TimeClusterCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []observer.ActiveCorrelation

	for _, cluster := range c.clusters {
		// Collect unique series IDs
		seen := make(map[observer.SeriesID]bool)
		for _, a := range cluster.anomalies {
			seen[a.SourceSeriesID] = true
		}
		memberSeriesIDs := make([]observer.SeriesID, 0, len(seen))
		for sid := range seen {
			memberSeriesIDs = append(memberSeriesIDs, sid)
		}
		sort.Slice(memberSeriesIDs, func(i, j int) bool { return memberSeriesIDs[i] < memberSeriesIDs[j] })
		metricNames := sortedUniqueMetricNames(cluster.anomalies)

		result = append(result, observer.ActiveCorrelation{
			Pattern:         fmt.Sprintf("time_cluster_%d", cluster.id),
			Title:           fmt.Sprintf("TimeCluster: %d anomalies", len(cluster.anomalies)),
			MemberSeriesIDs: memberSeriesIDs,
			MetricNames:     metricNames,
			Anomalies:       cluster.anomalies,
			FirstSeen:       cluster.minTimestamp,
			LastUpdated:     cluster.maxTimestamp,
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
