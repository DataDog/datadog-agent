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

// TestLifecycle_ActiveToStabilizing tests that a cluster transitions from Active
// to Stabilizing after the StabilizingTimeout has elapsed.
func TestLifecycle_ActiveToStabilizing(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add two events to form a cluster at baseTime
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Initially should be Active
	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Should have one cluster")
	assert.Equal(t, Active, clusters[0].State, "New cluster should be Active")

	// Advance time by 31 seconds and tick
	cs.Tick(baseTime.Add(36 * time.Second)) // 31s after last event

	// Should now be Stabilizing
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should still exist")
	assert.Equal(t, Stabilizing, clusters[0].State, "Cluster should be Stabilizing after 31s")
}

// TestLifecycle_StabilizingToResolved tests that a cluster transitions from
// Stabilizing to Resolved after the ResolvedTimeout has elapsed.
func TestLifecycle_StabilizingToResolved(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create cluster at baseTime
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Advance time by 2.5 minutes and tick
	cs.Tick(baseTime.Add(155 * time.Second)) // 2min 30s after last event

	// Cluster state should be Resolved
	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should still exist")
	assert.Equal(t, Resolved, clusters[0].State, "Cluster should be Resolved after 2.5 minutes")
}

// TestLifecycle_ResolvedExpiration tests that a cluster is removed after the
// ExpireTimeout has elapsed.
func TestLifecycle_ResolvedExpiration(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create cluster at baseTime
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Verify cluster exists
	assert.Len(t, cs.Clusters(), 1, "Cluster should exist initially")

	// Advance time by 11 minutes and tick
	cs.Tick(baseTime.Add(11 * time.Minute))

	// Cluster should be removed from ClusterSet
	assert.Len(t, cs.Clusters(), 0, "Cluster should be removed after 11 minutes")
}

// TestLifecycle_PendingExpiration tests that pending events are removed after
// the ExpireTimeout has elapsed.
func TestLifecycle_PendingExpiration(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add event that stays in pending (no compatible event to cluster with)
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})

	// Verify event is pending
	assert.Len(t, cs.Pending(), 1, "Event should be pending initially")

	// Advance time by 11 minutes and tick
	cs.Tick(baseTime.Add(11 * time.Minute))

	// Pending list should be empty
	assert.Len(t, cs.Pending(), 0, "Pending list should be empty after 11 minutes")
}

// TestLifecycle_NewEventResetsState tests that adding a new event to a cluster
// in Stabilizing state resets it back to Active.
func TestLifecycle_NewEventResetsState(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create cluster at baseTime
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Advance time to make cluster Stabilizing
	cs.Tick(baseTime.Add(40 * time.Second))
	clusters := cs.Clusters()
	assert.Equal(t, Stabilizing, clusters[0].State, "Cluster should be Stabilizing")

	// Add new event to cluster - must be within TimeWindow (30s) of last event
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(25 * time.Second), // Within 30s of last event at 5s
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/data",
		},
		Severity:  0.82,
		Direction: "decrease",
		Magnitude: 2000000000,
	})

	// State should return to Active after adding new event
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Should still have one cluster")
	assert.Equal(t, Active, clusters[0].State, "State should return to Active after new event")
}

// TestLifecycle_StateInSummary verifies that cluster state is accessible and
// properly reflects the lifecycle.
func TestLifecycle_StateInSummary(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create Resolved cluster
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Advance time to Resolved state
	cs.Tick(baseTime.Add(3 * time.Minute))

	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Should have one cluster")
	assert.Equal(t, Resolved, clusters[0].State, "Cluster should be Resolved")
	assert.Equal(t, "resolved", clusters[0].State.String(), "State string should be 'resolved'")
}

