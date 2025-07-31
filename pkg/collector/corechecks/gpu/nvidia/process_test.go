// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// Test device specifications constants
const (
	testDeviceUUID      = "test-uuid"
	testDeviceMemory    = 8589934592 // 8GB
	testDeviceCoreCount = 80
)

// TestNewProcessCollector tests process collector initialization
func TestNewProcessCollector(t *testing.T) {
	tests := []struct {
		name                    string
		computeProcessesError   error
		processUtilizationError error
		wantError               bool
		expectedAPICount        int
	}{
		{
			name:                    "BothApisSupported",
			computeProcessesError:   nil,
			processUtilizationError: nil,
			wantError:               false,
			expectedAPICount:        2,
		},
		{
			name:                    "OneApiSupported",
			computeProcessesError:   nil,
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			wantError:               false,
			expectedAPICount:        1,
		},
		{
			name:                    "NoApisSupported",
			computeProcessesError:   &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			wantError:               true,
			expectedAPICount:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo:              &safenvml.DeviceInfo{UUID: testDeviceUUID},
				computeProcessesError:   tt.computeProcessesError,
				processUtilizationError: tt.processUtilizationError,
			}

			safenvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

			collector, err := newProcessCollector(mockDevice)

			if tt.wantError {
				assert.ErrorIs(t, err, errUnsupportedDevice)
				assert.Nil(t, collector)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, collector)

				pc := collector.(*processCollector)
				assert.Len(t, pc.supportedAPICalls, tt.expectedAPICount)
			}
		})
	}
}

// TestCollectComputeProcesses tests the collectComputeProcesses helper function
func TestCollectComputeProcesses(t *testing.T) {
	tests := []struct {
		name          string
		processes     []nvml.ProcessInfo
		expectedCount int
	}{
		{
			name:          "NoComputeProcesses",
			processes:     []nvml.ProcessInfo{},
			expectedCount: 1, // Only memory.limit
		},
		{
			name: "SingleComputeProcess",
			processes: []nvml.ProcessInfo{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			expectedCount: 2, // memory.usage + memory.limit
		},
		{
			name: "MultipleComputeProcesses",
			processes: []nvml.ProcessInfo{
				{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
				{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
				{Pid: 1003, UsedGpuMemory: 536870912},  // 512MB
			},
			expectedCount: 4, // 3 memory.usage + 1 memory.limit
		},
	}

	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice.processes = tt.processes
			collector := &processCollector{device: mockDevice}
			metrics, err := collector.collectComputeProcesses()

			assert.NoError(t, err)
			assert.Len(t, metrics, tt.expectedCount)
		})
	}
}

// TestCollectComputeProcesses_Error tests error handling separately
func TestCollectComputeProcesses_Error(t *testing.T) {
	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
		computeProcessesError: &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_UNKNOWN},
	}

	collector := &processCollector{device: mockDevice}
	metrics, err := collector.collectComputeProcesses()

	assert.Error(t, err)
	assert.Len(t, metrics, 1) // Only memory.limit (still emitted on error)
}

// TestCollectProcessUtilization tests the collectProcessUtilization helper function
func TestCollectProcessUtilization(t *testing.T) {
	tests := []struct {
		name          string
		samples       []nvml.ProcessUtilizationSample
		expectedCount int
	}{
		{
			name:          "NoUtilizationProcesses",
			samples:       []nvml.ProcessUtilizationSample{},
			expectedCount: 2, // core.limit + sm_active
		},
		{
			name: "SingleUtilizationProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			expectedCount: 6, // 4 per-process + core.limit + sm_active
		},
		{
			name: "MultipleUtilizationProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 1003, TimeStamp: 1200, SmUtil: 80, MemUtil: 70, EncUtil: 35, DecUtil: 25},
			},
			expectedCount: 10, // 2×4 per-process + core.limit + sm_active
		},
		{
			name: "ZeroUtilizationValues",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 13001, TimeStamp: 4000, SmUtil: 0, MemUtil: 0, EncUtil: 0, DecUtil: 0},
			},
			expectedCount: 6, // 4 per-process + core.limit + sm_active
		},
	}

	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice.samples = tt.samples
			collector := &processCollector{device: mockDevice}
			metrics, err := collector.collectProcessUtilization()

			assert.NoError(t, err)
			assert.Len(t, metrics, tt.expectedCount)
		})
	}
}

