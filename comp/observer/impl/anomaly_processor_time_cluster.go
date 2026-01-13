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
	// ProximitySeconds is the maximum time difference between anomaly timestamps
	// for them to be considered part of the same cluster.
	// Default: 10 seconds.
	ProximitySeconds int64

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
		ProximitySeconds: 10,
		MinClusterSize:   2,
		WindowSeconds:    60,
	}
}

// timeCluster represents a group of temporally-related anomalies.
type timeCluster struct {
	id           int
	anomalies    map[string]observer.AnomalyOutput // keyed by Source for dedup
	minTimestamp int64                             // earliest anomaly timestamp
	maxTimestamp int64                             // latest anomaly timestamp
}

// TimeClusterCorrelator clusters anomalies based on timestamp proximity.
// Anomalies whose timestamps are within ProximitySeconds of each other are grouped together.
type TimeClusterCorrelator struct {
	config          TimeClusterConfig
	clusters        []*timeCluster
	nextClusterID   int
	currentDataTime int64
}

// NewTimeClusterCorrelator creates a new TimeClusterCorrelator with the given config.
func NewTimeClusterCorrelator(config TimeClusterConfig) *TimeClusterCorrelator {
	if config.ProximitySeconds == 0 {
		config.ProximitySeconds = 10
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
			anomalies:    map[string]observer.AnomalyOutput{anomaly.Source: anomaly},
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

// addToCluster adds an anomaly to a cluster, updating timestamps and deduping by source.
func (c *TimeClusterCorrelator) addToCluster(cluster *timeCluster, anomaly observer.AnomalyOutput) {
	// Dedup by source - keep the one with later timestamp (more recent)
	if existing, ok := cluster.anomalies[anomaly.Source]; ok {
		if anomaly.Timestamp > existing.Timestamp {
			cluster.anomalies[anomaly.Source] = anomaly
		}
	} else {
		cluster.anomalies[anomaly.Source] = anomaly
	}

	// Expand cluster timestamp range
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
		for source, anomaly := range other.anomalies {
			if existing, ok := merged.anomalies[source]; ok {
				if anomaly.Timestamp > existing.Timestamp {
					merged.anomalies[source] = anomaly
				}
			} else {
				merged.anomalies[source] = anomaly
			}
		}
		// Expand timestamp range
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
func (c *TimeClusterCorrelator) Flush() []observer.ReportOutput {
	c.evictOldClusters()
	return nil
}

// evictOldClusters removes clusters whose latest timestamp is outside the window.
func (c *TimeClusterCorrelator) evictOldClusters() {
	cutoff := c.currentDataTime - c.config.WindowSeconds
	newClusters := c.clusters[:0]
	for _, cluster := range c.clusters {
		if cluster.maxTimestamp >= cutoff {
			newClusters = append(newClusters, cluster)
		}
	}
	c.clusters = newClusters
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
			Signals:     sources,
			Anomalies:   anomalies,
			FirstSeen:   cluster.minTimestamp,
			LastUpdated: cluster.maxTimestamp,
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
