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

// TestNewProcessCollector tests process collector initialization
func TestNewProcessCollector(t *testing.T) {
	tests := []struct {
		name                    string
		computeProcessesError   error
		processUtilizationError error
		wantError               bool
		expectedApiCount        int
	}{
		{
			name:                    "BothApisSupported",
			computeProcessesError:   nil,
			processUtilizationError: nil,
			wantError:               false,
			expectedApiCount:        2,
		},
		{
			name:                    "OneApiSupported",
			computeProcessesError:   nil,
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			wantError:               false,
			expectedApiCount:        1,
		},
		{
			name:                    "NoApisSupported",
			computeProcessesError:   &safenvml.NvmlAPIError{APIName: "GetComputeRunningProcesses", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			wantError:               true,
			expectedApiCount:        0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				deviceInfo:              &safenvml.DeviceInfo{UUID: "test-uuid"},
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
				assert.Len(t, pc.supportedApiCalls, tt.expectedApiCount)
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
			expectedMetricCount: 0,
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
			expectedMetricCount: 7, // 3 compute + 4 utilization
			expectedPIDCounts:   map[string]int{"1234": 7},
			specificValidations: func(t *testing.T, metrics []Metric) {
				metricsByName := make(map[string]Metric)
				for _, metric := range metrics {
					metricsByName[metric.Name] = metric
				}
				assert.Equal(t, float64(536870912), metricsByName["memory.usage"].Value)
				assert.Equal(t, float64(75), metricsByName["core.utilization"].Value)
				assert.Equal(t, float64(60), metricsByName["dram_active"].Value)
				assert.Equal(t, float64(30), metricsByName["encoder_utilization"].Value)
				assert.Equal(t, float64(15), metricsByName["decoder_utilization"].Value)
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
			expectedMetricCount: 17, // 9 compute (3×3) + 8 utilization (2×4)
			expectedPIDCounts: map[string]int{
				"1001": 7, // 3 compute + 4 utilization
				"1002": 3, // 3 compute only
				"1003": 7, // 3 compute + 4 utilization
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
			expectedMetricCount: 7, // 3 compute + 4 utilization
			expectedPIDCounts: map[string]int{
				"2001": 3, // compute metrics
				"3001": 4, // utilization metrics
			},
			specificValidations: func(t *testing.T, metrics []Metric) {
				computeMetrics := 0
				utilizationMetrics := 0
				for _, metric := range metrics {
					switch metric.Name {
					case "memory.usage", "memory.limit", "core.limit":
						assert.Contains(t, metric.Tags, "pid:2001")
						computeMetrics++
					case "core.utilization", "dram_active", "encoder_utilization", "decoder_utilization":
						assert.Contains(t, metric.Tags, "pid:3001")
						utilizationMetrics++
					}
				}
				assert.Equal(t, 3, computeMetrics)
				assert.Equal(t, 4, utilizationMetrics)
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
			expectedMetricCount: 7, // 3 compute + 4 utilization
			expectedPIDCounts:   map[string]int{"13001": 7},
			specificValidations: func(t *testing.T, metrics []Metric) {
				metricsByName := make(map[string]Metric)
				for _, metric := range metrics {
					metricsByName[metric.Name] = metric
				}
				assert.Equal(t, float64(0), metricsByName["memory.usage"].Value)
				assert.Equal(t, float64(0), metricsByName["core.utilization"].Value)
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
					UUID:      "test-uuid",
					Memory:    8589934592,
					CoreCount: 80,
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
					UUID:      "test-uuid",
					Memory:    8589934592,
					CoreCount: 80,
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
