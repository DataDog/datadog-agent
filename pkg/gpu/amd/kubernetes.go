// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package amd

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ResourcePrefix is the namespace of the Kubernetes extended resources that
// the AMD GPU device plugin (ROCm/k8s-device-plugin) advertises: amd.com/gpu,
// or one resource per partition type such as amd.com/cpx_nps4 with the
// "mixed" resource naming strategy.
const ResourcePrefix = "amd.com/"

// xcpDeviceRegex matches the device plugin ID of a compute partition, which is
// the name of its platform device (/sys/devices/platform/amdgpu_xcp_<N>).
var xcpDeviceRegex = regexp.MustCompile(`^amdgpu_xcp_[0-9]+$`)

// MatchDevicePluginID returns the physical GPU that a device ID allocated by
// the AMD GPU device plugin refers to, as reported by the kubelet PodResources
// API. The plugin uses the PCI address of the GPU (e.g. 0000:19:00.0) for
// unpartitioned GPUs and the partition's platform device name (e.g.
// amdgpu_xcp_3) for compute partitions. A partition is mapped to its GPU
// through its DRM render node and the KFD topology. It returns nil if the ID
// is not one of the given devices.
func MatchDevicePluginID(sysRoot string, devices []*Device, id string) *Device {
	id = strings.ToLower(strings.TrimSpace(id))
	if pciAddressRegex.MatchString(id) {
		for _, dev := range devices {
			if dev.PCIBusID == id {
				return dev
			}
		}
		return nil
	}

	if !xcpDeviceRegex.MatchString(id) {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(sysRoot, "devices", "platform", id, "drm"))
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		minorStr, ok := strings.CutPrefix(entry.Name(), "renderD")
		if !ok {
			continue
		}
		minor, err := strconv.Atoi(minorStr)
		if err != nil {
			continue
		}
		for _, dev := range devices {
			if slices.Contains(dev.renderMinors, minor) {
				return dev
			}
		}
	}
	return nil
}
