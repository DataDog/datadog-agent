// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

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
		MinClusterSize:   2,
		WindowSeconds:    60,
	})

	// Two anomalies with nearby timestamps should cluster together
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		Timestamp: 105, // 5 seconds later, within 10s proximity
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Contains(t, correlations[0].Sources, "metric.a")
	assert.Contains(t, correlations[0].Sources, "metric.b")
}

func TestTimeClusterCorrelator_ProximityWindow(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   2,
		WindowSeconds:    60,
	})

	// Anomalies within proximity window should cluster
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		Timestamp: 108, // 8 seconds later, within 10s proximity
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_NotNearby(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   2,
		WindowSeconds:    60,
	})

	// Anomalies outside proximity window should NOT cluster
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		Timestamp: 150, // 50 seconds later, outside 10s proximity
	})

	correlations := c.ActiveCorrelations()
	// Neither cluster meets MinClusterSize of 2
	assert.Len(t, correlations, 0)
}

func TestTimeClusterCorrelator_MergeClusters(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   2,
		WindowSeconds:    60,
	})

	// Create two separate clusters
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Title:     "Anomaly B",
		Timestamp: 120, // Far enough to be separate (20s apart)
	})

	// Verify two separate clusters
	assert.Len(t, c.clusters, 2)

	// Add anomaly that bridges both clusters
	// Cluster A is at [100,100], cluster B is at [120,120]
	// An anomaly at 110 is:
	//   - Within 10s of cluster A: 110 >= 100-10=90 AND 110 <= 100+10=110 ✓
	//   - Within 10s of cluster B: 110 >= 120-10=110 AND 110 <= 120+10=130 ✓
	c.Process(observer.AnomalyOutput{
		Source:    "metric.c",
		Title:     "Anomaly C",
		Timestamp: 110, // Near both clusters
	})

	// Should now be merged into one cluster
	assert.Len(t, c.clusters, 1)
	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}

func TestTimeClusterCorrelator_DedupBySource(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   1, // Lower threshold to see single-anomaly clusters
		WindowSeconds:    60,
	})

	// Same source, later anomaly should replace earlier
	c.Process(observer.AnomalyOutput{
		Source:      "metric.a",
		Title:       "Anomaly A v1",
		Description: "first",
		Timestamp:   100,
	})
	c.Process(observer.AnomalyOutput{
		Source:      "metric.a",
		Title:       "Anomaly A v2",
		Description: "second",
		Timestamp:   105, // Later timestamp, should replace
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 1)
	assert.Equal(t, "second", correlations[0].Anomalies[0].Description)
}

func TestTimeClusterCorrelator_Eviction(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   1,
		WindowSeconds:    30,
	})

	// Add old anomaly
	c.Process(observer.AnomalyOutput{
		Source:    "metric.old",
		Title:     "Old Anomaly",
		Timestamp: 100,
	})

	// Add recent anomaly (advances currentDataTime)
	c.Process(observer.AnomalyOutput{
		Source:    "metric.new",
		Title:     "New Anomaly",
		Timestamp: 200, // 100 seconds later, old one should be evicted
	})

	// Flush should evict the old cluster
	c.Flush()

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, "metric.new", correlations[0].Sources[0])
}

func TestTimeClusterCorrelator_MinClusterSize(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		MinClusterSize:   3, // Require 3 anomalies
		WindowSeconds:    60,
	})

	// Add 2 nearby anomalies
	c.Process(observer.AnomalyOutput{
		Source:    "metric.a",
		Timestamp: 100,
	})
	c.Process(observer.AnomalyOutput{
		Source:    "metric.b",
		Timestamp: 105,
	})

	// Should not report - only 2 anomalies, need 3
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0)

	// Add third
	c.Process(observer.AnomalyOutput{
		Source:    "metric.c",
		Timestamp: 108,
	})

	// Now should report
	correlations = c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}

func TestTimeClusterCorrelator_DefaultConfig(t *testing.T) {
	config := DefaultTimeClusterConfig()
	assert.Equal(t, int64(10), config.ProximitySeconds)
	assert.Equal(t, 2, config.MinClusterSize)
	assert.Equal(t, int64(60), config.WindowSeconds)
}

func TestTimeClusterCorrelator_Name(t *testing.T) {
	c := NewTimeClusterCorrelator(DefaultTimeClusterConfig())
	assert.Equal(t, "time_cluster_correlator", c.Name())
}
