// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractSimpleGPUName(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  ResourceGPU
		found    bool
		expected string
	}{
		{
			name:     "nvidia generic",
			gpuName:  GpuNvidiaGeneric,
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "amd",
			gpuName:  GpuAMD,
			found:    true,
			expected: "amd",
		},
		{
			name:     "intel xe",
			gpuName:  GpuIntelXe,
			found:    true,
			expected: "intel",
		},
		{
			name:     "intel i915",
			gpuName:  GpuInteli915,
			found:    true,
			expected: "intel",
		},
		{
			name:     "nvidia MIG resource",
			gpuName:  ResourceGPU("nvidia.com/mig-3g.20gb"),
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "nvidia MIG different profile",
			gpuName:  ResourceGPU("nvidia.com/mig-1g.5gb"),
			found:    true,
			expected: "nvidia",
		},
		{
			name:     "unknown resource",
			gpuName:  ResourceGPU("cpu"),
			found:    false,
			expected: "",
		},
		{
			name:     "unrecognized gpu vendor",
			gpuName:  ResourceGPU("example.com/gpu"),
			found:    false,
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, found := ExtractSimpleGPUName(test.gpuName)
			assert.Equal(t, test.found, found)
			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestIsNvidiaKubernetesResource(t *testing.T) {
	tests := []struct {
		name     string
		resource string
		expected bool
	}{
		{
			name:     "nvidia generic GPU",
			resource: "nvidia.com/gpu",
			expected: true,
		},
		{
			name:     "nvidia MIG GPU",
			resource: "nvidia.com/mig-3g.20gb",
			expected: true,
		},
		{
			name:     "AMD GPU is not nvidia",
			resource: "amd.com/gpu",
			expected: false,
		},
		{
			name:     "CPU resource is not nvidia GPU",
			resource: "cpu",
			expected: false,
		},
		{
			name:     "intel GPU is not nvidia",
			resource: "gpu.intel.com/xe",
			expected: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, IsNvidiaKubernetesResource(test.resource))
		})
	}
}
