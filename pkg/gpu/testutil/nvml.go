// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
)

// ToDDNVMLDevices converts a slice of nvml.Device to a slice of ddnvml.Device
func ToDDNVMLDevices(t *testing.T, devices []nvml.Device) []*ddnvml.Device {
	ddnvmlDevices := make([]*ddnvml.Device, len(devices))
	for i, dev := range devices {
		dev, err := ddnvml.NewDevice(dev)
		require.NoError(t, err, "error converting nvml.Device %d to ddnvml.Device", i)
		ddnvmlDevices[i] = dev
	}
	return ddnvmlDevices
}

// GetDDNVMLMocksWithIndexes returns a slice of ddnvml.Device mocks with the given indexes
func GetDDNVMLMocksWithIndexes(t *testing.T, indexes ...int) []*ddnvml.Device {
	devices := make([]*ddnvml.Device, len(indexes))
	for i, idx := range indexes {
		devices[i] = GetDDNVMLMockWithIndex(t, idx)
	}
	return devices
}

// GetDDNVMLMockWithIndex returns a ddnvml.Device mock with the given index, based on the data
// present in mocks.go
func GetDDNVMLMockWithIndex(t *testing.T, index int) *ddnvml.Device {
	dev := GetDeviceMock(index)
	dddev, err := ddnvml.NewDevice(dev)
	require.NoError(t, err, "error converting nvml.Device to ddnvml.Device")
	return dddev
}
