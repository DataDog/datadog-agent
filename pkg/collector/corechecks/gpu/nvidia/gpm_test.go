// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
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

func TestGPMCollectorUnsupportedWhenDriverLacksGPM(t *testing.T) {
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithCustomLibHook(func(lib *nvmlmock.Interface) {
			lib.GpmSampleAllocFunc = func() (nvml.GpmSample, nvml.Return) {
				return nil, nvml.ERROR_NOT_SUPPORTED
			}
		}),
	)
	mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{})
	assert.Nil(t, collector)
	assert.ErrorIs(t, err, errUnsupportedDevice)
}

func TestGPMCollectorAllMetricsDeniedIsNotUnsupported(t *testing.T) {
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1"},
		2: {name: "metric2"},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
			1: {Return: nvml.ERROR_NO_PERMISSION},
			2: {Return: nvml.ERROR_NO_PERMISSION},
		}),
	)
	collector, err := newGPMCollector(nvmltestutil.PhysicalDevice(t, mockLib, 0), &CollectorDependencies{})
	require.Nil(t, collector)
	require.True(t, ddnvml.IsNoPermission(err), "unexpected error: %v", err)
	require.NotErrorIs(t, err, errUnsupportedDevice)
	require.Equal(t, mockLib.GpmSampleAllocCount(), mockLib.GpmSampleFreeCount())
}

func TestGPMCollectorMixedPermissionAndUnsupportedMetricsIsNotUnsupported(t *testing.T) {
	// GPM metrics are queried one by one, so the error of removeUnsupportedMetrics can be a join
	// of per-metric failures. A permission error must not be hidden by an unsupported error that
	// comes first in the joined error: permissions can be recovered, so the collector is retried.
	oldAllGpmMetrics := allGpmMetrics
	allGpmMetrics = map[nvml.GpmMetricId]gpmMetric{
		1: {name: "metric1"},
		2: {name: "metric2"},
	}
	t.Cleanup(func() { allGpmMetrics = oldAllGpmMetrics })

	// the first metric queried fails with unsupported, the second one with no permission
	getCalls := 0
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricsGetCallback(func(_ *nvml.GpmMetricsGetType) nvml.Return {
			getCalls++
			if getCalls == 1 {
				return nvml.ERROR_NOT_SUPPORTED
			}
			return nvml.ERROR_NO_PERMISSION
		}),
	)
	collector, err := newGPMCollector(nvmltestutil.PhysicalDevice(t, mockLib, 0), &CollectorDependencies{})
	require.Nil(t, collector)
	require.ErrorContains(t, err, "failed to remove unsupported metrics")
	require.True(t, ddnvml.IsNoPermission(err), "unexpected error: %v", err)
	require.NotErrorIs(t, err, errUnsupportedDevice)
	require.Equal(t, mockLib.GpmSampleAllocCount(), mockLib.GpmSampleFreeCount())
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

	// LegacySMActive disables the GR engine sm_active source, so only the GPM metrics are emitted here.
	collector, err := newGPMCollector(mockDevice, &CollectorDependencies{Config: gpuconfig.Config{LegacySMActive: true}})
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

func TestGPMCollectorSMActiveSources(t *testing.T) {
	const grEngineValue, smUtilValue = 61.0, 59.0

	for _, tc := range []struct {
		name             string
		config           gpuconfig.Config
		expectedPriority MetricPriority
		expectedValue    float64
	}{
		{name: "default", config: gpuconfig.Config{}, expectedPriority: Low, expectedValue: grEngineValue},
		{name: "preferred", config: gpuconfig.Config{PreferGrEngineSMActive: true}, expectedPriority: High, expectedValue: grEngineValue},
		{name: "legacy", config: gpuconfig.Config{LegacySMActive: true}, expectedPriority: High, expectedValue: smUtilValue},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mockLib := nvmltestutil.SetupMockNVML(t,
				testutil.WithGpmSupport(true),
				testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
					nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: grEngineValue, Return: nvml.SUCCESS},
					nvml.GPM_METRIC_SM_UTIL:       {Value: smUtilValue, Return: nvml.SUCCESS},
				}),
			)
			mockDevice := nvmltestutil.PhysicalDevice(t, mockLib, 0)

			collector, err := newGPMCollectorWithMetrics(mockDevice, map[nvml.GpmMetricId]gpmMetric{
				nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
				nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
			}, &CollectorDependencies{Config: tc.config})
			require.NoError(t, err)

			samples, err := collector.Collect()
			require.NoError(t, err)
			var smActive []*Metric
			for _, metric := range requireMetrics(t, samples) {
				if metric.Name == "sm_active" {
					smActive = append(smActive, metric)
				}
			}

			require.Len(t, smActive, 1)
			assert.Equal(t, tc.expectedPriority, smActive[0].Priority())
			assert.Equal(t, tc.expectedValue, smActive[0].Value)
		})
	}
}

