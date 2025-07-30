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
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// Mock device for process collector tests
type mockProcessDevice struct {
	safenvml.SafeDevice

	uuid                    string
	samples                 []nvml.ProcessUtilizationSample
	processUtilizationError error
}

func (m *mockProcessDevice) GetUUID() (string, error) {
	return m.uuid, nil
}

func (m *mockProcessDevice) GetProcessUtilization(_ uint64) ([]nvml.ProcessUtilizationSample, error) {
	if m.processUtilizationError != nil {
		return nil, m.processUtilizationError
	}
	return m.samples, nil
}

func TestNewProcessCollector(t *testing.T) {
	tests := []struct {
		name                    string
		processUtilizationError error
		wantError               bool
	}{
		{
			name:                    "API_Supported",
			processUtilizationError: nil,
			wantError:               false,
		},
		{
			name:                    "API_NotSupported",
			processUtilizationError: &safenvml.NvmlAPIError{APIName: "GetProcessUtilization", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED},
			wantError:               true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				uuid:                    "test-uuid",
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
				assert.Equal(t, process, collector.Name())
				assert.Equal(t, "test-uuid", collector.DeviceUUID())
			}
		})
	}
}

func TestProcessCollector_Collect(t *testing.T) {
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
			description:      "Should return 0 when no processes are running",
		},
		{
			name: "SingleProcess",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, SmUtil: 60, TimeStamp: 1000},
			},
			expectedSmActive: 60.0,
			description:      "Single process: median should equal the process utilization",
		},
		{
			name: "TwoConcurrentProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, SmUtil: 40, TimeStamp: 1000},
				{Pid: 5678, SmUtil: 30, TimeStamp: 1000},
			},
			expectedSmActive: 55.0,
			description:      "Two processes: median between max and sum",
		},
		{
			name: "HighUtilizationProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1234, SmUtil: 80, TimeStamp: 1000},
				{Pid: 5678, SmUtil: 60, TimeStamp: 1000},
			},
			expectedSmActive: 90.0,
			description:      "High utilization: sum is capped at 100",
		},
		{
			name: "ManySmallProcesses",
			samples: []nvml.ProcessUtilizationSample{
				{Pid: 1, SmUtil: 10, TimeStamp: 1000},
				{Pid: 2, SmUtil: 15, TimeStamp: 1000},
				{Pid: 3, SmUtil: 12, TimeStamp: 1000},
				{Pid: 4, SmUtil: 8, TimeStamp: 1000},
			},
			expectedSmActive: 30,
			description:      "Many small processes: median smooths between max and sum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				uuid:    "test-uuid",
				samples: tt.samples,
			}

			collector := &processCollector{device: mockDevice}
			m, err := collector.Collect()

			require.NoError(t, err, "Collect should not return error for valid samples")
			require.Len(t, m, 1, "Should return exactly one metric")

			metric := m[0]
			assert.Equal(t, "sm_active", metric.Name)
			assert.Equal(t, metrics.GaugeType, metric.Type)
			assert.InDelta(t, tt.expectedSmActive, metric.Value, 0.01, tt.description)
		})
	}
}

func TestProcessCollector_CollectErrorHandling(t *testing.T) {
	tests := []struct {
		name                    string
		processUtilizationError error
		expectedSmActive        float64
		expectError             bool
		description             string
	}{
		{
			name: "ERROR_NOT_FOUND",
			processUtilizationError: &safenvml.NvmlAPIError{
				APIName:       "GetProcessUtilization",
				NvmlErrorCode: nvml.ERROR_NOT_FOUND,
			},
			expectedSmActive: 0.0,
			expectError:      false,
			description:      "ERROR_NOT_FOUND should return 0 without error",
		},
		{
			name: "Other_API_Error",
			processUtilizationError: &safenvml.NvmlAPIError{
				APIName:       "GetProcessUtilization",
				NvmlErrorCode: nvml.ERROR_UNKNOWN,
			},
			expectError: true,
			description: "Other NVML errors should be returned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := &mockProcessDevice{
				uuid:                    "test-uuid",
				processUtilizationError: tt.processUtilizationError,
			}

			collector := &processCollector{device: mockDevice}
			metrics, err := collector.Collect()

			if tt.expectError {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				require.Len(t, metrics, 1, "Should return one metric even on ERROR_NOT_FOUND")
				assert.Equal(t, "sm_active", metrics[0].Name)
				assert.Equal(t, tt.expectedSmActive, metrics[0].Value, tt.description)
			}
		})
	}
}

func TestProcessCollector_TimestampUpdate(t *testing.T) {
	samples := []nvml.ProcessUtilizationSample{
		{Pid: 1234, SmUtil: 50, TimeStamp: 2000},
		{Pid: 5678, SmUtil: 30, TimeStamp: 1500}, // Older timestamp
		{Pid: 9012, SmUtil: 40, TimeStamp: 2500}, // Newest timestamp
	}

	mockDevice := &mockProcessDevice{
		uuid:    "test-uuid",
		samples: samples,
	}

	collector := &processCollector{device: mockDevice, lastTimestamp: 1000}
	_, err := collector.Collect()

	require.NoError(t, err)
	assert.Equal(t, uint64(2500), collector.lastTimestamp, "Should update to the latest timestamp")
}