// TestLifecycle_ActiveClustersFilter tests that ActiveClusters() returns only
// clusters in Active state.
func TestLifecycle_ActiveClustersFilter(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create first cluster (disk) - will become Resolved
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Create second cluster (cpu) - will become Stabilizing
	// Use 50 seconds to ensure it's > 30s but well under 2min from tick
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(50 * time.Second),
		Metric:    "system.cpu.user",
		Tags: map[string]string{
			"cpu": "cpu0",
		},
		Severity:  0.7,
		Direction: "increase",
		Magnitude: 50.0,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(55 * time.Second),
		Metric:    "system.cpu.system",
		Tags: map[string]string{
			"cpu": "cpu1",
		},
		Severity:  0.75,
		Direction: "increase",
		Magnitude: 45.0,
	})

	// Create third cluster (mem) - will stay Active
	// Use 2min 5s to ensure it's < 30s from tick
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(2*time.Minute + 5*time.Second),
		Metric:    "system.mem.used",
		Tags: map[string]string{
			"host": "server1",
		},
		Severity:  0.9,
		Direction: "increase",
		Magnitude: 1073741824,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(2*time.Minute + 10*time.Second),
		Metric:    "system.mem.free",
		Tags: map[string]string{
			"host": "server1",
		},
		Severity:  0.88,
		Direction: "decrease",
		Magnitude: 1073741824,
	})

	// Tick at baseTime + 2min + 25s:
	// - Disk: last event at 5s, tick at 2min 25s = 2min 20s ago > 2min (Resolved)
	// - CPU: last event at 55s, tick at 2min 25s = 1min 30s ago > 30s but < 2min (Stabilizing)
	// - Memory: last event at 2min 10s, tick at 2min 25s = 15s ago < 30s (Active)
	cs.Tick(baseTime.Add(2*time.Minute + 25*time.Second))

	// Verify total clusters
	allClusters := cs.Clusters()
	assert.Len(t, allClusters, 3, "Should have 3 total clusters")

	// Get only Active clusters
	activeClusters := cs.ActiveClusters()
	assert.Len(t, activeClusters, 1, "Should have 1 Active cluster")
	assert.Equal(t, "system.mem", activeClusters[0].Pattern.Family, "Active cluster should be memory")

	// Get only Resolved clusters
	resolvedClusters := cs.ResolvedClusters()
	assert.Len(t, resolvedClusters, 1, "Should have 1 Resolved cluster")
	assert.Equal(t, "system.disk", resolvedClusters[0].Pattern.Family, "Resolved cluster should be disk")
}

