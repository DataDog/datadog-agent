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
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// setupMockDevice creates a single mock physical device for testing with optional
// customization. The mock exposes exactly one MIG-disabled physical device.
func setupMockDevice(t *testing.T, extraMockOpts ...testutil.NvmlMockOption) ddnvml.Device {
	t.Helper()

	opts := append([]testutil.NvmlMockOption{
		testutil.WithMIGDisabled(),
		testutil.WithDeviceCount(1),
	}, extraMockOpts...)
	devices := setupMockDevices(t, opts...)
	require.NotEmpty(t, devices)
	return devices[0]
}

// setupMockDevices installs a mock NVML interface (configured via the given options)
// and returns all physical devices exposed by the resulting device cache.
func setupMockDevices(t *testing.T, mockOpts ...testutil.NvmlMockOption) []ddnvml.Device {
	t.Helper()

	nvmlMock := testutil.GetBasicNvmlMockWithOptions(mockOpts...)
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)
	return devices
}

func TestNewBaseCollector(t *testing.T) {
	mockDevice := setupMockDevice(t)

	tests := []struct {
		name        string
		apiCalls    []apiCallInfo
		expectError bool
	}{
		{
			name: "all APIs supported",
			apiCalls: []apiCallInfo{
				{
					Name:    "test_api1",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
				{
					Name:    "test_api2",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
			},
			expectError: false,
		},
		{
			name: "some APIs unsupported",
			apiCalls: []apiCallInfo{
				{
					Name:    "supported_api",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
				{
					Name: "unsupported_api",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) {
						return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("unsupported_api", nvml.ERROR_NOT_SUPPORTED)
					},
				},
			},
			expectError: false,
		},
		{
			name: "no APIs supported",
			apiCalls: []apiCallInfo{
				{
					Name: "unsupported_api1",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) {
						return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("unsupported_api1", nvml.ERROR_NOT_SUPPORTED)
					},
				},
				{
					Name: "unsupported_api2",
					Handler: func(ddnvml.Device, uint64) ([]Metric, uint64, error) {
						return nil, 0, ddnvml.NewNvmlAPIErrorOrNil("unsupported_api2", nvml.ERROR_NOT_SUPPORTED)
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewBaseCollector("test", mockDevice, tt.apiCalls)

			if tt.expectError {
				require.Error(t, err)
				require.Nil(t, collector)
			} else {
				require.NoError(t, err)
				require.NotNil(t, collector)
				require.Equal(t, testutil.GPUUUIDs[0], collector.DeviceUUID())
				require.Equal(t, CollectorName("test"), collector.Name())
			}
		})
	}
}

func TestBaseCollector_Collect(t *testing.T) {
	mockDevice := setupMockDevice(t)

	tests := []struct {
		name            string
		apiCalls        []apiCallInfo
		expectError     bool
		expectedMetrics []string
		expectedLen     int
	}{
		{
			name: "happy flow",
			apiCalls: []apiCallInfo{
				{
					Name: "metric1",
					Handler: func(_ ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
						return []Metric{
							{Name: "test.metric1", Value: 1.0, Type: metrics.GaugeType},
						}, 0, nil
					},
				},
				{
					Name: "metric2",
					Handler: func(_ ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
						return []Metric{
							{Name: "test.metric2", Value: 2.0, Type: metrics.GaugeType},
							{Name: "test.metric3", Value: 3.0, Type: metrics.GaugeType},
						}, 0, nil
					},
				},
			},
			expectError:     false,
			expectedMetrics: []string{"test.metric1", "test.metric2", "test.metric3"},
			expectedLen:     3,
		},
		{
			name: "partial errors",
			apiCalls: []apiCallInfo{
				{
					Name: "working_api",
					Handler: func(_ ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
						return []Metric{{Name: "test.working", Value: 1.0, Type: metrics.GaugeType}}, 0, nil
					},
				},
				{
					Name: "failing_api",
					Handler: func(_ ddnvml.Device, _ uint64) ([]Metric, uint64, error) {
						return nil, 0, errors.New("API call failed")
					},
				},
			},
			expectError:     true,
			expectedMetrics: []string{"test.working"},
			expectedLen:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector, err := NewBaseCollector("test", mockDevice, tt.apiCalls)
			require.NoError(t, err)

			collectedMetrics, err := collector.Collect()

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Len(t, collectedMetrics, tt.expectedLen)

			// Verify expected metrics are present
			metricNames := make([]string, len(collectedMetrics))
			for i, metric := range collectedMetrics {
				metricNames[i] = metric.Name
				require.Equal(t, metrics.GaugeType, metric.Type)
			}
			require.ElementsMatch(t, tt.expectedMetrics, metricNames)
		})
	}
}

func TestNewSamplingCollector(t *testing.T) {
	var timestamps [2]uint64 // Track first and second call timestamps
	var callCount int

	mockDevice := setupMockDevice(t)

	apiCalls := []apiCallInfo{
		{
			Name: "sampling_api",
			Handler: func(_ ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
				// Sampling collector - should receive non-zero timestamp after first call
				timestamps[callCount] = timestamp
				callCount++
				newTimestamp := timestamp + 10
				return []Metric{{Name: "test.sampling", Value: 1.0, Type: metrics.GaugeType}}, newTimestamp, nil
			},
		},
	}

	collector, err := newStatefulCollector("test_sampling", mockDevice, apiCalls)
	require.NoError(t, err)
	require.NotNil(t, collector)
	//reset the callCount as it was inceremented inside the collector ctor
	callCount = 0

	// First collect
	metrics1, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics1, 1)

	// Second collect - timestamp should be different
	metrics2, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics2, 1)

	// Verify timestamps increased between calls
	require.Equal(t, 2, callCount)
	require.NotZero(t, timestamps[0])                // First call should have non-zero timestamp
	require.Greater(t, timestamps[1], timestamps[0]) // Second call should have greater timestamp
}
