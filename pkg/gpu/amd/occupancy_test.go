// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Partitions of one GPU are summed, processes are summed, and the compute units
// of the device come from all of its KFD nodes.
func TestReadCUOccupancy(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:41:00.0", "amdgpu", MI300XAttributes("")))
	// GPU 0 is split in two partitions of 152 compute units each.
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDNode(3, 5100, 0, 0x4100, 90402)
	for node, simds := range map[int]uint64{1: 608, 2: 608, 3: 1216} {
		fs.SetKFDProperty(node, "simd_count", simds)
		fs.SetKFDProperty(node, "simd_per_cu", 4)
	}
	fs.AddKFDProcessOccupancy(100, 4101, 10)
	fs.AddKFDProcessOccupancy(100, 4102, 20)
	fs.AddKFDProcessOccupancy(200, 4101, 5)
	fs.AddKFDProcessOccupancy(200, 9999, 50) // unknown gpu_id
	// A process with memory on GPU 1 but no queues reports 0.
	fs.AddKFDProcess(300, 5100, 1024)
	fs.AddKFDProcessOccupancy(300, 5100, 0)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	assert.Equal(t, uint32(304), devices[0].ComputeUnits)
	assert.Equal(t, uint32(304), devices[1].ComputeUnits)

	occupied, complete, err := ReadCUOccupancy(fs.Root, devices)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, map[string]uint64{
		"amd-0000-27-00-0": 35,
		"amd-0000-41-00-0": 0,
	}, occupied)
}

func TestReadCUOccupancyWithoutProcesses(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	occupied, complete, err := ReadCUOccupancy(fs.Root, devices)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Empty(t, occupied)
}

// As for process memory, an unmapped KFD node withholds the sums.
func TestReadCUOccupancyWithUnmappedKFDNode(t *testing.T) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddKFDNode(1, 4101, 0, 0x2700, 90402)
	fs.AddKFDNode(2, 4102, 0, 0x2701, 90402)
	fs.AddKFDProcessOccupancy(100, 4101, 10)
	require.NoError(t, os.Remove(filepath.Join(fs.Root, "class/kfd/kfd/topology/nodes/2/properties")))

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	occupied, complete, err := ReadCUOccupancy(fs.Root, devices)
	require.NoError(t, err)
	assert.False(t, complete)
	assert.Empty(t, occupied)
}
