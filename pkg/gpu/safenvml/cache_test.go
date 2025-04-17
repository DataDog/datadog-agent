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
	require.Equal(t, 0, device0.Index)

	device1, ok := cache.GetByUUID(testutil.GPUUUIDs[1])
	require.True(t, ok)
	require.Equal(t, 1, device1.Index)

	// Verify we can't get the failed device
	_, ok = cache.GetByUUID("non-existent-uuid")
	require.False(t, ok)
}

func TestDeviceCacheGetByIndex(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
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
	require.Equal(t, 0, device.Index)

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
		testutil.WithSymbolsMock(allSymbols),
	)

	// Use WithMockNVML to set the mock
	WithMockNVML(t, mockNvml)

	// Create device cache
	cache, err := NewDeviceCache()
	require.NoError(t, err)
	require.NotNil(t, cache)

	// Test get all devices
	devices := cache.All()
	require.Len(t, devices, len(testutil.GPUUUIDs))

	for i, device := range devices {
		require.Equal(t, testutil.GPUUUIDs[i], device.UUID)
		require.Equal(t, testutil.GPUCores[i], device.CoreCount)
		require.Equal(t, i, device.Index)
	}
}

func TestDeviceCacheCores(t *testing.T) {
	// Create mock with all symbols available
	mockNvml := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithSymbolsMock(allSymbols),
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
