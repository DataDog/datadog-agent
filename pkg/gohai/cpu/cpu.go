// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright © 2015 Kentaro Kuribayashi <kentarok@gmail.com>
// Copyright 2014-present Datadog, Inc.

// Package cpu regroups collecting information about the CPU
package cpu

import (
	"github.com/DataDog/datadog-agent/pkg/gohai/utils"
)

// Info holds metadata about the host CPU
type Info struct {
	// VendorID the CPU vendor ID
	VendorID utils.Value[string] `json:"vendor_id"`
	// ModelName the CPU model
	ModelName utils.Value[string] `json:"model_name"`
	// CPUCores the number of cores for the CPU
	CPUCores utils.Value[uint64] `json:"cpu_cores"`
	// CPULogicalProcessors the number of logical core for the CPU
	CPULogicalProcessors utils.Value[uint64] `json:"cpu_logical_processors"`
	// Mhz the frequency for the CPU (Not available on ARM)
	Mhz utils.Value[float64] `json:"mhz"`
	// CacheSizeKB the cache size for the CPU in KB (Linux only)
	CacheSizeKB utils.Value[uint64] `json:"cache_size" unit:" KB"`
	// Family the CPU family
	Family utils.Value[string] `json:"family"`
	// Model the CPU model name
	Model utils.Value[string] `json:"model"`
	// Stepping the CPU stepping
	Stepping utils.Value[string] `json:"stepping"`

	// CPUPkgs the CPU pkg count (Windows and Linux ARM64 only)
	CPUPkgs utils.Value[uint64] `json:"cpu_pkgs"`
	// CPUNumaNodes the CPU numa node count (Windows and Linux ARM64 only)
	CPUNumaNodes utils.Value[uint64] `json:"cpu_numa_nodes"`
	// CacheSizeL1Bytes the CPU L1 cache size (Windows and Linux ARM64 only)
	CacheSizeL1Bytes utils.Value[uint64] `json:"cache_size_l1"`
	// CacheSizeL2Bytes the CPU L2 cache size (Windows and Linux ARM64 only)
	CacheSizeL2Bytes utils.Value[uint64] `json:"cache_size_l2"`
	// CacheSizeL3 the CPU L3 cache size (Windows and Linux ARM64 only)
	CacheSizeL3Bytes utils.Value[uint64] `json:"cache_size_l3"`
}

// CollectInfo returns an Info struct with every field initialized either to a value or an error.
// The method will try to collect as many fields as possible.
func CollectInfo() *Info {
	return getCPUInfo()
}

// AsJSON returns an interface which can be marshalled to a JSON and contains the value of non-errored fields.
func (info *Info) AsJSON() (interface{}, []string, error) {
	return utils.AsJSON(info, false)
}
