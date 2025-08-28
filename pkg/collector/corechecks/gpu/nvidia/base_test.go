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

func TestNewBaseCollector(t *testing.T) {
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	mockDevice := devices[0]

	tests := []struct {
		name        string
		apiCalls    []apiCallInfo
		expectError bool
	}{
		{
			name: "all APIs supported",
			apiCalls: []apiCallInfo{
				{
					Name:     "test_api1",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
				{
					Name:     "test_api2",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
			},
			expectError: false,
		},
		{
			name: "some APIs unsupported",
			apiCalls: []apiCallInfo{
				{
					Name:     "supported_api",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
				{
					Name: "unsupported_api",
					TestFunc: func(ddnvml.Device) error {
						return ddnvml.NewNvmlAPIErrorOrNil("unsupported_api", nvml.ERROR_NOT_SUPPORTED)
					},
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
			},
			expectError: false,
		},
		{
			name: "no APIs supported",
			apiCalls: []apiCallInfo{
				{
					Name: "unsupported_api1",
					TestFunc: func(ddnvml.Device) error {
						return ddnvml.NewNvmlAPIErrorOrNil("unsupported_api1", nvml.ERROR_NOT_SUPPORTED)
					},
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
				},
				{
					Name: "unsupported_api2",
					TestFunc: func(ddnvml.Device) error {
						return ddnvml.NewNvmlAPIErrorOrNil("unsupported_api2", nvml.ERROR_NOT_SUPPORTED)
					},
					CallFunc: func(ddnvml.Device, uint64) ([]Metric, uint64, error) { return []Metric{}, 0, nil },
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
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	mockDevice := devices[0]

	tests := []struct {
		name            string
		apiCalls        []apiCallInfo
		expectError     bool
		expectedMetrics []string
		expectedLen     int
	}{
		{
			name: "successful stateless collection",
			apiCalls: []apiCallInfo{
				{
					Name:     "metric1",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(device ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
						return []Metric{
							{Name: "test.metric1", Value: 1.0, Type: metrics.GaugeType},
						}, 0, nil
					},
				},
				{
					Name:     "metric2",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(device ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
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
			name: "collection with errors",
			apiCalls: []apiCallInfo{
				{
					Name:     "working_api",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(device ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
						return []Metric{{Name: "test.working", Value: 1.0, Type: metrics.GaugeType}}, 0, nil
					},
				},
				{
					Name:     "failing_api",
					TestFunc: func(ddnvml.Device) error { return nil },
					CallFunc: func(device ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
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
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	ddnvml.WithMockNVML(t, nvmlMock)

	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	devices := deviceCache.AllPhysicalDevices()
	require.NotEmpty(t, devices)
	mockDevice := devices[0]

	timestampTracker := make(map[int]uint64) // Track timestamps between calls

	apiCalls := []apiCallInfo{
		{
			Name:     "sampling_api",
			TestFunc: func(ddnvml.Device) error { return nil },
			CallFunc: func(device ddnvml.Device, timestamp uint64) ([]Metric, uint64, error) {
				// Sampling collector - should receive non-zero timestamp after first call
				timestampTracker[len(timestampTracker)] = timestamp
				newTimestamp := timestamp + 10
				return []Metric{{Name: "test.sampling", Value: 1.0, Type: metrics.GaugeType}}, newTimestamp, nil
			},
		},
	}

	collector, err := newSamplingCollector("test_sampling", mockDevice, apiCalls)
	require.NoError(t, err)
	require.NotNil(t, collector)

	// First collect
	metrics1, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics1, 1)

	// Second collect - timestamp should be different
	metrics2, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, metrics2, 1)

	// Verify timestamps increased between calls
	require.Len(t, timestampTracker, 2)
	require.NotZero(t, timestampTracker[0])                      // First call should have non-zero timestamp
	require.Greater(t, timestampTracker[1], timestampTracker[0]) // Second call should have greater timestamp
}
