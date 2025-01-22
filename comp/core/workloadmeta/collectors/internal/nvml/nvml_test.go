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

	foundIDs := make(map[string]bool)
	for _, gpu := range gpus {
		foundIDs[gpu.ID] = true

		require.Equal(t, nvidiaVendor, gpu.Vendor)
		require.Equal(t, testutil.DefaultGPUName, gpu.Name)
		require.Equal(t, testutil.DefaultGPUName, gpu.Device)
		require.Equal(t, "hopper", gpu.Architecture)
		require.Equal(t, testutil.DefaultGPUComputeCapMajor, gpu.ComputeCapability.Major)
		require.Equal(t, testutil.DefaultGPUComputeCapMinor, gpu.ComputeCapability.Minor)
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