// migTestDevices returns a MIG device and a GPU in MIG mode from the default mock MIG topology.
func migTestDevices(t *testing.T) (*ddnvml.MIGDevice, *ddnvml.PhysicalDevice) {
	t.Helper()
	devices, err := ddnvml.NewDeviceCache().All()
	require.NoError(t, err)
	var migDevice *ddnvml.MIGDevice
	var parent *ddnvml.PhysicalDevice
	for _, device := range devices {
		switch d := device.(type) {
		case *ddnvml.MIGDevice:
			if migDevice == nil {
				migDevice = d
			}
		case *ddnvml.PhysicalDevice:
			if parent == nil && len(d.MIGChildren) > 0 {
				parent = d
			}
		}
	}
	require.NotNil(t, migDevice)
	require.NotNil(t, parent)
	return migDevice, parent
}

func TestGPMCollectorDropsFalseMIGGraphicsUtil(t *testing.T) {
	for _, tc := range []struct {
		name      string
		accessErr error
		expectGR  bool
	}{
		{name: "MIG config capability", accessErr: nil, expectGR: true},
		{name: "no permission", accessErr: &ddnvml.NvmlAPIError{APIName: "GetGpuInstanceById", NvmlErrorCode: nvml.ERROR_NO_PERMISSION}, expectGR: false},
		{name: "inconclusive", accessErr: &ddnvml.NvmlAPIError{APIName: "GetGpuInstanceById", NvmlErrorCode: nvml.ERROR_FUNCTION_NOT_FOUND}, expectGR: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nvmltestutil.SetupMockNVML(t,
				testutil.WithDefaultMIGDevices(),
				testutil.WithGpmSupport(true),
				testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
					// A loaded MIG device (NVML reports 0 for GRAPHICS_UTIL without the MIG config capability; the
					// GPU instance access check removes it at build then)
					nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: 90, Return: nvml.SUCCESS},
					nvml.GPM_METRIC_SM_UTIL:       {Value: 80, Return: nvml.SUCCESS},
				}),
			)
			migDevice, parent := migTestDevices(t)

			original := migGpuInstanceAccessCheck
			t.Cleanup(func() { migGpuInstanceAccessCheck = original })
			var checked []int
			migGpuInstanceAccessCheck = func(p *ddnvml.PhysicalDevice, gpuInstanceID int) error {
				require.NotNil(t, p)
				checked = append(checked, gpuInstanceID)
				return tc.accessErr
			}

			for _, device := range []ddnvml.Device{migDevice, parent} {
				collector, err := newGPMCollectorWithMetrics(device, map[nvml.GpmMetricId]gpmMetric{
					nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
					nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
				}, &CollectorDependencies{})
				require.NoError(t, err)

				samples, err := collector.Collect()
				require.NoError(t, err)
				names := map[string]bool{}
				for _, metric := range requireMetrics(t, samples) {
					names[metric.Name] = true
				}
				assert.Equal(t, tc.expectGR, names["gr_engine_active"], device.GetDeviceInfo().UUID)
				assert.Equal(t, tc.expectGR, names["sm_active"], "reported from GRAPHICS_UTIL: %s", device.GetDeviceInfo().UUID)
				assert.True(t, names["sm_utilization"], device.GetDeviceInfo().UUID)
			}
			require.Equal(t, []int{migDevice.MIGInstanceID, parent.MIGChildren[0].MIGInstanceID}, checked)
		})
	}
}