// TestCollectProcessUtilization_Error tests error handling separately
func TestCollectProcessUtilization_Error(t *testing.T) {
	tests := []struct {
		name          string
		apiError      error
		expectedCount int
		expectError   bool
	}{
		{
			name:          "ProcessUtilizationAPIError_NOT_FOUND",
			apiError:      &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_FOUND},
			expectedCount: 2, // core.limit + sm_active (gracefully handled)
			expectError:   false,
		},
		{
			name:          "ProcessUtilizationAPIError_Other",
			apiError:      &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_UNKNOWN},
			expectedCount: 2, // core.limit + sm_active (still emitted on error)
			expectError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo: &safenvml.DeviceInfo{
					UUID:      testDeviceUUID,
					Memory:    testDeviceMemory,
					CoreCount: testDeviceCoreCount,
				},
				processUtilizationError: tt.apiError,
			}

			collector := &processCollector{device: mockDevice}
			metrics, err := collector.collectProcessUtilization()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Len(t, metrics, tt.expectedCount)
		})
	}
}

// TestProcessUtilizationSmActiveCalculation tests the sm_active median calculation logic
func TestProcessUtilizationSmActiveCalculation(t *testing.T) {
	tests := []struct {
		name             string
		samples          []nvml.ProcessUtilizationSample
		expectedSmActive float64
		description      string
	}{
		{
			name:             "NoProcesses",
			samples:          []nvml.ProcessUtilizationSample{},
			expectedSmActive: 0.0,
			description:      "No processes should result in sm_active = 0",
		},
		{
			name: "SingleProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			expectedSmActive: 75.0,
			description:      "Single process: median(75, min(75, 100)) = 75",
		},
		{
			name: "TwoProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 1003, TimeStamp: 1200, SmUtil: 80, MemUtil: 70, EncUtil: 35, DecUtil: 25},
			},
			expectedSmActive: 90.0,
			description:      "Two processes: median(max=80, min(sum=130, 100)) = median(80, 100) = 90",
		},
		{
			name: "SmUtilSumExceeds100",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 14001, TimeStamp: 5000, SmUtil: 70, MemUtil: 50, EncUtil: 25, DecUtil: 15},
				{Pid: 14002, TimeStamp: 5100, SmUtil: 60, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 14003, TimeStamp: 5200, SmUtil: 50, MemUtil: 30, EncUtil: 15, DecUtil: 5},
			},
			expectedSmActive: 85.0,
			description:      "Sum > 100: median(max=70, min(sum=180, 100)) = median(70, 100) = 85",
		},
		{
			name: "ZeroValues",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 13001, TimeStamp: 4000, SmUtil: 0, MemUtil: 0, EncUtil: 0, DecUtil: 0},
			},
			expectedSmActive: 0.0,
			description:      "Zero utilization: median(0, min(0, 100)) = 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo: &safenvml.DeviceInfo{
					UUID:      testDeviceUUID,
					Memory:    testDeviceMemory,
					CoreCount: testDeviceCoreCount,
				},
				samples: tt.samples,
			}

			collector := &processCollector{device: mockDevice}
			metrics, err := collector.collectProcessUtilization()

			require.NoError(t, err)

			// Find sm_active metric
			var smActive *Metric
			for i, metric := range metrics {
				if metric.Name == "sm_active" {
					smActive = &metrics[i]
					break
				}
			}

			require.NotNil(t, smActive, "sm_active metric should be present")
			assert.Equal(t, tt.expectedSmActive, smActive.Value, tt.description)
			assert.Empty(t, smActive.Tags, "sm_active should not have PID tags")
		})
	}
}

