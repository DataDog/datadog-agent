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

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const noUsableGPUSkipMessage = "NVIDIA GPU detected but NVML or device access is unavailable on this system, skipping test"

// RequireGPU skips the test if no usable NVIDIA GPU environment is detected on the system.
// A usable environment means that a GPU is present and NVML can enumerate at least one device.
func RequireGPU(t *testing.T) {
	t.Helper()

	hasGPU, hasUsableGPU := detectGPUEnvironment()
	if !hasGPU {
		t.Skip("No NVIDIA GPU detected on this system, skipping test")
	}
	if !hasUsableGPU {
		t.Skip(noUsableGPUSkipMessage)
	}
}

// HasGPU returns true if at least one NVIDIA GPU is detected on the system.
// Detection prefers lightweight procfs/sysfs checks and does not require NVML.
func HasGPU() bool {
	hasGPU, _ := detectGPUEnvironment()
	return hasGPU
}

func detectGPUEnvironment() (hasGPU bool, hasUsableGPU bool) {
	if hasGPUProcFS() || hasGPUSysFS() {
		hasGPU = true
	}

	if hasUsableNVMLGPU() {
		return true, true
	}

	return hasGPU, false
}

func hasGPUProcFS() bool {
	procPath := kernel.ProcFSRoot()
	nvidiaPath := filepath.Join(procPath, "driver", "nvidia", "gpus")

	info, err := os.Stat(nvidiaPath)
	if err != nil || !info.IsDir() {
		return false
	}

	entries, err := os.ReadDir(nvidiaPath)
	if err != nil {
		return false
	}

	return len(entries) > 0
}

func hasGPUSysFS() bool {
	sysPath := kernel.SysFSRoot()
	vendorPaths, err := filepath.Glob(filepath.Join(sysPath, "bus", "pci", "devices", "*", "vendor"))
	if err != nil {
		return false
	}

	for _, vendorPath := range vendorPaths {
		vendor, err := os.ReadFile(vendorPath)
		if err != nil {
			continue
		}
		if string(vendor) == "0x10de\n" || string(vendor) == "0x10de" {
			return true
		}
	}

	return false
}

func hasUsableNVMLGPU() bool {
	for _, libPath := range []string{
		"",
		"/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",
		"/run/nvidia/driver/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1",
	} {
		lib := nvml.New(nvml.WithLibraryPath(libPath))
		if lib == nil {
			continue
		}

		ret := lib.Init()
		if ret != nvml.SUCCESS && ret != nvml.ERROR_ALREADY_INITIALIZED {
			continue
		}

		count, countRet := lib.DeviceGetCount()
		if ret == nvml.SUCCESS {
			_ = lib.Shutdown()
		}

		if countRet == nvml.SUCCESS && count > 0 {
			return true
		}
	}

	return false
}
