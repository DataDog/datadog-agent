// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// mockFieldValues maps each field ID to its expected metric value.
var mockFieldValues = map[uint32]uint32{
	nvml.FI_DEV_MEMORY_TEMP:                                  42,
	nvml.FI_DEV_PCIE_REPLAY_COUNTER:                          7,
	nvml.FI_DEV_PERF_POLICY_THERMAL:                          85,
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX:                    1000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX:                    2000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX:                     3000,
	nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX:                     4000,
	nvml.FI_DEV_NVLINK_GET_SPEED:                             25000,
	nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON:                     24000,
	nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT:                16,
	nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL:            1,
	nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL:            2,
	nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL:            3,
	nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL:            4,
	nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL:              5,
	nvml.FI_DEV_NVLINK_COUNT_XMIT_PACKETS:                    6,
	nvml.FI_DEV_NVLINK_COUNT_RCV_PACKETS:                     7,
	nvml.FI_DEV_NVLINK_COUNT_XMIT_DISCARDS:                   8,
	nvml.FI_DEV_NVLINK_COUNT_MALFORMED_PACKET_ERRORS:         9,
	nvml.FI_DEV_NVLINK_COUNT_BUFFER_OVERRUN_ERRORS:           10,
	nvml.FI_DEV_NVLINK_COUNT_RCV_ERRORS:                      11,
	nvml.FI_DEV_NVLINK_COUNT_RCV_REMOTE_ERRORS:               12,
	nvml.FI_DEV_NVLINK_COUNT_RCV_GENERAL_ERRORS:              13,
	nvml.FI_DEV_NVLINK_COUNT_LOCAL_LINK_INTEGRITY_ERRORS:     14,
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_SUCCESSFUL_EVENTS: 15,
	nvml.FI_DEV_NVLINK_COUNT_LINK_RECOVERY_FAILED_EVENTS:     16,
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS:                17,
	nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_BER:                   18,
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_ERRORS:                   19,
	nvml.FI_DEV_NVLINK_COUNT_SYMBOL_BER:                      20,
	nvml.FI_DEV_C2C_LINK_ERROR_INTR:                          37,
	nvml.FI_DEV_C2C_LINK_ERROR_REPLAY:                        38,
	nvml.FI_DEV_C2C_LINK_ERROR_REPLAY_B2B:                    39,
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

	collected, err := collector.Collect()
	require.NoError(t, err)

	// The fields collector now emits raw field values and annotates how rates
	// should be calculated later in the pipeline.
	expectedValues := make(map[string][]float64)
	expectedRateModes := make(map[string][]RateCalculationMode)
	for _, fm := range allFieldMetrics {
		expectedValues[fm.name] = append(expectedValues[fm.name], float64(mockFieldValues[fm.fieldValueID]))
		expectedRateModes[fm.name] = append(expectedRateModes[fm.name], fm.rateCalculationMode)
	}

	emittedValues := make(map[string][]float64)
	emittedRateModes := make(map[string][]RateCalculationMode)
	for _, m := range collected {
		emittedValues[m.Name] = append(emittedValues[m.Name], m.Value)
		emittedRateModes[m.Name] = append(emittedRateModes[m.Name], m.RateCalculationMode)
	}

	require.Equal(t, len(expectedValues), len(emittedValues), "number of unique metric names should match")
	for name, expectedVals := range expectedValues {
		require.ElementsMatch(t, expectedVals, emittedValues[name], "values mismatch for metric %s", name)
		require.ElementsMatch(t, expectedRateModes[name], emittedRateModes[name], "rate modes mismatch for metric %s", name)
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

			collected, err := collector.Collect()
			require.NoError(t, err)

			// Run through RemoveDuplicateMetrics, same as the real check
			deduped := RemoveDuplicateMetrics(map[CollectorName][]*Metric{
				field: collected,
			})

			var nvlinkSpeed []*Metric
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

func TestFieldsCollectorPreservesRawValuesForRateMetrics(t *testing.T) {
	returnValues := make(map[uint32]uint32)
	device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
		d.GetFieldValuesFunc = fieldValuesMockFunc(returnValues, 0)
		return d
	})

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")

	positiveID := uint32(1)
	negativeID := uint32(2)
	zeroID := uint32(3)
	fc.fieldMetrics = []fieldValueMetric{
		{name: "deltaPositive", fieldValueID: positiveID, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
		{name: "deltaNegative", fieldValueID: negativeID, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
		{name: "deltaZero", fieldValueID: zeroID, metricType: metrics.GaugeType, rateCalculationMode: PerSecondRateCalculation},
	}

	returnValues[positiveID] = 1500
	returnValues[negativeID] = 500
	returnValues[zeroID] = 1000

	collected, err := fc.Collect()
	require.NoError(t, err)

	foundPositive, foundNegative, foundZero := false, false, false
	for _, m := range collected {
		if m.Name == "deltaPositive" {
			foundPositive = true
			require.Equal(t, 1500.0, m.Value)
			require.Equal(t, PerSecondRateCalculation, m.RateCalculationMode)
		}
		if m.Name == "deltaNegative" {
			foundNegative = true
			require.Equal(t, 500.0, m.Value)
			require.Equal(t, PerSecondRateCalculation, m.RateCalculationMode)
		}
		if m.Name == "deltaZero" {
			foundZero = true
			require.Equal(t, 1000.0, m.Value)
			require.Equal(t, PerSecondRateCalculation, m.RateCalculationMode)
		}
	}

	require.True(t, foundPositive, "deltaPositive metric should be present")
	require.True(t, foundNegative, "deltaNegative metric should be present")
	require.True(t, foundZero, "deltaZero metric should be present")
}

func TestFieldsCollectorRemovesUnsupportedField(t *testing.T) {
	returnValues := copyMockFieldValues()
	device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
		d.GetFieldValuesFunc = fieldValuesMockFunc(returnValues, nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS)
		return d
	})

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")

	for _, metric := range fc.fieldMetrics {
		require.NotEqual(t, "nvlink.errors.effective", metric.name)
	}
}

func TestFieldsCollectorTreatsInvalidArgumentAsUnsupportedOnlyWhenConfigured(t *testing.T) {
	returnValues := copyMockFieldValues()
	device := setupMockDevice(t, func(d *mock.Device) *mock.Device {
		d.GetFieldValuesFunc = func(fv []nvml.FieldValue) nvml.Return {
			for i := range fv {
				switch fv[i].FieldId {
				case nvml.FI_DEV_C2C_LINK_ERROR_INTR:
					fv[i].NvmlReturn = uint32(nvml.ERROR_INVALID_ARGUMENT)
				case nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS:
					fv[i].NvmlReturn = uint32(nvml.ERROR_INVALID_ARGUMENT)
				default:
					fv[i].NvmlReturn = uint32(nvml.SUCCESS)
					fv[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
					binary.LittleEndian.PutUint32(fv[i].Value[:], returnValues[fv[i].FieldId])
				}
			}
			return nvml.SUCCESS
		}
		return d
	})

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")

	foundC2CInterrupt := false
	foundNvlinkEffective := false
	for _, metric := range fc.fieldMetrics {
		switch metric.name {
		case "c2c.errors.interrupt":
			foundC2CInterrupt = true
		case "nvlink.errors.effective":
			foundNvlinkEffective = true
		}
	}

	require.False(t, foundC2CInterrupt, "c2c.errors.interrupt should be removed when INVALID_ARGUMENT is explicitly mapped to unsupported")
	require.True(t, foundNvlinkEffective, "nvlink.errors.effective should remain when INVALID_ARGUMENT is not explicitly mapped to unsupported")
}
