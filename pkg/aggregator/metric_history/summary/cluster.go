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
	// TODO: implement
}

// Clusters returns all current clusters
func (cs *ClusterSet) Clusters() []*AnomalyCluster {
	// TODO: implement
	return nil
}

// Pending returns events that haven't been clustered yet
func (cs *ClusterSet) Pending() []AnomalyEvent {
	return cs.pending
}
