// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// TestNewStatelessCollector tests stateless collector-specific initialization with dynamic API creation
func TestNewStatelessCollector(t *testing.T) {
	device := setupMockDevice(t, nil)

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
			// Override API factory to only include process memory
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_memory_usage",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return processMemorySample(device)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
					return tt.processes, nvml.SUCCESS
				}
				return device
			})

			collector, err := newStatelessCollector(mockDevice)
			require.NoError(t, err)

			processMetrics, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, processMetrics, tt.expectedCount)
		})
	}
}

// TestCollectProcessMemory_Error tests error handling with API failures
func TestCollectProcessMemory_Error(t *testing.T) {
	// Override API factory to only include process memory
	originalFactory := statelessAPIFactory
	defer func() { statelessAPIFactory = originalFactory }()

	statelessAPIFactory = func() []apiCallInfo {
		return []apiCallInfo{
			{
				Name: "process_memory_usage",
				Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
					return processMemorySample(device)
				},
			},
		}
	}

	mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
		device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
			return nil, nvml.ERROR_UNKNOWN
		}
		return device
	})

	collector, err := newStatelessCollector(mockDevice)
	require.NoError(t, err)

	processMetrics, err := collector.Collect()

	// Should get error but still have some metrics (memory.limit from processMemorySample)
	require.Error(t, err)
	require.Greater(t, len(processMetrics), 0) // Should still get memory.limit metric
}

// TestProcessMemoryMetricTags tests that process memory metrics have correct tags and priorities
func TestProcessMemoryMetricTags(t *testing.T) {
	// Override API factory to only include process memory
	originalFactory := statelessAPIFactory
	defer func() { statelessAPIFactory = originalFactory }()

	statelessAPIFactory = func() []apiCallInfo {
		return []apiCallInfo{
			{
				Name: "process_memory_usage",
				Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
					return processMemorySample(device)
				},
			},
		}
	}

	processes := []nvml.ProcessInfo{
		{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
		{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
	}

	mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
		device.GetComputeRunningProcessesFunc = func() ([]nvml.ProcessInfo, nvml.Return) {
			return processes, nvml.SUCCESS
		}
		return device
	})

	collector, err := newStatelessCollector(mockDevice)
	require.NoError(t, err)

	processMetrics, err := collector.Collect()
	require.NoError(t, err)

	// Should have exactly 3 metrics: 2 process.memory.usage + 1 memory.limit
	require.Len(t, processMetrics, 3)

	// Check process.memory.usage metrics have PID tags
	processMemoryMetrics := 0
	for _, metric := range processMetrics {
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

// TestNVLinkCollector_Initialization tests NVLink collector initialization (migrated from nvlink_test.go)
func TestNVLinkCollector_Initialization(t *testing.T) {
	tests := []struct {
		name        string
		customSetup func(*mock.Device) *mock.Device
		wantError   bool
		wantLinks   int
	}{
		{
			name: "Unsupported device",
			customSetup: func(device *mock.Device) *mock.Device {
				device.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
					return nvml.ERROR_NOT_SUPPORTED
				}
				device.GetUUIDFunc = func() (string, nvml.Return) {
					return "GPU-123", nvml.SUCCESS
				}
				return device
			},
			wantError: true,
		},
		{
			name: "Unknown error",
			customSetup: func(device *mock.Device) *mock.Device {
				device.GetFieldValuesFunc = func(_ []nvml.FieldValue) nvml.Return {
					return nvml.ERROR_UNKNOWN
				}
				device.GetUUIDFunc = func() (string, nvml.Return) {
					return "GPU-123", nvml.SUCCESS
				}
				return device
			},
			wantError: false,
		},
		{
			name: "Success with 4 links",
			customSetup: func(device *mock.Device) *mock.Device {
				device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
					require.Len(t, values, 1, "Expected one field value for total number of links, got %d", len(values))
					require.Equal(t, values[0].FieldId, uint32(nvml.FI_DEV_NVLINK_LINK_COUNT), "Expected field ID to be FI_DEV_NVLINK_LINK_COUNT, got %d", values[0].FieldId)
					require.Equal(t, values[0].ScopeId, uint32(0), "Expected scope ID to be 0, got %d", values[0].ScopeId)
					values[0].ValueType = uint32(nvml.VALUE_TYPE_SIGNED_INT)
					values[0].Value = [8]byte{4, 0, 0, 0, 0, 0, 0, 0} // 4 links
					return nvml.SUCCESS
				}
				device.GetUUIDFunc = func() (string, nvml.Return) {
					return "GPU-123", nvml.SUCCESS
				}
				return device
			},
			wantError: false,
			wantLinks: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include NVLink
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "nvlink_metrics",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							// Test the API first (like the original TestFunc)
							fields := []nvml.FieldValue{
								{
									FieldId: nvml.FI_DEV_NVLINK_LINK_COUNT,
									ScopeId: 0,
								},
							}
							if err := device.GetFieldValues(fields); err != nil {
								return nil, 0, err
							}
							// If test passes, return empty metrics for this test
							return []Metric{}, 0, nil
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, tt.customSetup)
			c, err := newStatelessCollector(mockDevice)

			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, c)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, c)
		})
	}
}

