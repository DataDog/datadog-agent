// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestDeviceCache(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithMIGDisabled(),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, len(testutil.GPUUUIDs), cache.Count())
}

func TestDeviceCachePartialFailure(t *testing.T) {
	// Create a mock that returns 3 devices but only 2 succeed
	baseDeviceGetCountFunc := func() (int, nvml.Return) {
		return 3, nvml.SUCCESS
	}

	baseDeviceGetHandleByIndexFunc := func(index int) (nvml.Device, nvml.Return) {
		if index == 2 {
			return nil, nvml.ERROR_INVALID_ARGUMENT
		}
		return testutil.GetDeviceMock(index), nvml.SUCCESS
	}

	// Create custom mock with specific config
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithMIGDisabled(),
	)

	// Override device count and get by index funcs
	mockNvml.DeviceGetCountFunc = baseDeviceGetCountFunc
	mockNvml.DeviceGetHandleByIndexFunc = baseDeviceGetHandleByIndexFunc

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, 2, cache.Count())

	// Verify we can get the working devices
	device0, ok := cache.GetByUUID(testutil.GPUUUIDs[0])
	require.True(t, ok)
	require.Equal(t, 0, device0.GetDeviceInfo().Index)

	device1, ok := cache.GetByUUID(testutil.GPUUUIDs[1])
	require.True(t, ok)
	require.Equal(t, 1, device1.GetDeviceInfo().Index)

	// Verify we can't get the failed device
	_, ok = cache.GetByUUID("non-existent-uuid")
	require.False(t, ok)
}

func TestDeviceCacheGetByIndex(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithMIGDisabled(),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test get by index
	device, err := cache.GetByIndex(0)
	require.NoError(t, err)
	require.Equal(t, 0, device.GetDeviceInfo().Index)

	// Test with invalid index
	_, err = cache.GetByIndex(-1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "index -1 out of range")

	// Test out of range index
	_, err = cache.GetByIndex(100)
	require.Error(t, err)
	require.Contains(t, err.Error(), "index 100 out of range")
}

func TestDeviceCacheSMVersionSet(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithMIGDisabled(),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test SM version set
	smVersions := cache.SMVersionSet()
	require.NotEmpty(t, smVersions)
	_, exists := smVersions[75] // 7*10 + 5
	require.True(t, exists)
}

func TestDeviceCacheAll(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols), // Default mock, MIG enabled for configured devices
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// cache.Count() should *only* counts physical devices
	require.Equal(t, len(testutil.GPUUUIDs), cache.Count(), "Didn't find expected number of physical devices in cache")

	// cache.All() includes all physical and MIG devices
	allDevices := cache.All()
	expectedTotalDevices := testutil.GetTotalExpectedDevices()
	require.Len(t, allDevices, expectedTotalDevices, "Cache is not filled correctly, some devices are missing")
}

func TestDeviceCacheCores(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
		testutil.WithMIGDisabled(),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test getting cores
	cores, err := cache.Cores(testutil.DefaultGpuUUID)
	require.NoError(t, err)
	require.Equal(t, uint64(testutil.DefaultGpuCores), cores)

	// Test with non-existent UUID
	_, err = cache.Cores("non-existent-uuid")
	require.Error(t, err)
	require.Contains(t, err.Error(), "device non-existent-uuid not found")
}

func TestDeviceCacheAllPhysicalDevices(t *testing.T) {
	// Test with MIG enabled
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
	)
	WithMockNVML(t, mockNvml)

	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test get all devices
	physicalDevices := cache.AllPhysicalDevices()
	require.Len(t, physicalDevices, len(testutil.GPUUUIDs))

	for i, device := range physicalDevices {
		require.Equal(t, testutil.GPUUUIDs[i], device.GetDeviceInfo().UUID)
		require.Equal(t, testutil.GPUCores[i], device.GetDeviceInfo().CoreCount)
		require.Equal(t, i, device.GetDeviceInfo().Index)
	}
}

func TestDeviceCacheAllMigDevices(t *testing.T) {
	// Test with MIG enabled
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
	)
	WithMockNVML(t, mockNvml)

	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	migDevices := cache.AllMigDevices()
	// Calculate expected number of MIG devices from testutil
	expectedTotalMigCount := testutil.GetTotalExpectedDevices() - len(testutil.GPUUUIDs)
	require.Len(t, migDevices, expectedTotalMigCount, "AllMigDevices should return all and only configured MIG instances")

	for _, migDevice := range migDevices {
		// Verify that the device is identified as a MIG device handle
		isMig, err := migDevice.IsMigDeviceHandle()
		require.NoError(t, err)
		require.True(t, isMig, "Device %s from AllMigDevices should be identified as a MIG device handle", migDevice.GetDeviceInfo().UUID)
	}
}
