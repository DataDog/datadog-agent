// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemporalScoringRanksEarliestBroadestNodeFirst(t *testing.T) {
	graph := IncidentGraph{
		ClusterID: "time_cluster_1",
		Nodes: []IncidentNode{
			{SeriesID: "A", MetricName: "metric.a", OnsetTime: 100, PersistenceCount: 2, PeakScore: 2.0},
			{SeriesID: "B", MetricName: "metric.b", OnsetTime: 103, PersistenceCount: 1, PeakScore: 1.2},
			{SeriesID: "C", MetricName: "metric.c", OnsetTime: 106, PersistenceCount: 1, PeakScore: 1.1},
		},
		Edges: []IncidentEdge{
			{From: "A", To: "B", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
			{From: "A", To: "C", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
			{From: "B", To: "C", Directed: true, Weight: 0.5, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MaxRootCandidates = 3

	series := rankSeriesRootCandidates(graph, cfg)
	require.Len(t, series, 3)

	assert.Equal(t, "A", series[0].ID)
	assert.Greater(t, series[0].Score, series[1].Score)
	assert.GreaterOrEqual(t, series[1].Score, series[2].Score)

	metric := rollupMetricRootCandidates(graph, series, 3)
	require.NotEmpty(t, metric)
	assert.Equal(t, "metric.a", metric[0].ID)

	paths := extractEvidencePaths(graph, series, cfg)
	require.NotEmpty(t, paths)
	assert.Contains(t, paths[0].Nodes, "A")
}

func TestSeverityFactorBoostsHighDeviationNodes(t *testing.T) {
	// Two nodes with identical onset, coverage, and persistence.
	// Node B has much higher PeakScore → severity should rank it higher.
	graph := IncidentGraph{
		ClusterID: "severity_test",
		Nodes: []IncidentNode{
			{SeriesID: "ns|metric.a|host:a", MetricName: "metric.a", OnsetTime: 100, PersistenceCount: 1, PeakScore: 1.0},
			{SeriesID: "ns|metric.b|host:b", MetricName: "metric.b", OnsetTime: 100, PersistenceCount: 1, PeakScore: 5.0},
		},
		Edges: []IncidentEdge{},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MaxRootCandidates = 2

	series := rankSeriesRootCandidates(graph, cfg)
	require.Len(t, series, 2)

	// B has higher severity (5.0/5.0=1.0 vs 1.0/5.0=0.2), so B should rank first.
	assert.Equal(t, "ns|metric.b|host:b", series[0].ID)
	assert.Greater(t, series[0].Score, series[1].Score)
}

func TestSpreadPenaltyPenalizesUnderrepresentedNamespace(t *testing.T) {
	// 4 nodes: 3 in namespace "app", 1 in namespace "cleanup".
	// cleanup node has early onset but no downstream coverage (bystander pattern)
	// and should be penalized by spread + lower severity.
	graph := IncidentGraph{
		ClusterID: "spread_test",
		Nodes: []IncidentNode{
			{SeriesID: "cleanup|metric.x|host:z", MetricName: "metric.x", OnsetTime: 99, PersistenceCount: 1, PeakScore: 1.5},
			{SeriesID: "app|metric.a|host:a", MetricName: "metric.a", OnsetTime: 100, PersistenceCount: 2, PeakScore: 4.0},
			{SeriesID: "app|metric.b|host:b", MetricName: "metric.b", OnsetTime: 103, PersistenceCount: 1, PeakScore: 3.0},
			{SeriesID: "app|metric.c|host:c", MetricName: "metric.c", OnsetTime: 105, PersistenceCount: 1, PeakScore: 2.0},
		},
		Edges: []IncidentEdge{
			{From: "app|metric.a|host:a", To: "app|metric.b|host:b", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
			{From: "app|metric.a|host:a", To: "app|metric.c|host:c", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MaxRootCandidates = 4

	series := rankSeriesRootCandidates(graph, cfg)
	require.NotEmpty(t, series)

	// The "app" namespace node (metric.a) should outrank the "cleanup" namespace
	// node: cleanup has no downstream coverage, lower severity, and is penalized
	// by spread (1/4 nodes vs expected 1/2 share).
	assert.Equal(t, "app|metric.a|host:a", series[0].ID,
		"app namespace node should rank higher than underrepresented cleanup bystander")

	// Verify cleanup node actually received a spread penalty.
	scored := scoreTemporalNodes(graph, cfg)
	for _, s := range scored {
		if s.Node.SeriesID == "cleanup|metric.x|host:z" {
			assert.Greater(t, s.SpreadPenalty, 0.0, "cleanup node should have non-zero spread penalty")
		}
		if s.Node.SeriesID == "app|metric.a|host:a" {
			assert.Equal(t, 0.0, s.SpreadPenalty, "app node should have zero spread penalty")
		}
	}
}

func TestSpreadPenaltyNoEffectSingleNamespace(t *testing.T) {
	// All nodes in same namespace → no spread penalty applied.
	graph := IncidentGraph{
		ClusterID: "single_ns_test",
		Nodes: []IncidentNode{
			{SeriesID: "app|metric.a|host:a", MetricName: "metric.a", OnsetTime: 100, PersistenceCount: 2, PeakScore: 3.0},
			{SeriesID: "app|metric.b|host:b", MetricName: "metric.b", OnsetTime: 103, PersistenceCount: 1, PeakScore: 1.0},
		},
		Edges: []IncidentEdge{
			{From: "app|metric.a|host:a", To: "app|metric.b|host:b", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MaxRootCandidates = 2

	scored := scoreTemporalNodes(graph, cfg)
	require.Len(t, scored, 2)

	for _, s := range scored {
		assert.Equal(t, 0.0, s.SpreadPenalty, "spread penalty should be zero in single-namespace cluster")
	}
}

func TestRCAConfidenceFlagsAmbiguityAndDataLimits(t *testing.T) {
	graph := IncidentGraph{
		ClusterID: "time_cluster_2",
		Nodes: []IncidentNode{
			{SeriesID: "A", OnsetTime: 100, PersistenceCount: 1},
			{SeriesID: "B", OnsetTime: 101, PersistenceCount: 1},
		},
		Edges: []IncidentEdge{
			{From: "A", To: "B", Directed: false, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.MinDataNodes = 3
	cfg.AmbiguousRootMargin = 0.2

	series := []RCARootCandidate{
		{ID: "A", Score: 0.55},
		{ID: "B", Score: 0.50},
	}

	confidence := buildRCAConfidence(graph, series, cfg)
	assert.True(t, confidence.DataLimited)
	assert.True(t, confidence.WeakDirectionality)
	assert.True(t, confidence.AmbiguousRoots)
	assert.Less(t, confidence.Score, 0.55)
}

func TestOnsetGapSignificanceFlagsAmbiguity(t *testing.T) {
	// Two candidates with different scores (not ambiguous by score gap)
	// but onset times very close relative to cluster window.
	graph := IncidentGraph{
		ClusterID:          "onset_gap_test",
		ClusterWindowStart: 100,
		ClusterWindowEnd:   200,
		Nodes: []IncidentNode{
			{SeriesID: "A", OnsetTime: 100, PersistenceCount: 1},
			{SeriesID: "B", OnsetTime: 105, PersistenceCount: 1},
		},
		Edges: []IncidentEdge{
			{From: "A", To: "B", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.AmbiguousRootMargin = 0.01 // very tight margin so score gap won't trigger

	// Scores differ enough for score-gap check to pass, but onset gap is small.
	series := []RCARootCandidate{
		{ID: "A", Score: 0.8, OnsetTime: 100},
		{ID: "B", Score: 0.6, OnsetTime: 105},
	}

	// Onset gap = |100-105| / (200-100) = 5/100 = 0.05 < 0.1 → ambiguous
	confidence := buildRCAConfidence(graph, series, cfg)
	assert.True(t, confidence.AmbiguousRoots,
		"onset gap 5%% of window should flag ambiguity")
}

func TestOnsetGapLargeDoesNotFlagAmbiguity(t *testing.T) {
	graph := IncidentGraph{
		ClusterID:          "onset_gap_large",
		ClusterWindowStart: 100,
		ClusterWindowEnd:   200,
		Nodes: []IncidentNode{
			{SeriesID: "A", OnsetTime: 100, PersistenceCount: 2},
			{SeriesID: "B", OnsetTime: 140, PersistenceCount: 1},
			{SeriesID: "C", OnsetTime: 160, PersistenceCount: 1},
		},
		Edges: []IncidentEdge{
			{From: "A", To: "B", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
			{From: "A", To: "C", Directed: true, Weight: 1.0, EdgeType: edgeTypeTimeProximity},
		},
	}

	cfg := DefaultRCAConfig()
	cfg.Enabled = true
	cfg.AmbiguousRootMargin = 0.01 // tight margin

	series := []RCARootCandidate{
		{ID: "A", Score: 0.9, OnsetTime: 100},
		{ID: "B", Score: 0.5, OnsetTime: 140},
	}

	// Onset gap = |100-140| / (200-100) = 40/100 = 0.4 > 0.1 → not ambiguous
	confidence := buildRCAConfidence(graph, series, cfg)
	assert.False(t, confidence.AmbiguousRoots,
		"onset gap 40%% of window should not flag ambiguity")
}
