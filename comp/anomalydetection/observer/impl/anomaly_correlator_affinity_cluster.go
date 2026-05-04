// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// affinityKeys is the set of tag key prefixes (with trailing colon) that gate
// cluster affinity. Two series are considered affinity-aligned when they share
// at least one tag whose key prefix is in this set.
var affinityKeys = map[string]struct{}{
	"host:":            {},
	"service:":         {},
	"env:":             {},
	"kube_namespace:":  {},
	"kube_deployment:": {},
	"container_name:":  {},
}

// AffinityClusterConfig configures the AffinityClusterCorrelator.
type AffinityClusterConfig struct {
	// ProximitySeconds is the maximum time difference between anomaly timestamps
	// for them to be considered part of the same cluster.
	// Default: 10 seconds.
	ProximitySeconds int64 `json:"proximity_seconds"`

	// WindowSeconds is how long to keep anomalies before eviction.
	// Default: 120 seconds.
	WindowSeconds int64 `json:"window_seconds"`

	// MinDistinctSeries is the minimum number of distinct source series a cluster
	// must contain to be reported in ActiveCorrelations.
	// Default: 2.
	MinDistinctSeries int `json:"min_distinct_series"`
}

// DefaultAffinityClusterConfig returns an AffinityClusterConfig with default values.
func DefaultAffinityClusterConfig() AffinityClusterConfig {
	return AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	}
}

// readAffinityClusterConfig reads AffinityCluster settings from the agent config.
func readAffinityClusterConfig(reader ConfigReader, prefix string) any {
	cfg := DefaultAffinityClusterConfig()
	if key := prefix + "min_distinct_series"; reader.IsKnown(key) {
		cfg.MinDistinctSeries = reader.GetInt(key)
	}
	return cfg
}

// affinityCluster represents a group of temporally and spatially affined anomalies.
type affinityCluster struct {
	id                  int
	anomalies           []observer.Anomaly
	minTimestamp        int64 // earliest anomaly timestamp
	maxTimestamp        int64 // latest anomaly timestamp
	maxSamplingInterval int64 // max SamplingIntervalSec across all anomalies in cluster
	distinctSources     map[string]struct{}
	affinityTags        map[string]struct{}
}

// AffinityClusterCorrelator clusters anomalies by timestamp proximity and tag
// affinity. It only reports clusters that contain at least MinDistinctSeries
// distinct source series sharing at least one affinity-keyed tag (host, service,
// env, kube_namespace, kube_deployment, container_name).
//
// Motivation: single-series blips are weak evidence of a real incident.
// Co-occurring blips on infra-affined neighbors are strong evidence.
type AffinityClusterCorrelator struct {
	config          AffinityClusterConfig
	clusters        []*affinityCluster
	nextClusterID   int
	currentDataTime int64
	mu              sync.RWMutex
}

// NewAffinityClusterCorrelator creates a new AffinityClusterCorrelator with the given config.
func NewAffinityClusterCorrelator(config AffinityClusterConfig) *AffinityClusterCorrelator {
	if config.ProximitySeconds == 0 {
		config.ProximitySeconds = 10
	}
	if config.WindowSeconds == 0 {
		config.WindowSeconds = 120
	}
	if config.MinDistinctSeries == 0 {
		config.MinDistinctSeries = 2
	}
	return &AffinityClusterCorrelator{
		config:   config,
		clusters: nil,
	}
}

// Name returns the correlator name.
func (c *AffinityClusterCorrelator) Name() string {
	return "affinity_cluster_correlator"
}

// ProcessAnomaly adds an anomaly, grouping it with clusters that are both
// temporally nearby and share at least one affinity tag.
func (c *AffinityClusterCorrelator) ProcessAnomaly(anomaly observer.Anomaly) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if anomaly.Timestamp > c.currentDataTime {
		c.currentDataTime = anomaly.Timestamp
	}

	var nearby []*affinityCluster
	for _, cluster := range c.clusters {
		if c.isNearCluster(anomaly.Timestamp, anomaly.SamplingIntervalSec, cluster) &&
			affinityShares(anomaly, cluster) {
			nearby = append(nearby, cluster)
		}
	}

	if len(nearby) == 0 {
		c.nextClusterID++
		newCluster := &affinityCluster{
			id:                  c.nextClusterID,
			anomalies:           []observer.Anomaly{anomaly},
			minTimestamp:        anomaly.Timestamp,
			maxTimestamp:        anomaly.Timestamp,
			maxSamplingInterval: anomaly.SamplingIntervalSec,
			distinctSources:     map[string]struct{}{anomaly.Source.Key(): {}},
			affinityTags:        extractAffinityTags(anomaly.Source.Tags),
		}
		c.clusters = append(c.clusters, newCluster)
	} else if len(nearby) == 1 {
		c.addToCluster(nearby[0], anomaly)
	} else {
		merged := c.mergeClusters(nearby)
		c.addToCluster(merged, anomaly)
	}
}

