// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func fecHistoryFieldValues() map[uint32]testutil.MockFieldValue {
	fieldValues := make(map[uint32]testutil.MockFieldValue, len(nvlinkFECHistoryFieldIDs))
	for i, fieldID := range nvlinkFECHistoryFieldIDs {
		fieldValues[fieldID] = testutil.NewFieldValue(uint64(100 + i))
	}
	return fieldValues
}

func TestNVLinkFECCollectorScopesAndBuckets(t *testing.T) {
	mockDevice := setupMockDevice(t,
		testutil.WithNVLinkLinkCount(1),
		testutil.WithFieldValuesFullOverride(fecHistoryFieldValues()),
	)
	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)
	require.Equal(t, nvlinkFEC, collector.Name())
	require.Equal(t, mockDevice.GetDeviceInfo().UUID, collector.Device().GetDeviceInfo().UUID)

	collectedMetrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs)+3)

	expectedLightErrors := 0.0
	expectedHeavyErrors := 0.0
	for bucket := range nvlinkFECHistoryFieldIDs {
		sample, ok := collectedMetrics[bucket].(*HistogramSample)
		require.True(t, ok)
		if bucket > 0 && bucket <= defaultNVLinkFECLightErrorThreshold {
			expectedLightErrors += float64(100 + bucket)
		} else if bucket > defaultNVLinkFECLightErrorThreshold {
			expectedHeavyErrors += float64(100 + bucket)
		}

		require.Equal(t, nvlinkFECHistoryMetricName, sample.Name)
		require.Equal(t, int64(100+bucket), sample.Value)
		require.Equal(t, Medium, sample.Priority())
		require.Contains(t, sample.Tags(), "nvlink_port:1")
		require.Equal(t, [2]float64{float64(bucket), float64(bucket)}, sample.Bounds)
		require.True(t, sample.Monotonic)
		require.False(t, sample.FlushFirstValue)
	}

	require.Equal(t, &Metric{
		baseSample:          baseSample{priority: Medium, tags: []string{"nvlink_port:1"}},
		Name:                nvlinkFECNoErrorsMetricName,
		Type:                metrics.GaugeType,
		Value:               100,
		RateCalculationMode: PerSecondRateCalculation,
	}, collectedMetrics[len(nvlinkFECHistoryFieldIDs)])
	require.Equal(t, &Metric{
		baseSample:          baseSample{priority: Medium, tags: []string{"nvlink_port:1"}},
		Name:                nvlinkFECLightErrorsMetricName,
		Type:                metrics.GaugeType,
		Value:               expectedLightErrors,
		RateCalculationMode: PerSecondRateCalculation,
	}, collectedMetrics[len(nvlinkFECHistoryFieldIDs)+1])
	require.Equal(t, &Metric{
		baseSample:          baseSample{priority: Medium, tags: []string{"nvlink_port:1"}},
		Name:                nvlinkFECHeavyErrorsMetricName,
		Type:                metrics.GaugeType,
		Value:               expectedHeavyErrors,
		RateCalculationMode: PerSecondRateCalculation,
	}, collectedMetrics[len(nvlinkFECHistoryFieldIDs)+2])
}

func TestNVLinkFECCollectorConfigurableLightErrorThreshold(t *testing.T) {
	pkgconfigsetup.Datadog().SetInTest(nvlinkFECLightErrorThresholdConfig, 2)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetInTest(nvlinkFECLightErrorThresholdConfig, defaultNVLinkFECLightErrorThreshold)
	})

	fieldValues := make(map[uint32]testutil.MockFieldValue, len(nvlinkFECHistoryFieldIDs))
	for i, fieldID := range nvlinkFECHistoryFieldIDs {
		fieldValues[fieldID] = testutil.NewFieldValue(uint64(i))
	}

	mockDevice := setupMockDevice(t,
		testutil.WithNVLinkLinkCount(1),
		testutil.WithFieldValuesFullOverride(fieldValues),
	)

	collector, err := newNVLinkFECCollector(mockDevice, &CollectorDependencies{
		Config: pkgconfigsetup.Datadog(),
	})
	require.NoError(t, err)

	collectedMetrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs)+3)
	severityMetrics := requireMetrics(t, collectedMetrics[len(nvlinkFECHistoryFieldIDs):])

	require.Equal(t, 0.0, severityMetrics[0].Value)
	require.Equal(t, 3.0, severityMetrics[1].Value)
	require.Equal(t, 117.0, severityMetrics[2].Value)
}

func TestNVLinkFECCollectorPartialFieldFailure(t *testing.T) {
	fieldValues := fecHistoryFieldValues()

	mock := nvmltestutil.SetupMockNVML(t,
		testutil.WithDeviceCount(1),
		testutil.WithNVLinkLinkCount(1),
		testutil.WithFieldValuesFullOverride(fieldValues),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mock, 0)

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

	// Modify the field values to test partial failure after initial support test
	// one field not supported, another with invalid value type
	fieldValues[nvlinkFECHistoryFieldIDs[3]] = testutil.FieldError(nvml.ERROR_NOT_SUPPORTED)
	fieldValues[nvlinkFECHistoryFieldIDs[7]] = testutil.MockFieldValue{Value: 9999, ValueType: nvml.ValueType(9999), Return: nvml.SUCCESS}
	mock.Device(0).SetFieldValues(fieldValues)

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.ErrorContains(t, err, "GetFieldValues(field=238, scope=0) is not supported by the GPU or driver")
	require.ErrorContains(t, err, "convert FEC history field 242 for scope 0")
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs)-2)
}

func TestNVLinkFECCollectorAllFieldsFail(t *testing.T) {
	fieldValues := fecHistoryFieldValues()
	mock := nvmltestutil.SetupMockNVML(t,
		testutil.WithDeviceCount(1),
		testutil.WithNVLinkLinkCount(1),
		testutil.WithFieldValuesFullOverride(fieldValues),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mock, 0)

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

	for fieldID := range fieldValues {
		fieldValues[fieldID] = testutil.FieldError(nvml.ERROR_NOT_SUPPORTED)
	}
	mock.Device(0).SetFieldValues(fieldValues)

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.Nil(t, collectedMetrics)
	require.ErrorContains(t, err, "GetFieldValues(field=235, scope=0) is not supported by the GPU or driver")
}

func TestNVLinkFECMetricSpecEntries(t *testing.T) {
	spec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)

	testCases := []struct {
		metricName string
		metricType string
	}{
		{metricName: nvlinkFECHistoryMetricName, metricType: "histogram"},
		{metricName: nvlinkFECNoErrorsMetricName, metricType: "gauge"},
		{metricName: nvlinkFECLightErrorsMetricName, metricType: "gauge"},
		{metricName: nvlinkFECHeavyErrorsMetricName, metricType: "gauge"},
	}

	for _, testCase := range testCases {
		metricSpec, ok := spec.Metrics[testCase.metricName]
		require.True(t, ok, "metric %s missing from spec", testCase.metricName)
		require.Equal(t, testCase.metricType, metricSpec.Metadata.MetricType)
		require.Contains(t, metricSpec.Tagsets, "nvlink")
		require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical))
		require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG))
		require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU))
	}
}
