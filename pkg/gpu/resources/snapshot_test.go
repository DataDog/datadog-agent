// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"encoding/json"
	"testing"

	collectors "github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
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

func TestSnapshotPlacementFacts(t *testing.T) {
	numa, c2c := 1, true
	gb200 := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{
		UUID: testUUID, PCIBusID: "0018:01:00.0", PCIDeviceID: 0x294110de, PCISubsystemID: 0x20ca10de,
		BoardPartNumber: "699-2G548-0200-000", BAR1TotalBytes: 128 << 30, NVLinkLinkCount: 18,
		NUMANode: &numa, PCIePath: []string{"0018:00:00.0"},
		ChassisSerial: "1821825880999", SlotNumber: 0, TrayIndex: 4, ModuleID: 2, HostID: 0,
		HostLink: safenvml.HostLinkInfo{
			C2CEnabled: &c2c, C2CLinkCount: 10, C2CMaxBandwidthMBps: 44712,
			PCIeMaxGeneration: 6, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16,
		},
	}}
	d := Snapshot([]safenvml.Device{gb200})[0]
	require.Equal(t, "294110de", d.PCIDeviceID)
	require.Equal(t, "20ca10de", d.PCISubsystemID)
	require.Equal(t, "699-2G548-0200-000", d.BoardPartNumber)
	require.Equal(t, uint64(128<<30), d.BAR1TotalBytes)
	require.Equal(t, 18, d.NVLinkLinkCount)
	require.Equal(t, 1, *d.NUMANode)
	require.Equal(t, []string{"0018:00:00.0"}, d.PCIePath)
	require.Equal(t, uint8(0), *d.SlotNumber, "slot 0 is valid")
	require.Equal(t, uint8(4), *d.TrayIndex)
	require.Equal(t, uint8(2), *d.ModuleID)
	require.Equal(t, &Platform{HostID: 0}, d.Platform)
	require.Equal(t, &HostLink{
		Protocol: "nvlink_c2c", PCIeMaxGeneration: 6, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16,
		PCIeMaxBandwidthBytesPerSecond: 121000000000, C2CLinkCount: 10, C2CMaxBandwidthMbps: 357696,
	}, d.HostLink)
	gb200.PCIePath[0] = "changed"
	require.Equal(t, "0018:00:00.0", d.PCIePath[0], "the snapshot owns its path")

	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Equal(t, float64(0), fields["gpu_slot_number"])
	require.Equal(t, map[string]any{"host_id": float64(0)}, fields["platform"])
}

func TestSnapshotHGXHostWithoutChassis(t *testing.T) {
	numa, c2c := 0, false
	b200 := &safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{
		UUID: testUUID, PCIBusID: "0000:8d:00.0", NUMANode: &numa,
		PCIePath: []string{"0000:8b:01.0", "0000:8a:00.0", "0000:89:00.0"},
		// NVML placeholders without an identified chassis.
		SlotNumber: 255, TrayIndex: 255, ModuleID: 7, HostID: 1,
		HostLink: safenvml.HostLinkInfo{C2CEnabled: &c2c, PCIeMaxGeneration: 5, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16},
	}}
	d := Snapshot([]safenvml.Device{b200})[0]
	require.Equal(t, 0, *d.NUMANode, "node 0 is valid")
	require.Equal(t, []string{"0000:8b:01.0", "0000:8a:00.0", "0000:89:00.0"}, d.PCIePath)
	require.Nil(t, d.SlotNumber)
	require.Nil(t, d.TrayIndex)
	require.Nil(t, d.ModuleID)
	require.Nil(t, d.Platform)
	require.Equal(t, &HostLink{
		Protocol: "pcie", PCIeMaxGeneration: 5, PCIeGPUMaxGeneration: 6, PCIeMaxWidth: 16,
		PCIeMaxBandwidthBytesPerSecond: 63015384615,
	}, d.HostLink)
}

func TestSnapshotUnknownPlacementFactsAreAbsent(t *testing.T) {
	d := Snapshot([]safenvml.Device{&safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{
		UUID: testUUID, HostLink: safenvml.HostLinkInfo{PCIeMaxGeneration: 99, PCIeMaxWidth: 16},
	}}})[0]
	require.Equal(t, &HostLink{PCIeMaxGeneration: 99, PCIeMaxWidth: 16}, d.HostLink, "no bandwidth for an unknown generation")

	d = Snapshot([]safenvml.Device{&safenvml.PhysicalDevice{DeviceInfo: safenvml.DeviceInfo{UUID: testUUID}}})[0]
	encoded, err := json.Marshal(d)
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(encoded, &fields))
	for _, name := range []string{
		"pcie_path", "gpu_numa_node", "pci_device_id", "pci_subsystem_id", "bar1_total_bytes",
		"board_part_number", "host_link", "gpu_slot_number", "gpu_tray_index", "gpu_module_id",
		"platform", "nvlink_link_count",
	} {
		require.NotContains(t, fields, name)
	}
}
