// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import "time"

// ClusterState represents the lifecycle state of a cluster
type ClusterState int

const (
	Active      ClusterState = iota // Recently received events
	Stabilizing                     // No events for 30s
	Resolved                        // No events for 2min
)

func (s ClusterState) String() string {
	switch s {
	case Active:
		return "active"
	case Stabilizing:
		return "stabilizing"
	case Resolved:
		return "resolved"
	default:
		return "unknown"
	}
}

// AnomalyEvent represents a single detected anomaly
type AnomalyEvent struct {
	Timestamp time.Time
	Metric    string
	Tags      map[string]string
	Severity  float64
	Direction string  // "increase" or "decrease"
	Magnitude float64 // absolute change amount
}

// TagPartition separates tags into constant vs varying
type TagPartition struct {
	ConstantTags map[string]string   // key -> value (same across all events)
	VaryingTags  map[string][]string // key -> distinct values seen
}

// MetricPattern captures the structure of metrics in a cluster
type MetricPattern struct {
	Family   string   // common prefix, e.g., "system.disk"
	Variants []string // differing suffixes, e.g., ["free", "used"]
}

// ClusterConfig controls clustering behavior
type ClusterConfig struct {
	TimeWindow         time.Duration // max time between events in same cluster
	StabilizingTimeout time.Duration // time without events before Stabilizing (default 30s)
	ResolvedTimeout    time.Duration // time without events before Resolved (default 2min)
	ExpireTimeout      time.Duration // time before cluster is removed (default 10min)
}

// DefaultClusterConfig returns sensible defaults
func DefaultClusterConfig() ClusterConfig {
	return ClusterConfig{
		TimeWindow:         30 * time.Second,
		StabilizingTimeout: 30 * time.Second,
		ResolvedTimeout:    2 * time.Minute,
		ExpireTimeout:      10 * time.Minute,
	}
}

// ClusterPattern combines metric and tag patterns for a cluster
type ClusterPattern struct {
	MetricPattern
	TagPartition
}

// AnomalyCluster represents a group of related anomaly events
type AnomalyCluster struct {
	ID        int
	Events    []AnomalyEvent
	Pattern   ClusterPattern
	FirstSeen time.Time
	LastSeen  time.Time
	State     ClusterState // Current lifecycle state
}

// SymmetryType indicates the relationship between metrics
type SymmetryType int

const (
	NoSymmetry   SymmetryType = iota
	Inverse                   // free↑ = used↓ (opposite directions, similar magnitude)
	Proportional              // read↑ ~ write↑ (same direction, correlated magnitude)
)

// SymmetryPattern describes a detected relationship between metrics
type SymmetryPattern struct {
	Type       SymmetryType
	Metrics    [2]string // the two metrics involved
	Confidence float64   // 0-1, how confident we are in the pattern
}

// ClusterSummary is the human-readable output for a cluster
type ClusterSummary struct {
	Headline    string   // e.g., "Disk space shift across 6 devices"
	Details     []string // bullet points of relevant information
	LikelyCause string   // heuristic guess, may be empty
}
