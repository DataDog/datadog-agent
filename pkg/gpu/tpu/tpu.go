// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tpu detects Google Cloud TPU chips. It has no NVML code, so that host
// tags can use it without linking NVML into every binary.
package tpu

import (
	"os"
	"path/filepath"
	"strings"
)

// pciVendorGoogle is the PCI vendor ID of Google, which also covers its NICs and disks.
const pciVendorGoogle = "0x1ae0"

// tpuGenerations maps the PCI device IDs of Cloud TPU chips, as listed by Google's
// tpu-info tool (AI-Hypercomputer/cloud-accelerator-diagnostics, tpu_info/device.py),
// to their generation.
var tpuGenerations = map[string]string{
	"0x0027": "tpu-v3", // also v2, told apart by the subsystem device below
	"0x005e": "tpu-v4",
	"0x0062": "tpu-v5p",
	"0x0063": "tpu-v5e",
	"0x006f": "tpu-v6e",
	"0x0076": "tpu7x",
}

// HasTPU reports whether the PCI devices under sysRoot include a Cloud TPU chip.
func HasTPU(sysRoot string) bool {
	return Generation(sysRoot) != ""
}

// Generation returns the generation of the first Cloud TPU chip under sysRoot, such
// as "tpu-v6e", or "" when there is none. sysfs that cannot be read counts as no TPU.
func Generation(sysRoot string) string {
	pciDir := filepath.Join(sysRoot, "bus", "pci", "devices")
	entries, err := os.ReadDir(pciDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if readPCIID(filepath.Join(pciDir, entry.Name(), "vendor")) != pciVendorGoogle {
			continue
		}
		device := filepath.Join(pciDir, entry.Name())
		if gen, ok := tpuGenerations[readPCIID(filepath.Join(device, "device"))]; ok {
			if gen == "tpu-v3" && readPCIID(filepath.Join(device, "subsystem_device")) == "0x004e" {
				return "tpu-v2"
			}
			return gen
		}
	}
	return ""
}

// readPCIID returns a PCI ID file such as "0x1ae0\n" as "0x1ae0", or "" on error.
func readPCIID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(string(data)))
}
