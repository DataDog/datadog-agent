// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package resources builds and publishes static GPU device inventory to REDAPL.
package resources

// Device contains a physical GPU's static resource fields. Unknown values are
// omitted; pointers mark fields where zero is a valid value.
type Device struct {
	Hostname           string    `json:"hostname"`
	UUID               string    `json:"gpu_uuid"`
	Vendor             string    `json:"gpu_vendor,omitempty"`
	Model              string    `json:"gpu_device,omitempty"`
	BoardPartNumber    string    `json:"board_part_number,omitempty"`
	Architecture       string    `json:"gpu_architecture,omitempty"`
	ComputeCapability  string    `json:"compute_capability,omitempty"`
	CoreCount          int       `json:"core_count,omitempty"`
	MemoryTotalBytes   uint64    `json:"memory_total_bytes,omitempty"`
	MemoryBusWidth     uint32    `json:"memory_bus_width,omitempty"`
	BAR1TotalBytes     uint64    `json:"bar1_total_bytes,omitempty"`
	DriverVersion      string    `json:"gpu_driver_version,omitempty"`
	VirtualizationMode string    `json:"gpu_virtualization_mode,omitempty"`
	PCIBusID           string    `json:"gpu_pci_bus_id,omitempty"`
	PCIDeviceID        string    `json:"pci_device_id,omitempty"`
	PCISubsystemID     string    `json:"pci_subsystem_id,omitempty"`
	PCIePath           *[]string `json:"pcie_path,omitempty"` // empty when the GPU is on the root complex
	NUMANode           *int      `json:"gpu_numa_node,omitempty"`
	FabricGUID         string    `json:"gpu_fabric_guid,omitempty"`
	FabricClusterUUID  string    `json:"gpu_fabric_cluster_uuid,omitempty"`
	FabricCliqueID     *uint32   `json:"gpu_fabric_clique_id,omitempty"`
	NVLinkDomain       string    `json:"nvlink_domain,omitempty"`
	NVLinkLinkCount    int       `json:"nvlink_link_count,omitempty"`
	BoardSerial        string    `json:"gpu_board_serial,omitempty"`
	ChassisSerial      string    `json:"gpu_chassis_serial,omitempty"`
	SlotNumber         *uint8    `json:"gpu_slot_number,omitempty"`
	TrayIndex          *uint8    `json:"gpu_tray_index,omitempty"`
	ModuleID           *uint8    `json:"gpu_module_id,omitempty"`
	Platform           *Platform `json:"platform,omitempty"`
	HostLink           *HostLink `json:"host_link,omitempty"`
}

// Platform is the GPU's hardware platform placement.
type Platform struct {
	// HostID is the NVML node index within the slot; zero is valid.
	HostID uint8 `json:"host_id"`
}

// HostLink holds the GPU-to-host protocol and its maximum capabilities.
// Negotiated values are metrics.
type HostLink struct {
	Protocol                       string `json:"protocol,omitempty"`
	PCIeMaxGeneration              int    `json:"pcie_max_generation,omitempty"`
	PCIeGPUMaxGeneration           int    `json:"pcie_gpu_max_generation,omitempty"`
	PCIeMaxWidth                   int    `json:"pcie_max_width,omitempty"`
	PCIeMaxBandwidthBytesPerSecond int64  `json:"pcie_max_bandwidth_bytes_per_second,omitempty"`
	C2CLinkCount                   int    `json:"c2c_link_count,omitempty"`
	C2CMaxBandwidthMbps            int64  `json:"c2c_max_bandwidth_mbps,omitempty"`
}
