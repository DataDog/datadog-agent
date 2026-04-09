// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && test

package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// RequireGPU skips the test if no NVIDIA GPUs are detected on the system.
// Detection is done by checking /proc/driver/nvidia/gpus without using NVML.
func RequireGPU(t *testing.T) {
	t.Helper()

	if !HasGPU() {
		t.Skip("No NVIDIA GPU detected on this system, skipping test")
	}
}

// HasGPU returns true if at least one NVIDIA GPU is detected on the system.
// Detection is done by checking /proc/driver/nvidia/gpus without using NVML.
func HasGPU() bool {
	procPath := kernel.ProcFSRoot()
	nvidiaPath := filepath.Join(procPath, "driver", "nvidia", "gpus")

	// Check if the NVIDIA directory exists
	info, err := os.Stat(nvidiaPath)
	if err != nil || !info.IsDir() {
		return false
	}

	// Read the directory to count GPU entries
	entries, err := os.ReadDir(nvidiaPath)
	if err != nil {
		return false
	}

	return len(entries) > 0
}
