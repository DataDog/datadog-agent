// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package amd

import (
	"os"
	"path/filepath"
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

func TestMatchDRADevice(t *testing.T) {
	fs, devices := partitionedHost(t)
	const claimUID = "c8593c85-440d-4156-b199-aea592ff83df"
	cdiDir := t.TempDir()
	// The spec layout of ROCm/k8s-gpu-dra-driver: one device per allocated
	// GPU or partition, plus a common device for every claim.
	require.NoError(t, os.WriteFile(filepath.Join(cdiDir, "k8s.gpu.amd.com-gpu_"+claimUID+".yaml"), []byte(`cdiVersion: 0.6.0
kind: k8s.gpu.amd.com/gpu
devices:
- name: common
  containerEdits:
    deviceNodes:
    - path: /dev/kfd
- name: `+claimUID+`-gpu-0-cpx-nps4
  containerEdits:
    deviceNodes:
    - path: /dev/dri/card1
    - path: /dev/dri/renderD130
`), 0o644))

	for name, tc := range map[string]struct {
		id          string
		cdi         []string
		expectedPCI string
	}{
		"whole GPU":                    {id: "gpu-0-128", expectedPCI: "0000:05:00.0"},
		"partition":                    {id: "gpu-1-130", expectedPCI: "0000:27:00.0"},
		"render node without KFD node": {id: "gpu-9-250"},
		"unknown render node":          {id: "gpu-3-200"},
		"name from the CDI spec": {id: "gpu-0-cpx-nps4", cdi: []string{
			"k8s.gpu.amd.com/gpu=common",
			"k8s.gpu.amd.com/gpu=" + claimUID + "-gpu-0-cpx-nps4",
		}, expectedPCI: "0000:27:00.0"},
		"other vendor's CDI device": {id: "gpu-0-cpx-nps4", cdi: []string{"k8s.gpu.nvidia.com/claim=" + claimUID + "-gpu-0"}},
		"claim without a spec":      {id: "gpu-0-cpx-nps4", cdi: []string{"k8s.gpu.amd.com/gpu=00000000-0000-0000-0000-000000000000-gpu-0"}},
		"UID escaping the spec dir": {id: "x", cdi: []string{"k8s.gpu.amd.com/gpu=../../../../../../../../../etc/x-gpu-0"}},
		"empty":                     {},
	} {
		t.Run(name, func(t *testing.T) {
			dev := MatchDRADevice(fs.Root, devices, tc.id, tc.cdi, []string{filepath.Join(cdiDir, "missing"), cdiDir})
			if tc.expectedPCI == "" {
				assert.Nil(t, dev)
				return
			}
			require.NotNil(t, dev)
			assert.Equal(t, tc.expectedPCI, dev.PCIBusID)
		})
	}
}

// Without access to the KFD topology, the render node of an unpartitioned GPU
// is resolved to its PCI device through sysfs.
func TestMatchDRADeviceWithoutKFD(t *testing.T) {
	fs := NewFakeSysfs(t)
	devDir := fs.AddPCIDevice("0000:83:00.0", "amdgpu", MI300XAttributes("2a77ed7921593a8b"))
	fs.AddCard("card1", devDir)
	renderDir := filepath.Join(fs.Root, "class", "drm", "renderD129")
	require.NoError(t, os.MkdirAll(renderDir, 0o755))
	require.NoError(t, os.Symlink(devDir, filepath.Join(renderDir, "device")))
	devices, err := Discover(fs.Root)
	require.NoError(t, err)

	dev := MatchDRADevice(fs.Root, devices, "gpu-1-129", nil, nil)
	require.NotNil(t, dev)
	assert.Equal(t, "0000:83:00.0", dev.PCIBusID)
	assert.Nil(t, MatchDRADevice(fs.Root, devices, "gpu-1-130", nil, nil))
}
