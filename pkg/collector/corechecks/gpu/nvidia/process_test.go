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

// TestProcessScenarios tests different process scenarios
func TestProcessScenarios(t *testing.T) {
	tests := []struct {
		name                string
		processes           []nvml.ProcessInfo
		samples             []nvml.ProcessUtilizationSample
		expectedMetricCount int
		expectedPIDCounts   map[string]int
		specificValidations func(t *testing.T, metrics []Metric)
	}{
		{
			name:                "NoRunningProcesses",
			processes:           []nvml.ProcessInfo{},
			samples:             []nvml.ProcessUtilizationSample{},
			expectedMetricCount: 3, //we expect the gpu.core.limit, gpu.memory.limit and gr_engine_active metrics to be emitted even if no processes are not running
			expectedPIDCounts:   map[string]int{},
		},
		{
			name: "SingleProcess",
			processes: []nvml.ProcessInfo{
				{Pid: 1234, UsedGpuMemory: 536870912}, // 512MB
			},
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, TimeStamp: 1000, SmUtil: 75, MemUtil: 60, EncUtil: 30, DecUtil: 15},
			},
			expectedMetricCount: 8, // 2 compute per-process + 6 utilization (4 per-process + 2 aggregated)
			expectedPIDCounts:   map[string]int{"1234": 8},
			specificValidations: func(t *testing.T, metrics []Metric) {
				metricsByName := make(map[string]Metric)
				for _, metric := range metrics {
					metricsByName[metric.Name] = metric
				}
				assert.Equal(t, float64(536870912), metricsByName["memory.usage"].Value)
				assert.Equal(t, float64(60), metricsByName["core.usage"].Value) // (75/100) * 80 = 60
				assert.Equal(t, float64(60), metricsByName["dram_active"].Value)
				assert.Equal(t, float64(30), metricsByName["encoder_utilization"].Value)
				assert.Equal(t, float64(15), metricsByName["decoder_utilization"].Value)

				// Validate limit metrics values match device specs
				assert.Equal(t, float64(testDeviceMemory), metricsByName["memory.limit"].Value)  // Device memory
				assert.Equal(t, float64(testDeviceCoreCount), metricsByName["core.limit"].Value) // Device core count

				// Validate limit metrics have aggregated PID tags
				assert.Contains(t, metricsByName["memory.limit"].Tags, "pid:1234")
				assert.Contains(t, metricsByName["core.limit"].Tags, "pid:1234")
			},
		},
		{
			name: "MultipleProcesses",
			processes: []nvml.ProcessInfo{
				{Pid: 1001, UsedGpuMemory: 1073741824}, // 1GB
				{Pid: 1002, UsedGpuMemory: 2147483648}, // 2GB
				{Pid: 1003, UsedGpuMemory: 536870912},  // 512MB
			},
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, TimeStamp: 1100, SmUtil: 50, MemUtil: 40, EncUtil: 20, DecUtil: 10},
				{Pid: 1003, TimeStamp: 1200, SmUtil: 80, MemUtil: 70, EncUtil: 35, DecUtil: 25},
				// Note: PID 1002 has no utilization sample
			},
			expectedMetricCount: 14, // 4 compute (3 per-process + 1 aggregated) + 10 utilization (2×4 per-process + 2 aggregated)
			expectedPIDCounts: map[string]int{
				"1001": 8, // 1 compute per-process + 4 utilization per-process + 3 aggregated
				"1002": 1, // 1 compute per-process only
				"1003": 5, // 1 compute per-process + 4 utilization per-process
			},
			specificValidations: func(t *testing.T, metrics []Metric) {
				metricsByName := make(map[string]Metric)
				for _, metric := range metrics {
					metricsByName[metric.Name] = metric
				}

				// Validate limit metrics values match device specs
				assert.Equal(t, float64(testDeviceMemory), metricsByName["memory.limit"].Value)  // Device memory
				assert.Equal(t, float64(testDeviceCoreCount), metricsByName["core.limit"].Value) // Device core count

				// Validate memory.limit has all compute process PIDs aggregated
				memoryLimitTags := metricsByName["memory.limit"].Tags
				assert.Contains(t, memoryLimitTags, "pid:1001")
				assert.Contains(t, memoryLimitTags, "pid:1002")
				assert.Contains(t, memoryLimitTags, "pid:1003")

				// Validate core.limit has only utilization process PIDs aggregated
				coreLimitTags := metricsByName["core.limit"].Tags
				assert.Contains(t, coreLimitTags, "pid:1001")
				assert.Contains(t, coreLimitTags, "pid:1003")
				assert.NotContains(t, coreLimitTags, "pid:1002") // No utilization sample for 1002
			},
		},
		{
			name: "ProcessPidMismatch",
			processes: []nvml.ProcessInfo{
				{Pid: 2001, UsedGpuMemory: 1073741824},
			},
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 3001, TimeStamp: 1500, SmUtil: 90, MemUtil: 85, EncUtil: 45, DecUtil: 35},
			},
			expectedMetricCount: 8, // 2 compute (1 per-process + 1 aggregated) + 6 utilization (4 per-process + 2 aggregated)
			expectedPIDCounts: map[string]int{
				"2001": 2, // 1 compute per-process + 1 aggregated memory.limit
				"3001": 6, // 4 utilization per-process + 2 aggregated
			},
			specificValidations: func(t *testing.T, metrics []Metric) {
				computeMetrics := 0
				utilizationMetrics := 0
				for _, metric := range metrics {
					switch metric.Name {
					case "memory.usage":
						assert.Contains(t, metric.Tags, "pid:2001")
						computeMetrics++
					case "memory.limit":
						assert.Contains(t, metric.Tags, "pid:2001")
						computeMetrics++
					case "core.usage", "dram_active", "encoder_utilization", "decoder_utilization":
						assert.Contains(t, metric.Tags, "pid:3001")
						utilizationMetrics++
					case "core.limit", "gr_engine_active":
						assert.Contains(t, metric.Tags, "pid:3001")
						utilizationMetrics++
					}
				}
				assert.Equal(t, 2, computeMetrics)
				assert.Equal(t, 6, utilizationMetrics)
			},
		},
		{
			name: "ZeroValues",
			processes: []nvml.ProcessInfo{
				{Pid: 13001, UsedGpuMemory: 0}, // Zero memory
			},
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 13001, TimeStamp: 4000, SmUtil: 0, MemUtil: 0, EncUtil: 0, DecUtil: 0}, // Zero utilization
			},
			expectedMetricCount: 8, // 2 compute + 6 utilization
			expectedPIDCounts:   map[string]int{"13001": 8},
			specificValidations: func(t *testing.T, metrics []Metric) {
				metricsByName := make(map[string]Metric)
				for _, metric := range metrics {
					metricsByName[metric.Name] = metric
				}
				assert.Equal(t, float64(0), metricsByName["memory.usage"].Value)
				assert.Equal(t, float64(0), metricsByName["core.usage"].Value)
				assert.Equal(t, float64(0), metricsByName["dram_active"].Value)
				assert.Equal(t, float64(0), metricsByName["encoder_utilization"].Value)
				assert.Equal(t, float64(0), metricsByName["decoder_utilization"].Value)
			},
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
				processes: tt.processes,
				samples:   tt.samples,
			}

			safenvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

			collector, err := newProcessCollector(mockDevice)
			require.NoError(t, err)

			metrics, err := collector.Collect()
			assert.NoError(t, err)
			assert.Len(t, metrics, tt.expectedMetricCount)

			// Verify PID counts
			if len(tt.expectedPIDCounts) > 0 {
				pidCounts := make(map[string]int)
				for _, metric := range metrics {
					// Extract PID from tags slice
					for _, tag := range metric.Tags {
						if len(tag) > 4 && tag[:4] == "pid:" {
							pid := tag[4:]
							pidCounts[pid]++
							break
						}
					}
				}
				for expectedPID, expectedCount := range tt.expectedPIDCounts {
					assert.Equal(t, expectedCount, pidCounts[expectedPID], "PID %s metric count mismatch", expectedPID)
				}
			}

			// Run specific validations if provided
			if tt.specificValidations != nil {
				tt.specificValidations(t, metrics)
			}
		})
	}
}

