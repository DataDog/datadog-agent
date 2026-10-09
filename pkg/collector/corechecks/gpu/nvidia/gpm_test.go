// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"math"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
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

	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
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

	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
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
	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
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

	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
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

	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
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

func TestGPMCollectorLegacySMActive(t *testing.T) {
	const smUtilValue = 42.0

	for _, tc := range []struct {
		name                string
		legacySMActive      bool
		expectedMetricNames []string
	}{
		{
			name:                "disabled",
			legacySMActive:      false,
			expectedMetricNames: []string{"sm_utilization"},
		},
		{
			name:                "enabled",
			legacySMActive:      true,
			expectedMetricNames: []string{"sm_utilization", "sm_active"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockLib := nvmltestutil.SetupMockNVML(t,
				testutil.WithGpmSupport(true),
				testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
					nvml.GPM_METRIC_SM_UTIL: {Value: smUtilValue, Return: nvml.SUCCESS},
				}),
			)
			mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

			collector, err := newGPMCollectorWithMetrics(
				mockDevice,
				map[nvml.GpmMetricId]gpmMetric{
					nvml.GPM_METRIC_SM_UTIL: {
						name:       "sm_utilization",
						metricType: metrics.GaugeType,
					},
				},
				&CollectorDependencies{Config: gpuconfig.Config{LegacySMActive: tc.legacySMActive}},
			)
			require.NoError(t, err)

			collectedMetrics, err := collector.Collect()
			require.NoError(t, err)
			require.Len(t, collectedMetrics, len(tc.expectedMetricNames))

			metricsByName := make(map[string]*Metric, len(collectedMetrics))
			for _, metric := range requireMetrics(t, collectedMetrics) {
				metricsByName[metric.Name] = metric
			}

			for _, name := range tc.expectedMetricNames {
				metric := metricsByName[name]
				require.NotNil(t, metric)
				assert.Equal(t, smUtilValue, metric.Value)
				assert.Equal(t, High, metric.Priority())
			}
		})
	}
}

// smCyclesTestValue is the response for a GPM metric in a given sample.
type smCyclesTestValue struct {
	value float64
	ret   nvml.Return
}

// smCyclesCounters simulates the raw SM cycle counters. NVML returns the counter value at Sample2, so the value for
// each sample index (0 and 1 are collected when creating the collector) is the cumulative counter at that sample.
type smCyclesCounters struct {
	elapsed []smCyclesTestValue
	active  []smCyclesTestValue
}

func counterValues(values ...float64) []smCyclesTestValue {
	result := make([]smCyclesTestValue, len(values))
	for i, value := range values {
		result[i] = smCyclesTestValue{value: value}
	}
	return result
}

const smCyclesTestSMUtil = 42.0

func newSMCyclesTestCollector(t *testing.T, counters smCyclesCounters, config gpuconfig.Config) *gpmCollector {
	t.Helper()
	sampleIndex := 0
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmSampleGetCallback(func(sample *testutil.MockGpmSample) nvml.Return {
			sample.GetIndex = sampleIndex
			sampleIndex++
			return nvml.SUCCESS
		}),
		testutil.WithGpmMetricsGetCallback(func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			sample2 := metrics.Sample2.(*testutil.MockGpmSample)
			response := smCyclesTestValue{ret: nvml.ERROR_NOT_SUPPORTED}
			switch nvml.GpmMetricId(metrics.Metrics[0].MetricId) {
			case nvml.GPM_METRIC_SM_UTIL:
				response = smCyclesTestValue{value: smCyclesTestSMUtil}
			case gpmMetricSMCyclesElapsed:
				response = counters.elapsed[sample2.GetIndex]
			case gpmMetricSMCyclesActive:
				response = counters.active[sample2.GetIndex]
			}
			metrics.Metrics[0].NvmlReturn = uint32(response.ret)
			metrics.Metrics[0].Value = response.value
			return nvml.SUCCESS
		}),
	)
	collector, err := newGPMCollector(nvmltestutil.PhysicalDevice(t, mockLib, 0), &CollectorDependencies{Config: config})
	require.NoError(t, err)
	return collector.(*gpmCollector)
}

// collectMetricsByName runs a collection and returns the emitted metrics by name, failing if any name is repeated.
func collectMetricsByName(t *testing.T, collector Collector) (map[string]*Metric, error) {
	t.Helper()
	samples, err := collector.Collect()
	metricsByName := make(map[string]*Metric, len(samples))
	for _, metric := range requireMetrics(t, samples) {
		require.NotContains(t, metricsByName, metric.Name, "metric %s emitted more than once", metric.Name)
		metricsByName[metric.Name] = metric
	}
	return metricsByName, err
}

