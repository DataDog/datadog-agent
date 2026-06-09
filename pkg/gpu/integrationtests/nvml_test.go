// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func TestGPMMetricsGetOverwritesMetricResultFields(t *testing.T) {
	testutil.RequireGPU(t)
	lib := initNVML(t)

	cache := safenvml.NewDeviceCache(safenvml.WithDeviceCacheLib(lib))
	devices, err := cache.AllPhysicalDevices()
	require.NoError(t, err)
	require.NotEmpty(t, devices)

	metricIDs := []nvml.GpmMetricId{
		nvml.GPM_METRIC_GRAPHICS_UTIL,
		nvml.GPM_METRIC_SM_UTIL,
		nvml.GPM_METRIC_SM_OCCUPANCY,
		nvml.GPM_METRIC_INTEGER_UTIL,
		nvml.GPM_METRIC_FP16_UTIL,
		nvml.GPM_METRIC_FP32_UTIL,
		nvml.GPM_METRIC_FP64_UTIL,
		nvml.GPM_METRIC_ANY_TENSOR_UTIL,
	}

	testedDevices := 0
	for _, device := range devices {
		deviceInfo := device.GetDeviceInfo()
		sample1 := allocateGPMSample(t, lib)
		defer func() { require.NoError(t, lib.GpmSampleFree(sample1)) }()
		sample2 := allocateGPMSample(t, lib)
		defer func() { require.NoError(t, lib.GpmSampleFree(sample2)) }()

		if !collectGPMSample(t, device, sample1) {
			t.Logf("Skipping GPU %s: GPM sample collection is unsupported", deviceInfo.UUID)
			continue
		}
		time.Sleep(100 * time.Millisecond)
		require.True(t, collectGPMSample(t, device, sample2), "second GPM sample collection should match first sample support for GPU %s", deviceInfo.UUID)

		testedDevices++
		metricsGet := seededGPMMetricsGet(sample1, sample2, metricIDs)
		err := lib.GpmMetricsGet(metricsGet)
		require.NoError(t, err, "GpmMetricsGet should succeed for GPU %s", deviceInfo.UUID)

		require.Equal(t, uint32(nvml.GPM_METRICS_GET_VERSION), metricsGet.Version, "NVML wrapper should overwrite the metrics get version")
		require.Equal(t, uint32(len(metricIDs)), metricsGet.NumMetrics, "NVML should preserve the requested metric count")
		for i, metricID := range metricIDs {
			metric := metricsGet.Metrics[i]
			require.Equal(t, uint32(metricID), metric.MetricId, "NVML should preserve the requested metric ID")
			require.NotEqual(t, sentinelNvmlReturn, metric.NvmlReturn, "NVML should overwrite NvmlReturn for metric %d on GPU %s", metricID, deviceInfo.UUID)
			require.NotEqual(t, sentinelMetricValue, metric.Value, "NVML should overwrite Value for metric %d on GPU %s", metricID, deviceInfo.UUID)
		}
	}

	require.Greater(t, testedDevices, 0, "expected at least one physical GPU with GPM support")
}

const (
	sentinelMetricsGetVersion uint32  = 0xabcddcba
	sentinelMetricID          uint32  = 0xfeedbeef
	sentinelNvmlReturn        uint32  = 0xdeadbeef
	sentinelMetricValue       float64 = -424242.424242
)

func allocateGPMSample(t *testing.T, lib safenvml.SafeNVML) nvml.GpmSample {
	t.Helper()

	sample, err := lib.GpmSampleAlloc()
	require.NoError(t, err)
	require.NotNil(t, sample)
	return sample
}

func collectGPMSample(t *testing.T, device safenvml.Device, sample nvml.GpmSample) bool {
	t.Helper()

	err := device.GpmSampleGet(sample)
	if safenvml.IsAPIUnsupportedOnDevice(err, device) {
		return false
	}
	require.NoError(t, err)
	return true
}

func seededGPMMetricsGet(sample1, sample2 nvml.GpmSample, metricIDs []nvml.GpmMetricId) *nvml.GpmMetricsGetType {
	metricsGet := &nvml.GpmMetricsGetType{
		Version:    sentinelMetricsGetVersion,
		NumMetrics: uint32(len(metricIDs)),
		Sample1:    sample1,
		Sample2:    sample2,
	}
	for i := range metricsGet.Metrics {
		metricsGet.Metrics[i] = nvml.GpmMetric{
			MetricId:   sentinelMetricID,
			NvmlReturn: sentinelNvmlReturn,
			Value:      sentinelMetricValue,
		}
	}
	for i, metricID := range metricIDs {
		metricsGet.Metrics[i].MetricId = uint32(metricID)
	}

	return metricsGet
}
