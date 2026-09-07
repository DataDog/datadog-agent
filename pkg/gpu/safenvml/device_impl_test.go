// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"errors"
	"maps"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestNewDevice(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Test device creation
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, device)
	require.Equal(t, testutil.GPUUUIDs[0], device.UUID)
	require.Equal(t, testutil.GPUCores[0], device.CoreCount)
	require.Equal(t, 0, device.Index)
	require.Equal(t, testutil.DefaultTotalMemory, device.Memory)
	require.Equal(t, uint32(75), device.SMVersion) // 7*10 + 5
}

func TestNewDeviceNVLinkLinkCount(t *testing.T) {
	tests := []struct {
		name            string
		options         []testutil.NvmlMockOption
		expectedCount   int
		expectedVersion string
	}{
		{
			name: "links present",
			options: []testutil.NvmlMockOption{
				testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1, NvLinkLinkCount: 2}),
			},
			expectedCount:   2,
			expectedVersion: "1.0",
		},
		{
			name: "disabled link",
			options: []testutil.NvmlMockOption{
				testutil.WithCapabilities(testutil.Capabilities{NvLinkGenerationSupported: 1}),
				testutil.WithNVLinkStates([]nvml.EnableState{nvml.FEATURE_DISABLED}, nil),
			},
			expectedCount:   1,
			expectedVersion: "1.0",
		},
		{
			name:          "no links",
			options:       []testutil.NvmlMockOption{testutil.WithNVLinkLinkCount(0)},
			expectedCount: 0,
		},
		{
			name:          "unsupported link count field",
			options:       []testutil.NvmlMockOption{testutil.WithUnsupportedFields(nvml.FI_DEV_NVLINK_LINK_COUNT)},
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockNvml := testutil.NewMockNVML(
				append(tt.options, testutil.WithSymbolsMock(allSymbols))...,
			)
			WithMockNVML(t, mockNvml)

			nvmlDev, ret := mockNvml.DeviceGetHandleByIndex(0)
			require.Equal(t, nvml.SUCCESS, ret)

			device, err := NewPhysicalDevice(nvmlDev)

			require.NoError(t, err)
			require.Equal(t, tt.expectedCount, device.NVLinkLinkCount)
			require.Equal(t, tt.expectedVersion, device.NVLinkVersion)
		})
	}
}

func TestNewDeviceUUIDFailure(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithDeviceOptions(0, testutil.WithCustomHook(func(device *testutil.MockDevice) {
			device.GetUUIDFunc = func() (string, nvml.Return) {
				return "", nvml.ERROR_INVALID_ARGUMENT
			}
		})),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Test device creation with failing UUID
	device, err := NewPhysicalDevice(mockNvml.Device(0))

	// Verify failure
	require.Error(t, err)
	require.Nil(t, device)

	// Check that it's the correct type of error using errors.As
	var nvmlErr *NvmlAPIError
	require.True(t, errors.As(err, &nvmlErr), "Expected error to be of type *NvmlAPIError")
	require.Equal(t, "GetUUID", nvmlErr.APIName)
	require.Equal(t, nvml.ERROR_INVALID_ARGUMENT, nvmlErr.NvmlErrorCode)
}

func TestDeviceWithMissingSymbol(t *testing.T) {
	// Create mock with MaxClockInfo symbol missing, not critical, should succeed
	symbols := maps.Clone(allSymbols)
	delete(symbols, toNativeName("GetMaxClockInfo"))

	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(symbols),
	)

	// Use WithMockNVML to set the mock
	WithPartialMockNVML(t, mockNvml, symbols)

	// Create device
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)
	require.NoError(t, err)
	require.NotNil(t, device)

	// Expect the cache fields to be populated correctly
	require.Equal(t, testutil.GPUUUIDs[0], device.UUID)

	// Test calling a method with a missing symbol
	_, err = device.GetMaxClockInfo(nvml.CLOCK_MEM)
	require.Error(t, err)

	// Check that it's the correct type of error using errors.As
	var nvmlErr *NvmlAPIError
	require.True(t, errors.As(err, &nvmlErr), "Expected error to be of type *NvmlAPIError")
	require.Equal(t, toNativeName("GetMaxClockInfo"), nvmlErr.APIName)
	require.Equal(t, nvml.ERROR_FUNCTION_NOT_FOUND, nvmlErr.NvmlErrorCode)
}

func TestDeviceSafeMethodSuccess(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.NewMockNVML(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device
	mockDevice := mockNvml.Device(0)
	device, err := NewPhysicalDevice(mockDevice)
	require.NoError(t, err)
	require.NotNil(t, device)

	// Test a method that calls the underlying NVML device
	memInfo, err := device.GetMemoryInfo()
	require.NoError(t, err)
	require.Equal(t, testutil.DefaultTotalMemory, memInfo.Total)

	// Test the embedded interface delegation
	cores, err := device.GetNumGpuCores()
	require.NoError(t, err)
	require.Equal(t, testutil.DefaultGpuCores, cores)
}

func TestGetGpuFabricInfoRequiresVersionedAPISymbol(t *testing.T) {
	symbols := maps.Clone(allSymbols)
	delete(symbols, toNativeName("GetGpuFabricInfoV"))

	device := &safeDeviceImpl{
		lib: &safeNvml{capabilities: symbols},
	}

	_, err := device.GetGpuFabricInfo()
	require.Error(t, err)
	require.True(t, IsUnsupported(err))
}
