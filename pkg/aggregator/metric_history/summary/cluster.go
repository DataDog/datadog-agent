// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

// ClusterSet manages incremental clustering of anomaly events
type ClusterSet struct {
	config   ClusterConfig
	clusters map[int]*AnomalyCluster
	pending  []AnomalyEvent
	nextID   int
}

// NewClusterSet creates a new cluster set with the given configuration
func NewClusterSet(cfg ClusterConfig) *ClusterSet {
	return &ClusterSet{
		config:   cfg,
		clusters: make(map[int]*AnomalyCluster),
		pending:  []AnomalyEvent{},
		nextID:   1,
	}
}

// Add inserts an anomaly event, either adding to existing cluster or pending
func (cs *ClusterSet) Add(event AnomalyEvent) {
	// 1. Try to find compatible existing cluster
	for _, cluster := range cs.clusters {
		if cs.isCompatibleWithCluster(event, cluster) {
			cs.addToCluster(cluster, event)
			return
		}
	}

	// 2. Try to form new cluster with pending events
	for i, pendingEvent := range cs.pending {
		if cs.compatible(event, pendingEvent) {
			// Create new cluster with both events
			newCluster := cs.createCluster([]AnomalyEvent{pendingEvent, event})
			cs.clusters[newCluster.ID] = newCluster

			// Remove pending event from pending list
			cs.pending = append(cs.pending[:i], cs.pending[i+1:]...)
			return
		}
	}

	// 3. Otherwise add to pending
	cs.pending = append(cs.pending, event)
}

// Clusters returns all current clusters
func (cs *ClusterSet) Clusters() []*AnomalyCluster {
	result := make([]*AnomalyCluster, 0, len(cs.clusters))
	for _, cluster := range cs.clusters {
		result = append(result, cluster)
	}
	return result
}

// Pending returns events that haven't been clustered yet
func (cs *ClusterSet) Pending() []AnomalyEvent {
	return cs.pending
}

// isCompatibleWithCluster checks if an event can be added to an existing cluster
func (cs *ClusterSet) isCompatibleWithCluster(event AnomalyEvent, cluster *AnomalyCluster) bool {
	// Check compatibility with all events in the cluster
	for _, clusterEvent := range cluster.Events {
		if !cs.compatible(event, clusterEvent) {
			return false
		}
	}
	return true
}

// compatible checks if two events are compatible for clustering
func (cs *ClusterSet) compatible(a, b AnomalyEvent) bool {
	// 1. Time proximity
	timeDiff := a.Timestamp.Sub(b.Timestamp)
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff > cs.config.TimeWindow {
		return false
	}

	// 2. Same metric family (at least 2 segments)
	familyA := getMetricFamily(a.Metric)
	familyB := getMetricFamily(b.Metric)
	if familyA != familyB || familyA == "" {
		return false
	}

	// 3. Tag key overlap (or both have no tags)
	if !hasTagKeyOverlap(a.Tags, b.Tags) {
		return false
	}

	return true
}

// getMetricFamily returns first 2 dot-separated segments
// "system.disk.free" -> "system.disk"
// "system.cpu.user.total" -> "system.cpu"
// "foo" -> "" (not enough segments)
func getMetricFamily(metric string) string {
	parts := splitMetric(metric)
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

// hasTagKeyOverlap returns true if both have no tags, or share at least one key
func hasTagKeyOverlap(tagsA, tagsB map[string]string) bool {
	// Both have no tags -> compatible
	if (tagsA == nil || len(tagsA) == 0) && (tagsB == nil || len(tagsB) == 0) {
		return true
	}

	// One has tags, one doesn't -> NOT compatible
	if (tagsA == nil || len(tagsA) == 0) || (tagsB == nil || len(tagsB) == 0) {
		return false
	}

	// Both have tags - check for at least one key overlap
	for keyA := range tagsA {
		if _, exists := tagsB[keyA]; exists {
			return true
		}
	}
	return false
}

// createCluster creates a new cluster from a list of events
func (cs *ClusterSet) createCluster(events []AnomalyEvent) *AnomalyCluster {
	id := cs.nextID
	cs.nextID++

	// Find first and last seen times
	firstSeen := events[0].Timestamp
	lastSeen := events[0].Timestamp
	for _, event := range events[1:] {
		if event.Timestamp.Before(firstSeen) {
			firstSeen = event.Timestamp
		}
		if event.Timestamp.After(lastSeen) {
			lastSeen = event.Timestamp
		}
	}

	// Extract pattern
	metricPattern := ExtractMetricPattern(events)
	tagPartition := PartitionTags(events)

	return &AnomalyCluster{
		ID:     id,
		Events: events,
		Pattern: ClusterPattern{
			MetricPattern: metricPattern,
			TagPartition:  tagPartition,
		},
		FirstSeen: firstSeen,
		LastSeen:  lastSeen,
	}
}

// addToCluster adds an event to an existing cluster and updates the pattern
func (cs *ClusterSet) addToCluster(cluster *AnomalyCluster, event AnomalyEvent) {
	// Append to events
	cluster.Events = append(cluster.Events, event)

	// Update FirstSeen/LastSeen times
	if event.Timestamp.Before(cluster.FirstSeen) {
		cluster.FirstSeen = event.Timestamp
	}
	if event.Timestamp.After(cluster.LastSeen) {
		cluster.LastSeen = event.Timestamp
	}

	// Recompute Pattern
	metricPattern := ExtractMetricPattern(cluster.Events)
	tagPartition := PartitionTags(cluster.Events)
	cluster.Pattern = ClusterPattern{
		MetricPattern: metricPattern,
		TagPartition:  tagPartition,
	}
}
