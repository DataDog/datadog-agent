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

func TestAffinityClusterCorrelator_Name(t *testing.T) {
	c := NewAffinityClusterCorrelator(DefaultAffinityClusterConfig())
	assert.Equal(t, "affinity_cluster_correlator", c.Name())
}

func TestAffinityClusterCorrelator_DefaultConfig(t *testing.T) {
	cfg := DefaultAffinityClusterConfig()
	assert.Equal(t, int64(10), cfg.ProximitySeconds)
	assert.Equal(t, int64(120), cfg.WindowSeconds)
	assert.Equal(t, 2, cfg.MinDistinctSeries)
}

// TestAffinityClusterCorrelator_SameHostClusters verifies that two series with the
// same host tag and nearby timestamps are grouped into one reported cluster.
func TestAffinityClusterCorrelator_SameHostClusters(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1", "env:prod"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "mem.rss", Tags: []string{"host:web-1", "env:prod"}},
		Timestamp: 105,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
	assert.Len(t, correlations[0].Members, 2)
	assert.Contains(t, correlations[0].Pattern, "affinity_cluster_")
	assert.Contains(t, correlations[0].Title, "AffinityCluster:")
}

// TestAffinityClusterCorrelator_DifferentHostsNoAffinity verifies that two series
// on different hosts do not share affinity and form separate clusters.
func TestAffinityClusterCorrelator_DifferentHostsNoAffinity(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-2"}},
		Timestamp: 105,
	})

	// Two clusters exist internally but neither has >= 2 distinct sources
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0, "different-host series should not share affinity")
}

// TestAffinityClusterCorrelator_ServiceAffinityMatch verifies service: tag is
// treated as an affinity key.
func TestAffinityClusterCorrelator_ServiceAffinityMatch(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "trace.hits", Tags: []string{"service:checkout", "env:prod"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "trace.latency", Tags: []string{"service:checkout", "env:prod"}},
		Timestamp: 104,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

// TestAffinityClusterCorrelator_KubeNamespaceAffinity verifies kube_namespace: is
// an affinity key.
func TestAffinityClusterCorrelator_KubeNamespaceAffinity(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "pod.cpu", Tags: []string{"kube_namespace:payments"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "pod.mem", Tags: []string{"kube_namespace:payments"}},
		Timestamp: 107,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 2)
}

// TestAffinityClusterCorrelator_NoTagsWildcardSeed verifies that a cluster seeded
// by a tagless series acts as a wildcard and collects any nearby anomaly, but the
// cluster is NOT emitted (affinityTags will remain empty after the seed, so it
// never passes the len(affinityTags) >= 1 gate).
func TestAffinityClusterCorrelator_NoTagsWildcardSeedNotEmitted(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	// First anomaly has no affinity-keyed tags — seeds an empty affinityTags cluster.
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a"},
		Timestamp: 100,
	})
	// Second anomaly also has no tags — joins via wildcard (cluster.affinityTags empty).
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.b"},
		Timestamp: 105,
	})

	// Internally one cluster with 2 distinct sources, but affinityTags is still empty.
	assert.Len(t, c.clusters, 1)
	assert.Len(t, c.clusters[0].distinctSources, 2)

	// Not emitted because affinityTags is empty.
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0)
}

// TestAffinityClusterCorrelator_NoTagsJoinedByTagged verifies that a tagless cluster
// is upgraded to an affinity cluster when a tagged series joins it.
func TestAffinityClusterCorrelator_NoTagsJoinedByTagged(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	// Tagless seed
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a"},
		Timestamp: 100,
	})
	// Tagged series nearby — joins via wildcard, seeds affinityTags
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.b", Tags: []string{"host:web-1"}},
		Timestamp: 105,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1, "cluster should be emitted once tagged series seeds affinityTags")
	assert.Len(t, correlations[0].Anomalies, 2)
}

