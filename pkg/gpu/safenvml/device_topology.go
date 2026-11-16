// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"slices"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"

	"github.com/DataDog/datadog-agent/pkg/gpu/amd/sysfsroot"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pci"
)

// HostLinkInfo holds the maximum capabilities of a GPU-to-host link. Zero and
// nil values are unknown. Negotiated (current) values are metrics, not cached.
type HostLinkInfo struct {
	// C2CEnabled is true when the GPU reaches its host over NVLink-C2C, and
	// false when NVML reports that it does not or that the device has no C2C.
	C2CEnabled *bool
	// C2CLinkCount is the number of NVLink-C2C links, only set when C2C is enabled.
	C2CLinkCount int
	// C2CMaxBandwidthMBps is the highest per-link C2C bandwidth reported by
	// NVML, in megabytes per second. Only set when C2C is enabled.
	C2CMaxBandwidthMBps uint64
	// PCIeMaxGeneration is supported by the GPU and the system together.
	PCIeMaxGeneration int
	// PCIeGPUMaxGeneration is supported by the GPU itself.
	PCIeGPUMaxGeneration int
	// PCIeMaxWidth is the maximum lane count, not the current width.
	PCIeMaxWidth int
}

// newPCISysfs returns the sysfs reader for PCI facts, rooted where the GPU
// checks find the device tree. The host sysfs is often only partly mounted in
// containers, but the PCI device tree isn't namespaced, so the container's own
// /sys is tried next. Replaced in tests.
var newPCISysfs = func() *pci.Sysfs {
	return pci.NewSysfs(sysfsroot.DeviceSysFSRoot(), "/sys")
}

// fillTopologyAttributes caches static PCI, NUMA and host-link facts. They are
// queried once per device construction, never per metric collection.
// Unsupported or failed queries leave the values unknown.
func (d *DeviceInfo) fillTopologyAttributes(dev SafeDevice) {
	if partNumber, err := dev.GetBoardPartNumber(); err == nil {
		d.BoardPartNumber = strings.TrimSpace(partNumber)
	} else if !IsUnsupported(err) {
		singleton.logDeviceWarning(d.UUID, "cannot get board part number: %v", err)
	}
	if bar1, err := dev.GetBAR1MemoryInfo(); err == nil {
		d.BAR1TotalBytes = bar1.Bar1Total
	} else if !IsUnsupported(err) {
		singleton.logDeviceWarning(d.UUID, "cannot get BAR1 memory info: %v", err)
	}
	d.HostLink = d.queryHostLink(dev)

	if d.PCIBusID == "" {
		return
	}
	sysfs := newPCISysfs()
	// Host NUMA locality comes from sysfs only. nvmlDeviceGetNumaNodeId is
	// documented for platforms where GPUs are NUMA nodes, so it can report
	// the GPU's own memory node instead of the host node closest to it.
	if node, ok := sysfs.NUMANode(d.PCIBusID); ok {
		d.NUMANode = &node
	}
	if path, err := sysfs.UpstreamBridges(d.PCIBusID); err == nil {
		d.PCIePath = path
	} else if logLimiter.ShouldLog() {
		log.Debugf("cannot read PCIe path of GPU %s at %s: %v", d.UUID, d.PCIBusID, err)
	}
}

func (d *DeviceInfo) queryHostLink(dev SafeDevice) HostLinkInfo {
	var link HostLinkInfo
	queries := []struct {
		name   string
		get    func() (int, error)
		target *int
	}{
		{"max PCIe link generation", dev.GetMaxPcieLinkGeneration, &link.PCIeMaxGeneration},
		{"GPU max PCIe link generation", dev.GetGpuMaxPcieLinkGeneration, &link.PCIeGPUMaxGeneration},
		{"max PCIe link width", dev.GetMaxPcieLinkWidth, &link.PCIeMaxWidth},
	}
	for _, query := range queries {
		if value, err := query.get(); err == nil && value > 0 {
			*query.target = value
		} else if err != nil && !IsUnsupported(err) {
			singleton.logDeviceWarning(d.UUID, "cannot get %s: %v", query.name, err)
		}
	}

	mode, err := dev.GetC2cModeInfo()
	var nvmlErr *NvmlAPIError
	switch {
	case err == nil:
		enabled := mode.IsC2cEnabled != 0
		link.C2CEnabled = &enabled
	case errors.As(err, &nvmlErr) && nvmlErr.NvmlErrorCode == nvml.ERROR_NOT_SUPPORTED:
		// The device has no C2C. A missing symbol (old driver) stays unknown.
		disabled := false
		link.C2CEnabled = &disabled
	case !IsUnsupported(err):
		singleton.logDeviceWarning(d.UUID, "cannot get C2C mode: %v", err)
	}
	if link.C2CEnabled != nil && *link.C2CEnabled {
		link.C2CLinkCount, link.C2CMaxBandwidthMBps = d.queryC2CLinks(dev)
	}
	return link
}

// queryC2CLinks returns the C2C link count and the highest per-link maximum
// bandwidth, zero when unknown.
func (d *DeviceInfo) queryC2CLinks(dev SafeDevice) (int, uint64) {
	countField := []nvml.FieldValue{{FieldId: nvml.FI_DEV_C2C_LINK_COUNT}}
	if err := dev.GetFieldValues(countField); err != nil || nvml.Return(countField[0].NvmlReturn) != nvml.SUCCESS {
		singleton.logDeviceWarning(d.UUID, "cannot get C2C link count")
		return 0, 0
	}
	count, err := nvmlFieldValueToInt(countField[0])
	if err != nil || count <= 0 {
		return 0, 0
	}
	bandwidthFields := make([]nvml.FieldValue, count)
	for link := range bandwidthFields {
		bandwidthFields[link] = nvml.FieldValue{FieldId: nvml.FI_DEV_C2C_LINK_GET_MAX_BW, ScopeId: uint32(link)}
	}
	if err := dev.GetFieldValues(bandwidthFields); err != nil {
		singleton.logDeviceWarning(d.UUID, "cannot get C2C link bandwidth: %v", err)
		return count, 0
	}
	bandwidths := make([]uint64, 0, count)
	for _, field := range bandwidthFields {
		if nvml.Return(field.NvmlReturn) != nvml.SUCCESS {
			continue
		}
		if value, err := nvmlFieldValueToInt(field); err == nil && value > 0 {
			bandwidths = append(bandwidths, uint64(value))
		}
	}
	if len(bandwidths) == 0 {
		return count, 0
	}
	return count, slices.Max(bandwidths)
}
