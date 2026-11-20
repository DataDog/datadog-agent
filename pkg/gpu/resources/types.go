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

	// GPUPeers lists the host-local GPUs that are directly attached over NVLink or that report
	// at least one supported P2P capability, ordered by UUID.
	GPUPeers []GPUPeer `json:"gpu_peers,omitempty"`
	// UnresolvedNVLinks lists the NVLink ports whose remote end is not a host-local GPU,
	// ordered by port. Switch ports stay here until switch identities are published.
	UnresolvedNVLinks []UnresolvedNVLink `json:"unresolved_nvlinks,omitempty"`
}

// GPUPeer describes another GPU of the same host from the point of view of one GPU.
type GPUPeer struct {
	UUID            string           `json:"gpu_uuid"`
	P2PCapabilities *P2PCapabilities `json:"p2p_capabilities,omitempty"`
	DirectNVLinks   []DirectNVLink   `json:"direct_nvlinks,omitempty"`
}

// P2PCapabilities holds the peer-to-peer capabilities of an ordered GPU pair: true is supported,
// false is explicitly unsupported or disabled and absent is unknown.
type P2PCapabilities struct {
	Read    *bool `json:"read,omitempty"`
	Write   *bool `json:"write,omitempty"`
	Atomics *bool `json:"atomics,omitempty"`
	NVLink  *bool `json:"nvlink,omitempty"`
	PCI     *bool `json:"pci,omitempty"`
}

// DirectNVLink is one local NVLink port that terminates at a peer GPU.
type DirectNVLink struct {
	Port           int32  `json:"nvlink_port"`
	RemotePCIBusID string `json:"remote_pci_bus_id,omitempty"`
	Version        string `json:"version,omitempty"`
}

// UnresolvedNVLink is one local NVLink port without a resolved GPU or switch identity.
type UnresolvedNVLink struct {
	Port           int32  `json:"nvlink_port"`
	RemoteType     string `json:"remote_type,omitempty"`
	RemotePCIBusID string `json:"remote_pci_bus_id,omitempty"`
	Version        string `json:"version,omitempty"`
}