// TestNVLinkCollector_Collection tests NVLink collector collection (migrated from nvlink_test.go)
func TestNVLinkCollector_Collection(t *testing.T) {
	tests := []struct {
		name             string
		nvlinkStates     []nvml.EnableState
		nvlinkErrors     []error
		expectedActive   int
		expectedInactive int
		expectError      bool
	}{
		{
			name: "All links active",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, nil, nil},
			expectedActive:   3,
			expectedInactive: 0,
			expectError:      false,
		},
		{
			name: "Mixed active and inactive links",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_DISABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, nil, nil},
			expectedActive:   2,
			expectedInactive: 1,
			expectError:      false,
		},
		{
			name: "Error getting link state",
			nvlinkStates: []nvml.EnableState{
				nvml.FEATURE_ENABLED,
				nvml.FEATURE_ENABLED,
			},
			nvlinkErrors:     []error{nil, errors.New("unknown error")},
			expectedActive:   1,
			expectedInactive: 0,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include NVLink with full implementation
			originalFactory := statelessAPIFactory
			defer func() { statelessAPIFactory = originalFactory }()

			statelessAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "nvlink_metrics",
						Handler: func(device safenvml.Device, _ uint64) ([]Metric, uint64, error) {
							return nvlinkSample(device)
						},
					},
				}
			}

			// Create collector with mock device
			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
					values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					values[0].Value = [8]byte{byte(len(tt.nvlinkStates)), 0, 0, 0, 0, 0, 0, 0}
					return nvml.SUCCESS
				}
				device.GetNvLinkStateFunc = func(link int) (nvml.EnableState, nvml.Return) {
					if link >= len(tt.nvlinkStates) {
						return 0, nvml.ERROR_INVALID_ARGUMENT
					}
					if tt.nvlinkErrors[link] != nil {
						return 0, nvml.ERROR_UNKNOWN
					}
					return tt.nvlinkStates[link], nvml.SUCCESS
				}
				device.GetUUIDFunc = func() (string, nvml.Return) {
					return "GPU-123", nvml.SUCCESS
				}
				return device
			})

			collector, err := newStatelessCollector(mockDevice)
			require.NoError(t, err)

			// Collect metrics
			allMetrics, err := collector.Collect()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			// Verify metrics, as we still expect to have all 3 metrics even if some errors were returned
			require.Len(t, allMetrics, 3)

			// Check total links metric
			require.Equal(t, float64(len(tt.nvlinkStates)), allMetrics[0].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[0].Type)

			// Check active links metric
			require.Equal(t, float64(tt.expectedActive), allMetrics[1].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[1].Type)

			// Check inactive links metric
			require.Equal(t, float64(tt.expectedInactive), allMetrics[2].Value)
			require.Equal(t, metrics.GaugeType, allMetrics[2].Type)
		})
	}
}
