// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"encoding/binary"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestNVLinkFECCollectorScopesAndBuckets(t *testing.T) {
	type fieldRequest struct {
		fieldID uint32
		scopeID uint32
	}

	var requests []fieldRequest
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			if len(values) == 1 && values[0].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
				values[0].NvmlReturn = uint32(nvml.SUCCESS)
				values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
				binary.LittleEndian.PutUint32(values[0].Value[:], 1)
				return nvml.SUCCESS
			}

			require.Len(t, values, len(nvlinkFECHistoryFieldIDs))
			for i := range values {
				requests = append(requests, fieldRequest{fieldID: values[i].FieldId, scopeID: values[i].ScopeId})
				require.Equal(t, nvlinkFECHistoryFieldIDs[i], values[i].FieldId)
				require.Equal(t, uint32(0), values[i].ScopeId)
				values[i].NvmlReturn = uint32(nvml.SUCCESS)
				values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG_LONG)
				binary.LittleEndian.PutUint64(values[i].Value[:], uint64(100+i))
			}
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)
	require.Equal(t, nvlinkFEC, collector.Name())
	require.Equal(t, mockDevice.GetDeviceInfo().UUID, collector.DeviceUUID())

	requests = nil
	collectedMetrics, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs))
	require.Len(t, requests, len(nvlinkFECHistoryFieldIDs))

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
	fieldValueCalls := 0
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			if len(values) == 1 && values[0].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
				values[0].NvmlReturn = uint32(nvml.SUCCESS)
				values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
				binary.LittleEndian.PutUint32(values[0].Value[:], 1)
				return nvml.SUCCESS
			}

			fieldValueCalls++
			require.Len(t, values, len(nvlinkFECHistoryFieldIDs))
			for i := range values {
				values[i].NvmlReturn = uint32(nvml.SUCCESS)
				values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG_LONG)
				binary.LittleEndian.PutUint64(values[i].Value[:], uint64(i+1))
			}
			if fieldValueCalls > 1 {
				values[3].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
				values[7].ValueType = uint32(9999)
			}
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

	collectedMetrics, err := collector.Collect()
	require.Error(t, err)
	require.ErrorContains(t, err, "GetFieldValues(field=238, scope=0) is not supported by the GPU or driver")
	require.ErrorContains(t, err, "convert FEC history field 242 for scope 0")
	require.Len(t, collectedMetrics, len(nvlinkFECHistoryFieldIDs)-2)
}

func TestNVLinkFECCollectorAllFieldsFail(t *testing.T) {
	fieldValueCalls := 0
	mockDevice := setupMockDeviceWithLibOpts(t, func(device *mock.Device) *mock.Device {
		testutil.WithMockAllDeviceFunctions()(device)
		device.GetFieldValuesFunc = func(values []nvml.FieldValue) nvml.Return {
			if len(values) == 1 && values[0].FieldId == nvml.FI_DEV_NVLINK_LINK_COUNT {
				values[0].NvmlReturn = uint32(nvml.SUCCESS)
				values[0].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_INT)
				binary.LittleEndian.PutUint32(values[0].Value[:], 1)
				return nvml.SUCCESS
			}

			fieldValueCalls++
			require.Len(t, values, len(nvlinkFECHistoryFieldIDs))
			for i := range values {
				values[i].NvmlReturn = uint32(nvml.SUCCESS)
				values[i].ValueType = uint32(nvml.VALUE_TYPE_UNSIGNED_LONG_LONG)
				if fieldValueCalls > 1 {
					values[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
				}
			}
			return nvml.SUCCESS
		}
		return device
	})

	collector, err := newNVLinkFECCollector(mockDevice, nil)
	require.NoError(t, err)

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
