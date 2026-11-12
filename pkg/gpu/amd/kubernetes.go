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

	"go.yaml.in/yaml/v3"
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

const (
	// DRADriverName is the Dynamic Resource Allocation driver of AMD GPUs
	// (ROCm/k8s-gpu-dra-driver), the resource name kubelet reports for the
	// devices it allocates.
	DRADriverName = "gpu.amd.com"

	// draCDIDevicePrefix starts the CDI device names of AMD DRA allocations:
	// "k8s.gpu.amd.com/gpu=<claim UID>-<device name>".
	draCDIDevicePrefix = "k8s.gpu.amd.com/gpu="
	// draCDISpecPrefix starts the file name of the CDI spec of a claim:
	// "k8s.gpu.amd.com-gpu_<claim UID>.yaml".
	draCDISpecPrefix = "k8s.gpu.amd.com-gpu_"
	// claimUIDLength is the length of a Kubernetes UID.
	claimUIDLength = 36
)

var (
	// draDeviceNameRegex matches the DRA device name of a GPU or partition,
	// "gpu-<card minor>-<render minor>".
	draDeviceNameRegex = regexp.MustCompile(`^gpu-[0-9]+-([0-9]+)$`)
	// renderNodeRegex matches a DRM render node path and captures its minor.
	renderNodeRegex = regexp.MustCompile(`^/dev/dri/renderD([0-9]+)$`)
)

// MatchDRADevice returns the device allocated through the AMD DRA driver with
// the given device name and CDI devices, or nil. The device name carries the
// render node of the GPU or partition; for other names (such as the
// partitions the driver creates itself), the render node is read from the CDI
// spec of the claim, in cdiSpecDirs.
func MatchDRADevice(sysRoot string, devices []*Device, deviceName string, cdiDevices, cdiSpecDirs []string) *Device {
	if m := draDeviceNameRegex.FindStringSubmatch(deviceName); m != nil {
		if minor, err := strconv.Atoi(m[1]); err == nil {
			if dev := deviceByRenderMinor(sysRoot, devices, minor); dev != nil {
				return dev
			}
		}
	}
	for _, cdiName := range cdiDevices {
		key, ok := strings.CutPrefix(cdiName, draCDIDevicePrefix)
		if !ok || len(key) <= claimUIDLength {
			continue // another vendor, or the "common" device of every claim
		}
		// Kubelet lists the CDI devices of every device of the claim before
		// this one, so only this device's key (optionally "-slot<N>") counts.
		if rest := key[claimUIDLength+1:]; rest != deviceName && !strings.HasPrefix(rest, deviceName+"-") {
			continue
		}
		for _, minor := range draCDIRenderMinors(cdiSpecDirs, key) {
			if dev := deviceByRenderMinor(sysRoot, devices, minor); dev != nil {
				return dev
			}
		}
	}
	return nil
}

// deviceByRenderMinor returns the device owning a DRM render node: from the
// render nodes of its KFD nodes, or, when the KFD topology is not readable,
// from the PCI device of the render node in sysfs (the render node of an
// unpartitioned GPU).
func deviceByRenderMinor(sysRoot string, devices []*Device, minor int) *Device {
	for _, dev := range devices {
		if slices.Contains(dev.renderMinors, minor) {
			return dev
		}
	}
	target, err := filepath.EvalSymlinks(filepath.Join(sysRoot, "class", "drm", "renderD"+strconv.Itoa(minor), "device"))
	if err != nil {
		return nil
	}
	pciBusID := strings.ToLower(filepath.Base(target))
	for _, dev := range devices {
		if dev.PCIBusID == pciBusID {
			return dev
		}
	}
	return nil
}

// draCDISpec is the part of a CDI specification used here.
type draCDISpec struct {
	Devices []struct {
		Name           string `yaml:"name"`
		ContainerEdits struct {
			DeviceNodes []struct {
				Path string `yaml:"path"`
			} `yaml:"deviceNodes"`
		} `yaml:"containerEdits"`
	} `yaml:"devices"`
}

// draCDIRenderMinors returns the render node minors of the CDI device
// "<claim UID>-<device name>", from the spec file of the claim.
func draCDIRenderMinors(cdiSpecDirs []string, deviceKey string) []int {
	claimUID := deviceKey[:claimUIDLength]
	if strings.ContainsAny(claimUID, `/\`) {
		return nil // the UID comes from kubelet; it is joined to a path below
	}
	var data []byte
	var err error
	for _, dir := range cdiSpecDirs {
		if data, err = os.ReadFile(filepath.Join(dir, draCDISpecPrefix+claimUID+".yaml")); err == nil {
			break
		}
	}
	if err != nil {
		return nil
	}
	var spec draCDISpec
	if yaml.Unmarshal(data, &spec) != nil {
		return nil
	}
	var minors []int
	for _, device := range spec.Devices {
		if device.Name != deviceKey {
			continue
		}
		for _, node := range device.ContainerEdits.DeviceNodes {
			if m := renderNodeRegex.FindStringSubmatch(node.Path); m != nil {
				if minor, err := strconv.Atoi(m[1]); err == nil {
					minors = append(minors, minor)
				}
			}
		}
	}
	return minors
}
