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

func TestTimeClusterer_Name(t *testing.T) {
	tc := NewTimeClusterer(DefaultTimeClustererConfig())
	assert.Equal(t, "time_clusterer", tc.Name())
}

func TestTimeClusterer_EmptyState(t *testing.T) {
	tc := NewTimeClusterer(DefaultTimeClustererConfig())

	regions := tc.ActiveRegions()
	assert.Empty(t, regions, "should have no regions initially")
}

func TestTimeClusterer_SingleSignalNoCluster(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	tc := NewTimeClusterer(config)

	// Add single signal
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
	})
	tc.Flush()

	// Should not form cluster (min size = 2)
	regions := tc.ActiveRegions()
	assert.Empty(t, regions, "single signal should not form cluster with MinClusterSize=2")
}

func TestTimeClusterer_TwoSignalsSameSourceFormsCluster(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.SlackWindow = 30
	config.DedupBySource = false // Disable dedup to keep all signals
	tc := NewTimeClusterer(config)

	// Add two signals from same source, close in time
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
	})
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1010,
		Value:     110.0,
	})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1, "should form one cluster")

	region := regions[0]
	assert.Equal(t, "metric.cpu", region.Source)
	assert.Equal(t, int64(1000), region.TimeRange.Start)
	assert.Equal(t, int64(1010), region.TimeRange.End)
	assert.Len(t, region.Signals, 2)
}

func TestTimeClusterer_SignalsBeyondSlackWindowFormSeparateClusters(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1 // Allow single-signal clusters for this test
	config.SlackWindow = 10
	tc := NewTimeClusterer(config)

	// Add two signals far apart
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
	})
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1100, // 100 seconds later, beyond slack window
		Value:     110.0,
	})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 2, "should form two separate clusters")
}

func TestTimeClusterer_DifferentSourcesFormSeparateClusters(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1
	tc := NewTimeClusterer(config)

	// Add signals from different sources at same time
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
	})
	tc.Process(observer.Signal{
		Source:    "metric.memory",
		Timestamp: 1000,
		Value:     200.0,
	})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 2, "different sources should form separate clusters")

	sources := []string{regions[0].Source, regions[1].Source}
	assert.Contains(t, sources, "metric.cpu")
	assert.Contains(t, sources, "metric.memory")
}

func TestTimeClusterer_MergesOverlappingClusters(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.SlackWindow = 20
	config.DedupBySource = false // Keep all signals
	tc := NewTimeClusterer(config)

	// Add signals that should merge into one cluster
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1010, Value: 101.0})
	tc.Flush()

	// Add more signals that extend the cluster
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1025, Value: 102.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1030, Value: 103.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1, "should merge into single cluster")

	region := regions[0]
	assert.Equal(t, int64(1000), region.TimeRange.Start)
	assert.Equal(t, int64(1030), region.TimeRange.End)
	assert.Len(t, region.Signals, 4)
}

func TestTimeClusterer_EvictsOldClusters(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1
	config.RetentionWindow = 100
	tc := NewTimeClusterer(config)

	// Add old signal
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1, "should have one cluster")

	// Add new signal far in the future
	tc.Process(observer.Signal{Source: "metric.memory", Timestamp: 1200, Value: 200.0})
	tc.Flush()

	regions = tc.ActiveRegions()
	require.Len(t, regions, 1, "old cluster should be evicted")
	assert.Equal(t, "metric.memory", regions[0].Source)
}

func TestTimeClusterer_DedupBySource(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.DedupBySource = true
	tc := NewTimeClusterer(config)

	// Add multiple signals from same source (simulating duplicate detections)
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1005, Value: 101.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1010, Value: 102.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1)

	// With dedup, should only keep most recent per source
	// Since all signals have same source, should keep only one
	assert.Len(t, regions[0].Signals, 1)
	assert.Equal(t, int64(1010), regions[0].Signals[0].Timestamp)
}

func TestTimeClusterer_NoDedupKeepsAll(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.DedupBySource = false // Disable dedup
	tc := NewTimeClusterer(config)

	// Add multiple signals from same source
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1005, Value: 101.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1010, Value: 102.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1)

	// Without dedup, should keep all signals
	assert.Len(t, regions[0].Signals, 3)
}