func TestGPMCollectorSkipsMIGAccessCheckOnRegularGPUs(t *testing.T) {
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
			nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: 0, Return: nvml.SUCCESS},
		}),
	)
	original := migGpuInstanceAccessCheck
	t.Cleanup(func() { migGpuInstanceAccessCheck = original })
	migGpuInstanceAccessCheck = func(*ddnvml.PhysicalDevice, int) error {
		t.Fatal("the GPU instance access must only be checked for MIG")
		return nil
	}

	collector, err := newGPMCollectorWithMetrics(nvmltestutil.PhysicalDevice(t, mockLib, 0), map[nvml.GpmMetricId]gpmMetric{
		nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
	}, &CollectorDependencies{})
	require.NoError(t, err)
	require.Contains(t, collector.(*gpmCollector).metricsToCollect, nvml.GPM_METRIC_GRAPHICS_UTIL)
}

func TestGPMCollectorChecksMIGAccessWithoutMIGDevices(t *testing.T) {
	mockLib := nvmltestutil.SetupMockNVML(t,
		testutil.WithDefaultMIGDevices(),
		testutil.WithGpmSupport(true),
		testutil.WithMIGDeviceCountCallback(func(int) int { return 0 }),
		testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
			nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: 0, Return: nvml.SUCCESS},
			nvml.GPM_METRIC_SM_UTIL:       {Value: 40, Return: nvml.SUCCESS},
		}),
	)
	// A GPU in MIG mode without MIG devices, e.g. before the DRA driver creates the first one.
	parent := nvmltestutil.PhysicalDevice(t, mockLib, testutil.DefaultMIGParentDeviceIdx)
	require.True(t, parent.HasMIGFeatureEnabled)
	require.Empty(t, parent.MIGChildren)

	original := migGpuInstanceAccessCheck
	t.Cleanup(func() { migGpuInstanceAccessCheck = original })
	var checked []int
	migGpuInstanceAccessCheck = func(_ *ddnvml.PhysicalDevice, gpuInstanceID int) error {
		checked = append(checked, gpuInstanceID)
		// NVML checks the permission before looking the GPU instance up
		return &ddnvml.NvmlAPIError{APIName: "GetGpuInstanceById", NvmlErrorCode: nvml.ERROR_NO_PERMISSION}
	}

	collector, err := newGPMCollectorWithMetrics(parent, map[nvml.GpmMetricId]gpmMetric{
		nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
		nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
	}, &CollectorDependencies{})
	require.NoError(t, err)
	require.Equal(t, []int{0}, checked, "checked at build, with any GPU instance ID")
	require.NotContains(t, collector.(*gpmCollector).metricsToCollect, nvml.GPM_METRIC_GRAPHICS_UTIL, "removed by the check, not by the per-sample guard")

	samples, err := collector.Collect()
	require.NoError(t, err)
	names := map[string]bool{}
	for _, metric := range requireMetrics(t, samples) {
		names[metric.Name] = true
	}
	require.False(t, names["gr_engine_active"])
	require.False(t, names["sm_active"], "reported from GRAPHICS_UTIL")
	require.True(t, names["sm_utilization"])
	require.Len(t, checked, 1, "not checked again on collection")
}

