// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeClusterRCABuilderBuild(t *testing.T) {
	cfg := DefaultRCAConfig()
	cfg.Correlator = "time_cluster"
	cfg.OnsetEpsilonSeconds = 1
	cfg.MaxEdgeLagSeconds = 10

	builder := newTimeClusterRCABuilder(cfg)

	corr := observer.ActiveCorrelation{
		Pattern: "time_cluster_7",
		Title:   "TimeCluster: 3 anomalies",
		Anomalies: []observer.AnomalyOutput{
			{Source: "metric.a:avg", SourceSeriesID: "ns|metric.a:avg|host:a", Timestamp: 100},
			{Source: "metric.a:avg", SourceSeriesID: "ns|metric.a:avg|host:a", Timestamp: 102},
			{Source: "metric.b:avg", SourceSeriesID: "ns|metric.b:avg|host:b", Timestamp: 101},
			{Source: "metric.c:avg", SourceSeriesID: "ns|metric.c:avg|host:c", Timestamp: 106},
		},
		MemberSeriesIDs: []observer.SeriesID{
			"ns|metric.a:avg|host:a",
			"ns|metric.b:avg|host:b",
			"ns|metric.c:avg|host:c",
		},
		FirstSeen:   100,
		LastUpdated: 106,
	}

	graph, err := builder.build(corr)
	require.NoError(t, err)

	assert.Equal(t, "time_cluster_7", graph.ClusterID)
	assert.Equal(t, int64(100), graph.ClusterWindowStart)
	assert.Equal(t, int64(106), graph.ClusterWindowEnd)
	assert.Len(t, graph.Nodes, 3)

	nodeA := findIncidentNode(t, graph.Nodes, "ns|metric.a:avg|host:a")
	assert.Equal(t, int64(100), nodeA.OnsetTime)
	assert.Equal(t, 2, nodeA.PersistenceCount)

	// 3 pairwise edges expected for 3 nodes.
	assert.Len(t, graph.Edges, 3)

	undirectedAB := findIncidentEdge(graph.Edges, "ns|metric.a:avg|host:a", "ns|metric.b:avg|host:b")
	require.NotNil(t, undirectedAB)
	assert.False(t, undirectedAB.Directed)

	directedAC := findIncidentEdge(graph.Edges, "ns|metric.a:avg|host:a", "ns|metric.c:avg|host:c")
	require.NotNil(t, directedAC)
	assert.True(t, directedAC.Directed)
}

func TestTimeClusterRCABuilderSupportsPattern(t *testing.T) {
	builder := newTimeClusterRCABuilder(DefaultRCAConfig())

	assert.True(t, builder.supports(observer.ActiveCorrelation{Pattern: "time_cluster_1"}))
	assert.True(t, builder.supports(observer.ActiveCorrelation{Title: "TimeCluster: 2 anomalies"}))
	assert.False(t, builder.supports(observer.ActiveCorrelation{Pattern: "graphsketch_cluster_1"}))
}

func findIncidentNode(t *testing.T, nodes []IncidentNode, seriesID string) IncidentNode {
	t.Helper()
	for _, node := range nodes {
		if node.SeriesID == seriesID {
			return node
		}
	}
	t.Fatalf("node %s not found", seriesID)
	return IncidentNode{}
}

func findIncidentEdge(edges []IncidentEdge, from, to string) *IncidentEdge {
	for i := range edges {
		if edges[i].From == from && edges[i].To == to {
			return &edges[i]
		}
	}
	return nil
}
