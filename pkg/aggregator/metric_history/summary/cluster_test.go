// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package summary

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestClusterSet_TimeAdjacent tests that two events with the same metric
// and close in time (5 seconds apart) cluster together.
func TestClusterSet_TimeAdjacent(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/data",
		},
		Severity:  0.85,
		Direction: "decrease",
		Magnitude: 2000000000,
	}

	cs.Add(event1)
	cs.Add(event2)

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Events 5s apart with same metric should cluster together")
	assert.Len(t, clusters[0].Events, 2, "Cluster should contain both events")
	assert.Empty(t, cs.Pending(), "No events should be pending")

	// Verify time boundaries
	assert.Equal(t, event1.Timestamp, clusters[0].FirstSeen, "FirstSeen should be first event time")
	assert.Equal(t, event2.Timestamp, clusters[0].LastSeen, "LastSeen should be second event time")
}

// TestClusterSet_SameFamily tests that events from the same metric family
// (e.g., system.disk.free and system.disk.used) cluster together when close in time.
func TestClusterSet_SameFamily(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	}

	cs.Add(event1)
	cs.Add(event2)

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Events from same metric family (system.disk) should cluster together")
	assert.Len(t, clusters[0].Events, 2, "Cluster should contain both events")

	// Verify metric pattern
	assert.Equal(t, "system.disk", clusters[0].Pattern.Family, "Family should be system.disk")
	assert.ElementsMatch(t, []string{"free", "used"}, clusters[0].Pattern.Variants, "Should have both variants")
}

// TestClusterSet_TimeDistant tests that events with the same metric but far apart
// in time (5 minutes > 30s window) don't cluster together.
func TestClusterSet_TimeDistant(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Minute), // 5 minutes apart
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "decrease",
		Magnitude: 2000000000,
	}

	cs.Add(event1)
	cs.Add(event2)

	// Events too far apart should either form separate clusters or one pending
	clusters := cs.Clusters()
	totalEvents := 0
	for _, c := range clusters {
		totalEvents += len(c.Events)
	}
	totalEvents += len(cs.Pending())

	assert.Equal(t, 2, totalEvents, "Both events should be tracked but not in same cluster")

	// If both are clustered, they should be in separate clusters
	if len(clusters) == 2 {
		assert.Len(t, clusters[0].Events, 1, "Each cluster should have one event")
		assert.Len(t, clusters[1].Events, 1, "Each cluster should have one event")
	}
}

// TestClusterSet_DifferentFamilies tests that events from different metric families
// (e.g., system.disk and system.cpu) don't cluster together even at the same time.
func TestClusterSet_DifferentFamilies(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	}
	event2 := AnomalyEvent{
		Timestamp: now, // Same time
		Metric:    "system.cpu.user",
		Tags: map[string]string{
			"cpu": "cpu0",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 50.0,
	}

	cs.Add(event1)
	cs.Add(event2)

	// Different families should not cluster together
	clusters := cs.Clusters()
	totalEvents := 0
	for _, c := range clusters {
		totalEvents += len(c.Events)
	}
	totalEvents += len(cs.Pending())

	assert.Equal(t, 2, totalEvents, "Both events should be tracked")

	// If both are in clusters, they should be separate
	if len(clusters) >= 2 {
		// Find the disk and cpu clusters
		var diskCluster, cpuCluster *AnomalyCluster
		for _, c := range clusters {
			if c.Pattern.Family == "system.disk" {
				diskCluster = c
			} else if c.Pattern.Family == "system.cpu" {
				cpuCluster = c
			}
		}

		if diskCluster != nil {
			assert.Len(t, diskCluster.Events, 1, "Disk cluster should have one event")
		}
		if cpuCluster != nil {
			assert.Len(t, cpuCluster.Events, 1, "CPU cluster should have one event")
		}
	}
}

