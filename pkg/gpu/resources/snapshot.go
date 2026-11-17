// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package resources

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	gpuutil "github.com/DataDog/datadog-agent/pkg/util/gpu"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// Snapshot builds resource fields from cached physical device attributes.
func Snapshot(devices []safenvml.Device) []Device {
	result := make([]Device, 0, len(devices))
	for _, device := range devices {
		if _, physical := device.(*safenvml.PhysicalDevice); !physical {
			continue
		}
		info := device.GetDeviceInfo()
		if info.VirtualizationMode == nvml.GPU_VIRTUALIZATION_MODE_VGPU {
			continue
		}
		d := Device{
			UUID:               strings.ToLower(info.UUID),
			Vendor:             "nvidia",
			Model:              gpuutil.NormalizeGPUDeviceName(info.Name),
			PCIBusID:           info.PCIBusID,
			DriverVersion:      info.DriverVersion,
			Architecture:       gpuutil.ArchToString(info.Architecture),
			ComputeCapability:  fmt.Sprintf("%d.%d", info.SMVersion/10, info.SMVersion%10),
			CoreCount:          info.CoreCount,
			MemoryTotalBytes:   info.Memory,
			MemoryBusWidth:     info.MemoryBusWidth,
			VirtualizationMode: info.VirtualizationModeName,
			BoardSerial:        strings.ToLower(info.BoardSerial),
			ChassisSerial:      strings.ToLower(info.ChassisSerial),
			FabricGUID:         info.FabricGUID,
			FabricClusterUUID:  info.FabricClusterUUID,
		}
		if info.FabricClusterUUID != "" {
			clique := info.FabricCliqueID
			d.FabricCliqueID = &clique
			d.NVLinkDomain = fmt.Sprintf("%s:%d", info.FabricClusterUUID, clique)
		}
		fillPlacement(&d, info)
		result = append(result, d)
	}
	return result
}

// fillPlacement copies the cached PCI, NUMA, placement and host-link facts.
func fillPlacement(d *Device, info *safenvml.DeviceInfo) {
	d.BoardPartNumber = info.BoardPartNumber
	d.BAR1TotalBytes = info.BAR1TotalBytes
	d.NVLinkLinkCount = info.NVLinkLinkCount
	if info.PCIDeviceID != 0 {
		d.PCIDeviceID = fmt.Sprintf("%08x", info.PCIDeviceID)
	}
	if info.PCISubsystemID != 0 {
		d.PCISubsystemID = fmt.Sprintf("%08x", info.PCISubsystemID)
	}
	if info.PCIePath != nil {
		// A known empty path (GPU on the root complex) is published as [].
		path := append([]string{}, info.PCIePath...)
		d.PCIePath = &path
	}
	if info.NUMANode != nil {
		node := *info.NUMANode
		d.NUMANode = &node
	}
	// Like the matching metric tags, placement is only known with an
	// identified chassis: without one, NVML reports zero or 255 placeholders.
	if d.ChassisSerial != "" {
		slot, tray, module := info.SlotNumber, info.TrayIndex, info.ModuleID
		d.SlotNumber, d.TrayIndex, d.ModuleID = &slot, &tray, &module
		d.Platform = &Platform{HostID: info.HostID}
	}
	d.HostLink = hostLink(info.HostLink)
}

func hostLink(info safenvml.HostLinkInfo) *HostLink {
	link := HostLink{
		PCIeMaxGeneration:    info.PCIeMaxGeneration,
		PCIeGPUMaxGeneration: info.PCIeGPUMaxGeneration,
		PCIeMaxWidth:         info.PCIeMaxWidth,
	}
	if info.PCIeMaxGeneration > 0 && info.PCIeMaxWidth > 0 {
		// Same computation as the gpu.pci.link.speed.max metric.
		if bandwidth, err := gpuutil.PCIeLinkBytesPerSecond(info.PCIeMaxGeneration, info.PCIeMaxWidth); err == nil {
			link.PCIeMaxBandwidthBytesPerSecond = int64(bandwidth)
		}
	}
	if info.C2CEnabled != nil {
		link.Protocol = "pcie"
		if *info.C2CEnabled {
			link.Protocol = "nvlink_c2c"
			link.C2CLinkCount = info.C2CLinkCount
			// NVML reports megabytes per second; the schema uses megabits.
			link.C2CMaxBandwidthMbps = int64(info.C2CMaxBandwidthMBps) * 8
		}
	}
	if link == (HostLink{}) {
		return nil
	}
	return &link
}
