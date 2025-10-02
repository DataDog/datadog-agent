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

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// setupMockDevice creates a mock device for testing with optional customization.
// If customize is nil, returns a basic mock device with all functions enabled.
// If customize is provided, allows overriding specific device functions.
func setupMockDevice(t *testing.T, customize func(device *mock.Device) *mock.Device) ddnvml.Device {
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	device := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions())

	// Apply customization if provided
	if customize != nil {
		device = customize(device)
	}

	// Set up the device handle function
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		if index == 0 {
			return device, nvml.SUCCESS
		}
		return nil, nvml.ERROR_INVALID_ARGUMENT
	}

	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	return devices[0]
}

func TestNewBaseCollector(t *testing.T) {
	mockDevice := setupMockDevice(t, nil)

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
	mockDevice := setupMockDevice(t, nil)

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

	mockDevice := setupMockDevice(t, nil)

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
