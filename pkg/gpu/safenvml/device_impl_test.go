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
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestNewDevice(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Test device creation
	mockDevice := testutil.GetDeviceMock(0)
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

func TestNewDeviceUUIDFailure(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create a mock device that fails when getting UUID
	failingMockDevice := &nvmlmock.Device{
		GetUUIDFunc: func() (string, nvml.Return) {
			return "", nvml.ERROR_INVALID_ARGUMENT
		},
		GetCudaComputeCapabilityFunc: func() (int, int, nvml.Return) {
			return 7, 5, nvml.SUCCESS
		},
		GetNumGpuCoresFunc: func() (int, nvml.Return) {
			return testutil.DefaultGpuCores, nvml.SUCCESS
		},
		GetIndexFunc: func() (int, nvml.Return) {
			return 0, nvml.SUCCESS
		},
		GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
			return nvml.Memory{Total: testutil.DefaultTotalMemory}, nvml.SUCCESS
		},
	}

	// Test device creation with failing UUID
	device, err := NewPhysicalDevice(failingMockDevice)

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

	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(symbols),
	)

	// Use WithMockNVML to set the mock
	WithPartialMockNVML(t, mockNvml, symbols)

	// Create device
	mockDevice := testutil.GetDeviceMock(0)
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
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device
	mockDevice := testutil.GetDeviceMock(0)
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