func TestTimeClusterer_MinClusterSizeFilter(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 3
	config.SlackWindow = 30
	tc := NewTimeClusterer(config)

	// Add cluster with 2 signals (below minimum)
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1010, Value: 101.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	assert.Empty(t, regions, "cluster with 2 signals should be filtered (min=3)")

	// Add third signal
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1020, Value: 102.0})
	tc.Flush()

	regions = tc.ActiveRegions()
	require.Len(t, regions, 1, "cluster with 3 signals should pass filter")
}

func TestTimeClusterer_SortsByStartTimeDescending(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1
	config.SlackWindow = 5        // Small window so signals don't cluster together
	config.RetentionWindow = 1000 // Large enough to keep all signals
	tc := NewTimeClusterer(config)

	// Add signals in non-chronological order from different sources
	// Use moderate time gaps (beyond slack but within retention)
	tc.Process(observer.Signal{Source: "metric.memory", Timestamp: 100, Value: 200.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 50, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.disk", Timestamp: 150, Value: 300.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 3, "should have 3 separate clusters")

	// Should be sorted by start time, most recent first
	assert.Equal(t, int64(150), regions[0].TimeRange.Start)
	assert.Equal(t, int64(100), regions[1].TimeRange.Start)
	assert.Equal(t, int64(50), regions[2].TimeRange.Start)
}

func TestTimeClusterer_SignalsWithinSlackWindowCluster(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.SlackWindow = 50
	tc := NewTimeClusterer(config)

	// Add signal
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})

	// Add another signal within slack window (before first signal)
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 960, Value: 99.0})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1, "signals within slack window should cluster")

	region := regions[0]
	assert.Equal(t, int64(960), region.TimeRange.Start)
	assert.Equal(t, int64(1000), region.TimeRange.End)
}

func TestTimeClusterer_ComplexScenario(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 2
	config.SlackWindow = 30
	config.RetentionWindow = 200
	tc := NewTimeClusterer(config)

	// Cluster 1: metric.cpu from 1000-1020
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1000, Value: 100.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1010, Value: 101.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1020, Value: 102.0})

	// Cluster 2: metric.memory from 1050-1060
	tc.Process(observer.Signal{Source: "metric.memory", Timestamp: 1050, Value: 200.0})
	tc.Process(observer.Signal{Source: "metric.memory", Timestamp: 1060, Value: 201.0})

	// Cluster 3: metric.cpu again from 1200-1210 (separate from cluster 1)
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1200, Value: 103.0})
	tc.Process(observer.Signal{Source: "metric.cpu", Timestamp: 1210, Value: 104.0})

	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 3, "should have 3 clusters")

	// Verify clusters are correct
	cpuClusters := 0
	memoryClusters := 0
	for _, region := range regions {
		if region.Source == "metric.cpu" {
			cpuClusters++
		} else if region.Source == "metric.memory" {
			memoryClusters++
		}
	}
	assert.Equal(t, 2, cpuClusters, "should have 2 CPU clusters")
	assert.Equal(t, 1, memoryClusters, "should have 1 memory cluster")
}

func TestTimeClusterer_TagsPreserved(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1
	tc := NewTimeClusterer(config)

	tags := []string{"env:prod", "service:api"}
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
		Tags:      tags,
	})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1)
	require.Len(t, regions[0].Signals, 1)

	assert.Equal(t, tags, regions[0].Signals[0].Tags)
}

func TestTimeClusterer_ScorePreserved(t *testing.T) {
	config := DefaultTimeClustererConfig()
	config.MinClusterSize = 1
	tc := NewTimeClusterer(config)

	score := 5.5
	tc.Process(observer.Signal{
		Source:    "metric.cpu",
		Timestamp: 1000,
		Value:     100.0,
		Score:     &score,
	})
	tc.Flush()

	regions := tc.ActiveRegions()
	require.Len(t, regions, 1)
	require.Len(t, regions[0].Signals, 1)

	assert.NotNil(t, regions[0].Signals[0].Score)
	assert.Equal(t, 5.5, *regions[0].Signals[0].Score)
}
