// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

// Package gpu provides utilities for interacting with GPU resources.
package gpu

import "github.com/NVIDIA/go-nvml/pkg/nvml"

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
	case 10: // nvml.DEVICE_ARCH_BLACKWELL in newer versions of go-nvml
		// note: we hardcode the enum to avoid updating to an untested newer go-nvml version
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
