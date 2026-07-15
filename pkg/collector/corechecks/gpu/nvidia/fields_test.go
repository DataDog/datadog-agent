// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestFieldsCollector_AllMetricsEmitted(t *testing.T) {
	device := setupMockDevice(t)

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	collected, err := collector.Collect()
	require.NoError(t, err)

	// The fields collector now emits raw field values and annotates how rates
	// should be calculated later in the pipeline.
	expectedValues := make(map[string][]float64)
	expectedRateModes := make(map[string][]RateCalculationMode)
	for _, fm := range allFieldMetrics {
		expectedValues[fm.name] = append(expectedValues[fm.name], float64(testutil.DefaultFieldValues[fm.fieldValueID].Value))
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
func TestFieldsCollectorPreservesRawValuesForRateMetrics(t *testing.T) {
	returnValues := make(map[uint32]testutil.MockFieldValue)
	device := setupMockDevice(t, testutil.WithFieldValuesFullOverride(returnValues))

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

	returnValues[positiveID] = testutil.NewFieldValue(1500)
	returnValues[negativeID] = testutil.NewFieldValue(500)
	returnValues[zeroID] = testutil.NewFieldValue(1000)

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
	device := setupMockDevice(t,
		testutil.WithUnsupportedFields(nvml.FI_DEV_NVLINK_COUNT_EFFECTIVE_ERRORS),
	)

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")

	for _, metric := range fc.fieldMetrics {
		require.NotEqual(t, "nvlink.errors.effective", metric.name)
	}
}

func TestFieldsCollectorTreatsInvalidArgumentAsUnsupportedOnlyWhenConfigured(t *testing.T) {
	device := setupMockDevice(t,
		testutil.WithInvalidArgumentFields(nvml.FI_DEV_C2C_LINK_ERROR_INTR),
	)

	collector, err := newFieldsCollector(device, nil)
	require.NoError(t, err)

	fc, ok := collector.(*fieldsCollector)
	require.True(t, ok, "expected *fieldsCollector")

	foundC2CInterrupt := false
	for _, metric := range fc.fieldMetrics {
		switch metric.name {
		case "c2c.errors.interrupt":
			foundC2CInterrupt = true
		}
	}

	require.False(t, foundC2CInterrupt, "c2c.errors.interrupt should be removed when INVALID_ARGUMENT is explicitly mapped to unsupported")
}
