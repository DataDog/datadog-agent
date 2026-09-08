// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"
)

func TestGpuArchToString(t *testing.T) {
	tests := []struct {
		arch     nvml.DeviceArchitecture
		expected string
	}{
		{nvml.DEVICE_ARCH_KEPLER, "kepler"},
		{nvml.DEVICE_ARCH_UNKNOWN, "unknown"},
		{nvml.DeviceArchitecture(3751), "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			require.Equal(t, tt.expected, ArchToString(tt.arch))
		})
	}
}

func TestPCIInfoToBusID(t *testing.T) {
	tests := []struct {
		name     string
		pciInfo  nvml.PciInfo
		expected string
	}{
		{
			name: "typical Linux BDF",
			pciInfo: nvml.PciInfo{
				Domain: 0,
				Bus:    0x65,
				Device: 0,
			},
			expected: "0000:65:00.0",
		},
		{
			name: "domain wider than four hex digits",
			pciInfo: nvml.PciInfo{
				Domain: 0x12345,
				Bus:    0xab,
				Device: 0x1e,
			},
			expected: "12345:ab:1e.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, PCIInfoToBusID(tt.pciInfo))
		})
	}
}