// isNearCluster checks if a timestamp is within proximity of the cluster's time range.
// Proximity is widened by the max SamplingIntervalSec across the cluster and the
// incoming anomaly, so slow-sampling series (e.g. 15s redis checks) can join
// clusters formed by faster-sampling series.
func (c *AffinityClusterCorrelator) isNearCluster(ts int64, incomingInterval int64, cluster *affinityCluster) bool {
	proximity := c.config.ProximitySeconds
	if cluster.maxSamplingInterval > proximity {
		proximity = cluster.maxSamplingInterval
	}
	if incomingInterval > proximity {
		proximity = incomingInterval
	}
	// Cap proximity at half the window to prevent pathological sampling intervals
	// (e.g. 3600s) from clustering all anomalies together.
	maxProximity := c.config.WindowSeconds / 2
	if maxProximity > 0 && proximity > maxProximity {
		proximity = maxProximity
	}
	return ts >= cluster.minTimestamp-proximity && ts <= cluster.maxTimestamp+proximity
}

// affinityShares returns true if the anomaly's source shares at least one
// affinity-keyed tag with the cluster's affinityTags set. If the cluster has
// no affinityTags yet (first anomaly seeds an empty set), it matches any
// anomaly (wildcard/seed).
func affinityShares(a observer.Anomaly, cluster *affinityCluster) bool {
	if len(cluster.affinityTags) == 0 {
		return true
	}
	for _, tag := range a.Source.Tags {
		idx := strings.IndexByte(tag, ':')
		if idx < 0 {
			continue
		}
		keyWithColon := tag[:idx+1]
		if _, ok := affinityKeys[keyWithColon]; !ok {
			continue
		}
		if _, ok := cluster.affinityTags[tag]; ok {
			return true
		}
	}
	return false
}

// extractAffinityTags returns the subset of tags whose key prefix is in affinityKeys.
func extractAffinityTags(tags []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, tag := range tags {
		idx := strings.IndexByte(tag, ':')
		if idx < 0 {
			continue
		}
		keyWithColon := tag[:idx+1]
		if _, ok := affinityKeys[keyWithColon]; ok {
			result[tag] = struct{}{}
		}
	}
	return result
}

// addToCluster appends an anomaly to a cluster, updating timestamps, the
// distinct-source set, and the affinity-tag set.
func (c *AffinityClusterCorrelator) addToCluster(cluster *affinityCluster, anomaly observer.Anomaly) {
	cluster.anomalies = append(cluster.anomalies, anomaly)
	if anomaly.Timestamp < cluster.minTimestamp {
		cluster.minTimestamp = anomaly.Timestamp
	}
	if anomaly.Timestamp > cluster.maxTimestamp {
		cluster.maxTimestamp = anomaly.Timestamp
	}
	if anomaly.SamplingIntervalSec > cluster.maxSamplingInterval {
		cluster.maxSamplingInterval = anomaly.SamplingIntervalSec
	}
	cluster.distinctSources[anomaly.Source.Key()] = struct{}{}
	for tag := range extractAffinityTags(anomaly.Source.Tags) {
		cluster.affinityTags[tag] = struct{}{}
	}
}

// mergeClusters merges multiple clusters into the first, removing the rest.
func (c *AffinityClusterCorrelator) mergeClusters(clusters []*affinityCluster) *affinityCluster {
	if len(clusters) == 0 {
		return nil
	}
	merged := clusters[0]
	for _, other := range clusters[1:] {
		merged.anomalies = append(merged.anomalies, other.anomalies...)
		if other.minTimestamp < merged.minTimestamp {
			merged.minTimestamp = other.minTimestamp
		}
		if other.maxTimestamp > merged.maxTimestamp {
			merged.maxTimestamp = other.maxTimestamp
		}
		if other.maxSamplingInterval > merged.maxSamplingInterval {
			merged.maxSamplingInterval = other.maxSamplingInterval
		}
		for k := range other.distinctSources {
			merged.distinctSources[k] = struct{}{}
		}
		for k := range other.affinityTags {
			merged.affinityTags[k] = struct{}{}
		}
	}
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

// Advance evicts clusters whose latest timestamp is outside the window.
func (c *AffinityClusterCorrelator) Advance(dataTime int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if dataTime > c.currentDataTime {
		c.currentDataTime = dataTime
	}
	c.evictOldClustersLocked()
}

// Reset clears all internal state for reanalysis.
func (c *AffinityClusterCorrelator) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clusters = c.clusters[:0]
	c.nextClusterID = 0
	c.currentDataTime = 0
}

// evictOldClustersLocked removes clusters whose latest timestamp is outside the window.
// Caller must hold c.mu.
func (c *AffinityClusterCorrelator) evictOldClustersLocked() {
	cutoff := c.currentDataTime - c.config.WindowSeconds
	newClusters := c.clusters[:0]
	for _, cluster := range c.clusters {
		if cluster.maxTimestamp >= cutoff {
			newClusters = append(newClusters, cluster)
		}
	}
	c.clusters = newClusters
}

// ActiveCorrelations returns clusters that meet the multi-series affinity gate:
// at least MinDistinctSeries distinct source series and at least one shared
// affinity tag.
func (c *AffinityClusterCorrelator) ActiveCorrelations() []observer.ActiveCorrelation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var result []observer.ActiveCorrelation
	for _, cluster := range c.clusters {
		if len(cluster.distinctSources) < c.config.MinDistinctSeries {
			continue
		}
		if len(cluster.affinityTags) < 1 {
			continue
		}
		result = append(result, observer.ActiveCorrelation{
			Pattern:     fmt.Sprintf("affinity_cluster_%d", cluster.id),
			Title:       fmt.Sprintf("AffinityCluster: %d series, %d anomalies", len(cluster.distinctSources), len(cluster.anomalies)),
			Members:     sortedUniqueMembers(cluster.anomalies),
			Anomalies:   cluster.anomalies,
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
