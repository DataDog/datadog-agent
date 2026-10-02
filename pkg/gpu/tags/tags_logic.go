// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package tags

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	amdPCIVendorID = "0x1002"
	amdgpuDriver   = "amdgpu"
)

var (
	// drmCardRegex matches DRM card nodes (card0, card12) but not connectors (card0-DP-1).
	drmCardRegex = regexp.MustCompile(`^card[0-9]+$`)
	// pciAddressRegex matches a PCI function address such as 0000:c1:00.0.
	pciAddressRegex = regexp.MustCompile(`^[0-9a-f]{4,8}:[0-9a-f]{2}:[0-9a-f]{2}\.[0-7]$`)
)

// getTags returns a slice of tags indicating GPU presence
func getTags() []string {
	if hasNvidiaGPU() || hasAMDGPU() {
		return []string{"gpu_host:true"}
	}

	return nil
}

// hasNvidiaGPU reports whether the NVIDIA driver lists at least one GPU.
func hasNvidiaGPU() bool {
	// Get the host's proc directory path
	procPath := kernel.ProcFSRoot()
	nvidiaPath := filepath.Join(procPath, "driver", "nvidia", "gpus")

	// Check if the NVIDIA directory exists
	if _, err := os.Stat(nvidiaPath); err != nil {
		if os.IsNotExist(err) {
			return false
		}
		log.Warnf("Failed to check NVIDIA GPU directory: %v", err)
		return false
	}

	// Read the directory to count GPU entries
	entries, err := os.ReadDir(nvidiaPath)
	if err != nil {
		log.Warnf("Failed to read NVIDIA GPU directory: %v", err)
		return false
	}

	// If we have at least one entry, we have a GPU
	return len(entries) > 0
}

// hasAMDGPU reports whether the host has an AMD GPU bound to the amdgpu kernel
// driver. It looks at the DRM cards in sysfs, as the GPU check does to discover
// AMD GPUs (pkg/gpu/amd), and needs neither ROCm nor the compute (KFD)
// interface. It is kept separate from that package so that computing host tags
// does not link it into every binary.
//
// This runs on every Linux host, so a sysfs that cannot be read is only logged
// at debug level.
func hasAMDGPU() bool {
	drmDir := filepath.Join(kernel.SysFSRoot(), "class", "drm")
	entries, err := os.ReadDir(drmDir)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Debugf("Failed to read DRM directory: %v", err)
		}
		return false
	}

	for _, entry := range entries {
		if !drmCardRegex.MatchString(entry.Name()) {
			continue
		}
		device, err := filepath.EvalSymlinks(filepath.Join(drmDir, entry.Name(), "device"))
		if err != nil {
			continue
		}
		// Compute partitions (amdgpu_xcp_*) are platform devices, not PCI functions.
		if !pciAddressRegex.MatchString(strings.ToLower(filepath.Base(device))) {
			continue
		}
		vendor, err := os.ReadFile(filepath.Join(device, "vendor"))
		if err != nil || strings.TrimSpace(string(vendor)) != amdPCIVendorID {
			continue
		}
		if boundToAmdgpu(device) {
			return true
		}
	}

	return false
}

// boundToAmdgpu reports whether the PCI device at devicePath is bound to the
// amdgpu driver, from its driver link or, when absent, its uevent file.
func boundToAmdgpu(devicePath string) bool {
	if target, err := os.Readlink(filepath.Join(devicePath, "driver")); err == nil {
		return filepath.Base(target) == amdgpuDriver
	}

	content, err := os.ReadFile(filepath.Join(devicePath, "uevent"))
	if err != nil {
		return false
	}
	for line := range strings.Lines(string(content)) {
		if strings.TrimSpace(line) == "DRIVER="+amdgpuDriver {
			return true
		}
	}

	return false
}
