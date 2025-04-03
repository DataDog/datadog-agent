// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test && nvml

// Package testutil provides utilities for testing the NVML package.
package testutil

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/nvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// ToDDNVMLDevices converts a slice of nvml.Device to a slice of ddnvml.Device
func ToDDNVMLDevices(t testing.TB, devices []nvml.Device) []*ddnvml.Device {
	ddnvmlDevices := make([]*ddnvml.Device, len(devices))
	for i, dev := range devices {
		dev, err := ddnvml.NewDevice(dev)
		require.NoError(t, err, "error converting nvml.Device %d to ddnvml.Device", i)
		ddnvmlDevices[i] = dev
	}
	return ddnvmlDevices
}

// GetDDNVMLMocksWithIndexes returns a slice of ddnvml.Device mocks with the given indexes
func GetDDNVMLMocksWithIndexes(t testing.TB, indexes ...int) []*ddnvml.Device {
	devices := make([]*ddnvml.Device, len(indexes))
	for i, idx := range indexes {
		devices[i] = GetDDNVMLMockWithIndex(t, idx)
	}
	return devices
}

// GetDDNVMLMockWithIndex returns a ddnvml.Device mock with the given index, based on the data
// present in mocks.go
func GetDDNVMLMockWithIndex(t testing.TB, index int) *ddnvml.Device {
	dev := testutil.GetDeviceMock(index)
	dddev, err := ddnvml.NewDevice(dev)
	require.NoError(t, err, "error converting nvml.Device to ddnvml.Device")
	return dddev
}

// RequireDevicesEqual checks that the two devices are equal by comparing their UUIDs, which gives a better
// output than using require.Equal on the devices themselves
func RequireDevicesEqual(t *testing.T, expected, actual *ddnvml.Device, msgAndArgs ...interface{}) {
	extraFmt := ""
	if len(msgAndArgs) > 0 {
		extraFmt = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...) + ": "
	}

	require.Equal(t, expected.UUID, actual.UUID, "%sUUIDs do not match", extraFmt)
}

// RequireDeviceListsEqual checks that the two device lists are equal by comparing their UUIDs, which gives a better
// output than using require.ElementsMatch on the lists themselves
func RequireDeviceListsEqual(t *testing.T, expected, actual []*ddnvml.Device, msgAndArgs ...interface{}) {
	extraFmt := ""
	if len(msgAndArgs) > 0 {
		extraFmt = fmt.Sprintf(msgAndArgs[0].(string), msgAndArgs[1:]...) + ": "
	}

	require.Len(t, actual, len(expected), "%sdevice lists have different lengths", extraFmt)

	for i := range expected {
		require.Equal(t, expected[i].UUID, actual[i].UUID, "%sUUIDs do not match for element %d", extraFmt, i)
	}
}
