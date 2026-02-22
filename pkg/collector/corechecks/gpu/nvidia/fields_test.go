// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// mockFieldValues maps each field ID to its expected metric value.
var mockFieldValues = map[uint32]uint32{
	nvml.FI_DEV_MEMORY_TEMP:                       42,
	nvml.FI_DEV_PCIE_REPLAY_COUNTER:               7,
	nvml.FI_DEV_PERF_POLICY_THERMAL:               85,
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX:         1000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX:         2000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX:          3000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX:          4000,
	nvml.FI_DEV_NVLINK_GET_SPEED:                  25000,
	nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON:          24000,
	nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT:     16,
	nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL: 1,
	nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL: 2,
	nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL: 3,
	nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL: 4,
	nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL:   5,
}

// fieldValuesMockFunc returns a GetFieldValuesFunc that reads values from the
// provided map. The test can mutate the map between Collect calls to control
// exactly what each collection sees.
// If unsupportedField is non-zero, that field ID returns ERROR_NOT_SUPPORTED.
func fieldValuesMockFunc(values map[uint32]uint32, unsupportedField uint32) func([]nvml.FieldValue) nvml.Return {
	return func(fv []nvml.FieldValue) nvml.Return {
		for i := range fv {
			if unsupportedField != 0 && fv[i].FieldId == unsupportedField {
				fv[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
			} else {
				fv[i].NvmlReturn = uint32(nvml.SUCCESS)
				fv[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
				binary.LittleEndian.PutUint32(fv[i].Value[:], values[fv[i].FieldId])
			}
		}
		return nvml.SUCCESS
	}
}

// copyMockFieldValues returns a mutable copy of mockFieldValues.
func copyMockFieldValues() map[uint32]uint32 {
	m := make(map[uint32]uint32, len(mockFieldValues))
	for k, v := range mockFieldValues {
		m[k] = v
	}
	return m
}

func TestFieldsCollector_AllMetricsEmitted(t *testing.T) {
	returnValues := copyMockFieldValues()
	device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
		d.GetFieldValuesFunc = fieldValuesMockFunc(returnValues, 0)
		return d
	})

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc.now = func() time.Time {
		t := now
		now = now.Add(time.Second)
		return t
	}

	// First collect: sets baseline for rate metrics (they won't be emitted yet)
	_, err = fc.Collect()
	require.NoError(t, err)

	// Bump rate metric values by a fixed delta before the second collect.
	// With a 1s clock step, rate = rateDelta / 1s = rateDelta.
	// This is intentionally different from the base values so the test
	// would fail if rate computation were accidentally skipped (the raw
	// value would be base + rateDelta, not rateDelta).
	const rateDelta uint32 = 500
	for _, fm := range allFieldMetrics {
		if fm.computeRate {
			returnValues[fm.fieldValueID] += rateDelta
		}
	}

	// Second collect: all metrics including rate-computed ones should appear
	collected, err := fc.Collect()
	require.NoError(t, err)

	// Build expected values.
	// Non-rate: the map holds mockFieldValues[fieldID] (unchanged) → expected = base.
	// Rate:     delta = rateDelta, timeDelta = 1s → expected = rateDelta.
	expectedValues := make(map[string][]float64)
	for _, fm := range allFieldMetrics {
		if fm.computeRate {
			expectedValues[fm.name] = append(expectedValues[fm.name], float64(rateDelta))
		} else {
			expectedValues[fm.name] = append(expectedValues[fm.name], float64(mockFieldValues[fm.fieldValueID]))
		}
	}

	emittedValues := make(map[string][]float64)
	for _, m := range collected {
		emittedValues[m.Name] = append(emittedValues[m.Name], m.Value)
	}

	require.Equal(t, len(expectedValues), len(emittedValues), "number of unique metric names should match")
	for name, expectedVals := range expectedValues {
		require.ElementsMatch(t, expectedVals, emittedValues[name], "values mismatch for metric %s", name)
	}
}

