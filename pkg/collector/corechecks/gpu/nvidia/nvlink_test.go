// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"errors"
	"fmt"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// mockSafeDevice is a mock implementation of the Device interface
// It only implements the methods needed for testing the nvlink collector
type mockSafeDevice struct {
	ddnvml.SafeDevice // Embed the interface to satisfy it

	fieldValuesFunc func([]nvml.FieldValue) error
	nvLinkStateFunc func(int) (nvml.EnableState, error)
	uuid            string
}

// GetDeviceInfo implements ddnvml.Device interface
func (m *mockSafeDevice) GetDeviceInfo() *ddnvml.DeviceInfo {
	return &ddnvml.DeviceInfo{UUID: m.uuid}
}

// GetFieldValues implements SafeDevice.GetFieldValues
func (m *mockSafeDevice) GetFieldValues(values []nvml.FieldValue) error {
	if m.fieldValuesFunc != nil {
		return m.fieldValuesFunc(values)
	}
	return fmt.Errorf("GetFieldValues not implemented")
}

// GetNvLinkState implements SafeDevice.GetNvLinkState
func (m *mockSafeDevice) GetNvLinkState(link int) (nvml.EnableState, error) {
	if m.nvLinkStateFunc != nil {
		return m.nvLinkStateFunc(link)
	}
	return 0, fmt.Errorf("GetNvLinkState not implemented")
}

// GetUUID implements SafeDevice.GetUUID
func (m *mockSafeDevice) GetUUID() (string, error) {
	return m.uuid, nil
}

func TestNewNVLinkCollector(t *testing.T) {
	tests := []struct {
		name      string
		mockSetup func() ddnvml.Device
		wantError bool
		wantLinks int
	}{
		{
			name: "Unsupported device",
			mockSetup: func() ddnvml.Device {
				return &mockSafeDevice{
					fieldValuesFunc: func(_ []nvml.FieldValue) error {
						return &ddnvml.NvmlAPIError{APIName: "GetFieldValues", NvmlErrorCode: nvml.ERROR_NOT_SUPPORTED}
					},
					uuid: "GPU-123",
				}
			},
			wantError: true,
		},
		{
			name: "Unknown error",
			mockSetup: func() ddnvml.Device {
				return &mockSafeDevice{
					fieldValuesFunc: func(_ []nvml.FieldValue) error {
						return &ddnvml.NvmlAPIError{APIName: "GetFieldValues", NvmlErrorCode: nvml.ERROR_UNKNOWN}
					},
					uuid: "GPU-123",
				}
			},
			wantError: true,
		},
		{
			name: "Success with 4 links",
			mockSetup: func() ddnvml.Device {
				return &mockSafeDevice{
					fieldValuesFunc: func(values []nvml.FieldValue) error {
						require.Len(t, values, 1, "Expected one field value for total number of links, got %d", len(values))
						require.Equal(t, values[0].FieldId, uint32(nvml.FI_DEV_NVLINK_LINK_COUNT), "Expected field ID to be FI_DEV_NVLINK_LINK_COUNT, got %d", values[0].FieldId)
						require.Equal(t, values[0].ScopeId, uint32(0), "Expected scope ID to be 0, got %d", values[0].ScopeId)
						values[0].ValueType = uint32(nvml.VALUE_TYPE_SIGNED_INT)
						values[0].Value = [8]byte{4, 0, 0, 0, 0, 0, 0, 0} // 4 links
						return nil
					},
					uuid: "GPU-123",
				}
			},
			wantError: false,
			wantLinks: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDevice := tt.mockSetup()
			c, err := newNVLinkCollector(mockDevice)

			if tt.wantError {
				require.Error(t, err)
				require.Nil(t, c)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, c)

			typedC, ok := c.(*nvlinkCollector)
			require.True(t, ok)
			require.Equal(t, tt.wantLinks, typedC.totalNVLinks)
		})
	}
}

func TestNVLinkCollector_Collect(t *testing.T) {
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
			// Create a mock device using our mockSafeDevice
			mockDevice := &mockSafeDevice{
				fieldValuesFunc: func(values []nvml.FieldValue) error {
					values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					values[0].Value = [8]byte{byte(len(tt.nvlinkStates)), 0, 0, 0, 0, 0, 0, 0}
					return nil
				},
				nvLinkStateFunc: func(link int) (nvml.EnableState, error) {
					if link >= len(tt.nvlinkStates) {
						return 0, fmt.Errorf("link index out of range")
					}
					return tt.nvlinkStates[link], tt.nvlinkErrors[link]
				},
				uuid: "GPU-123",
			}

			// Create collector
			collector, err := newNVLinkCollector(mockDevice)
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
