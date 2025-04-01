// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvml

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestNewDevice(t *testing.T) {
	mockDevice := testutil.GetDeviceMock(0)
	device, err := NewDevice(mockDevice)
	require.NoError(t, err)
	require.NotNil(t, device)
	require.Equal(t, testutil.GPUUUIDs[0], device.UUID)
	require.Equal(t, testutil.GPUCores[0], device.CoreCount)
	require.Equal(t, 0, device.Index)
	require.Equal(t, testutil.DefaultTotalMemory, device.Memory)
	require.Equal(t, uint32(75), device.SMVersion) // 7*10 + 5
}

func TestNewDeviceUUIDFailure(t *testing.T) {
	// Create a mock device that fails when getting UUID
	mockDevice := &nvmlmock.Device{
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

	device, err := NewDevice(mockDevice)
	require.Error(t, err)
	require.Nil(t, device)
	require.Contains(t, err.Error(), "error getting UUID")
}

func TestNewDeviceCache(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, len(testutil.GPUUUIDs), cache.Count())
}

func TestNewDeviceCachePartialFailure(t *testing.T) {
	// Create a mock that returns 3 devices but only 2 succeed
	mockNvml := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 3, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			if index == 2 {
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			return testutil.GetDeviceMock(index), nvml.SUCCESS
		},
	}

	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, 2, cache.Count())

	// Verify we can get the working devices
	device0, err := cache.GetByIndex(0)
	require.NoError(t, err)
	require.Equal(t, 0, device0.Index)

	device1, err := cache.GetByIndex(1)
	require.NoError(t, err)
	require.Equal(t, 1, device1.Index)

	// Verify we can't get the failed device
	_, err = cache.GetByIndex(2)
	require.Error(t, err)
}

func TestNewDeviceCacheDeviceUUIDFailure(t *testing.T) {
	// Create a mock that returns 2 devices, but one fails when getting UUID
	mockNvml := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 2, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			if index == 1 {
				// Return a device that fails when getting UUID
				return &nvmlmock.Device{
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
						return 1, nvml.SUCCESS
					},
					GetMemoryInfoFunc: func() (nvml.Memory, nvml.Return) {
						return nvml.Memory{Total: testutil.DefaultTotalMemory}, nvml.SUCCESS
					},
				}, nvml.SUCCESS
			}
			return testutil.GetDeviceMock(index), nvml.SUCCESS
		},
	}

	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)
	require.NotNil(t, cache)
	require.Equal(t, 1, cache.Count())

	// Verify we can get the working device
	device0, err := cache.GetByIndex(0)
	require.NoError(t, err)
	require.Equal(t, 0, device0.Index)

	// Verify we can't get the failed device
	_, err = cache.GetByIndex(1)
	require.Error(t, err)
}

func TestDeviceCacheGetByUUID(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)

	device, ok := cache.GetByUUID(testutil.DefaultGpuUUID)
	require.True(t, ok)
	require.Equal(t, testutil.DefaultGpuUUID, device.UUID)

	_, ok = cache.GetByUUID("non-existent-uuid")
	require.False(t, ok)
}

func TestDeviceCacheGetByIndex(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)

	device, err := cache.GetByIndex(0)
	require.NoError(t, err)
	require.Equal(t, 0, device.Index)

	_, err = cache.GetByIndex(-1)
	require.Error(t, err)
}

func TestDeviceCacheSMVersionSet(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)

	smVersions := cache.SMVersionSet()
	require.Len(t, smVersions, 1)
	_, exists := smVersions[75] // 7*10 + 5
	require.True(t, exists)
}

func TestDeviceCacheAll(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)

	devices := cache.All()
	require.Len(t, devices, len(testutil.GPUUUIDs))
	for i, device := range devices {
		require.Equal(t, testutil.GPUUUIDs[i], device.UUID)
		require.Equal(t, testutil.GPUCores[i], device.CoreCount)
	}
}

func TestDeviceCacheCores(t *testing.T) {
	mockNvml := testutil.GetBasicNvmlMock()
	cache, err := NewDeviceCacheWithOptions(mockNvml)
	require.NoError(t, err)

	cores, err := cache.Cores(testutil.DefaultGpuUUID)
	require.NoError(t, err)
	require.Equal(t, uint64(testutil.DefaultGpuCores), cores)

	_, err = cache.Cores("non-existent-uuid")
	require.Error(t, err)
}
