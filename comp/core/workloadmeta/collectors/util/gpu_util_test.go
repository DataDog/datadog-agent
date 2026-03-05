// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// envMapToSlice converts a map of env vars to slice format (KEY=VALUE)
func envMapToSlice(envs map[string]string) []string {
	if envs == nil {
		return nil
	}
	result := make([]string, 0, len(envs))
	for k, v := range envs {
		result = append(result, k+"="+v)
	}
	return result
}

// TestIsGPUUUID tests the GPU UUID validation regex
func TestIsGPUUUID(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		// Valid GPU UUIDs
		{name: "valid GPU UUID", value: "GPU-aec058b1-c18e-236e-c14d-49d2990fda0f", expected: true},
		{name: "valid GPU UUID uppercase", value: "GPU-AEC058B1-C18E-236E-C14D-49D2990FDA0F", expected: true},
		{name: "valid GPU UUID mixed case", value: "GPU-Aec058b1-C18e-236E-c14d-49D2990fda0F", expected: true},
		// Valid MIG UUIDs (modern format)
		{name: "valid MIG UUID", value: "MIG-aec058b1-c18e-236e-c14d-49d2990fda0f", expected: true},
		// Valid MIG UUIDs (legacy format)
		{name: "valid MIG legacy format", value: "MIG-GPU-aec058b1-c18e-236e-c14d-49d2990fda0f/0/0", expected: true},
		{name: "valid MIG legacy format multi-digit", value: "MIG-GPU-aec058b1-c18e-236e-c14d-49d2990fda0f/1/3", expected: true},
		// Invalid values (user overrides)
		{name: "special value all", value: "all", expected: false},
		{name: "special value none", value: "none", expected: false},
		{name: "special value void", value: "void", expected: false},
		{name: "device index 0", value: "0", expected: false},
		{name: "device index 1", value: "1", expected: false},
		{name: "device indices comma-separated", value: "0,1,2", expected: false},
		{name: "short invalid UUID", value: "GPU-xxx", expected: false},
		{name: "invalid format no prefix", value: "aec058b1-c18e-236e-c14d-49d2990fda0f", expected: false},
		{name: "invalid format wrong prefix", value: "CUDA-aec058b1-c18e-236e-c14d-49d2990fda0f", expected: false},
		{name: "empty string", value: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsGPUUUID(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExtractGPUDeviceIDs tests ExtractGPUDeviceIDsFromEnvVars and ExtractGPUDeviceIDsFromEnvMap
// with the same test cases, ensuring consistent behavior across different environments.
func TestExtractGPUDeviceIDs(t *testing.T) {
	// Valid GPU UUIDs for testing (using proper 8-4-4-4-12 format)
	validGPU1 := "GPU-aec058b1-c18e-236e-c14d-49d2990fda0f"
	validGPU2 := "GPU-b8ea3855-276c-c9cb-b366-c6fa655957c5"
	validGPU3 := "GPU-00cc6634-6a30-b312-2dc9-731a61cb17a9"
	validMIG := "MIG-aec058b1-c18e-236e-c14d-49d2990fda0f"

	tests := []struct {
		name     string
		envMap   map[string]string
		isECS    bool
		isK8s    bool
		expected []string
	}{
		// Environment detection tests
		{
			name:     "ECS extracts GPU",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": validGPU1},
			isECS:    true,
			expected: []string{validGPU1},
		},
		{
			name:     "Kubernetes extracts GPU",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": validGPU1},
			isK8s:    true,
			expected: []string{validGPU1},
		},
		{
			name:     "Standalone returns nil",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": validGPU1},
			expected: nil,
		},
		// Parsing tests (in K8s environment)
		{
			name:     "single GPU UUID",
			envMap:   map[string]string{"PATH": "/usr/bin", "NVIDIA_VISIBLE_DEVICES": validGPU1},
			isK8s:    true,
			expected: []string{validGPU1},
		},
		{
			name:     "multiple GPU UUIDs",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": validGPU1 + "," + validGPU2 + "," + validGPU3},
			isK8s:    true,
			expected: []string{validGPU1, validGPU2, validGPU3},
		},
		{
			name:     "MIG UUID (modern format)",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": validMIG},
			isK8s:    true,
			expected: []string{validMIG},
		},
		// User override detection in Kubernetes (should return nil to trigger fallback)
		{
			name:     "K8s user override - all",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "all"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		{
			name:     "K8s user override - none",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "none"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		{
			name:     "K8s user override - void",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "void"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		{
			name:     "K8s user override - device index",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "0"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		{
			name:     "K8s user override - device indices",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "0,1"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		{
			name:     "K8s user override - short invalid UUID",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "GPU-xxx"},
			isK8s:    true,
			expected: nil, // Invalid UUID, fall back to PodResources API
		},
		// ECS does NOT validate UUIDs (users can't override env vars set by ECS agent)
		{
			name:     "ECS allows all (ECS agent sets this)",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "all"},
			isECS:    true,
			expected: []string{"all"}, // ECS doesn't validate
		},
		{
			name:     "ECS allows index (edge case)",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": "0"},
			isECS:    true,
			expected: []string{"0"}, // ECS doesn't validate
		},
		// Edge cases
		{
			name:     "empty value",
			envMap:   map[string]string{"NVIDIA_VISIBLE_DEVICES": ""},
			isK8s:    true,
			expected: nil,
		},
		{
			name:     "no NVIDIA_VISIBLE_DEVICES",
			envMap:   map[string]string{"PATH": "/usr/bin"},
			isK8s:    true,
			expected: nil,
		},
		{
			name:     "nil map",
			envMap:   nil,
			isK8s:    true,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment detection vars first (CI may have them set)
			// t.Setenv with empty string effectively clears the var for this test
			t.Setenv("KUBERNETES_SERVICE_PORT", "")
			t.Setenv("KUBERNETES_SERVICE_HOST", "")
			t.Setenv("ECS_CONTAINER_METADATA_URI_V4", "")
			t.Setenv("ECS_CONTAINER_METADATA_URI", "")

			// Now set the specific environment for this test case
			if tt.isECS {
				t.Setenv("ECS_CONTAINER_METADATA_URI_V4", "http://169.254.170.2/v4")
			}
			if tt.isK8s {
				t.Setenv("KUBERNETES_SERVICE_PORT", "443")
			}

			// Test map-based function (used by containerd)
			resultMap := ExtractGPUDeviceIDsFromEnvMap(tt.envMap)
			assert.Equal(t, tt.expected, resultMap, "ExtractGPUDeviceIDsFromEnvMap")

			// Test slice-based function (used by docker)
			envSlice := envMapToSlice(tt.envMap)
			resultSlice := ExtractGPUDeviceIDsFromEnvVars(envSlice)
			assert.Equal(t, tt.expected, resultSlice, "ExtractGPUDeviceIDsFromEnvVars")
		})
	}
}
