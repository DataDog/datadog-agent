// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func requireMIGTests(t *testing.T) {
	t.Helper()

	if os.Getenv("RUN_MIG_TESTS") == "" {
		t.Skip("Skipping MIG tests: RUN_MIG_TESTS environment variable not set")
	}
}

// TestMIGDeviceListing tests that MIG devices can be listed and have valid properties.
func TestMIGDeviceListing(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))

	migDevices, err := cache.AllMigDevices()
	require.NoError(t, err, "Should be able to get MIG devices")
	require.NotEmpty(t, migDevices, "Should have at least one MIG device")

	for _, device := range migDevices {
		info := device.GetDeviceInfo()
		t.Logf("MIG device %s has %d cores", info.UUID, info.CoreCount)
		require.NotEmpty(t, info.UUID, "MIG device UUID should not be empty")
		require.NotEmpty(t, info.Name, "MIG device name should not be empty")

		// We cannot know for sure the real expected value, but we can be sure that it's greater than 500. This way we avoid
		// issues when we're just reporting the number of multiprocessors instead of the actual number of cores.
		require.Greater(t, info.CoreCount, 500, "MIG device should have more than 500 cores")
	}
}

// TestMIGParentChildCoreCount tests that the parent device's core count equals
// the sum of its MIG children's core counts.
func TestMIGParentChildCoreCount(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))

	physicalDevices, err := cache.AllPhysicalDevices()
	require.NoError(t, err, "Should be able to get physical devices")

	foundMIGParent := false
	for _, device := range physicalDevices {
		physDev, ok := device.(*safenvml.PhysicalDevice)
		require.True(t, ok, "Device should be a PhysicalDevice")

		if !physDev.HasMIGFeatureEnabled || len(physDev.MIGChildren) == 0 {
			t.Logf("Physical device %s has no MIG children, core count is %d", physDev.GetDeviceInfo().UUID, physDev.GetDeviceInfo().CoreCount)
			continue
		}

		foundMIGParent = true
		parentInfo := physDev.GetDeviceInfo()

		childCoreSum := 0
		for _, migChild := range physDev.MIGChildren {
			childInfo := migChild.GetDeviceInfo()
			childCoreSum += childInfo.CoreCount
		}

		require.Equal(t, parentInfo.CoreCount, childCoreSum,
			"Parent device core count should equal sum of MIG children core counts")
		t.Logf("Parent device %s has %d cores, sum of MIG children core counts is %d", parentInfo.UUID, parentInfo.CoreCount, childCoreSum)
	}

	require.True(t, foundMIGParent, "Should have at least one physical device with MIG enabled and children")
}

// TestMIGEnableModeWithSafeNVML verifies that MIG mode can be enabled while safenvml is active.
func TestMIGEnableModeWithSafeNVML(t *testing.T) {
	testutil.RequireGPU(t)
	requireMIGTests(t)

	if os.Geteuid() != 0 {
		t.Skip("Skipping MIG enable test: requires root to change MIG mode")
	}

	if _, err := exec.LookPath("nvidia-smi"); err != nil {
		t.Skip("Skipping MIG enable test: nvidia-smi not found")
	}

	lib := initNVML(t)
	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	_, err := cache.AllPhysicalDevices()
	require.NoError(t, err, "Should be able to list physical devices with safenvml")

	deviceCount, err := lib.DeviceGetCount()
	require.NoError(t, err, "Should be able to get device count")
	require.Greater(t, deviceCount, 0, "Should have at least one GPU")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-smi", "-mig", "0")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("nvidia-smi timed out disabling MIG: %s", string(output))
	}
	require.NoError(t, err, "nvidia-smi should disable MIG mode, output: %s", string(output))

	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err, "Should get device handle for index %d", i)

		migMode, _, err := device.GetMigMode()
		require.NoError(t, err, "Should get MIG mode for GPU %d", i)
		require.Equal(t, nvml.DEVICE_MIG_DISABLE, migMode, "MIG mode should be disabled for GPU %d", i)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd = exec.CommandContext(ctx, "nvidia-smi", "-mig", "1")
	output, err = cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("nvidia-smi timed out enabling MIG: %s", string(output))
	}
	require.NoError(t, err, "nvidia-smi should enable MIG mode, output: %s", string(output))

	for i := 0; i < deviceCount; i++ {
		device, err := lib.DeviceGetHandleByIndex(i)
		require.NoError(t, err, "Should get device handle for index %d", i)

		migMode, _, err := device.GetMigMode()
		require.NoError(t, err, "Should get MIG mode for GPU %d", i)
		require.Equal(t, nvml.DEVICE_MIG_ENABLE, migMode, "MIG mode should be enabled for GPU %d", i)
	}
}
