// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

// IncidentGraph is the internal RCA model built from an active correlation.
type IncidentGraph struct {
	ClusterID          string         `json:"cluster_id"`
	ClusterWindowStart int64          `json:"cluster_window_start"`
	ClusterWindowEnd   int64          `json:"cluster_window_end"`
	Nodes              []IncidentNode `json:"nodes"`
	Edges              []IncidentEdge `json:"edges"`
}

// IncidentNode represents one anomalous series in an incident graph.
type IncidentNode struct {
	SeriesID         string   `json:"series_id"`
	MetricName       string   `json:"metric_name"`
	OnsetTime        int64    `json:"onset_time"`
	PersistenceCount int      `json:"persistence_count"`
	PeakScore        float64  `json:"peak_score"`
	Tags             []string `json:"tags,omitempty"`
}

// IncidentEdge captures pairwise support between incident nodes.
type IncidentEdge struct {
	From       string  `json:"from"`
	To         string  `json:"to"`
	EdgeType   string  `json:"edge_type"`
	Weight     float64 `json:"weight"`
	LagSeconds *int64  `json:"lag_seconds,omitempty"`
	Directed   bool    `json:"directed"`
}

const edgeTypeTimeProximity = "time_proximity"

// RCAResult is the observer-facing RCA output attached to correlations.
type RCAResult struct {
	CorrelationPattern   string             `json:"correlation_pattern"`
	RootCandidatesSeries []RCARootCandidate `json:"root_candidates_series"`
	RootCandidatesMetric []RCARootCandidate `json:"root_candidates_metric"`
	EvidencePaths        []RCAEvidencePath  `json:"evidence_paths"`
	Confidence           RCAConfidence      `json:"confidence"`
	Summary              string             `json:"summary"`
}

// RCARootCandidate is a ranked root hypothesis at series or metric level.
type RCARootCandidate struct {
	ID        string   `json:"id"`
	Score     float64  `json:"score"`
	OnsetTime int64    `json:"onset_time"`
	Why       []string `json:"why,omitempty"`
}

// RCAEvidencePath provides one concrete path-like explanation.
type RCAEvidencePath struct {
	Nodes []string `json:"nodes"`
	Score float64  `json:"score"`
	Why   string   `json:"why,omitempty"`
}

// RCAConfidence summarizes uncertainty in RCA ranking.
type RCAConfidence struct {
	Score              float64 `json:"score"`
	DataLimited        bool    `json:"data_limited"`
	WeakDirectionality bool    `json:"weak_directionality"`
	AmbiguousRoots     bool    `json:"ambiguous_roots"`
}