// TestAffinityClusterCorrelator_MinDistinctSeriesGate verifies that a cluster with
// only one distinct source is not emitted.
func TestAffinityClusterCorrelator_MinDistinctSeriesGate(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	// Same source twice
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 104,
	})

	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0, "single distinct source should not be emitted")
}

// TestAffinityClusterCorrelator_NonAffinityTagsIgnored verifies that tags whose key
// is not in affinityKeys do not create affinity (e.g. "datacenter:us-east").
func TestAffinityClusterCorrelator_NonAffinityTagsIgnored(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a", Tags: []string{"datacenter:us-east"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.b", Tags: []string{"datacenter:us-east"}},
		Timestamp: 103,
	})

	// Even though they share "datacenter:us-east", that key is not in affinityKeys.
	// The first cluster has empty affinityTags, so the second joins via wildcard.
	// But affinityTags stays empty → not emitted.
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0, "non-affinity-key tags should not produce emissions")
}

// TestAffinityClusterCorrelator_TemporalProximityStillGates verifies that
// temporally distant anomalies with the same host do NOT cluster.
func TestAffinityClusterCorrelator_TemporalProximityStillGates(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "mem.rss", Tags: []string{"host:web-1"}},
		Timestamp: 200, // 100 seconds apart, outside 10s proximity
	})

	// Two separate clusters, each with one distinct source → neither emitted
	correlations := c.ActiveCorrelations()
	assert.Len(t, correlations, 0)
	assert.Len(t, c.clusters, 2, "temporally distant anomalies should be in separate clusters")
}

// TestAffinityClusterCorrelator_MergeClusters verifies that a bridging anomaly
// merges two nearby clusters.
func TestAffinityClusterCorrelator_MergeClusters(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	// Two clusters 20 seconds apart
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.a", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.b", Tags: []string{"host:web-1"}},
		Timestamp: 120,
	})

	assert.Len(t, c.clusters, 2)

	// Bridge anomaly joins both clusters
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.c", Tags: []string{"host:web-1"}},
		Timestamp: 110,
	})

	assert.Len(t, c.clusters, 1, "bridge anomaly should merge the two clusters")
	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Len(t, correlations[0].Anomalies, 3)
}

// TestAffinityClusterCorrelator_Eviction verifies that old clusters are dropped
// by Advance.
func TestAffinityClusterCorrelator_Eviction(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     30,
		MinDistinctSeries: 1, // lower threshold so we can observe eviction
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.old", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "metric.new", Tags: []string{"host:web-1"}},
		Timestamp: 200,
	})

	c.Advance(200)

	// Old cluster (maxTimestamp=100) is below cutoff (200-30=170)
	assert.Len(t, c.clusters, 1)
	assert.Equal(t, "metric.new", c.clusters[0].anomalies[0].Source.Name)
}

// TestAffinityClusterCorrelator_Reset verifies that Reset clears all state.
func TestAffinityClusterCorrelator_Reset(t *testing.T) {
	c := NewAffinityClusterCorrelator(DefaultAffinityClusterConfig())

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	assert.Len(t, c.clusters, 1)

	c.Reset()
	assert.Len(t, c.clusters, 0)
	assert.Equal(t, 0, c.nextClusterID)
	assert.Equal(t, int64(0), c.currentDataTime)
}

// TestAffinityClusterCorrelator_SamplingIntervalWidensProximity verifies that
// slow-sampling series can join clusters formed by faster-sampling series.
func TestAffinityClusterCorrelator_SamplingIntervalWidensProximity(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.cpu.sys", Tags: []string{"host:cache-1"}},
		Timestamp:           100,
		SamplingIntervalSec: 15,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:              observer.SeriesDescriptor{Name: "redis.info.latency_ms", Tags: []string{"host:cache-1"}},
		Timestamp:           115,
		SamplingIntervalSec: 15,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1, "slow-sampling same-host anomalies 15s apart should cluster")
	assert.Len(t, correlations[0].Anomalies, 2)
}