func TestGPMCollectorSMCyclesSMActive(t *testing.T) {
	// Realistic magnitudes from an H100: ~4.4e15 elapsed SM cycles, and 132 SMs at 1980 MHz adding ~2.6e11
	// elapsed SM cycles per second.
	const base = 4.4e15
	const interval = 2.6136e11
	counters := smCyclesCounters{
		elapsed: counterValues(base, base+interval, base+2*interval, base+3*interval, base+4*interval),
		active:  counterValues(1e13, 1e13, 1e13+0.6*interval, 1e13+1.6*interval, 1e13+1.6*interval),
	}

	collector := newSMCyclesTestCollector(t, counters, gpuconfig.Config{})
	for i, expected := range []float64{60, 100, 0} {
		emitted, err := collectMetricsByName(t, collector)
		require.NoError(t, err)
		require.Contains(t, emitted, "sm_active", "collection %d", i)
		assert.InDelta(t, expected, emitted["sm_active"].Value, 1e-6, "collection %d", i)

		// SM_UTIL keeps being reported as sm_utilization, and the raw counters are not emitted.
		require.Contains(t, emitted, "sm_utilization")
		assert.Equal(t, smCyclesTestSMUtil, emitted["sm_utilization"].Value)
		assert.Len(t, emitted, 2)
	}
}

func TestGPMCollectorSMCyclesSMActivePriority(t *testing.T) {
	counters := smCyclesCounters{
		elapsed: counterValues(0, 1000, 2000),
		active:  counterValues(0, 0, 600),
	}

	for _, tc := range []struct {
		name             string
		config           gpuconfig.Config
		expectedPriority MetricPriority
		expectedValue    float64
	}{
		{name: "default", config: gpuconfig.Config{}, expectedPriority: Low, expectedValue: 60},
		{name: "preferred", config: gpuconfig.Config{PreferSMCyclesSMActive: true}, expectedPriority: High, expectedValue: 60},
		{
			// The legacy sm_active (SM_UTIL) takes precedence, so sm_active is not derived from the counters.
			name:             "preferred with legacy",
			config:           gpuconfig.Config{PreferSMCyclesSMActive: true, LegacySMActive: true},
			expectedPriority: High,
			expectedValue:    smCyclesTestSMUtil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emitted, err := collectMetricsByName(t, newSMCyclesTestCollector(t, counters, tc.config))
			require.NoError(t, err)
			require.Contains(t, emitted, "sm_active")
			assert.Equal(t, tc.expectedPriority, emitted["sm_active"].Priority())
			assert.InDelta(t, tc.expectedValue, emitted["sm_active"].Value, 1e-9)
		})
	}
}

func TestGPMCollectorSMCyclesSMActiveUnavailable(t *testing.T) {
	valid := counterValues(0, 1000, 2000)
	invalid := func(value smCyclesTestValue) []smCyclesTestValue {
		return []smCyclesTestValue{value, value, value}
	}

	for _, tc := range []struct {
		name     string
		counters smCyclesCounters
	}{
		{name: "elapsed unsupported", counters: smCyclesCounters{elapsed: invalid(smCyclesTestValue{ret: nvml.ERROR_NOT_SUPPORTED}), active: valid}},
		{name: "active unsupported", counters: smCyclesCounters{elapsed: valid, active: invalid(smCyclesTestValue{ret: nvml.ERROR_NOT_SUPPORTED})}},
		{name: "active NaN", counters: smCyclesCounters{elapsed: valid, active: invalid(smCyclesTestValue{value: math.NaN()})}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			emitted, err := collectMetricsByName(t, newSMCyclesTestCollector(t, tc.counters, gpuconfig.Config{PreferSMCyclesSMActive: true}))
			require.NoError(t, err)
			assert.NotContains(t, emitted, "sm_active")
			require.Contains(t, emitted, "sm_utilization", "other GPM metrics should still be collected")
		})
	}
}

