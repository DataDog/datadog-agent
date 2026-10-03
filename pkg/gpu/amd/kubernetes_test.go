// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// partitionedHost has an unpartitioned GPU at 0000:05:00.0 (render node 128)
// and a GPU at 0000:27:00.0 split in two partitions whose render nodes 129
// and 130 belong to amdgpu_xcp_1 and amdgpu_xcp_2, the layout the AMD device
// plugin reads to name partition devices.
func partitionedHost(t *testing.T) (*FakeSysfs, []*Device) {
	fs := NewFakeSysfs(t)
	fs.AddCard("card0", fs.AddPCIDevice("0000:05:00.0", "amdgpu", MI300XAttributes("")))
	fs.AddCard("card1", fs.AddPCIDevice("0000:27:00.0", "amdgpu", MI300XAttributes("00c0ffee00c0ffee")))
	fs.AddKFDNode(1, 4101, 0, 0x0500, 90402)
	fs.SetKFDRenderMinor(1, 128)
	fs.AddKFDNode(2, 4102, 0, 0x2700, 90402)
	fs.SetKFDRenderMinor(2, 129)
	fs.AddKFDNode(3, 4103, 0, 0x2701, 90402)
	fs.SetKFDRenderMinor(3, 130)
	fs.AddPartitionRenderNode("amdgpu_xcp_1", 129)
	fs.AddPartitionRenderNode("amdgpu_xcp_2", 130)
	fs.AddPartitionRenderNode("amdgpu_xcp_9", 250) // render node without a KFD node

	devices, err := Discover(fs.Root)
	require.NoError(t, err)
	require.Len(t, devices, 2)
	return fs, devices
}

func TestMatchDevicePluginID(t *testing.T) {
	fs, devices := partitionedHost(t)

	for id, expectedPCI := range map[string]string{
		"0000:05:00.0":   "0000:05:00.0",
		"0000:27:00.0":   "0000:27:00.0",
		"0000:27:00.0 ":  "0000:27:00.0",
		"0000:C1:00.0":   "", // not on this host
		"amdgpu_xcp_1":   "0000:27:00.0",
		"amdgpu_xcp_2":   "0000:27:00.0",
		"AMDGPU_XCP_2":   "0000:27:00.0",
		"amdgpu_xcp_9":   "", // no KFD node for its render node
		"amdgpu_xcp_42":  "", // no such partition
		"amdgpu_xcp_../": "",
		"gpu-0":          "",
		"":               "",
	} {
		dev := MatchDevicePluginID(fs.Root, devices, id)
		if expectedPCI == "" {
			assert.Nil(t, dev, "id %q", id)
			continue
		}
		require.NotNil(t, dev, "id %q", id)
		assert.Equal(t, expectedPCI, dev.PCIBusID, "id %q", id)
	}
}