// TestAffinityClusterCorrelator_TitleFormat verifies the Title format.
func TestAffinityClusterCorrelator_TitleFormat(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "mem.rss", Tags: []string{"host:web-1"}},
		Timestamp: 102,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, "AffinityCluster: 2 series, 2 anomalies", correlations[0].Title)
}

// TestAffinityClusterCorrelator_PatternFormat verifies the Pattern field.
func TestAffinityClusterCorrelator_PatternFormat(t *testing.T) {
	c := NewAffinityClusterCorrelator(AffinityClusterConfig{
		ProximitySeconds:  10,
		WindowSeconds:     120,
		MinDistinctSeries: 2,
	})

	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "cpu.user", Tags: []string{"host:web-1"}},
		Timestamp: 100,
	})
	c.ProcessAnomaly(observer.Anomaly{
		Source:    observer.SeriesDescriptor{Name: "mem.rss", Tags: []string{"host:web-1"}},
		Timestamp: 102,
	})

	correlations := c.ActiveCorrelations()
	require.Len(t, correlations, 1)
	assert.Equal(t, "affinity_cluster_1", correlations[0].Pattern)
}

// TestAffinityClusterCorrelator_MultipleAffinityKeys verifies that different
// affinity keys (host, service, env, kube_*) all work.
func TestAffinityClusterCorrelator_MultipleAffinityKeys(t *testing.T) {
	keys := []string{
		"host:web-1",
		"service:checkout",
		"env:prod",
		"kube_namespace:payments",
		"kube_deployment:api",
		"container_name:nginx",
	}

	for _, tag := range keys {
		t.Run(tag, func(t *testing.T) {
			c := NewAffinityClusterCorrelator(AffinityClusterConfig{
				ProximitySeconds:  10,
				WindowSeconds:     120,
				MinDistinctSeries: 2,
			})
			c.ProcessAnomaly(observer.Anomaly{
				Source:    observer.SeriesDescriptor{Name: "metric.a", Tags: []string{tag}},
				Timestamp: 100,
			})
			c.ProcessAnomaly(observer.Anomaly{
				Source:    observer.SeriesDescriptor{Name: "metric.b", Tags: []string{tag}},
				Timestamp: 104,
			})
			correlations := c.ActiveCorrelations()
			require.Len(t, correlations, 1, "tag %q should enable affinity clustering", tag)
		})
	}
}

// TestExtractAffinityTags verifies the tag extraction helper.
func TestExtractAffinityTags(t *testing.T) {
	tags := []string{
		"host:web-1",
		"datacenter:us-east", // not an affinity key
		"service:checkout",
		"version:1.2.3", // not an affinity key
		"env:prod",
	}
	got := extractAffinityTags(tags)
	assert.Equal(t, map[string]struct{}{
		"host:web-1":       {},
		"service:checkout": {},
		"env:prod":         {},
	}, got)
}

// TestExtractAffinityTags_NoColonTag verifies tags without colons are ignored.
func TestExtractAffinityTags_NoColonTag(t *testing.T) {
	tags := []string{"notakey", "host:web-1"}
	got := extractAffinityTags(tags)
	assert.Equal(t, map[string]struct{}{"host:web-1": {}}, got)
}

// TestAffinityShares_EmptyClusterIsWildcard verifies that a cluster with no
// affinityTags matches any anomaly.
func TestAffinityShares_EmptyClusterIsWildcard(t *testing.T) {
	cluster := &affinityCluster{affinityTags: map[string]struct{}{}}
	a := observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.x", Tags: []string{"host:web-9"}},
	}
	assert.True(t, affinityShares(a, cluster))
}

// TestAffinityShares_NoMatchingTag verifies that non-matching tags return false.
func TestAffinityShares_NoMatchingTag(t *testing.T) {
	cluster := &affinityCluster{
		affinityTags: map[string]struct{}{"host:web-1": {}},
	}
	a := observer.Anomaly{
		Source: observer.SeriesDescriptor{Name: "metric.x", Tags: []string{"host:web-2"}},
	}
	assert.False(t, affinityShares(a, cluster))
}
