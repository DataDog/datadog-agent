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

func TestTimeClusterCorrelator_BasicClustering(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Two anomalies with nearby timestamps should cluster together
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.b",
		SourceSeriesID: "ns|metric.b|",
		Title:          "Anomaly B",
		Timestamp:      105, // 5 seconds later, within 10s proximity
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Contains(t, correlations[0].MemberSeriesIDs, observer.SeriesID("ns|metric.a|"))
	assert.Contains(t, correlations[0].MemberSeriesIDs, observer.SeriesID("ns|metric.b|"))
}

func TestTimeClusterCorrelator_ProximityWindow(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Anomalies within proximity window should cluster
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.b",
		SourceSeriesID: "ns|metric.b|",
		Title:          "Anomaly B",
		Timestamp:      108, // 8 seconds later, within 10s proximity
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_NotNearby(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Anomalies outside proximity window should form separate clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.b",
		SourceSeriesID: "ns|metric.b|",
		Title:          "Anomaly B",
		Timestamp:      150, // 50 seconds later, outside 10s proximity
	})

	correlations := c.ActiveCorrelations()
	// Each anomaly forms its own cluster
	assert.Len(t, correlations, 2)
	assert.Len(t, correlations[0].Anomalies, 1)
	assert.Len(t, correlations[1].Anomalies, 1)
}

func TestTimeClusterCorrelator_MergeClusters(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Create two separate clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.b",
		SourceSeriesID: "ns|metric.b|",
		Title:          "Anomaly B",
		Timestamp:      120, // Far enough to be separate (20s apart)
	})

	// Verify two separate clusters
	assert.Len(t, c.clusters, 2)

	// Add anomaly that bridges both clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.c",
		SourceSeriesID: "ns|metric.c|",
		Title:          "Anomaly C",
		Timestamp:      110, // Near both clusters
	})

	// Should now be merged into one cluster
	assert.Len(t, c.clusters, 1)
	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}

func TestTimeClusterCorrelator_SameSeriesMultipleAnomalies(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Multiple anomalies from the same series should all be kept
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A v1",
		Description:    "first",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Title:          "Anomaly A v2",
		Description:    "second",
		Timestamp:      105,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	// Single unique series
	assert.Len(t, correlations[0].MemberSeriesIDs, 1)
}

func TestTimeClusterCorrelator_TaggedVariants(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Same metric name, different tags = different SourceSeriesIDs
	// Both should be separate members in the cluster
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|host:A",
		Title:          "Anomaly from host A",
		Timestamp:      100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|host:B",
		Title:          "Anomaly from host B",
		Timestamp:      102,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Contains(t, correlations[0].MemberSeriesIDs, observer.SeriesID("ns|metric.a|host:A"))
	assert.Contains(t, correlations[0].MemberSeriesIDs, observer.SeriesID("ns|metric.a|host:B"))
}

func TestTimeClusterCorrelator_Eviction(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    30,
	})

	// Add old anomaly
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.old",
		SourceSeriesID: "ns|metric.old|",
		Title:          "Old Anomaly",
		Timestamp:      100,
	})

	// Add recent anomaly (advances currentDataTime)
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.new",
		SourceSeriesID: "ns|metric.new|",
		Title:          "New Anomaly",
		Timestamp:      200, // 100 seconds later, old one should be evicted
	})

	// Flush should evict the old cluster
	c.Advance(200)

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, observer.SeriesID("ns|metric.new|"), correlations[0].MemberSeriesIDs[0])
}

func TestTimeClusterCorrelator_SingletonCluster(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// A single anomaly should form a cluster of one
	c.ProcessAnomaly(observer.Anomaly{
		Source:         "metric.a",
		SourceSeriesID: "ns|metric.a|",
		Timestamp:      100,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 1)
}

func TestTimeClusterCorrelator_DefaultConfig(t *testing.T) {
	config := DefaultTimeClusterConfig()
	assert.Equal(t, int64(10), config.ProximitySeconds)
	assert.Equal(t, int64(60), config.WindowSeconds)
}

func TestTimeClusterCorrelator_Name(t *testing.T) {
	c := NewTimeClusterCorrelator(DefaultTimeClusterConfig())
	assert.Equal(t, "time_cluster_correlator", c.Name())
}
