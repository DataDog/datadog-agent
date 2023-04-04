// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package inventories

import (
	"github.com/DataDog/gohai/cpu"
	"github.com/DataDog/gohai/memory"
	"github.com/DataDog/gohai/network"
	"github.com/DataDog/gohai/platform"

	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// for testing purpose
var (
	cpuGet      = cpu.Get
	memoryGet   = memory.Get
	networkGet  = network.Get
	platformGet = platform.Get
)

// HostMetadata contains metadata about the host
type HostMetadata struct {
	// from gohai/cpu
	CPUCores             uint64  `json:"cpu_cores"`
	CPULogicalProcessors uint64  `json:"cpu_logical_processors"`
	CPUVendor            string  `json:"cpu_vendor"`
	CPUModel             string  `json:"cpu_model"`
	CPUModelID           string  `json:"cpu_model_id"`
	CPUFamily            string  `json:"cpu_family"`
	CPUStepping          string  `json:"cpu_stepping"`
	CPUFrequency         float64 `json:"cpu_frequency"`
	CPUCacheSize         uint64  `json:"cpu_cache_size"`

	// from gohai/platform
	KernelName      string `json:"kernel_name"`
	KernelRelease   string `json:"kernel_release"`
	KernelVersion   string `json:"kernel_version"`
	OS              string `json:"os"`
	CPUArchitecture string `json:"cpu_architecture"`

	// from gohai/memory
	MemoryTotalKb     uint64 `json:"memory_total_kb"`
	MemorySwapTotalKb uint64 `json:"memory_swap_total_kb"`

	// from gohai/network
	IPAddress   string `json:"ip_address"`
	IPv6Address string `json:"ipv6_address"`
	MacAddress  string `json:"mac_address"`

	// from the agent itself
	AgentVersion        string `json:"agent_version"`
	CloudProvider       string `json:"cloud_provider"`
	CloudProviderSource string `json:"cloud_provider_source"`
	OsVersion           string `json:"os_version"`

	// From file system
	HypervisorGuestUUID string `json:"hypervisor_guest_uuid"`
	DmiProductUUID      string `json:"dmi_product_uuid"`
	DmiBoardAssetTag    string `json:"dmi_board_asset_tag"`
	DmiBoardVendor      string `json:"dmi_board_vendor"`
}

// For now we simply logs warnings from gohai.
func logWarnings(warnings []string) {
	for _, w := range warnings {
		logInfof("gohai: %s", w)
	}
}

func fetchFromMetadata(key string, metadata AgentMetadata) string {
	if value, ok := metadata[key]; ok {
		if stringValue, ok := value.(string); ok {
			return stringValue
		}
		logErrorf("'%s' is not a string in metadata cache", key) //nolint:errcheck
		return ""
	}
	logInfof("'%s' not found in metadata cache", key)
	return ""
}

// getHostMetadata returns the metadata show in the 'host' table
func getHostMetadata() *HostMetadata {
	metadata := &HostMetadata{}

	cpuInfo, warnings, err := cpuGet()
	if err != nil {
		logErrorf("Failed to retrieve cpu metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		metadata.CPUCores = cpuInfo.CpuCores
		metadata.CPULogicalProcessors = cpuInfo.CpuLogicalProcessors
		metadata.CPUVendor = cpuInfo.VendorId
		metadata.CPUModel = cpuInfo.ModelName
		metadata.CPUModelID = cpuInfo.Model
		metadata.CPUFamily = cpuInfo.Family
		metadata.CPUStepping = cpuInfo.Stepping
		metadata.CPUFrequency = cpuInfo.Mhz
		metadata.CPUCacheSize = cpuInfo.CacheSizeBytes
	}

	platformInfo, warnings, err := platformGet()
	if err != nil {
		logErrorf("failed to retrieve host platform metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		metadata.KernelName = platformInfo.KernelName
		metadata.KernelRelease = platformInfo.KernelRelease
		metadata.KernelVersion = platformInfo.KernelVersion
		metadata.OS = platformInfo.OS
		metadata.CPUArchitecture = platformInfo.HardwarePlatform
	}

	memoryInfo, warnings, err := memoryGet()
	if err != nil {
		logErrorf("failed to retrieve host memory metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		metadata.MemoryTotalKb = memoryInfo.TotalBytes / 1024
		metadata.MemorySwapTotalKb = memoryInfo.SwapTotalBytes / 1024
	}

	networkInfo, warnings, err := networkGet()
	if err != nil {
		logErrorf("failed to retrieve host network metadata from gohai: %s", err) //nolint:errcheck
	} else {
		logWarnings(warnings)

		metadata.IPAddress = networkInfo.IpAddress
		metadata.IPv6Address = networkInfo.IpAddressv6
		metadata.MacAddress = networkInfo.MacAddress
	}

	metadata.AgentVersion = version.AgentVersion

	metadata.CloudProvider = fetchFromMetadata(string(HostCloudProvider), agentMetadata)
	metadata.CloudProviderSource = fetchFromMetadata(string(HostCloudProviderSource), hostMetadata)
	metadata.OsVersion = fetchFromMetadata(string(HostOSVersion), hostMetadata)

	metadata.HypervisorGuestUUID = dmi.GetHypervisorUUID()
	metadata.DmiProductUUID = dmi.GetProductUUID()
	metadata.DmiBoardAssetTag = dmi.GetBoardAssetTag()
	metadata.DmiBoardVendor = dmi.GetBoardVendor()

	return metadata
}
