// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
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

// TestNewProcessCollector_TimestampInitialization tests that the constructor initializes timestamp
func TestNewProcessCollector_TimestampInitialization(t *testing.T) {
	mockDevice := &mockProcessDevice{
		deviceInfo:              &safenvml.DeviceInfo{UUID: testDeviceUUID},
		computeProcessesError:   nil,
		processUtilizationError: nil,
	}

	safenvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

	timeBefore := uint64(time.Now().Unix())
	collector, err := newProcessCollector(mockDevice)
	timeAfter := uint64(time.Now().Unix())

	assert.NoError(t, err)
	assert.NotNil(t, collector)

	pc := collector.(*processCollector)
	assert.GreaterOrEqual(t, pc.lastTimestamp, timeBefore, "Timestamp should be initialized to current time")
	assert.LessOrEqual(t, pc.lastTimestamp, timeAfter, "Timestamp should be initialized to current time")
	assert.Greater(t, pc.lastTimestamp, uint64(0), "Timestamp should be greater than 0")
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
			expectedCount: 2, // sm_active + core.limit
		},
		{
			name: "SingleUtilizationProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			expectedCount: 6, // 4 per-process + sm_active + core.limit
		},
		{
			name: "MultipleUtilizationProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 1003, TimeStamp: 1200, SmUtil: 80, MemUtil: 70, EncUtil: 35, DecUtil: 25},
			},
			expectedCount: 10, // 2×4 per-process + sm_active + core.limit
		},
		{
			name: "ZeroUtilizationValues",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 13001, TimeStamp: 4000, SmUtil: 0, MemUtil: 0, EncUtil: 0, DecUtil: 0},
			},
			expectedCount: 6, // 4 per-process + sm_active + core.limit
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
			expectedCount: 2, // sm_active + core.limit (gracefully handled)
			expectError:   false,
		},
		{
			name:          "ProcessUtilizationAPIError_Other",
			apiError:      &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_UNKNOWN},
			expectedCount: 2, // sm_active + core.limit (still emitted on error)
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

// TestProcessUtilizationTimestampUpdate tests timestamp tracking behavior
func TestProcessUtilizationTimestampUpdate(t *testing.T) {
	tests := []struct {
		name             string
		initialTimestamp uint64
		samples          []nvml.ProcessUtilizationSample
		apiError         error
	}{
		{
			name:             "TimestampUpdatedOnSuccess",
			initialTimestamp: 1000,
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 7001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
			},
			apiError: nil,
		},
		{
			name:             "TimestampUpdatedOnNotFoundError",
			initialTimestamp: 1000,
			samples:          nil,
			apiError:         &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_FOUND},
		},
		{
			name:             "TimestampUpdatedOnOtherError",
			initialTimestamp: 1000,
			samples:          nil,
			apiError:         &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_UNKNOWN},
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
				samples:                 tt.samples,
				processUtilizationError: tt.apiError,
			}

			collector := &processCollector{device: mockDevice, lastTimestamp: tt.initialTimestamp}
			timeBefore := uint64(time.Now().Unix())
			_, err := collector.collectProcessUtilization()
			timeAfter := uint64(time.Now().Unix())

			// Timestamp should be updated to current time regardless of API success/failure
			assert.GreaterOrEqual(t, collector.lastTimestamp, timeBefore, "Timestamp should be updated to current time")
			assert.LessOrEqual(t, collector.lastTimestamp, timeAfter, "Timestamp should be updated to current time")
			assert.Greater(t, collector.lastTimestamp, tt.initialTimestamp, "Timestamp should be newer than initial")

			// Check error handling
			if tt.apiError != nil {
				var nvmlErr *safenvml.NvmlAPIError
				if errors.As(tt.apiError, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
					assert.NoError(t, err, "ERROR_NOT_FOUND should be handled gracefully")
				} else {
					assert.Error(t, err, "Other errors should be returned")
				}
			} else {
				assert.NoError(t, err, "Successful API call should not return error")
			}
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
			expectedMetricCount: 8, // 2 compute + 6 utilization (including sm_active)
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
			expectedMetricCount:   6, // 6 utilization only (including sm_active)
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
	assert.Len(t, metrics, 3) // memory.limit + sm_active + core.limit
}

// TestProcessUtilization_SmActiveCalculation tests the sm_active median calculation logic
func TestProcessUtilization_SmActiveCalculation(t *testing.T) {
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
			description:      "median of (max=0 + sum=0) / 2 = 0",
		},
		{
			name: "SingleProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 60},
			},
			expectedSmActive: 60.0,
			description:      "median of (max=60 + sum=60) / 2 = 60",
		},
		{
			name: "MultipleProcesses_SumUnderCap",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 30},
				{Pid: 1002, SmUtil: 40},
			},
			expectedSmActive: 55.0,
			description:      "median of (max=40 + sum=70) / 2 = 55",
		},
		{
			name: "MultipleProcesses_SumOverCap",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 80},
				{Pid: 1002, SmUtil: 60},
				{Pid: 1003, SmUtil: 40},
			},
			expectedSmActive: 90.0,
			description:      "median of (max=80 + sum=100) / 2 = 90, sum capped at 100",
		},
		{
			name: "ZeroUtilization",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 0},
				{Pid: 1002, SmUtil: 0},
			},
			expectedSmActive: 0.0,
			description:      "median of (max=0 + sum=0) / 2 = 0",
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

			assert.NoError(t, err)

			// Find the sm_active metric
			var smActiveMetric *Metric
			for _, metric := range metrics {
				if metric.Name == "sm_active" {
					smActiveMetric = &metric
					break
				}
			}

			assert.NotNil(t, smActiveMetric, "sm_active metric should always be emitted")
			assert.Equal(t, tt.expectedSmActive, smActiveMetric.Value, "sm_active value should match expected calculation: %s", tt.description)
			assert.Nil(t, smActiveMetric.Tags, "sm_active should have no tags (device-wide metric)")
		})
	}
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
