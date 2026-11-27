// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

// Package gpu provides utilities for interacting with GPU resources.
package gpu

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
)

// ArchToString converts a NVML device architecture to a string.
func ArchToString(arch nvml.DeviceArchitecture) string {
	switch arch {
	case nvml.DEVICE_ARCH_KEPLER:
		return "kepler"
	case nvml.DEVICE_ARCH_MAXWELL:
		return "maxwell"
	case nvml.DEVICE_ARCH_PASCAL:
		return "pascal"
	case nvml.DEVICE_ARCH_VOLTA:
		return "volta"
	case nvml.DEVICE_ARCH_TURING:
		return "turing"
	case nvml.DEVICE_ARCH_AMPERE:
		return "ampere"
	case nvml.DEVICE_ARCH_ADA:
		return "ada"
	case nvml.DEVICE_ARCH_HOPPER:
		return "hopper"
	case nvml.DEVICE_ARCH_BLACKWELL:
		return "blackwell"
	case nvml.DEVICE_ARCH_UNKNOWN:
		return "unknown"
	default:
		// Distinguish invalid and unknown, NVML can return unknown but we should always
		// be able to process the return value of NVML. If we reach this part, we forgot
		// to add a new case for a new architecture.
		return "invalid"
	}
}

// VirtualizationModeToString converts an NVML GPU virtualization mode to a tag value.
func VirtualizationModeToString(mode nvml.GpuVirtualizationMode) string {
	switch mode {
	case nvml.GPU_VIRTUALIZATION_MODE_NONE:
		return "none"
	case nvml.GPU_VIRTUALIZATION_MODE_HOST_VGPU:
		return "host_vgpu"
	case nvml.GPU_VIRTUALIZATION_MODE_PASSTHROUGH:
		return "passthrough"
	case nvml.GPU_VIRTUALIZATION_MODE_HOST_VSGA:
		return "host_vsga"
	case nvml.GPU_VIRTUALIZATION_MODE_VGPU:
		return "vgpu"
	default:
		return "unknown"
	}
}

// PCIInfoToBusID formats NVML PCI information as a normalized PCI BDF.
// NVML exposes domain, bus, and device as numeric fields, but not the PCI
// function. For NVIDIA GPUs, the GPU function is the .0 function; companion
// functions, when present, represent auxiliary devices such as audio.
func PCIInfoToBusID(pciInfo nvml.PciInfo) string {
	return strings.ToLower(fmt.Sprintf("%04x:%02x:%02x.0", pciInfo.Domain, pciInfo.Bus, pciInfo.Device))
}

// FabricClusterUUIDFromNVMLInfo formats the native fabric UUID for GPU tags.
func FabricClusterUUIDFromNVMLInfo(clusterUUID [16]uint8) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", clusterUUID[0:4], clusterUUID[4:6], clusterUUID[6:8], clusterUUID[8:10], clusterUUID[10:16])
}

// FabricInfoToTags returns the completed fabric identity used by GPU tags.
func FabricInfoToTags(fabricInfo nvml.GpuFabricInfo_v2) (string, uint32, bool) {
	if fabricInfo.State != nvml.GPU_FABRIC_STATE_COMPLETED ||
		nvml.Return(fabricInfo.Status) != nvml.SUCCESS ||
		fabricInfo.ClusterUuid == [16]uint8{} {
		return "", 0, false
	}

	return FabricClusterUUIDFromNVMLInfo(fabricInfo.ClusterUuid), fabricInfo.CliqueId, true
}

// PlatformFabricGUID formats the NVLink fabric GUID like nvidia-smi
// ("0x" + the first 8 bytes, little endian), or "" when not reported.
func PlatformFabricGUID(info nvml.PlatformInfo) string {
	guid := binary.LittleEndian.Uint64(info.IbGuid[:8])
	if guid == 0 {
		return ""
	}
	return fmt.Sprintf("0x%016x", guid)
}

// PlatformChassisSerial returns the NUL-padded chassis serial as a string.
func PlatformChassisSerial(info nvml.PlatformInfo) string {
	serial := info.ChassisSerialNumber[:]
	if end := bytes.IndexByte(serial, 0); end >= 0 {
		serial = serial[:end]
	}
	return strings.TrimSpace(string(serial))
}