func TestGPMCollectorSMCyclesSMActiveCollectionError(t *testing.T) {
	counters := smCyclesCounters{
		elapsed: counterValues(0, 1000, 2000, 3000, 4000),
		active:  []smCyclesTestValue{{value: 0}, {value: 0}, {ret: nvml.ERROR_UNKNOWN}, {value: 1000}, {value: 1600}},
	}
	collector := newSMCyclesTestCollector(t, counters, gpuconfig.Config{})

	// The counters cannot be read for this sample, but the other metrics are still collected.
	emitted, err := collectMetricsByName(t, collector)
	require.Error(t, err)
	assert.NotContains(t, emitted, "sm_active")
	assert.Contains(t, emitted, "sm_utilization")

	// The counters are cumulative, so the next value covers both intervals.
	emitted, err = collectMetricsByName(t, collector)
	require.NoError(t, err)
	require.Contains(t, emitted, "sm_active")
	assert.InDelta(t, 50, emitted["sm_active"].Value, 1e-9)
}

func TestGPMCollectorSMCyclesSMActiveInvalidDeltas(t *testing.T) {
	for _, tc := range []struct {
		name     string
		counters smCyclesCounters
	}{
		{
			name:     "no elapsed cycles",
			counters: smCyclesCounters{elapsed: counterValues(1000, 1000, 1000, 2000), active: counterValues(0, 0, 0, 600)},
		},
		{
			name:     "counters reset",
			counters: smCyclesCounters{elapsed: counterValues(5000, 6000, 100, 1100), active: counterValues(3000, 3500, 50, 650)},
		},
		{
			name:     "active greater than elapsed",
			counters: smCyclesCounters{elapsed: counterValues(0, 1000, 2000, 3000), active: counterValues(0, 0, 1500, 2100)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			collector := newSMCyclesTestCollector(t, tc.counters, gpuconfig.Config{})

			emitted, err := collectMetricsByName(t, collector)
			require.NoError(t, err)
			assert.NotContains(t, emitted, "sm_active", "no sm_active should be emitted for an invalid interval")

			// The next interval is computed against the last reading.
			emitted, err = collectMetricsByName(t, collector)
			require.NoError(t, err)
			require.Contains(t, emitted, "sm_active")
			assert.InDelta(t, 60, emitted["sm_active"].Value, 1e-9)
		})
	}
}

func TestGPMCollectorSMCyclesSMActiveFloatRounding(t *testing.T) {
	// Above 2^53 the float64 counters are rounded, so on a fully active interval the active delta can be slightly
	// greater than the elapsed one. Around 1e17 the unit in the last place is 16, so the tolerance is 32 cycles.
	const elapsed0, active0, delta = 1e17, 5e16, 2.6e11
	for _, tc := range []struct {
		name     string
		excess   float64
		expected bool
	}{
		{name: "within rounding error", excess: 32, expected: true},
		{name: "beyond rounding error", excess: 48, expected: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			counters := smCyclesCounters{
				elapsed: counterValues(elapsed0, elapsed0, elapsed0+delta),
				active:  counterValues(active0, active0, active0+delta+tc.excess),
			}
			emitted, err := collectMetricsByName(t, newSMCyclesTestCollector(t, counters, gpuconfig.Config{}))
			require.NoError(t, err)
			if !tc.expected {
				assert.NotContains(t, emitted, "sm_active")
				return
			}
			require.Contains(t, emitted, "sm_active")
			assert.Equal(t, 100.0, emitted["sm_active"].Value)
		})
	}
}

func TestGPMCollectorSMCyclesSMActiveNotDerivedOnMIG(t *testing.T) {
	// The SM cycle counters haven't been validated for MIG instances, so they are not queried for MIG devices.
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricsGetCallback(func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			metricID := nvml.GpmMetricId(metrics.Metrics[0].MetricId)
			require.NotContains(t, []nvml.GpmMetricId{gpmMetricSMCyclesElapsed, gpmMetricSMCyclesActive}, metricID)
			metrics.Metrics[0].NvmlReturn = uint32(nvml.SUCCESS)
			return nvml.SUCCESS
		}),
	)
	migDevice := &ddnvml.MIGDevice{
		DeviceInfo:    ddnvml.DeviceInfo{UUID: "MIG-test"},
		Parent:        nvmltestutil.PhysicalDevice(t, mockLib, 0),
		MIGInstanceID: 1,
	}

	collector, err := newGPMCollector(migDevice, &CollectorDependencies{Config: gpuconfig.Config{PreferSMCyclesSMActive: true}})
	require.NoError(t, err)
	emitted, err := collectMetricsByName(t, collector)
	require.NoError(t, err)
	assert.NotContains(t, emitted, "sm_active")
}