func TestGPMCollectorDropsImpossibleMIGGraphicsUtil(t *testing.T) {
	for _, tc := range []struct {
		name              string
		mig               bool
		parent            bool
		graphicsUtil      float64
		smUtil            float64
		smUtilUnsupported bool
		legacySMActive    bool
		expectGR          bool
	}{
		{name: "MIG device, SMs in use while the graphics engine reads idle", mig: true, graphicsUtil: 0, smUtil: 80, expectGR: false},
		{name: "MIG device, idle", mig: true, graphicsUtil: 0, smUtil: 0, expectGR: true},
		// 1g.35gb on an H200: NVML and DCGM both report about half the SM utilization
		{name: "MIG device, graphics engine below SM utilization", mig: true, graphicsUtil: 50, smUtil: 89, expectGR: true},
		{name: "regular GPU", mig: false, graphicsUtil: 0, smUtil: 80, expectGR: true},
		{name: "GPU in MIG mode, SMs in use while the graphics engine reads idle", mig: true, parent: true, graphicsUtil: 0, smUtil: 80, expectGR: false},
		{name: "MIG device without SM utilization", mig: true, graphicsUtil: 0, smUtil: 80, smUtilUnsupported: true, expectGR: true},
		{name: "MIG device with legacy sm_active", mig: true, graphicsUtil: 0, smUtil: 80, legacySMActive: true, expectGR: false},
		{name: "MIG device, SM utilization below 1% while the graphics engine reads idle", mig: true, graphicsUtil: 0, smUtil: 0.5, expectGR: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var device ddnvml.Device
			smUtilReturn := nvml.SUCCESS
			if tc.smUtilUnsupported {
				smUtilReturn = nvml.ERROR_NOT_SUPPORTED
			}
			if tc.mig {
				nvmltestutil.SetupMockNVML(t,
					testutil.WithDefaultMIGDevices(),
					testutil.WithGpmSupport(true),
					testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
						nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: tc.graphicsUtil, Return: nvml.SUCCESS},
						nvml.GPM_METRIC_SM_UTIL:       {Value: tc.smUtil, Return: smUtilReturn},
					}),
				)
				migDevice, parent := migTestDevices(t)
				device = migDevice
				if tc.parent {
					device = parent
				}
				original := migGpuInstanceAccessCheck
				t.Cleanup(func() { migGpuInstanceAccessCheck = original })
				migGpuInstanceAccessCheck = func(*ddnvml.PhysicalDevice, int) error { return nil } // MIG config capability
			} else {
				mockLib := nvmltestutil.SetupMockNVML(t,
					testutil.WithGpmSupport(true),
					testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
						nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: tc.graphicsUtil, Return: nvml.SUCCESS},
						nvml.GPM_METRIC_SM_UTIL:       {Value: tc.smUtil, Return: nvml.SUCCESS},
					}),
				)
				device = nvmltestutil.PhysicalDevice(t, mockLib, 0)
			}

			collector, err := newGPMCollectorWithMetrics(device, map[nvml.GpmMetricId]gpmMetric{
				nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
				nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
			}, &CollectorDependencies{Config: gpuconfig.Config{LegacySMActive: tc.legacySMActive}})
			require.NoError(t, err)
			samples, err := collector.Collect()
			require.NoError(t, err)
			names := map[string]bool{}
			for _, metric := range requireMetrics(t, samples) {
				names[metric.Name] = true
			}
			require.Equal(t, tc.expectGR, names["gr_engine_active"])
			// once seen, GRAPHICS_UTIL is not reported anymore on the device until it reads busy again
			require.Equal(t, !tc.expectGR, collector.(*gpmCollector).graphicsUtilSuppressed)
			require.Equal(t, !tc.smUtilUnsupported, names["sm_utilization"])
			switch {
			case tc.legacySMActive:
				// reported from SM_UTIL, not affected
				require.True(t, names["sm_active"])
			default:
				require.Equal(t, tc.expectGR, names["sm_active"])
			}
		})
	}
}