// TestClusterSet_TagCompatibility tests that events with overlapping tag keys
// cluster together, but events with disjoint tag keys don't.
func TestClusterSet_TagCompatibility(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	// Events with overlapping tag key "device"
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
			"host":   "web-01",
		},
		Severity:  0.8,
		Direction: "decrease",
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/data",
			"region": "us-east-1", // Different key but has "device" in common
		},
		Severity:  0.85,
		Direction: "decrease",
	}

	// Event with completely different tag keys
	event3 := AnomalyEvent{
		Timestamp: now.Add(10 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"container_id": "abc123", // No overlap with device/host/region
		},
		Severity:  0.75,
		Direction: "decrease",
	}

	cs.Add(event1)
	cs.Add(event2)
	cs.Add(event3)

	clusters := cs.Clusters()

	// event1 and event2 should cluster (share "device" tag key)
	// event3 might be separate or pending (no overlapping tag keys)
	assert.GreaterOrEqual(t, len(clusters), 1, "Should have at least one cluster")

	// Find cluster with event1 and event2
	foundClusterWithOverlap := false
	for _, c := range clusters {
		if len(c.Events) >= 2 {
			// Check if this cluster has events with overlapping tags
			hasDevice := false
			for _, e := range c.Events {
				if _, ok := e.Tags["device"]; ok {
					hasDevice = true
					break
				}
			}
			if hasDevice {
				foundClusterWithOverlap = true
				// Verify device is in varying tags (different values)
				assert.Contains(t, c.Pattern.VaryingTags, "device", "device should be varying")
			}
		}
	}

	assert.True(t, foundClusterWithOverlap, "Should find cluster with overlapping tag keys")
}

// TestClusterSet_PatternUpdates tests that as events are added to a cluster,
// the Pattern.TagPartition updates correctly to show varying tags.
func TestClusterSet_PatternUpdates(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	// Add 3 events with same metric and family, varying devices, constant region
	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
			"region": "us-east-1",
		},
		Severity:  0.8,
		Direction: "decrease",
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/data",
			"region": "us-east-1",
		},
		Severity:  0.85,
		Direction: "decrease",
	}
	event3 := AnomalyEvent{
		Timestamp: now.Add(10 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/var",
			"region": "us-east-1",
		},
		Severity:  0.75,
		Direction: "decrease",
	}

	cs.Add(event1)
	cs.Add(event2)
	cs.Add(event3)

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "All events should cluster together")

	cluster := clusters[0]
	assert.Len(t, cluster.Events, 3, "Cluster should contain all 3 events")

	// Verify pattern shows varying device and constant region
	assert.Contains(t, cluster.Pattern.VaryingTags, "device", "device should be varying")
	assert.ElementsMatch(t, []string{"/", "/data", "/var"}, cluster.Pattern.VaryingTags["device"],
		"Should capture all device values")

	assert.Contains(t, cluster.Pattern.ConstantTags, "region", "region should be constant")
	assert.Equal(t, "us-east-1", cluster.Pattern.ConstantTags["region"], "region value should be us-east-1")
}

