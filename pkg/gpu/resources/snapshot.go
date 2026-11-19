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
		result = append(result, d)
	}
	return result
}
