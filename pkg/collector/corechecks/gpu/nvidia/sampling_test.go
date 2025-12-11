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

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
)

// TestNewSampleCollector tests sampling collector initialization
func TestNewSampleCollector(t *testing.T) {
	tests := []struct {
		name                  string
		customSetup           func(*mock.Device) *mock.Device
		expectError           bool
		expectedSupportedAPIs int
	}{
		{
			name:                  "Supported",
			customSetup:           nil, // Use default setup with all functions enabled
			expectError:           false,
			expectedSupportedAPIs: 5,
		},
		{
			name: "Unsupported",
			customSetup: func(device *mock.Device) *mock.Device {
				// Make all sampling APIs return ERROR_NOT_SUPPORTED
				device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					return nil, nvml.ERROR_NOT_SUPPORTED
				}
				device.GetSamplesFunc = func(_ nvml.SamplingType, _ uint64) (nvml.ValueType, []nvml.Sample, nvml.Return) {
					return nvml.ValueType(0), nil, nvml.ERROR_NOT_SUPPORTED
				}
				return device
			},
			expectError:           true,
			expectedSupportedAPIs: 0, // Not relevant when error expected
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := setupMockDevice(t, tt.customSetup)

			collector, err := newSamplingCollector(mockDevice, &CollectorDependencies{})

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, collector)
			} else {
				require.NoError(t, err)
				require.NotNil(t, collector)

				bc := collector.(*baseCollector)
				require.Len(t, bc.supportedAPIs, tt.expectedSupportedAPIs)
			}
		})
	}
}

// TestCollectProcessUtilization tests the process utilization collection
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include process utilization
			originalFactory := sampleAPIFactory
			defer func() { sampleAPIFactory = originalFactory }()

			sampleAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_utilization",
						Handler: func(device safenvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
							return processUtilizationSample(device, lastTimestamp)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					return tt.samples, nvml.SUCCESS
				}
				return device
			})

			collector, err := newSamplingCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			processMetrics, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, processMetrics, tt.expectedCount)
		})
	}
}

// TestCollectProcessUtilization_Error tests error handling
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
			// Override API factory to only include process utilization
			originalFactory := sampleAPIFactory
			defer func() { sampleAPIFactory = originalFactory }()

			sampleAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_utilization",
						Handler: func(device safenvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
							return processUtilizationSample(device, lastTimestamp)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					var nvmlErr *safenvml.NvmlAPIError
					if errors.As(tt.apiError, &nvmlErr) {
						return nil, nvmlErr.NvmlErrorCode
					}
					return nil, nvml.ERROR_UNKNOWN
				}
				return device
			})

			collector, err := newSamplingCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			processMetrics, err := collector.Collect()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, processMetrics, tt.expectedCount)
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
			// Override API factory to only include process utilization
			originalFactory := sampleAPIFactory
			defer func() { sampleAPIFactory = originalFactory }()

			sampleAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_utilization",
						Handler: func(device safenvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
							return processUtilizationSample(device, lastTimestamp)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					if tt.apiError != nil {
						var nvmlErr *safenvml.NvmlAPIError
						if errors.As(tt.apiError, &nvmlErr) {
							return nil, nvmlErr.NvmlErrorCode
						}
						return nil, nvml.ERROR_UNKNOWN
					}
					return tt.samples, nvml.SUCCESS
				}
				return device
			})

			collector, err := newSamplingCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			bc := collector.(*baseCollector)
			bc.lastTimestamps["process_utilization"] = tt.initialTimestamp

			timeBefore := uint64(time.Now().UnixMicro())
			_, err = collector.Collect()
			timeAfter := uint64(time.Now().UnixMicro())

			// Timestamp should be updated to current time regardless of API success/failure
			newTimestamp := bc.lastTimestamps["process_utilization"]
			require.GreaterOrEqual(t, newTimestamp, timeBefore, "Timestamp should be updated to current time")
			require.LessOrEqual(t, newTimestamp, timeAfter, "Timestamp should be updated to current time")
			require.Greater(t, newTimestamp, tt.initialTimestamp, "Timestamp should be newer than initial")

			// Check error handling
			if tt.apiError != nil {
				var nvmlErr *safenvml.NvmlAPIError
				if errors.As(tt.apiError, &nvmlErr) && errors.Is(nvmlErr.NvmlErrorCode, nvml.ERROR_NOT_FOUND) {
					require.NoError(t, err, "ERROR_NOT_FOUND should be handled gracefully")
				} else {
					require.Error(t, err, "Other errors should be returned")
				}
			} else {
				require.NoError(t, err, "Successful API call should not return error")
			}
		})
	}
}

// TestProcessUtilization_SmActiveCalculation tests the sm_active calculation logic
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
			description:      "average of (max=0 + sum=0) / 2 = 0",
		},
		{
			name: "SingleProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 60},
			},
			expectedSmActive: 60.0,
			description:      "average of (max=60 + sum=60) / 2 = 60",
		},
		{
			name: "MultipleProcesses_SumUnderCap",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 30},
				{Pid: 1002, SmUtil: 40},
			},
			expectedSmActive: 55.0,
			description:      "average of (max=40 + sum=70) / 2 = 55",
		},
		{
			name: "MultipleProcesses_SumOverCap",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 80},
				{Pid: 1002, SmUtil: 60},
				{Pid: 1003, SmUtil: 40},
			},
			expectedSmActive: 90.0,
			description:      "average of (max=80 + sum=100) / 2 = 90, sum capped at 100",
		},
		{
			name: "ZeroUtilization",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1001, SmUtil: 0},
				{Pid: 1002, SmUtil: 0},
			},
			expectedSmActive: 0.0,
			description:      "average of (max=0 + sum=0) / 2 = 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Override API factory to only include process utilization
			originalFactory := sampleAPIFactory
			defer func() { sampleAPIFactory = originalFactory }()

			sampleAPIFactory = func() []apiCallInfo {
				return []apiCallInfo{
					{
						Name: "process_utilization",
						Handler: func(device safenvml.Device, lastTimestamp uint64) ([]Metric, uint64, error) {
							return processUtilizationSample(device, lastTimestamp)
						},
					},
				}
			}

			mockDevice := setupMockDevice(t, func(device *mock.Device) *mock.Device {
				device.GetProcessUtilizationFunc = func(_ uint64) ([]nvml.ProcessUtilizationSample, nvml.Return) {
					return tt.samples, nvml.SUCCESS
				}
				return device
			})

			collector, err := newSamplingCollector(mockDevice, &CollectorDependencies{})
			require.NoError(t, err)

			processMetrics, err := collector.Collect()
			require.NoError(t, err)

			// Find the sm_active metric
			var smActiveMetric *Metric
			for _, metric := range processMetrics {
				if metric.Name == "sm_active" {
					smActiveMetric = &metric
					break
				}
			}

			require.NotNil(t, smActiveMetric, "sm_active metric should always be emitted")
			require.Equal(t, tt.expectedSmActive, smActiveMetric.Value, "sm_active value should match expected calculation: %s", tt.description)
			require.Nil(t, smActiveMetric.Tags, "sm_active should have no tags (device-wide metric)")
		})
	}
}