func TestGPMCollectorResumesGraphicsUtilOnceValid(t *testing.T) {
	graphicsUtil, smUtil := 0.0, 80.0
	nvmltestutil.SetupMockNVML(t,
		testutil.WithDefaultMIGDevices(),
		testutil.WithGpmSupport(true),
		testutil.WithGpmMetricsGetCallback(func(metrics *nvml.GpmMetricsGetType) nvml.Return {
			for i := range metrics.Metrics[:metrics.NumMetrics] {
				metrics.Metrics[i].NvmlReturn = uint32(nvml.SUCCESS)
				switch nvml.GpmMetricId(metrics.Metrics[i].MetricId) {
				case nvml.GPM_METRIC_GRAPHICS_UTIL:
					metrics.Metrics[i].Value = graphicsUtil
				case nvml.GPM_METRIC_SM_UTIL:
					metrics.Metrics[i].Value = smUtil
				}
			}
			return nvml.SUCCESS
		}),
	)
	migDevice, _ := migTestDevices(t)
	original := migGpuInstanceAccessCheck
	t.Cleanup(func() { migGpuInstanceAccessCheck = original })
	migGpuInstanceAccessCheck = func(*ddnvml.PhysicalDevice, int) error { return nil } // MIG config capability

	collector, err := newGPMCollectorWithMetrics(migDevice, map[nvml.GpmMetricId]gpmMetric{
		nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
		nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
	}, &CollectorDependencies{})
	require.NoError(t, err)

	collectGR := func() (float64, bool) {
		samples, err := collector.Collect()
		require.NoError(t, err)
		for _, metric := range requireMetrics(t, samples) {
			if metric.Name == "gr_engine_active" {
				return metric.Value, true
			}
		}
		return 0, false
	}

	// An impossible value suppresses the metric, and an idle sample (0 and 0) does not prove it valid again
	_, reported := collectGR()
	require.False(t, reported)
	graphicsUtil, smUtil = 0, 0
	_, reported = collectGR()
	require.False(t, reported)

	// The graphics engine reads busy again: the value is valid and reported again
	graphicsUtil, smUtil = 90, 80
	value, reported := collectGR()
	require.True(t, reported)
	require.Equal(t, 90.0, value)
	graphicsUtil, smUtil = 0, 0
	value, reported = collectGR()
	require.True(t, reported)
	require.Equal(t, 0.0, value, "idle is reported once the value is valid")

	// Suppressed again if the impossible value comes back
	graphicsUtil, smUtil = 0, 80
	_, reported = collectGR()
	require.False(t, reported)
}

func TestGPMCollectorMIGSMActiveFollowsGraphicsUtil(t *testing.T) {
	for _, tc := range []struct {
		name      string
		accessErr error
		expected  bool
	}{
		{name: "with the MIG config capability", expected: true},
		{name: "without it, not reported rather than a false 0", accessErr: ddnvml.NewNvmlAPIErrorOrNil("GetGpuInstanceById", nvml.ERROR_NO_PERMISSION), expected: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nvmltestutil.SetupMockNVML(t,
				testutil.WithDefaultMIGDevices(),
				testutil.WithGpmSupport(true),
				testutil.WithGpmMetricValues(map[nvml.GpmMetricId]testutil.MockGpmMetricValue{
					nvml.GPM_METRIC_GRAPHICS_UTIL: {Value: 50, Return: nvml.SUCCESS},
					nvml.GPM_METRIC_SM_UTIL:       {Value: 89, Return: nvml.SUCCESS},
				}),
			)
			migDevice, _ := migTestDevices(t)
			original := migGpuInstanceAccessCheck
			t.Cleanup(func() { migGpuInstanceAccessCheck = original })
			migGpuInstanceAccessCheck = func(*ddnvml.PhysicalDevice, int) error { return tc.accessErr }

			collector, err := newGPMCollectorWithMetrics(migDevice, map[nvml.GpmMetricId]gpmMetric{
				nvml.GPM_METRIC_GRAPHICS_UTIL: {name: "gr_engine_active", metricType: metrics.GaugeType},
				nvml.GPM_METRIC_SM_UTIL:       {name: "sm_utilization", metricType: metrics.GaugeType},
			}, &CollectorDependencies{})
			require.NoError(t, err)
			samples, err := collector.Collect()
			require.NoError(t, err)
			var smActive []*Metric
			for _, metric := range requireMetrics(t, samples) {
				if metric.Name == "sm_active" {
					smActive = append(smActive, metric)
				}
			}
			if !tc.expected {
				require.Empty(t, smActive, "SM_UTIL, the share of SMs in use, is not reported as sm_active")
				return
			}
			require.Len(t, smActive, 1)
			require.Equal(t, 50.0, smActive[0].Value, "the time fraction, as on other GPUs")
			require.Equal(t, Low, smActive[0].Priority(), "below the eBPF source")
		})
	}
}