// TestClusterSet_RealDiskScenario tests a realistic scenario with 6 disk events
// (3 free, 3 used) from different devices, all within 15 seconds.
// Expected: All 6 in one cluster with Family="system.disk", Variants=["free","used"],
// VaryingTags includes device.
func TestClusterSet_RealDiskScenario(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	events := []AnomalyEvent{
		{
			Timestamp: now,
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/",
				"device_name": "disk3s1s1",
			},
			Severity:  0.82,
			Direction: "decrease",
			Magnitude: 5368709120,
		},
		{
			Timestamp: now.Add(3 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/VM",
				"device_name": "disk3s4",
			},
			Severity:  0.91,
			Direction: "increase",
			Magnitude: 8589934592,
		},
		{
			Timestamp: now.Add(6 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Data",
				"device_name": "disk3s5",
			},
			Severity:  0.85,
			Direction: "decrease",
			Magnitude: 32212254720,
		},
		{
			Timestamp: now.Add(9 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/System/Volumes/Update",
				"device_name": "disk3s6",
			},
			Severity:  0.87,
			Direction: "increase",
			Magnitude: 4294967296,
		},
		{
			Timestamp: now.Add(12 * time.Second),
			Metric:    "system.disk.free",
			Tags: map[string]string{
				"device":      "/System/Volumes/Preboot",
				"device_name": "disk3s2",
			},
			Severity:  0.78,
			Direction: "decrease",
			Magnitude: 2147483648,
		},
		{
			Timestamp: now.Add(15 * time.Second),
			Metric:    "system.disk.used",
			Tags: map[string]string{
				"device":      "/private/var/vm",
				"device_name": "disk3s3",
			},
			Severity:  0.89,
			Direction: "increase",
			Magnitude: 6442450944,
		},
	}

	// Add all events
	for _, event := range events {
		cs.Add(event)
	}

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "All 6 disk events should form a single cluster")

	cluster := clusters[0]
	assert.Len(t, cluster.Events, 6, "Cluster should contain all 6 events")

	// Verify metric pattern
	assert.Equal(t, "system.disk", cluster.Pattern.Family, "Family should be system.disk")
	assert.ElementsMatch(t, []string{"free", "used"}, cluster.Pattern.Variants,
		"Should have both free and used variants")

	// Verify tag pattern - both device and device_name should be varying
	assert.Contains(t, cluster.Pattern.VaryingTags, "device", "device should be varying")
	assert.Contains(t, cluster.Pattern.VaryingTags, "device_name", "device_name should be varying")

	assert.Len(t, cluster.Pattern.VaryingTags["device"], 6,
		"Should have 6 distinct device values")
	assert.Len(t, cluster.Pattern.VaryingTags["device_name"], 6,
		"Should have 6 distinct device_name values")

	// Verify time boundaries
	assert.Equal(t, events[0].Timestamp, cluster.FirstSeen, "FirstSeen should be first event time")
	assert.Equal(t, events[5].Timestamp, cluster.LastSeen, "LastSeen should be last event time")

	// No pending events
	assert.Empty(t, cs.Pending(), "All events should be clustered")
}

// TestClusterSet_PendingToCluster tests the progression from pending to clustered.
// Add one event (goes to pending), then add a compatible event.
// Both should now be in a cluster, pending should be empty.
func TestClusterSet_PendingToCluster(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
	}

	// Add first event
	cs.Add(event1)

	// After one event, it might be pending (no cluster yet)
	initialClusters := cs.Clusters()
	initialPending := cs.Pending()

	// Should have exactly 1 event total (either pending or in cluster)
	totalEvents := len(initialPending)
	for _, c := range initialClusters {
		totalEvents += len(c.Events)
	}
	assert.Equal(t, 1, totalEvents, "Should have exactly 1 event tracked")

	// Add compatible event
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/data",
		},
		Severity:  0.85,
		Direction: "decrease",
	}

	cs.Add(event2)

	// Now both should be in a cluster
	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Should have one cluster after second compatible event")
	assert.Len(t, clusters[0].Events, 2, "Cluster should contain both events")
	assert.Empty(t, cs.Pending(), "No events should be pending after clustering")
}

