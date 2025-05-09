// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package gpu provides utilities for interacting with GPU resources.
package gpu

import "strings"

// ResourceGPU represents a GPU resource
type ResourceGPU string

// Resource name prefixes
const (
	// GpuNvidiaGeneric is the resource name for a generic NVIDIA GPU
	GpuNvidiaGeneric ResourceGPU = "nvidia.com/gpu"
	// GpuAMD is the resource name for an AMD GPU
	GpuAMD ResourceGPU = "amd.com/gpu"
	// GpuIntelXe is the resource name for an Intel Xe GPU
	GpuIntelXe ResourceGPU = "gpu.intel.com/xe"
	// GpuInteli915 is the resource name for an Intel i915 GPU
	GpuInteli915 ResourceGPU = "gpu.intel.com/i915"
	// GpuNvidiaMigPrefix is the prefix for NVIDIA MIG GPU resources
	GpuNvidiaMigPrefix ResourceGPU = "nvidia.com/mig"
)

// longToShortGPUName maps a GPU resource to a simplified name
var longToShortGPUName = map[ResourceGPU]string{
	GpuNvidiaGeneric: "nvidia",
	GpuAMD:           "amd",
	GpuIntelXe:       "intel",
	GpuInteli915:     "intel",
}

// ExtractSimpleGPUName returns a simplified GPU name.
// If the resource is not recognized, the second return value is false.
func ExtractSimpleGPUName(gpuName ResourceGPU) (string, bool) {
	val, ok := longToShortGPUName[gpuName]
	if ok {
		return val, true
	}

	// More complex cases (eg. nvidia.com/mig-3g.20gb => nvidia)
	switch {
	case strings.HasPrefix(string(gpuName), string(GpuNvidiaMigPrefix)):
		return "nvidia", true
	}

	// Not a GPU resource (or not recognized)
	return "", false
}

// IsNvidiaKubernetesResource returns true if the resource name is a Kubernetes resource
// for an NVIDIA GPU, either a generic GPU or a MIG GPU.
func IsNvidiaKubernetesResource(resourceName string) bool {
	return strings.HasPrefix(resourceName, string(GpuNvidiaMigPrefix)) ||
		resourceName == string(GpuNvidiaGeneric)
}
