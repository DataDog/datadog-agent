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
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.b"},

		Title:     "Anomaly B",
		Timestamp: 105, // 5 seconds later, within 10s proximity
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Len(t, correlations[0].Members, 2)
}

func TestTimeClusterCorrelator_ProximityWindow(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Anomalies within proximity window should cluster
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.b"},

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
		WindowSeconds:    60,
	})

	// Anomalies outside proximity window should form separate clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.b"},

		Title:     "Anomaly B",
		Timestamp: 150, // 50 seconds later, outside 10s proximity
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
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:     "Anomaly A",
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.b"},

		Title:     "Anomaly B",
		Timestamp: 120, // Far enough to be separate (20s apart)
	})

	// Verify two separate clusters
	assert.Len(t, c.clusters, 2)

	// Add anomaly that bridges both clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.c"},

		Title:     "Anomaly C",
		Timestamp: 110, // Near both clusters
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
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:       "Anomaly A v1",
		Description: "first",
		Timestamp:   100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Title:       "Anomaly A v2",
		Description: "second",
		Timestamp:   105,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	// Single unique series (same Source descriptor)
	assert.Len(t, correlations[0].Members, 1)
}

func TestTimeClusterCorrelator_TaggedVariants(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// Same metric name, different tags = different Source descriptors
	// Both should be separate members in the cluster
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a", Tags: []string{"host:A"}},
		Title:     "Anomaly from host A",
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a", Tags: []string{"host:B"}},
		Title:     "Anomaly from host B",
		Timestamp: 102,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Len(t, correlations[0].Members, 2)
}

func TestTimeClusterCorrelator_Eviction(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    30,
	})

	// Add old anomaly
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.old"},

		Title:     "Old Anomaly",
		Timestamp: 100,
	})

	// Add recent anomaly (advances currentDataTime)
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.new"},

		Title:     "New Anomaly",
		Timestamp: 200, // 100 seconds later, old one should be evicted
	})

	// Flush should evict the old cluster
	c.Advance(200)

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, "metric.new", correlations[0].Members[0].Name)
}

func TestTimeClusterCorrelator_SingletonCluster(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
	})

	// A single anomaly should form a cluster of one
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Timestamp: 100,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 1)
}

func TestTimeClusterCorrelator_MinClusterSize(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    60,
		MinClusterSize:   3,
	})

	// Create a cluster of 2 and a cluster of 3
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.a"},

		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.b"},

		Timestamp: 105,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.c"},

		Timestamp: 200,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.d"},

		Timestamp: 205,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.e"},

		Timestamp: 208,
	})

	// Internally both clusters exist
	assert.Len(t, c.clusters, 2)

	// Only the cluster with 3 anomalies should appear in output
	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)

	clusters := c.GetClusters()
	require.Len(t, clusters, 1)
	assert.Equal(t, 3, clusters[0].AnomalyCount)

	// Once the small cluster grows to meet threshold, it should appear
	c.ProcessAnomaly(observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.f"},

		Timestamp: 103,
	})
	correlations = c.ActiveCorrelations()
	assert.Len(t, correlations, 2)
}

func TestTimeClusterCorrelator_SamplingIntervalWidensProximity(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    120,
	})

	// Two anomalies 15s apart — beyond the 10s proximity window.
	// But both have SamplingIntervalSec=15, so the effective proximity
	// should widen to 15s and they should cluster together.
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.cpu.sys"},
		Timestamp:           100,
		SamplingIntervalSec: 15,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.info.latency_ms"},
		Timestamp:           115,
		SamplingIntervalSec: 15,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1, "slow-sampling anomalies 15s apart should cluster with SamplingIntervalSec=15")
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_SamplingIntervalNoEffectWhenClose(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    120,
	})

	// Two anomalies 5s apart — within the base 10s proximity.
	// SamplingIntervalSec shouldn't change the result.
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "trace.hits"},
		Timestamp:           100,
		SamplingIntervalSec: 10,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "trace.latency"},
		Timestamp:           105,
		SamplingIntervalSec: 10,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_SamplingIntervalMixed(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    120,
	})

	// Fast-sampling anomaly at t=100, slow-sampling at t=120 (20s gap).
	// The slow anomaly's SamplingIntervalSec=20 should widen proximity
	// enough to join the existing cluster.
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "trace.hits"},
		Timestamp:           100,
		SamplingIntervalSec: 10,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.cpu.sys"},
		Timestamp:           120,
		SamplingIntervalSec: 20,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1, "slow-sampling anomaly should join fast-sampling cluster when SamplingIntervalSec covers the gap")
	assert.Len(t, correlations[0].Anomalies, 2)
}

func TestTimeClusterCorrelator_SamplingIntervalTooFar(t *testing.T) {
	c := NewTimeClusterCorrelator(TimeClusterConfig{
		ProximitySeconds: 10,
		WindowSeconds:    120,
	})

	// Two anomalies 30s apart. SamplingIntervalSec=15 widens to 15s
	// but 30s is still too far.
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.cpu.sys"},
		Timestamp:           100,
		SamplingIntervalSec: 15,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.mem.rss"},
		Timestamp:           130,
		SamplingIntervalSec: 15,
	})

	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 2, "30s gap exceeds even widened proximity of 15s — should be separate clusters")
}

func TestTimeClusterCorrelator_DefaultConfig(t *testing.T) {
	config := DefaultTimeClusterConfig()
	assert.Equal(t, int64(10), config.ProximitySeconds)
	assert.Equal(t, int64(120), config.WindowSeconds)
}

func TestTimeClusterCorrelator_Name(t *testing.T) {
	c := NewTimeClusterCorrelator(DefaultTimeClusterConfig())
	assert.Equal(t, "time_cluster_correlator", c.Name())
}