// TestProcessUtilizationTimestampUpdate tests timestamp tracking behavior
func TestProcessUtilizationTimestampUpdate(t *testing.T) {
	tests := []struct {
		name             string
		initialTimestamp uint64
		samples          []nvml.ProcessUtilizationSample
		expectedFinalTS  uint64
	}{
		{
			name:             "TimestampUpdate",
			initialTimestamp: 1000,
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 7001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 7002, TimeStamp: 1200, SmUtil: 60, MemUtil: 50, EncUtil: 25, DecUtil: 15},
				{Pid: 7003, TimeStamp: 1150, SmUtil: 70, MemUtil: 60, EncUtil: 30, DecUtil: 20},
			},
			expectedFinalTS: 1200, // Highest timestamp
		},
		{
			name:             "NoTimestampUpdate",
			initialTimestamp: 2000,
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 8001, TimeStamp: 1800, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 8002, TimeStamp: 1900, SmUtil: 60, MemUtil: 50, EncUtil: 25, DecUtil: 15},
			},
			expectedFinalTS: 2000, // Should remain unchanged
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo: &safenvml.DeviceInfo{
					UUID:      testDeviceUUID,
					Memory:    testDeviceMemory,
					CoreCount: testDeviceCoreCount,
				},
				samples: tt.samples,
			}

			collector := &processCollector{device: mockDevice, lastTimestamp: tt.initialTimestamp}
			_, err := collector.collectProcessUtilization()

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFinalTS, collector.lastTimestamp)
		})
	}
}

// TestProcessCollector_Collect tests the full Collect() flow
func TestProcessCollector_Collect(t *testing.T) {
	tests := []struct {
		name                    string
		processes               []nvml.ProcessInfo
		samples                 []nvml.ProcessUtilizationSample
		computeProcessesError   error
		processUtilizationError error
		expectedMetricCount     int
	}{
		{
			name: "BothAPIsSuccess",
			processes: []nvml.ProcessInfo{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			expectedMetricCount: 8, // 2 compute + 6 utilization
		},
		{
			name: "ComputeOnlySuccess",
			processes: []nvml.ProcessInfo{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			expectedMetricCount:     2, // 2 compute only
		},
		{
			name: "UtilizationOnlySuccess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			computeProcessesError: &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			expectedMetricCount:   6, // 6 utilization only
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo: &safenvml.DeviceInfo{
					UUID:      testDeviceUUID,
					Memory:    testDeviceMemory,
					CoreCount: testDeviceCoreCount,
				},
				processes:               tt.processes,
				samples:                 tt.samples,
				computeProcessesError:   tt.computeProcessesError,
				processUtilizationError: tt.processUtilizationError,
			}

			// Create collector with appropriate API support
			collector := &processCollector{device: mockDevice}

			// Simulate removeUnsupportedMetrics logic
			if tt.computeProcessesError == nil || !safenvml.IsUnsupported(tt.computeProcessesError) {
				collector.supportedAPICalls = append(collector.supportedAPICalls, apiCallFactory[0]) // memory_usage
			}
			if tt.processUtilizationError == nil || !safenvml.IsUnsupported(tt.processUtilizationError) {
				collector.supportedAPICalls = append(collector.supportedAPICalls, apiCallFactory[1]) // process_utilization
			}

			metrics, err := collector.Collect()

			assert.NoError(t, err)
			assert.Len(t, metrics, tt.expectedMetricCount)
		})
	}
}

// TestProcessCollector_Collect_WithErrors tests error handling separately
func TestProcessCollector_Collect_WithErrors(t *testing.T) {
	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
		computeProcessesError:   &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_UNKNOWN},
		processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_FOUND},
	}

	// Create collector with appropriate API support
	collector := &processCollector{device: mockDevice}
	// Both APIs are supported (not ERROR_NOT_SUPPORTED)
	collector.supportedAPICalls = append(collector.supportedAPICalls, apiCallFactory...)

	metrics, err := collector.Collect()

	assert.Error(t, err)      // ERROR_UNKNOWN should be returned
	assert.Len(t, metrics, 3) // memory.limit + core.limit + sm_active
}

// Mock device for process collector tests
type mockProcessDevice struct {
	safenvml.SafeDevice

	deviceInfo              *safenvml.DeviceInfo
	processes               []nvml.ProcessInfo
	samples                 []nvml.ProcessUtilizationSample
	computeProcessesError   error
	processUtilizationError error
}

func (m *mockProcessDevice) GetDeviceInfo() *safenvml.DeviceInfo {
	return m.deviceInfo
}

func (m *mockProcessDevice) GetComputeRunningProcesses() ([]nvml.ProcessInfo, error) {
	if m.computeProcessesError != nil {
		return nil, m.computeProcessesError
	}
	return m.processes, nil
}

func (m *mockProcessDevice) GetProcessUtilization(_ uint64) ([]nvml.ProcessUtilizationSample, error) {
	if m.processUtilizationError != nil {
		return nil, m.processUtilizationError
	}
	return m.samples, nil
}
