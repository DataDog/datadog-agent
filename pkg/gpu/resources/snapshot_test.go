// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	collectors "github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSnapshotUsesCachedAttributes(t *testing.T) {
	physical := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{
		UUID: testUUID, Name: "Tesla T4", Architecture: nvml.DEVICE_ARCH_TURING,
		SMVersion: 75, CoreCount: 2560, Memory: 16 << 30, PCIBusID: "10000:01:00.0",
		DriverVersion: "580.1", VirtualizationModeName: "none", MemoryBusWidth: 256,
		BoardSerial: "BOARD", ChassisSerial: "CHASSIS", FabricGUID: "0x00000000000000ab",
		FabricClusterUUID: "01020304-0506-0708-090a-0b0c0d0e0f10",
	}}
	snapshot := Snapshot([]safenvml.Device{physical, &safenvml.MIGDevice{}})
	require.Len(t, snapshot, 1)
	d := snapshot[0]
	require.Equal(t, "tesla_t4", d.Model)
	require.Equal(t, "7.5", d.ComputeCapability)
	require.Equal(t, uint64(16<<30), d.MemoryTotalBytes)
	require.Equal(t, "none", d.VirtualizationMode)
	require.Equal(t, uint32(0), *d.FabricCliqueID)
	require.Equal(t, d.FabricClusterUUID+":0", d.NVLinkDomain)
	tags := taglist.NewTagList()
	collectors.ExtractGPUTags(&workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{ID: testUUID}, Vendor: "nvidia", Device: "Tesla T4", PCIBusID: physical.PCIBusID,
		BoardSerial: physical.BoardSerial, ChassisSerial: physical.ChassisSerial,
		FabricGUID: physical.FabricGUID, FabricClusterUUID: physical.FabricClusterUUID,
	}, tags)
	low, _, _, _ := tags.Compute()
	for _, tag := range []string{
		"gpu_device:" + d.Model, "gpu_uuid:" + d.UUID, "gpu_pci_bus_id:" + d.PCIBusID,
		"gpu_board_serial:" + d.BoardSerial, "gpu_chassis_serial:" + d.ChassisSerial,
		"gpu_fabric_guid:" + d.FabricGUID, "gpu_fabric_cluster_uuid:" + d.FabricClusterUUID,
	} {
		require.Contains(t, low, tag)
	}
	physical.Name = "changed"
	require.Equal(t, "tesla_t4", d.Model)
}

func TestSnapshotExcludesVirtualDevices(t *testing.T) {
	physical := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{VirtualizationMode: nvml.GPU_VIRTUALIZATION_MODE_VGPU}}
	require.Empty(t, Snapshot([]safenvml.Device{physical, &safenvml.MIGDevice{}}))
}

func TestSnapshotFabricMembership(t *testing.T) {
	const cluster = "01020304-0506-0708-090a-0b0c0d0e0f10"
	for _, tc := range []struct {
		name   string
		clique uint32
		domain string
	}{
		{name: "zero is known", clique: 0, domain: cluster + ":0"},
		{name: "full NVML range", clique: 4294967295, domain: cluster + ":4294967295"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			device := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{
				FabricClusterUUID: cluster, FabricCliqueID: tc.clique,
			}}
			result := Snapshot([]safenvml.Device{device})
			require.Equal(t, &tc.clique, result[0].FabricCliqueID)
			require.Equal(t, tc.domain, result[0].NVLinkDomain)
		})
	}
	result := Snapshot([]safenvml.Device{&safenvml.PhysicalDevice{}})
	require.Nil(t, result[0].FabricCliqueID)
	require.Empty(t, result[0].NVLinkDomain)
}
