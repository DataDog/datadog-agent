// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"encoding/json"
	"strings"
	"unsafe"
)

// Inventory is an owned, allowlisted inventory envelope. Configuration,
// credentials, cloud identifiers, and network interface dumps have no fields.
// Timestamp follows the native inventory envelope's Unix nanoseconds.
type Inventory struct {
	Hostname   string                  `json:"hostname"`
	UUID       string                  `json:"uuid"`
	Timestamp  int64                   `json:"timestamp"`
	Agent      *AgentInventoryMetadata `json:"agent_metadata,omitempty"`
	Host       *HostInventoryMetadata  `json:"host_metadata,omitempty"`
	SystemInfo *HostSystemInfoMetadata `json:"host_system_info_metadata,omitempty"`
}

// HostSystemInfoMetadata carries only the native device hardware fields.
// SerialNumber and Identifier require pseudonymization before persistence.
type HostSystemInfoMetadata struct {
	Manufacturer string `json:"manufacturer"`
	ModelNumber  string `json:"model_number"`
	SerialNumber string `json:"serial_number"`
	ModelName    string `json:"model_name"`
	ChassisType  string `json:"chassis_type"`
	Identifier   string `json:"identifier"`
}

// AgentInventoryMetadata carries the registration identity and feature flags
// used by device discovery. It cannot contain Agent configuration or secrets.
type AgentInventoryMetadata struct {
	AgentVersion                     string `json:"agent_version"`
	PackageVersion                   string `json:"package_version"`
	Flavor                           string `json:"flavor"`
	InfrastructureMode               string `json:"infrastructure_mode"`
	AgentStartupTimeMS               int64  `json:"agent_startup_time_ms"`
	FeatureProcessEnabled            bool   `json:"feature_process_enabled"`
	FeatureProcessesContainerEnabled bool   `json:"feature_processes_container_enabled"`
	FeatureNetworksEnabled           bool   `json:"feature_networks_enabled"`
	FeatureNetworksHTTPEnabled       bool   `json:"feature_networks_http_enabled"`
	FeatureNetworksHTTPSEnabled      bool   `json:"feature_networks_https_enabled"`
}

// HostInventoryMetadata carries hardware and OS enrichment, including the
// primary addresses which the coordinator must pseudonymize before storage.
type HostInventoryMetadata struct {
	CPUCores             uint64  `json:"cpu_cores"`
	CPULogicalProcessors uint64  `json:"cpu_logical_processors"`
	CPUVendor            string  `json:"cpu_vendor"`
	CPUModel             string  `json:"cpu_model"`
	CPUModelID           string  `json:"cpu_model_id"`
	CPUFamily            string  `json:"cpu_family"`
	CPUStepping          string  `json:"cpu_stepping"`
	CPUFrequency         float64 `json:"cpu_frequency"`
	CPUCacheSize         uint64  `json:"cpu_cache_size"`
	KernelName           string  `json:"kernel_name"`
	KernelRelease        string  `json:"kernel_release"`
	OS                   string  `json:"os"`
	OSVersion            string  `json:"os_version"`
	CPUArchitecture      string  `json:"cpu_architecture"`
	MemoryTotalKb        uint64  `json:"memory_total_kb"`
	MemorySwapTotalKb    uint64  `json:"memory_swap_total_kb"`
	IPAddress            string  `json:"ip_address"`
	IPv6Address          string  `json:"ipv6_address"`
	MacAddress           string  `json:"mac_address"`
	AgentVersion         string  `json:"agent_version"`
}

// MarshalJSON allows sanitized inventories to use the normal metadata serializer.
func (i *Inventory) MarshalJSON() ([]byte, error) {
	type plain Inventory
	return json.Marshal((*plain)(i))
}

// InventorySize charges every owned allocation. It can measure a borrowed
// projection before CloneInventory is allowed to allocate its independent copy.
func InventorySize(i *Inventory) int64 {
	if i == nil {
		return 0
	}
	n := int64(unsafe.Sizeof(*i)) + int64(len(i.Hostname)+len(i.UUID))
	if a := i.Agent; a != nil {
		n += int64(unsafe.Sizeof(*a)) + int64(len(a.AgentVersion)+len(a.PackageVersion)+len(a.Flavor)+len(a.InfrastructureMode))
	}
	if h := i.Host; h != nil {
		n += int64(unsafe.Sizeof(*h))
		for _, value := range hostInventoryStrings(h) {
			n += int64(len(*value))
		}
	}
	if h := i.SystemInfo; h != nil {
		n += int64(unsafe.Sizeof(*h))
		for _, value := range hostSystemInfoStrings(h) {
			n += int64(len(*value))
		}
	}
	return n
}

// CloneInventory is used only after a producer reserves InventorySize bytes.
func CloneInventory(i *Inventory) *Inventory {
	if i == nil {
		return nil
	}
	out := *i
	out.Hostname, out.UUID = strings.Clone(i.Hostname), strings.Clone(i.UUID)
	if i.Agent != nil {
		a := *i.Agent
		a.AgentVersion, a.PackageVersion = strings.Clone(a.AgentVersion), strings.Clone(a.PackageVersion)
		a.Flavor, a.InfrastructureMode = strings.Clone(a.Flavor), strings.Clone(a.InfrastructureMode)
		out.Agent = &a
	}
	if i.Host != nil {
		h := *i.Host
		for _, value := range hostInventoryStrings(&h) {
			*value = strings.Clone(*value)
		}
		out.Host = &h
	}
	if i.SystemInfo != nil {
		h := *i.SystemInfo
		for _, value := range hostSystemInfoStrings(&h) {
			*value = strings.Clone(*value)
		}
		out.SystemInfo = &h
	}
	return &out
}

func hostSystemInfoStrings(h *HostSystemInfoMetadata) [6]*string {
	return [6]*string{&h.Manufacturer, &h.ModelNumber, &h.SerialNumber, &h.ModelName, &h.ChassisType, &h.Identifier}
}

func hostInventoryStrings(h *HostInventoryMetadata) [14]*string {
	return [14]*string{&h.CPUVendor, &h.CPUModel, &h.CPUModelID, &h.CPUFamily, &h.CPUStepping,
		&h.KernelName, &h.KernelRelease, &h.OS, &h.OSVersion, &h.CPUArchitecture,
		&h.IPAddress, &h.IPv6Address, &h.MacAddress, &h.AgentVersion}
}