// TestLifecycle_MultipleTicks tests that calling Tick multiple times as time
// advances correctly transitions states through Active → Stabilizing → Resolved.
func TestLifecycle_MultipleTicks(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create cluster at baseTime
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// Tick 1: 10s after last event - should be Active
	cs.Tick(baseTime.Add(15 * time.Second))
	clusters := cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should exist")
	assert.Equal(t, Active, clusters[0].State, "Should be Active at 10s")

	// Tick 2: 35s after last event - should be Stabilizing
	cs.Tick(baseTime.Add(40 * time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should exist")
	assert.Equal(t, Stabilizing, clusters[0].State, "Should be Stabilizing at 35s")

	// Tick 3: 1 minute after last event - still Stabilizing
	cs.Tick(baseTime.Add(65 * time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should exist")
	assert.Equal(t, Stabilizing, clusters[0].State, "Should still be Stabilizing at 1min")

	// Tick 4: 2.5 minutes after last event - should be Resolved
	cs.Tick(baseTime.Add(155 * time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should exist")
	assert.Equal(t, Resolved, clusters[0].State, "Should be Resolved at 2.5min")

	// Tick 5: 5 minutes after last event - still Resolved
	cs.Tick(baseTime.Add(5*time.Minute + 5*time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 1, "Cluster should exist")
	assert.Equal(t, Resolved, clusters[0].State, "Should still be Resolved at 5min")

	// Tick 6: 11 minutes after last event - should be expired
	cs.Tick(baseTime.Add(11*time.Minute + 5*time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 0, "Cluster should be expired at 11min")
}

// TestLifecycle_CustomTimeouts tests that custom timeout configurations work correctly.
func TestLifecycle_CustomTimeouts(t *testing.T) {
	// Create config with short timeouts for testing
	cfg := ClusterConfig{
		TimeWindow:         30 * time.Second,
		StabilizingTimeout: 10 * time.Second, // Short stabilizing timeout
		ResolvedTimeout:    30 * time.Second, // Short resolved timeout
		ExpireTimeout:      60 * time.Second, // Short expire timeout
	}
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Create cluster
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(5 * time.Second),
		Metric:    "system.disk.used",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.85,
		Direction: "increase",
		Magnitude: 1000000000,
	})

	// After 15s, should be Stabilizing (> 10s)
	cs.Tick(baseTime.Add(20 * time.Second))
	clusters := cs.Clusters()
	assert.Equal(t, Stabilizing, clusters[0].State, "Should be Stabilizing after 15s")

	// After 35s, should be Resolved (> 30s)
	cs.Tick(baseTime.Add(40 * time.Second))
	clusters = cs.Clusters()
	assert.Equal(t, Resolved, clusters[0].State, "Should be Resolved after 35s")

	// After 65s, should be expired (> 60s)
	cs.Tick(baseTime.Add(70 * time.Second))
	clusters = cs.Clusters()
	assert.Len(t, clusters, 0, "Should be expired after 65s")
}

// TestLifecycle_PendingNotExpiredYet tests that pending events are kept if they
// haven't exceeded the ExpireTimeout yet.
func TestLifecycle_PendingNotExpiredYet(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add event that stays in pending
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})

	// Verify event is pending
	assert.Len(t, cs.Pending(), 1, "Event should be pending")

	// Tick at 5 minutes - should still be in pending (< 10 min expiry)
	cs.Tick(baseTime.Add(5 * time.Minute))
	assert.Len(t, cs.Pending(), 1, "Event should still be pending at 5 minutes")

	// Tick at 9 minutes - should still be in pending
	cs.Tick(baseTime.Add(9 * time.Minute))
	assert.Len(t, cs.Pending(), 1, "Event should still be pending at 9 minutes")

	// Tick at 11 minutes - should be expired
	cs.Tick(baseTime.Add(11 * time.Minute))
	assert.Len(t, cs.Pending(), 0, "Event should be expired at 11 minutes")
}

// TestLifecycle_MixedPendingExpiration tests that only expired pending events
// are removed, while recent ones are kept.
func TestLifecycle_MixedPendingExpiration(t *testing.T) {
	cfg := DefaultClusterConfig()
	cs := NewClusterSet(cfg)

	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add old event
	cs.Add(AnomalyEvent{
		Timestamp: baseTime,
		Metric:    "system.disk.free",
		Tags: map[string]string{
			"device": "/",
		},
		Severity:  0.8,
		Direction: "decrease",
		Magnitude: 1000000000,
	})

	// Add recent event
	cs.Add(AnomalyEvent{
		Timestamp: baseTime.Add(9 * time.Minute),
		Metric:    "system.cpu.user",
		Tags: map[string]string{
			"cpu": "cpu0",
		},
		Severity:  0.7,
		Direction: "increase",
		Magnitude: 50.0,
	})

	// Both should be pending
	assert.Len(t, cs.Pending(), 2, "Should have 2 pending events")

	// Tick at 10 minutes - old event should be expired, recent one kept
	cs.Tick(baseTime.Add(10 * time.Minute))
	pending := cs.Pending()
	assert.Len(t, pending, 1, "Should have 1 pending event after expiration")
	assert.Equal(t, "system.cpu.user", pending[0].Metric, "Should keep the recent CPU event")
}

// TestLifecycle_StateStringValues tests the String() method for ClusterState.
func TestLifecycle_StateStringValues(t *testing.T) {
	assert.Equal(t, "active", Active.String())
	assert.Equal(t, "stabilizing", Stabilizing.String())
	assert.Equal(t, "resolved", Resolved.String())

	// Test unknown state
	unknownState := ClusterState(99)
	assert.Equal(t, "unknown", unknownState.String())
}
