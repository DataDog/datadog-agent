// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// TestNewStatelessCollector tests stateless collector-specific initialization with dynamic API creation
func TestNewStatelessCollector(t *testing.T) {
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	safenvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := safenvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	device := devices[0]

	// Test that the stateless collector creates the expected dynamic API set
	collector, err := newStatelessCollector(device)
	require.NoError(t, err)
	require.NotNil(t, collector)

	// Verify it's a baseCollector with the expected name
	bc := collector.(*baseCollector)
	require.Equal(t, stateless, bc.name)
	require.NotEmpty(t, bc.supportedAPIs) // Should have at least some supported APIs

	// Test collection works
	metrics, err := collector.Collect()
	require.NoError(t, err)
	require.NotEmpty(t, metrics) // Should have some metrics
}

// TestCollectProcessMemory tests the process memory collection with different process scenarios
func TestCollectProcessMemory(t *testing.T) {
	tests := []struct {
		name          string
		processes     []nvml.ProcessInfo
		expectedCount int
	}{
		{
			name:          "NoProcesses",
			processes:     []nvml.ProcessInfo{},
			expectedCount: 1, // Only memory.limit
		},
		{
			name: "SingleProcess",
			processes: []nvml.ProcessInfo{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			expectedCount: 2, // process.memory.usage + memory.limit
		},
		{
			name: "MultipleProcesses",
			processes: []nvml.ProcessInfo{
				{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
				{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
				{Pid: 1003, UsedGpuMemory: 536870912},  // 512MB
			},
			expectedCount: 4, // 3 process.memory.usage + 1 memory.limit
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override the mock to return specific processes
			nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
			device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())
			// Override GetComputeRunningProcesses to return test processes
			device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
				return tt.processes, nvml.SUCCESS
			}
			nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
				if index == 0 {
					return device, nvml.SUCCESS
				}
				return nil, nvml.ERROR_INVALID_ARGUMENT
			}
			safenvml.WithMockNVML(t, nvmlMock)

			deviceCache, err := safenvml.NewDeviceCache()
			require.NoError(t, err)
			devices := deviceCache.AllPhysicalDevices()
			require.NotEmpty(t, devices)
			mockDevice := devices[0]

			collector, err := newStatelessCollector(mockDevice)
			require.NoError(t, err)

			metrics, err := collector.Collect()
			require.NoError(t, err)

			// Count process memory related metrics
			var processMemoryCount int
			for _, metric := range metrics {
				if metric.Name == "process.memory.usage" || metric.Name == "memory.limit" {
					processMemoryCount++
				}
			}
			require.GreaterOrEqual(t, processMemoryCount, tt.expectedCount-1) // Allow for other metrics but check minimums
		})
	}
}

// TestCollectProcessMemory_Error tests error handling with API failures
func TestCollectProcessMemory_Error(t *testing.T) {
	// Override the mock to return error for GetComputeRunningProcesses
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())
	// Override GetComputeRunningProcesses to return an error
	device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
		return nil, nvml.ERROR_UNKNOWN
	}
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return device, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}
	safenvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := safenvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	mockDevice := devices[0]

	collector, err := newStatelessCollector(mockDevice)
	require.NoError(t, err)

	metrics, err := collector.Collect()

	// Should get error but still have some metrics (from other APIs)
	require.Error(t, err)
	require.Greater(t, len(metrics), 0) // Some metrics should still be collected
}

// TestProcessMemoryMetricTags tests that process memory metrics have correct tags and priorities
func TestProcessMemoryMetricTags(t *testing.T) {
	processes := []nvml.ProcessInfo{
		{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
		{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
	}

	// Override the mock to return specific processes
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled(), testutil.WithMockAllFunctions())
	device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())
	device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
		return processes, nvml.SUCCESS
	}
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return device, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}
	safenvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := safenvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	mockDevice := devices[0]

	collector, err := newStatelessCollector(mockDevice)
	require.NoError(t, err)

	metrics, err := collector.Collect()
	require.NoError(t, err)

	// Check process.memory.usage metrics have PID tags
	processMemoryMetrics := 0
	for _, metric := range metrics {
		if metric.Name == "process.memory.usage" {
			processMemoryMetrics++
			require.Len(t, metric.Tags, 1, "process.memory.usage should have exactly one tag")
			require.Contains(t, metric.Tags[0], "pid:", "process.memory.usage should have pid tag")
			require.Equal(t, High, metric.Priority, "process.memory.usage should have High priority")
		}
		if metric.Name == "memory.limit" {
			require.Len(t, metric.Tags, 2, "memory.limit should have PID tags for all processes")
			require.Equal(t, High, metric.Priority, "memory.limit should have High priority")
		}
	}
	require.Equal(t, 2, processMemoryMetrics, "Should have process.memory.usage for each process")
}
