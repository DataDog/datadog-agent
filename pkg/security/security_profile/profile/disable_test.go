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

// TestProfile_DisabledStateIsPersisted verifies the disabled state round-trips through the
// SecurityProfile protobuf: a profile disabled before encoding decodes back as disabled, and an
// enabled one decodes back as enabled. This is what lets a max-size-disabled workload reload as
// disabled instead of coming back enabled and re-learning.
func TestProfile_DisabledStateIsPersisted(t *testing.T) {
	selector := cgroupModel.WorkloadSelector{Image: "img", Tag: "v1"}

	t.Run("disabled survives encode/decode", func(t *testing.T) {
		src := New(WithWorkloadSelector(selector))
		src.Disable()
		require.False(t, src.IsEnabled())

		buf, err := src.Encode(config.Profile)
		require.NoError(t, err)

		loaded := New(WithWorkloadSelector(selector))
		require.NoError(t, loaded.DecodeFromReader(buf, config.Profile))
		assert.False(t, loaded.IsEnabled(), "a disabled profile must decode as disabled")
	})

	t.Run("enabled survives encode/decode", func(t *testing.T) {
		src := New(WithWorkloadSelector(selector))
		require.True(t, src.IsEnabled())

		buf, err := src.Encode(config.Profile)
		require.NoError(t, err)

		loaded := New(WithWorkloadSelector(selector))
		require.NoError(t, loaded.DecodeFromReader(buf, config.Profile))
		assert.True(t, loaded.IsEnabled(), "an enabled profile must decode as enabled")
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
