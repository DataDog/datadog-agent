// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"slices"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
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

// maxC2CLinks bounds the link count read from the driver. Real devices have
// around ten links; a larger value is treated as garbage.
const maxC2CLinks = 64

// queryC2CLinks returns the C2C link count and the highest per-link maximum
// bandwidth, zero when unknown.
func (d *DeviceInfo) queryC2CLinks(dev SafeDevice) (int, uint64) {
	countField := []nvml.FieldValue{{FieldId: nvml.FI_DEV_C2C_LINK_COUNT}}
	if err := dev.GetFieldValues(countField); err != nil {
		singleton.logDeviceWarning(d.UUID, "cannot get C2C link count: %v", err)
		return 0, 0
	}
	if ret := nvml.Return(countField[0].NvmlReturn); ret != nvml.SUCCESS {
		singleton.logDeviceWarning(d.UUID, "cannot get C2C link count: %v", ret)
		return 0, 0
	}
	count, err := nvmlFieldValueToInt(countField[0])
	if err != nil || count <= 0 {
		return 0, 0
	}
	if count > maxC2CLinks {
		singleton.logDeviceWarning(d.UUID, "ignoring implausible C2C link count %d", count)
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
