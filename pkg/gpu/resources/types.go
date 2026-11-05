// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package resources builds and publishes static GPU device inventory to REDAPL.
package resources

// Device contains a physical GPU's static resource fields.
type Device struct {
	Hostname           string  `json:"hostname"`
	UUID               string  `json:"gpu_uuid"`
	Vendor             string  `json:"gpu_vendor,omitempty"`
	Model              string  `json:"gpu_device,omitempty"`
	Architecture       string  `json:"gpu_architecture,omitempty"`
	ComputeCapability  string  `json:"compute_capability,omitempty"`
	CoreCount          int     `json:"core_count,omitempty"`
	MemoryTotalBytes   uint64  `json:"memory_total_bytes,omitempty"`
	MemoryBusWidth     uint32  `json:"memory_bus_width,omitempty"`
	DriverVersion      string  `json:"gpu_driver_version,omitempty"`
	VirtualizationMode string  `json:"gpu_virtualization_mode,omitempty"`
	PCIBusID           string  `json:"gpu_pci_bus_id,omitempty"`
	FabricGUID         string  `json:"gpu_fabric_guid,omitempty"`
	FabricClusterUUID  string  `json:"gpu_fabric_cluster_uuid,omitempty"`
	FabricCliqueID     *uint32 `json:"gpu_fabric_clique_id,omitempty"`
	NVLinkDomain       string  `json:"nvlink_domain,omitempty"`
	BoardSerial        string  `json:"gpu_board_serial,omitempty"`
	ChassisSerial      string  `json:"gpu_chassis_serial,omitempty"`
}
