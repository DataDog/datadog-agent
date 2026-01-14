// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sort"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// TimeClustererConfig configures the TimeClusterer processor.
type TimeClustererConfig struct {
	// SlackWindow is the time window (in seconds) for clustering signals.
	// Signals within this window are considered part of the same cluster.
	// Default: 30 seconds
	SlackWindow int64

	// MinClusterSize is the minimum number of signals required to form a cluster.
	// Clusters with fewer signals are discarded. Default: 2
	MinClusterSize int

	// RetentionWindow is how long (in seconds) to keep clusters after their last update.
	// Default: 300 seconds (5 minutes)
	RetentionWindow int64

	// DedupBySource, if true, only keeps the most recent signal per source within a cluster.
	// Default: true
	DedupBySource bool
}

// DefaultTimeClustererConfig returns a TimeClustererConfig with sensible defaults.
func DefaultTimeClustererConfig() TimeClustererConfig {
	return TimeClustererConfig{
		SlackWindow:     30,
		MinClusterSize:  2,
		RetentionWindow: 300,
		DedupBySource:   true,
	}
}

// TimeClusterer clusters point-based signals into time-based regions.
// It implements both SignalProcessor (to consume signals) and ClusterState (to expose regions).
//
// Algorithm:
//  1. Receive point signals via Process()
//  2. Group signals by source and time proximity
//  3. Merge overlapping clusters
//  4. Expose active clusters as SignalRegions via ActiveRegions()
//
// This is useful for:
//   - Converting point anomalies (from LightESD, CUSUM) into regions
//   - Identifying correlated anomalies across time
//   - Reducing noise by requiring minimum cluster size
type TimeClusterer struct {
	config TimeClustererConfig

	// clusters maps source to list of active clusters for that source
	clusters map[string][]*signalCluster

	// currentTime tracks the most recent signal timestamp seen
	currentTime int64
}

// signalCluster represents a group of signals clustered by time proximity.
type signalCluster struct {
	source    string
	signals   []observer.Signal
	startTime int64 // earliest signal timestamp
	endTime   int64 // latest signal timestamp
}

// NewTimeClusterer creates a new TimeClusterer processor.
func NewTimeClusterer(config TimeClustererConfig) *TimeClusterer {
	return &TimeClusterer{
		config:   config,
		clusters: make(map[string][]*signalCluster),
	}
}

// Name returns the processor name for debugging.
func (tc *TimeClusterer) Name() string {
	return "time_clusterer"
}

// Process adds a signal to the clustering engine.
// Signals are grouped by source and time proximity.
func (tc *TimeClusterer) Process(signal observer.Signal) {
	// Update current time
	if signal.Timestamp > tc.currentTime {
		tc.currentTime = signal.Timestamp
	}

	// Find or create cluster for this signal's source
	sourceClusters := tc.clusters[signal.Source]

	// Try to add to an existing cluster (within slack window)
	added := false
	for _, cluster := range sourceClusters {
		// Check if signal is within slack window of cluster
		if signal.Timestamp >= cluster.startTime-tc.config.SlackWindow &&
			signal.Timestamp <= cluster.endTime+tc.config.SlackWindow {
			// Add to this cluster
			cluster.signals = append(cluster.signals, signal)

			// Update cluster time bounds
			if signal.Timestamp < cluster.startTime {
				cluster.startTime = signal.Timestamp
			}
			if signal.Timestamp > cluster.endTime {
				cluster.endTime = signal.Timestamp
			}

			added = true
			break
		}
	}

	// If not added to existing cluster, create new one
	if !added {
		newCluster := &signalCluster{
			source:    signal.Source,
			signals:   []observer.Signal{signal},
			startTime: signal.Timestamp,
			endTime:   signal.Timestamp,
		}
		tc.clusters[signal.Source] = append(tc.clusters[signal.Source], newCluster)
	}
}

// Flush updates internal state by merging overlapping clusters and evicting old ones.
// This is called periodically to maintain the cluster state.
func (tc *TimeClusterer) Flush() {
	// Merge overlapping clusters for each source
	for source := range tc.clusters {
		tc.clusters[source] = tc.mergeClusters(tc.clusters[source])
	}

	// Evict old clusters
	tc.evictOldClusters()
}

// mergeClusters merges overlapping clusters for a single source.
// Two clusters are merged if their time ranges overlap or are within slack window.
func (tc *TimeClusterer) mergeClusters(clusters []*signalCluster) []*signalCluster {
	if len(clusters) <= 1 {
		return clusters
	}

	// Sort clusters by start time
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].startTime < clusters[j].startTime
	})

	// Merge overlapping clusters
	merged := []*signalCluster{clusters[0]}

	for i := 1; i < len(clusters); i++ {
		current := clusters[i]
		lastMerged := merged[len(merged)-1]

		// Check if current cluster overlaps with last merged cluster
		if current.startTime <= lastMerged.endTime+tc.config.SlackWindow {
			// Merge current into lastMerged
			lastMerged.signals = append(lastMerged.signals, current.signals...)
			if current.endTime > lastMerged.endTime {
				lastMerged.endTime = current.endTime
			}
			if current.startTime < lastMerged.startTime {
				lastMerged.startTime = current.startTime
			}
		} else {
			// No overlap, add as new cluster
			merged = append(merged, current)
		}
	}

	return merged
}

// evictOldClusters removes clusters that are too old (beyond retention window).
func (tc *TimeClusterer) evictOldClusters() {
	cutoffTime := tc.currentTime - tc.config.RetentionWindow

	for source, clusters := range tc.clusters {
		// Filter out old clusters
		active := make([]*signalCluster, 0, len(clusters))
		for _, cluster := range clusters {
			if cluster.endTime >= cutoffTime {
				active = append(active, cluster)
			}
		}

		if len(active) == 0 {
			delete(tc.clusters, source)
		} else {
			tc.clusters[source] = active
		}
	}
}

// ActiveRegions returns currently active signal regions (implements ClusterState).
// Only returns clusters that meet the minimum size requirement.
func (tc *TimeClusterer) ActiveRegions() []observer.SignalRegion {
	var regions []observer.SignalRegion

	for source, clusters := range tc.clusters {
		for _, cluster := range clusters {
			// Apply minimum cluster size filter
			if len(cluster.signals) < tc.config.MinClusterSize {
				continue
			}

			// Deduplicate signals by source if configured
			signals := cluster.signals
			if tc.config.DedupBySource {
				signals = tc.dedupSignals(signals)
			}

			// Create SignalRegion
			region := observer.SignalRegion{
				Source: source,
				TimeRange: observer.TimeRange{
					Start: cluster.startTime,
					End:   cluster.endTime,
				},
				Signals: signals,
			}
			regions = append(regions, region)
		}
	}

	// Sort by start time (most recent first)
	sort.Slice(regions, func(i, j int) bool {
		return regions[i].TimeRange.Start > regions[j].TimeRange.Start
	})

	return regions
}

// dedupSignals removes duplicate signals, keeping only the most recent per source.
func (tc *TimeClusterer) dedupSignals(signals []observer.Signal) []observer.Signal {
	// Map from source to most recent signal
	bySource := make(map[string]observer.Signal)

	for _, sig := range signals {
		existing, exists := bySource[sig.Source]
		if !exists || sig.Timestamp > existing.Timestamp {
			bySource[sig.Source] = sig
		}
	}

	// Convert map back to slice
	result := make([]observer.Signal, 0, len(bySource))
	for _, sig := range bySource {
		result = append(result, sig)
	}

	// Sort by timestamp
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp < result[j].Timestamp
	})

	return result
}
