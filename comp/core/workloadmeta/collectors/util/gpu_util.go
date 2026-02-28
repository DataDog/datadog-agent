// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config/env"
)

// gpuUUIDRegex matches valid NVIDIA GPU and MIG device UUID formats.
//
// Supported formats:
//   - GPU UUID: GPU-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (physical GPU)
//   - MIG UUID (modern): MIG-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
//   - MIG UUID (legacy): MIG-GPU-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx/gi/ci
//
// The UUID portion follows the standard 8-4-4-4-12 hexadecimal format.
//
// References:
//   - NVML API (nvmlDeviceGetUUID): https://docs.nvidia.com/deploy/nvml-api/group__nvmlDeviceQueries.html
//   - NVIDIA Container Toolkit: https://github.com/NVIDIA/nvidia-container-toolkit/blob/main/cmd/nvidia-container-runtime/README.md
//   - MIG User Guide: https://docs.nvidia.com/datacenter/tesla/mig-user-guide/
//
// This regex is used to validate that NVIDIA_VISIBLE_DEVICES contains actual GPU UUIDs
// set by the NVIDIA device plugin, rather than user overrides like "all", "none", or indices.
var gpuUUIDRegex = regexp.MustCompile(
	`^(?:` +
		`GPU-[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}|` + // Physical GPU
		`MIG-[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}|` + // Modern MIG format
		`MIG-GPU-[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}/\d+/\d+` + // Legacy MIG format
		`)$`,
)

// IsGPUUUID returns true if the value is a valid NVIDIA GPU or MIG device UUID.
// This distinguishes device plugin-assigned UUIDs from user overrides like "all", "none", or indices.
func IsGPUUUID(value string) bool {
	return gpuUUIDRegex.MatchString(value)
}

// areAllValidGPUUUIDs returns true if all values in the slice are valid GPU UUIDs.
// Returns false if the slice is empty or contains any non-UUID value (e.g., "all", "none", "0").
//
// This is used to detect user overrides of NVIDIA_VISIBLE_DEVICES. The NVIDIA device plugin
// always returns a list of UUIDs, never mixed with special values. If we see any non-UUID value,
// it indicates user override and we should fall back to PodResources API.
func areAllValidGPUUUIDs(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if !IsGPUUUID(v) {
			return false
		}
	}
	return true
}

// NVIDIAVisibleDevicesEnvVar is the environment variable set by NVIDIA container runtime
// or Kubernetes device plugin to specify which GPUs are visible to the container.
// Values can be GPU UUIDs (e.g., "GPU-uuid1,GPU-uuid2"), special values like "all", "none", "void",
// or MIG instance identifiers.
const NVIDIAVisibleDevicesEnvVar = "NVIDIA_VISIBLE_DEVICES"

// ExtractGPUDeviceIDs parses GPU device identifiers from NVIDIA_VISIBLE_DEVICES environment variable.
// This is the core parsing function that extracts GPU IDs from a list of environment variables.
//
// Returns:
//   - GPU UUIDs as a slice (e.g., ["GPU-uuid1", "GPU-uuid2"])
//   - Special values like ["all"], ["none"], ["void"] are preserved
//   - nil if the env var is not set or has an empty value
func ExtractGPUDeviceIDs(envVars []string) []string {
	prefix := NVIDIAVisibleDevicesEnvVar + "="
	for _, e := range envVars {
		if value, found := strings.CutPrefix(e, prefix); found {
			if value == "" {
				return nil
			}
			return strings.Split(value, ",")
		}
	}
	return nil
}

// ShouldExtractGPUDeviceIDsFromConfig returns true if GPU device IDs should be extracted
// from container config/spec based on the current environment.
//
// Supported environments:
//   - ECS: env var set by ECS agent in task definition
//   - Kubernetes (non-GKE): env var set by NVIDIA device plugin via Allocate() API
//
// Not supported (returns false):
//   - Docker standalone: env var injected at runtime by nvidia-container-toolkit, not in config
//   - Standalone containerd: env var may be injected at runtime, not in spec
//   - GKE: uses custom device plugin + gVisor runtime that ignores NVIDIA_VISIBLE_DEVICES
//
// Note: GKE is not explicitly detected here. On GKE, the env var is either not set in a useful way
// or ignored by the runtime, so extraction returns nil and falls back to PodResources API.
func ShouldExtractGPUDeviceIDsFromConfig() bool {
	return env.IsECS() || env.IsKubernetes()
}

// ExtractGPUDeviceIDsFromEnvVars extracts GPU device IDs from environment variables
// if the current environment supports it. This combines the environment check
// with the extraction logic for convenience.
//
// In Kubernetes environments, this function validates that all extracted values are valid GPU UUIDs.
// If any non-UUID value is detected (e.g., "all", "none", "0"), it returns nil to indicate
// a potential user override, allowing the caller to fall back to PodResources API.
//
// Use this function when you have a list of environment variable strings (e.g., from container config).
func ExtractGPUDeviceIDsFromEnvVars(envVars []string) []string {
	if !ShouldExtractGPUDeviceIDsFromConfig() {
		return nil
	}
	ids := ExtractGPUDeviceIDs(envVars)
	// In Kubernetes, validate UUIDs to detect user overrides
	// ECS is excluded because users cannot override env vars set by ECS agent
	if env.IsKubernetes() && !areAllValidGPUUUIDs(ids) {
		return nil
	}
	return ids
}

// ExtractGPUDeviceIDsFromEnvMap extracts GPU device IDs from an environment variable map
// if the current environment supports it.
//
// In Kubernetes environments, this function validates that all extracted values are valid GPU UUIDs.
// If any non-UUID value is detected (e.g., "all", "none", "0"), it returns nil to indicate
// a potential user override, allowing the caller to fall back to PodResources API.
//
// Use this function when you have a map of environment variables (e.g., from containerd spec parsing).
func ExtractGPUDeviceIDsFromEnvMap(envs map[string]string) []string {
	if !ShouldExtractGPUDeviceIDsFromConfig() {
		return nil
	}
	if val, ok := envs[NVIDIAVisibleDevicesEnvVar]; ok && val != "" {
		ids := strings.Split(val, ",")
		// In Kubernetes, validate UUIDs to detect user overrides
		// ECS is excluded because users cannot override env vars set by ECS agent
		if env.IsKubernetes() && !areAllValidGPUUUIDs(ids) {
			return nil
		}
		return ids
	}
	return nil
}
