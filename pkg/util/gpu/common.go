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
	gpuNvidiaGeneric ResourceGPU = "nvidia.com/gpu"
	gpuAMD           ResourceGPU = "amd.com/gpu"
	gpuIntelXe       ResourceGPU = "gpu.intel.com/xe"
	gpuInteli915     ResourceGPU = "gpu.intel.com/i915"

	gpuNvidiaMigPrefix ResourceGPU = "nvidia.com/mig"
)

// longToShortGPUName maps a GPU resource to a simplified name
var longToShortGPUName = map[ResourceGPU]string{
	gpuNvidiaGeneric: "nvidia",
	gpuAMD:           "amd",
	gpuIntelXe:       "intel",
	gpuInteli915:     "intel",
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
	case strings.HasPrefix(string(gpuName), string(gpuNvidiaMigPrefix)):
		return "nvidia", true
	}

	// Not a GPU resource (or not recognized)
	return "", false
}
