// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package nvml

import (
	"context"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestPull(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		store:   wmetaMock,
		nvmlLib: nvmlMock,
	}

	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, len(testutil.GPUUUIDs), len(gpus))
	var expectedActivePIDs []int
	for _, proc := range testutil.DefaultProcessInfo {
		expectedActivePIDs = append(expectedActivePIDs, int(proc.Pid))
	}

	foundIDs := make(map[string]bool)
	for _, gpu := range gpus {
		foundIDs[gpu.ID] = true
		require.Equal(t, testutil.DefaultNvidiaDriverVersion, gpu.DriverVersion)
		require.Equal(t, nvidiaVendor, gpu.Vendor)
		require.Equal(t, testutil.DefaultGPUName, gpu.Name)
		require.Equal(t, testutil.DefaultGPUName, gpu.Device)
		require.Equal(t, "hopper", gpu.Architecture)
		require.Equal(t, testutil.DefaultGPUComputeCapMajor, gpu.ComputeCapability.Major)
		require.Equal(t, testutil.DefaultGPUComputeCapMinor, gpu.ComputeCapability.Minor)
		require.Equal(t, testutil.DefaultTotalMemory, gpu.TotalMemory)
		require.Equal(t, testutil.DefaultMaxClockRates[workloadmeta.GPUSM], gpu.MaxClockRates[workloadmeta.GPUSM])
		require.Equal(t, testutil.DefaultMaxClockRates[workloadmeta.GPUMemory], gpu.MaxClockRates[workloadmeta.GPUMemory])
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
	}

	for _, uuid := range testutil.GPUUUIDs {
		require.True(t, foundIDs[uuid], "GPU with UUID %s not found", uuid)
	}
}

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
			require.Equal(t, tt.expected, gpuArchToString(tt.arch))
		})
	}
}

func TestGpuProcessInfoUpdate(t *testing.T) {
	wmetaMock := testutil.GetWorkloadMetaMock(t)
	nvmlMock := testutil.GetBasicNvmlMock()

	c := &collector{
		id:      collectorID,
		catalog: workloadmeta.NodeAgent,
		store:   wmetaMock,
		nvmlLib: nvmlMock,
	}

	// First pull to populate the store with initial PIDs
	c.Pull(context.Background())

	gpus := wmetaMock.ListGPUs()
	require.Equal(t, len(testutil.GPUUUIDs), len(gpus))

	var expectedActivePIDs []int
	for _, proc := range testutil.DefaultProcessInfo {
		expectedActivePIDs = append(expectedActivePIDs, int(proc.Pid))
	}

	for _, gpu := range gpus {
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
	}

	// Now change those PIDs and make sure the store is updated and we get a complete override
	// of the previous PIDs
	expectedActivePIDs = []int{9761, 1234}
	newProcessInfo := []nvml.ProcessInfo{
		{Pid: uint32(expectedActivePIDs[0]), UsedGpuMemory: 100},
		{Pid: uint32(expectedActivePIDs[1]), UsedGpuMemory: 200},
	}
	oldProcessInfo := testutil.DefaultProcessInfo
	t.Cleanup(func() { testutil.DefaultProcessInfo = oldProcessInfo })

	testutil.DefaultProcessInfo = newProcessInfo

	c.Pull(context.Background())
	gpus = wmetaMock.ListGPUs()
	require.Equal(t, len(testutil.GPUUUIDs), len(gpus))

	for _, gpu := range gpus {
		require.Equal(t, expectedActivePIDs, gpu.ActivePIDs)
	}
}