func TestFieldsCollector_NvlinkSpeedPriority(t *testing.T) {
	tests := []struct {
		name             string
		unsupportedField uint32 // field ID to mark as unsupported; 0 means all supported
		expectPriority   MetricPriority
		expectValue      float64
	}{
		{
			name:           "both supported, newer wins after dedup",
			expectPriority: MediumLow,
			expectValue:    float64(mockFieldValues[nvml.FI_DEV_NVLINK_GET_SPEED]),
		},
		{
			name:             "newer unsupported, legacy selected",
			unsupportedField: nvml.FI_DEV_NVLINK_GET_SPEED,
			expectPriority:   Low,
			expectValue:      float64(mockFieldValues[nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON]),
		},
		{
			name:             "legacy unsupported, newer selected",
			unsupportedField: nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON,
			expectPriority:   MediumLow,
			expectValue:      float64(mockFieldValues[nvml.FI_DEV_NVLINK_GET_SPEED]),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			returnValues := copyMockFieldValues()
			device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
				d.GetFieldValuesFunc = fieldValuesMockFunc(returnValues, tt.unsupportedField)
				return d
			})

			collector, err := newFieldsCollector(device, nil)
			require.NoError(t, err)

			fc, ok := collector.(*fieldsCollector)
			require.True(t, ok, "expected *fieldsCollector")
			now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			fc.now = func() time.Time {
				t := now
				now = now.Add(time.Second)
				return t
			}

			// Two collects so rate metrics are also present
			_, err = fc.Collect()
			require.NoError(t, err)

			for _, fm := range allFieldMetrics {
				if fm.computeRate {
					returnValues[fm.fieldValueID] += 500
				}
			}

			collected, err := fc.Collect()
			require.NoError(t, err)

			// Run through RemoveDuplicateMetrics, same as the real check
			deduped := RemoveDuplicateMetrics(map[CollectorName][]Metric{
				field: collected,
			})

			var nvlinkSpeed []Metric
			for _, m := range deduped {
				if m.Name == "nvlink.speed" {
					nvlinkSpeed = append(nvlinkSpeed, m)
				}
			}

			require.Len(t, nvlinkSpeed, 1, "exactly one nvlink.speed metric should survive dedup")
			require.Equal(t, tt.expectPriority, nvlinkSpeed[0].Priority)
			require.Equal(t, tt.expectValue, nvlinkSpeed[0].Value)
		})
	}
}

func TestFieldsCollectorNegativeDelta(t *testing.T) {
	returnValues := make(map[uint32]uint32)
	device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
		d.GetFieldValuesFunc = fieldValuesMockFunc(returnValues, 0)
		return d
	})

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	fc.now = func() time.Time {
		t := now
		now = now.Add(time.Second)
		return t
	}

	deltaPositiveID := uint32(1)
	deltaNegativeID := uint32(2)
	deltaZeroID := uint32(3)
	fc.fieldMetrics = []fieldValueMetric{
		{name: "deltaPositive", fieldValueID: deltaPositiveID, metricType: metrics.GaugeType, computeRate: true},
		{name: "deltaNegative", fieldValueID: deltaNegativeID, metricType: metrics.GaugeType, computeRate: true},
		{name: "deltaZero", fieldValueID: deltaZeroID, metricType: metrics.GaugeType, computeRate: true},
	}

	baseValue := uint32(1000)
	returnValues[deltaPositiveID] = baseValue
	returnValues[deltaNegativeID] = baseValue
	returnValues[deltaZeroID] = baseValue

	// First collection, ignore these values
	_, err = fc.Collect()
	require.NoError(t, err)

	// Now increment to create the deltas we want
	delta := uint32(500)
	returnValues[deltaPositiveID] = returnValues[deltaPositiveID] + delta
	returnValues[deltaNegativeID] = returnValues[deltaNegativeID] - delta

	collected, err := fc.Collect()
	require.NoError(t, err)

	foundPositive, foundNegative, foundZero := false, false, false
	for _, m := range collected {
		if m.Name == "deltaPositive" {
			foundPositive = true
			require.Equal(t, float64(delta), m.Value)
		}
		if m.Name == "deltaNegative" {
			foundNegative = true
			require.Equal(t, float64(0), m.Value)
		}
		if m.Name == "deltaZero" {
			foundZero = true
			require.Equal(t, float64(0), m.Value)
		}
	}

	require.True(t, foundPositive, "deltaPositive metric should be present")
	require.True(t, foundNegative, "deltaNegative metric should be present")
	require.True(t, foundZero, "deltaZero metric should be present")
}
