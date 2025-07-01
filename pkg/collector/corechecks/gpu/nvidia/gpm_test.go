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

	"github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestGPMCollectorSupportDetection(t *testing.T) {
	// Device does not support GPM metrics
	mockLib := &mockGpmNvml{}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 0},
	}

	mocklib := testutil.GetBasicNvmlMock()
	safenvml.WithMockNVML(t, mocklib)

	collector, err := newGPMCollector(mockDevice)
	assert.Nil(t, collector)
	assert.ErrorIs(t, err, errUnsupportedDevice)
	assert.Equal(t, 0, mockLib.freedSamples, "all allocated samples should be freed (none allocated in this case)")
}

func TestGPMCollectorSampleAllocFailure(t *testing.T) {
	mockLib := &mockGpmNvml{
		enableAllocFailure: true,
		failOnAlloc:        1, // fail on the second allocation
	}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
	}

	safenvml.WithMockNVML(t, mockLib)

	collector, err := newGPMCollector(mockDevice)
	assert.Nil(t, collector)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to allocate GPM sample")
	assert.Equal(t, 1, mockLib.freedSamples, "allocated sample should be freed on error")
}

func TestGPMCollectorAllMetricsUnsupported(t *testing.T) {
	// Setup: all metrics will be marked as unsupported by GpmMetricsGet
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1"},
		2: {name: "metric2"},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			// Mark all as unsupported
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				metrics.Metrics[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
			}
			return nvml.SUCCESS
		},
	}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
	}

	safenvml.WithMockNVML(t, mockLib)
	collector, err := newGPMCollector(mockDevice)
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

	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			for i := 0; i < int(metrics.NumMetrics); i++ {
				if metrics.Metrics[i].MetricId == 2 {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
				} else {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
				}
			}
			return nvml.SUCCESS
		},
	}
	mockDevice := &mockGpmDevice{
		gpmSupport: nvml.GpmSupport{IsSupportedDevice: 1},
	}

	safenvml.WithMockNVML(t, mockLib)

	collector, err := newGPMCollector(mockDevice)
	assert.NoError(t, err)
	assert.NotNil(t, collector)
	gpmCol := collector.(*gpmCollector)
	assert.Contains(t, gpmCol.metricsToCollect, nvml.GpmMetricId(1), "supported metric should remain")
	assert.NotContains(t, gpmCol.metricsToCollect, 2, "unsupported metric should be removed")
}

func TestGPMCollectorCollectSample(t *testing.T) {
	calls := 0
	collector := &gpmCollector{
		device: &mockGpmDevice{
			GpmSampleGetFunc: func(_ nvml.GpmSample) error {
				calls++
				return nil
			},
		},
		samples:             [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 1}, &gpmSample{id: 2}},
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
	collector := &gpmCollector{
		samples:             [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 1}, &gpmSample{id: 2}},
		nextSampleToCollect: 0, // about to overwrite samples[0] next
	}
	last, secondLast := collector.getLastTwoSamples()
	assert.Equal(t, &gpmSample{id: 2}, last)
	assert.Equal(t, &gpmSample{id: 1}, secondLast)

	collector.nextSampleToCollect = 1
	last, secondLast = collector.getLastTwoSamples()
	assert.Equal(t, &gpmSample{id: 1}, last)
	assert.Equal(t, &gpmSample{id: 2}, secondLast)
}

func TestGPMCollectorCollectReturnsMetrics(t *testing.T) {
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1", metricType: 1},
		2: {name: "metric2", metricType: 2},
		3: {name: "metric3", metricType: 1},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	mockLib := &mockGpmNvml{
		metricsGetFunc: func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				if metrics.Metrics[i].MetricId == 2 {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.ERROR_NOT_SUPPORTED)
				} else {
					metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
					metrics.Metrics[i].Value = 42.0 + float64(metrics.Metrics[i].MetricId)
				}
			}
			return nvml.SUCCESS
		},
	}
	mockDevice := &mockGpmDevice{
		gpmSupport:       nvml.GpmSupport{IsSupportedDevice: 1},
		GpmSampleGetFunc: func(_ nvml.GpmSample) error { return nil },
	}

	safenvml.WithMockNVML(t, mockLib)

	collector, err := newGPMCollector(mockDevice)
	require.NoError(t, err)
	gpmCol := collector.(*gpmCollector)

	// Pre-fill samples so calculateGpmMetrics works
	gpmCol.samples = [sampleBufferSize]nvml.GpmSample{&gpmSample{id: 1}, &gpmSample{id: 2}}
	gpmCol.nextSampleToCollect = 0

	result, err := gpmCol.Collect()
	assert.NoError(t, err)
	assert.Len(t, result, 2)

	foundMetrics := make(map[string]bool)
	for _, metric := range result {
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
}

// mockGpmNvml mocks the SafeNVML interface with specific methods to test the GPMCollector
type mockGpmNvml struct {
	nvml.Interface

	freedSamples       int
	allocCount         int
	enableAllocFailure bool
	failOnAlloc        int
	metricsGetFunc     func(metrics *nvml.GpmMetricsGetType) nvml.Return
}

func (m *mockGpmNvml) GpmSampleAlloc() (nvml.GpmSample, nvml.Return) {
	if m.enableAllocFailure && m.allocCount == m.failOnAlloc {
		return nil, nvml.ERROR_UNKNOWN
	}
	m.allocCount++
	return &gpmSample{id: m.allocCount}, nvml.SUCCESS
}

func (m *mockGpmNvml) GpmSampleFree(_ nvml.GpmSample) nvml.Return {
	m.freedSamples++
	return nvml.SUCCESS
}

func (m *mockGpmNvml) GpmMetricsGet(metrics *nvml.GpmMetricsGetType) nvml.Return {
	if m.metricsGetFunc != nil {
		return m.metricsGetFunc(metrics)
	}
	return nvml.SUCCESS
}

type gpmSample struct {
	nvml.GpmSample
	id int
}

type mockGpmDevice struct {
	safenvml.SafeDevice

	gpmSupport       nvml.GpmSupport
	GpmSampleGetFunc func(sample nvml.GpmSample) error
}

func (m *mockGpmDevice) GpmQueryDeviceSupport() (nvml.GpmSupport, error) {
	return m.gpmSupport, nil
}

func (m *mockGpmDevice) GpmSampleGet(sample nvml.GpmSample) error {
	if m.GpmSampleGetFunc != nil {
		return m.GpmSampleGetFunc(sample)
	}
	return nil
}
