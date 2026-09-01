// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nvmltestutil "github.com/DataDog/datadog-agent/pkg/gpu/safenvml/testutil"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func allocTestGPMSamples(t *testing.T, mock *testutil.MockNVML) [sampleBufferSize]nvml.GpmSample {
	t.Helper()
	var samples [sampleBufferSize]nvml.GpmSample
	for i := range samples {
		sample, ret := mock.GpmSampleAlloc()
		require.Equal(t, nvml.SUCCESS, ret)
		samples[i] = sample
	}
	return samples
}

func TestGPMCollectorSupportDetection(t *testing.T) {
	mockLib := nvmltestutil.SetupMockNVML(t, testutil.WithGpmSupport(false))
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

	collector, err := newGPMCollector(mockDevice, nil)
	assert.Nil(t, collector)
	assert.ErrorIs(t, err, errUnsupportedDevice)
	assert.Equal(t, 2, mockLib.GpmSampleFreeCount(), "all allocated samples should be freed")
}

func TestGPMCollectorSampleAllocFailure(t *testing.T) {
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSampleAllocFailure(2),
		testutil.WithGpmSupport(true),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

	collector, err := newGPMCollector(mockDevice, nil)
	assert.Nil(t, collector)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to allocate GPM sample")
	assert.Equal(t, 1, mockLib.GpmSampleFreeCount(), "allocated sample should be freed on error")
}

func TestGPMCollectorAllMetricsUnsupported(t *testing.T) {
	// Setup: all metrics will be marked as unsupported by GpmMetricsGet
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1"},
		2: {name: "metric2"},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
			1: {Return: nvml.ERROR_NOT_SUPPORTED},
			2: {Return: nvml.ERROR_NOT_SUPPORTED},
		}),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)
	collector, err := newGPMCollector(mockDevice, nil)
	assert.Nil(t, collector)
	assert.ErrorIs(t, err, errUnsupportedDevice)
}

func TestGPMCollectorSomeMetricsUnsupported(t *testing.T) {
	// Setup: only one metric is supported
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1"},
		2: {name: "metric2"},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
			1: {Return: nvml.SUCCESS},
			2: {Return: nvml.ERROR_NOT_SUPPORTED},
		}),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

	collector, err := newGPMCollector(mockDevice, nil)
	assert.NoError(t, err)
	assert.NotNil(t, collector)
	gpmCol := collector.(*gpmCollector)
	assert.Contains(t, gpmCol.metricsToCollect, nvml.GpmMetricId(1), "supported metric should remain")
	assert.NotContains(t, gpmCol.metricsToCollect, 2, "unsupported metric should be removed")
}

func TestGPMCollectorCollectSample(t *testing.T) {
	calls := 0
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmSampleGetCallback(func(_ *testutil.MockGpmSample) nvml.Return {
			calls++
			return nvml.SUCCESS
		}),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)
	collector := &gpmCollector{
		device:              mockDevice,
		samples:             allocTestGPMSamples(t, mockLib),
		nextSampleToCollect: 0,
	}

	err := collector.collectSample()
	assert.NoError(t, err)
	assert.Equal(t, 1, calls, "GpmSampleGet should be called once")
	assert.Equal(t, 1, collector.nextSampleToCollect, "nextSampleToCollect should advance")

	err = collector.collectSample()
	assert.NoError(t, err)
	assert.Equal(t, 2, calls, "GpmSampleGet should be called twice")
	assert.Equal(t, 0, collector.nextSampleToCollect, "nextSampleToCollect should loop back")
}

func TestGPMCollectorGetLastTwoSamples(t *testing.T) {
	mockLib := testutil.NewMockNVML()
	samples := allocTestGPMSamples(t, mockLib)
	collector := &gpmCollector{
		samples:             samples,
		nextSampleToCollect: 0, // about to overwrite samples[0] next
	}
	last, secondLast := collector.getLastTwoSamples()
	assert.Same(t, samples[1], last)
	assert.Same(t, samples[0], secondLast)

	collector.nextSampleToCollect = 1
	last, secondLast = collector.getLastTwoSamples()
	assert.Same(t, samples[0], last)
	assert.Same(t, samples[1], secondLast)
}

func TestGPMCollectorCollectReturnsMetrics(t *testing.T) {
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1", metricType: 1},
		2: {name: "metric2", metricType: 2},
		3: {name: "metric3", metricType: 1},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	getIndex := 0
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmMetricsGetCallback(func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			// Check that we got metrics passed in the correct order.
			// Sample 1 needs to be the older sample, and sample 2 the newer one.
			sample1 := metrics.Sample1.(*testutil.MockGpmSample)
			sample2 := metrics.Sample2.(*testutil.MockGpmSample)
			assert.Greater(t, sample2.GetIndex, sample1.GetIndex)

			for i := range metrics.Metrics[:metrics.NumMetrics] {
				if metrics.Metrics[i].MetricId == 2 {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
				} else {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
					metrics.Metrics[i].Value = 42.0 + float64(metrics.Metrics[i].MetricId)
				}
			}
			return nvml.SUCCESS
		}),
		testutil.WithGpmSupport(true),
		testutil.WithGpmSampleGetCallback(func(sample *testutil.MockGpmSample) nvml.Return {
			sample.GetIndex = getIndex
			getIndex++
			return nvml.SUCCESS
		}),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

	collector, err := newGPMCollector(mockDevice, nil)
	require.NoError(t, err)
	gpmCol := collector.(*gpmCollector)

	result, err := gpmCol.Collect()
	assert.NoError(t, err)
	assert.Len(t, result, 2)

	foundMetrics := make(map[string]bool)
	for _, metric := range requireMetrics(t, result) {
		foundMetrics[metric.Name] = true

		switch metric.Name {
		case "metric1":
			assert.Equal(t, 43.0, metric.Value)
		case "metric3":
			assert.Equal(t, 45.0, metric.Value)
		}

		assert.Equal(t, metrics.MetricType(1), metric.Type)
	}

	assert.True(t, foundMetrics["metric1"])
	assert.True(t, foundMetrics["metric3"])
	assert.Equal(t, 2, mockLib.GpmSampleAllocCount())
}
