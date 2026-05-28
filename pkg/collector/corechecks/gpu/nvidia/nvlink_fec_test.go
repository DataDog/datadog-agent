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
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func fecHistoryFieldValues() map[uint32]testutil.MockFieldValue {
	fieldValues := make(map[uint32]testutil.MockFieldValue, len(nvlinkFECHistoryFieldIDs))
	for i, fieldID := range nvlinkFECHistoryFieldIDs {
		fieldValues[fieldID] = testutil.ULongLongFieldValue(uint64(100 + i))
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
	require.Equal(t, mockDevice.GetDeviceInfo().UUID, collector.DeviceUUID())

	collectedMetrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs))

	for bucket := range nvlinkFECHistoryFieldIDs {
		metric := collectedMetrics[bucket]

		require.Equal(t, nvlinkFECHistoryMetricName, metric.Name)
		require.Equal(t, metrics.HistogramType, metric.Type)
		require.Equal(t, float64(100+bucket), metric.Value)
		require.Equal(t, Medium, metric.Priority)
		require.Contains(t, metric.Tags, "nvlink_port:1")
		require.NotNil(t, metric.HistogramBucket)
		require.Equal(t, [2]float64{float64(bucket), float64(bucket + 1)}, metric.HistogramBucket.Bounds)
		require.True(t, metric.HistogramBucket.Monotonic)
		require.False(t, metric.HistogramBucket.FlushFirstValue)
	}
}

func TestNVLinkFECCollectorPartialFieldFailure(t *testing.T) {
	fieldValues := fecHistoryFieldValues()

	mockDevice := setupMockDevice(t, testutil.WithNVLinkLinkCount(1), testutil.WithFieldValuesFullOverride(fieldValues))

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

	// Modify the field values to test partial failure after initial support test
	// one field not supported, another with invalid value type
	fieldValues[nvlinkFECHistoryFieldIDs[3]] = testutil.FieldError(nvml.ERROR_NOT_SUPPORTED)
	fieldValues[nvlinkFECHistoryFieldIDs[7]] = testutil.MockFieldValue{Value: 9999, ValueType: nvml.ValueType(9999), Return: nvml.SUCCESS}

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.ErrorContains(t, err, "GetFieldValues(field=238, scope=0) is not supported by the GPU or driver")
	require.ErrorContains(t, err, "convert FEC history field 242 for scope 0")
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs)-2)
}

func TestNVLinkFECCollectorAllFieldsFail(t *testing.T) {
	fieldValues := fecHistoryFieldValues()
	mockDevice := setupMockDevice(t, testutil.WithNVLinkLinkCount(1), testutil.WithFieldValuesFullOverride(fieldValues))

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

	for fieldID := range fieldValues {
		fieldValues[fieldID] = testutil.FieldError(nvml.ERROR_NOT_SUPPORTED)
	}

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.Nil(t, collectedMetrics)
	require.ErrorContains(t, err, "GetFieldValues(field=235, scope=0) is not supported by the GPU or driver")
}

func TestNVLinkFECMetricSpecEntries(t *testing.T) {
	spec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)

	metricSpec, ok := spec.Metrics[nvlinkFECHistoryMetricName]
	require.True(t, ok, "metric %s missing from spec", nvlinkFECHistoryMetricName)
	require.Equal(t, "histogram", metricSpec.Metadata.MetricType)
	require.Contains(t, metricSpec.Tagsets, "nvlink")
	require.True(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModePhysical))
	require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeMIG))
	require.False(t, metricSpec.SupportsDeviceMode(gpuspec.DeviceModeVGPU))
}
