// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func initNVML(t *testing.T) safenvml.SafeNVML {
	lib, err := safenvml.GetSafeNvmlLib()
	require.NoError(t, err, "NVML library should be available")
	require.NotNil(t, lib, "NVML library should not be nil")

	return lib
}

// TestNVMLInitialization tests that NVML can be initialized on the current system.
// This is a basic sanity check that the GPU driver and NVML library are available.
func TestNVMLInitialization(t *testing.T) {
	testutil.RequireGPU(t)
	lib := initNVML(t)

	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err, "Should be able to get device count")
	require.Greater(t, deviceCount, 0, "Should have at least one GPU")

	t.Logf("Found %d GPU device(s)", deviceCount)
}

// TestDeviceCacheInitialization tests that the device cache can be initialized
// and returns valid device information.
func TestDeviceCacheInitialization(t *testing.T) {
	testutil.RequireGPU(t)
	lib := initNVML(t)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	require.NotNil(t, cache, "Device cache should not be nil")

	devices, err := cache.All()
	require.NoError(t, err, "Should be able to get all devices")
	require.NotEmpty(t, devices, "Should have at least one device in cache")

	for _, device := range devices {
		info := device.GetDeviceInfo()
		require.NotEmpty(t, info.UUID, "Device UUID should not be empty")
		require.NotEmpty(t, info.Name, "Device name should not be empty")
		require.Greater(t, info.CoreCount, 0, "Device should have cores")
		require.Greater(t, info.Memory, uint64(0), "Device should have memory")

		t.Logf("Device: %s (UUID: %s, Cores: %d, Memory: %d MB)",
			info.Name, info.UUID, info.CoreCount, info.Memory/1024/1024)
	}
}

// TestDeviceBasicProperties tests that we can read basic properties from GPU devices.
func TestDeviceBasicProperties(t *testing.T) {
	testutil.RequireGPU(t)
	lib := initNVML(t)

	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err)

	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err, "Should get device handle for index %d", i)

		// Test name
		name, err := device.GetName()
		require.NoError(t, err, "Should get device name")
		require.NotEmpty(t, name, "Device name should not be empty")

		// Test UUID
		uuid, err := device.GetUUID()
		require.NoError(t, err, "Should get device UUID")
		require.NotEmpty(t, uuid, "Device UUID should not be empty")

		// Test memory info
		memInfo, err := device.GetMemoryInfo()
		require.NoError(t, err, "Should get memory info")
		require.Greater(t, memInfo.Total, uint64(0), "Total memory should be > 0")

		// Test CUDA compute capability
		major, minor, err := device.GetCudaComputeCapability()
		require.NoError(t, err, "Should get CUDA compute capability")
		require.Greater(t, major, 0, "CUDA major version should be > 0")

		t.Logf("GPU %d: %s (SM %d.%d, Memory: %d MB)",
			i, name, major, minor, memInfo.Total/1024/1024)
	}
}