// TestTimestampManagement tests timestamp update logic
func TestTimestampManagement(t *testing.T) {
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
				computeProcessesError:   &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
				processUtilizationError: nil,
				samples:                 tt.samples,
			}

			safenvml.WithMockNVML(t, testutil.GetBasicNvmlMock())

			collector, err := newProcessCollector(mockDevice)
			require.NoError(t, err)

			pc := collector.(*processCollector)
			pc.lastTimestamp = tt.initialTimestamp

			_, err = collector.Collect()
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedFinalTS, pc.lastTimestamp)
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

// TestCollectComputeProcesses_ApiFailure tests that memory.limit is still emitted when GetComputeRunningProcesses fails, but error is returned
func TestCollectComputeProcesses_ApiFailure(t *testing.T) {
	expectedError := &safenvml.NvmlAPIError{
		APIName:       "GetComputeRunningProcesses",
		NvmlErrorCode: nvml.ERROR_UNKNOWN,
	}

	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
		computeProcessesError: expectedError,
	}

	collector := &processCollector{device: mockDevice}
	metrics, err := collector.collectComputeProcesses()

	// Should return the original error (not handled gracefully)
	assert.Error(t, err)
	assert.Equal(t, expectedError, err)

	// Should still emit exactly 1 metric: memory.limit
	assert.Len(t, metrics, 1)

	// Verify memory.limit metric
	memLimit := metrics[0]
	assert.Equal(t, "memory.limit", memLimit.Name)
	assert.Equal(t, float64(testDeviceMemory), memLimit.Value)
	assert.Empty(t, memLimit.Tags, "memory.limit should have empty tags when API fails")
}