// TestClusterSet_MultipleClusters tests that different metric families
// form separate clusters. Add disk events, then cpu events.
// Should form 2 separate clusters.
func TestClusterSet_MultipleClusters(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	// Add disk events
	diskEvent1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
	}
	diskEvent2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/data",
		},
		Severity:  0.85,
		Direction: "increase",
	}

	// Add CPU events
	cpuEvent1 := AnomalyEvent{
		Timestamp: now.Add(10 * time.Second),
		Metric:    "system.cpu.user",
		Tags: map[string]string{
			"cpu": "cpu0",
		},
		Severity:  0.7,
		Direction: "increase",
	}
	cpuEvent2 := AnomalyEvent{
		Timestamp: now.Add(15 * time.Second),
		Metric:    "system.cpu.system",
		Tags: map[string]string{
			"cpu": "cpu1",
		},
		Severity:  0.75,
		Direction: "increase",
	}

	cs.Add(diskEvent1)
	cs.Add(diskEvent2)
	cs.Add(cpuEvent1)
	cs.Add(cpuEvent2)

	clusters := cs.Clusters()
	assert.Len(t, clusters, 2, "Should have 2 separate clusters (disk and cpu)")

	// Find disk and cpu clusters
	var diskCluster, cpuCluster *AnomalyCluster
	for _, c := range clusters {
		if c.Pattern.Family == "system.disk" {
			diskCluster = c
		} else if c.Pattern.Family == "system.cpu" {
			cpuCluster = c
		}
	}

	assert.NotNil(t, diskCluster, "Should have a disk cluster")
	assert.NotNil(t, cpuCluster, "Should have a cpu cluster")

	assert.Len(t, diskCluster.Events, 2, "Disk cluster should have 2 events")
	assert.Len(t, cpuCluster.Events, 2, "CPU cluster should have 2 events")

	assert.ElementsMatch(t, []string{"free", "used"}, diskCluster.Pattern.Variants,
		"Disk cluster should have free and used variants")
	assert.ElementsMatch(t, []string{"system", "user"}, cpuCluster.Pattern.Variants,
		"CPU cluster should have system and user variants")
}

// TestClusterSet_NoTags tests clustering behavior when events have no tags.
// Events with no tags should still cluster based on metric family and time.
func TestClusterSet_NoTags(t *testing.T) {
	cfg := ClusterConfig{
		TimeWindow: 30 * time.Second,
	}
	cs := NewClusterSet(cfg)

	now := time.Now()

	event1 := AnomalyEvent{
		Timestamp: now,
		Metric:    "system.load.1",
		Tags:      map[string]string{}, // No tags
		Severity:  0.8,
		Direction: "increase",
	}
	event2 := AnomalyEvent{
		Timestamp: now.Add(5 * time.Second),
		Metric:    "system.load.1",
		Tags:      nil, // nil tags
		Severity:  0.85,
		Direction: "increase",
	}

	cs.Add(event1)
	cs.Add(event2)

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Events with no tags should still cluster")
	assert.Len(t, clusters[0].Events, 2, "Cluster should contain both events")

	// Tag partition should be empty
	assert.Empty(t, clusters[0].Pattern.ConstantTags, "Should have no constant tags")
	assert.Empty(t, clusters[0].Pattern.VaryingTags, "Should have no varying tags")
}

// TestClusterSet_EdgeCases tests various edge cases
func TestClusterSet_EdgeCases(t *testing.T) {
	t.Run("EmptyClusterSet", func(t *testing.T) {
		cfg := DefaultClusterConfig()
		cs := NewClusterSet(cfg)

		assert.Empty(t, cs.Clusters(), "New cluster set should have no clusters")
		assert.Empty(t, cs.Pending(), "New cluster set should have no pending events")
	})

	t.Run("DefaultConfig", func(t *testing.T) {
		cfg := DefaultClusterConfig()
		assert.Equal(t, 30*time.Second, cfg.TimeWindow, "Default time window should be 30s")
	})

	t.Run("SingleEvent", func(t *testing.T) {
		cfg := ClusterConfig{TimeWindow: 30 * time.Second}
		cs := NewClusterSet(cfg)

		event := AnomalyEvent{
			Timestamp: time.Now(),
			Metric:    "system.mem.used",
			Tags:      map[string]string{"host": "server1"},
			Severity:  0.9,
			Direction: "increase",
		}

		cs.Add(event)

		// Single event should either be in a cluster or pending
		totalEvents := len(cs.Pending())
		for _, c := range cs.Clusters() {
			totalEvents += len(c.Events)
		}
		assert.Equal(t, 1, totalEvents, "Should track the single event")
	})
}
