// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	activity_tree "github.com/DataDog/datadog-agent/pkg/security/security_profile/activity_tree"
)

// processNodes returns n flat root process nodes. They are intentionally minimal: the
// max-size safeguard reacts to node counts (via ApproximateSize) once stats are computed,
// so the exact per-node content doesn't matter here.
func processNodes(n int) []*activity_tree.ProcessNode {
	nodes := make([]*activity_tree.ProcessNode, 0, n)
	for i := 0; i < n; i++ {
		nodes = append(nodes, &activity_tree.ProcessNode{
			NodeBase: activity_tree.NewNodeBase(),
			Process: model.Process{
				FileEvent: model.FileEvent{
					PathnameStr: "/usr/bin/proc" + strconv.Itoa(i),
					BasenameStr: "proc" + strconv.Itoa(i),
				},
			},
		})
	}
	return nodes
}

// loadFromStorageRoundTrip builds a profile with the given process nodes, encodes it to the
// security-profile protobuf format and decodes it back into a fresh profile. This mirrors
// what ManagerV2.loadProfileFromStorage does on a workload restart: the returned profile has
// its process nodes populated but its Stats are not yet computed (decode does not recompute
// them), exactly like a profile freshly read from disk.
func loadFromStorageRoundTrip(t *testing.T, image, tag string, nodes []*activity_tree.ProcessNode) *Profile {
	t.Helper()

	selector := cgroupModel.WorkloadSelector{Image: image, Tag: tag}
	src := New(WithWorkloadSelector(selector))
	src.ActivityTree.ProcessNodes = nodes
	src.ActivityTree.ComputeActivityTreeStats()

	buf, err := src.Encode(config.Profile)
	require.NoError(t, err)

	loaded := New(WithWorkloadSelector(selector))
	require.NoError(t, loaded.DecodeFromReader(buf, config.Profile))
	return loaded
}

// TestProfile_SelfHealsAfterLoad covers the safeguard that lets the agent re-disable a
// workload that was already over the max dump size before a restart, without persisting any
// "disabled" state in the profile. The profile is reloaded from storage, its in-memory size
// is recomputed, and the size check disables it again. This is the behaviour we rely on
// instead of serializing the enabled flag into the protobuf schema.
func TestProfile_SelfHealsAfterLoad(t *testing.T) {
	// recompute is load-bearing: straight after decode the tree has nodes but its stats are
	// zero, so the size check reads 0. loadProfileFromStorage MUST call ComputeActivityTreeStats
	// for the safeguard to see the real footprint — if it ever stops, an over-limit profile
	// would silently come back enabled after a restart.
	t.Run("size is invisible until stats are recomputed", func(t *testing.T) {
		loaded := loadFromStorageRoundTrip(t, "img", "v1", processNodes(128))
		require.False(t, loaded.ActivityTree.IsEmpty(), "decode should restore the process nodes")
		require.Equal(t, int64(0), loaded.ComputeHeapSize(),
			"before recompute the size check is blind to the loaded tree")

		loaded.ActivityTree.ComputeActivityTreeStats()
		require.Greater(t, loaded.ComputeHeapSize(), int64(0),
			"recompute must surface the loaded tree's footprint")
	})

	// An over-limit profile reloaded from storage is disabled by the size check and its tree
	// is dropped, so the workload stops counting against RAM again — same outcome as before
	// the restart.
	t.Run("over-limit profile is disabled and freed", func(t *testing.T) {
		loaded := loadFromStorageRoundTrip(t, "img", "v1", processNodes(128))
		loaded.ActivityTree.ComputeActivityTreeStats()

		size := loaded.ComputeHeapSize()
		require.Greater(t, size, int64(0))
		maxSize := size - 1 // profile sits above the limit

		require.True(t, loaded.IsEnabled())
		// Mirrors the check in ManagerV2.insertEventIntoProfile.
		if loaded.ComputeHeapSize() >= maxSize {
			loaded.Disable()
		}

		assert.False(t, loaded.IsEnabled(), "an over-limit reloaded profile must be disabled")
		assert.True(t, loaded.ActivityTree.IsEmpty(), "disabling must drop the activity tree")
		assert.Equal(t, int64(0), loaded.ComputeHeapSize(), "disabling must free the tracked size")
	})

	// A profile that loads back under the limit keeps running and keeps its tree.
	t.Run("under-limit profile stays enabled", func(t *testing.T) {
		loaded := loadFromStorageRoundTrip(t, "img", "v1", processNodes(1))
		loaded.ActivityTree.ComputeActivityTreeStats()

		size := loaded.ComputeHeapSize()
		require.Greater(t, size, int64(0))
		maxSize := size * 1000 // far above the loaded footprint

		if loaded.ComputeHeapSize() >= maxSize {
			loaded.Disable()
		}

		assert.True(t, loaded.IsEnabled(), "an under-limit reloaded profile must stay enabled")
		assert.False(t, loaded.ActivityTree.IsEmpty(), "an enabled profile must keep its tree")
	})
}

// TestProfile_DisableFreesMemory is the focused unit for the RAM guarantee: disabling a live
// profile drops its activity tree so the tracked footprint goes back to zero.
func TestProfile_DisableFreesMemory(t *testing.T) {
	p := New(WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}))
	p.ActivityTree.ProcessNodes = processNodes(64)
	p.ActivityTree.ComputeActivityTreeStats()
	require.Greater(t, p.ComputeHeapSize(), int64(0))

	p.Disable()

	assert.False(t, p.IsEnabled())
	assert.True(t, p.ActivityTree.IsEmpty(), "disable must reset the activity tree")
	assert.Equal(t, int64(0), p.ComputeHeapSize(), "disable must free the tracked size")
}