// TestCollectProcessUtilization_ApiFailure tests that core.limit and gr_engine_active are still emitted when GetProcessUtilization fails
func TestCollectProcessUtilization_ApiFailure(t *testing.T) {
	mockDevice := &mockProcessDevice{
		deviceInfo: &safenvml.DeviceInfo{
			UUID:      testDeviceUUID,
			Memory:    testDeviceMemory,
			CoreCount: testDeviceCoreCount,
		},
		processUtilizationError: &safenvml.NvmlAPIError{
			APIName:       "GetProcessUtilization",
			NvmlErrorCode: nvml.ERROR_NOT_FOUND,
		},
	}

	collector := &processCollector{device: mockDevice}
	metrics, err := collector.collectProcessUtilization()

	// Should return no error (handled gracefully)
	assert.NoError(t, err)

	// Should emit exactly 2 metrics: core.limit + gr_engine_active
	assert.Len(t, metrics, 2)

	// Find the metrics
	var coreLimit, grEngine *Metric
	for i, metric := range metrics {
		if metric.Name == "core.limit" {
			coreLimit = &metrics[i]
		} else if metric.Name == "gr_engine_active" {
			grEngine = &metrics[i]
		}
	}

	// Verify core.limit metric
	require.NotNil(t, coreLimit)
	assert.Equal(t, float64(testDeviceCoreCount), coreLimit.Value)
	assert.Empty(t, coreLimit.Tags, "core.limit should have empty tags when API fails")

	// Verify gr_engine_active metric
	require.NotNil(t, grEngine)
	assert.Equal(t, float64(0), grEngine.Value, "gr_engine_active should be 0 when no processes")
	assert.Empty(t, grEngine.Tags, "gr_engine_active should have empty tags when API fails")
}
