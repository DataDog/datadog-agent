// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"slices"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestProcessSysprobeStats(t *testing.T) {
	// Create a mock sender
	mockSender := mocksender.NewMockSender("gpu")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		testutil.GetWorkloadMetaMock(t),
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	// Set NVML mock to return a single device with the default UUID
	nvmlMock := &nvmlmock.Interface{
		DeviceGetCountFunc: func() (int, nvml.Return) {
			return 1, nvml.SUCCESS
		},
		DeviceGetHandleByIndexFunc: func(index int) (nvml.Device, nvml.Return) {
			return testutil.GetDeviceMock(index), nvml.SUCCESS
		},
	}
	ddnvml.WithMockNVML(t, nvmlMock)

	// Set device cache mock
	deviceCache, err := ddnvml.NewDeviceCache()
	require.NoError(t, err)
	check.deviceCache = deviceCache

	// Set up GPU and container tags
	containerID := "container1"
	containerTags := []string{"container_id:" + containerID}
	gpuTags := []string{"gpu_uuid:" + testutil.DefaultGpuUUID, "gpu_vendor:nvidia", "gpu_arch:pascal"}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID), "foo", containerTags, nil, nil, nil)

	check.deviceTags = map[string][]string{
		testutil.DefaultGpuUUID: gpuTags,
	}

	// Create test data
	stats := model.GPUStats{
		Metrics: []model.StatsTuple{
			{
				Key: model.StatsKey{
					PID:         123,
					ContainerID: containerID,
					DeviceUUID:  testutil.DefaultGpuUUID,
				},
				UtilizationMetrics: model.UtilizationMetrics{
					UsedCores: 50,
					Memory: model.MemoryMetrics{
						CurrentBytes: 1024,
					},
				},
			},
		},
	}

	gpuToContainersMap := map[string][]*workloadmeta.Container{
		testutil.DefaultGpuUUID: {
			{
				EntityID: workloadmeta.EntityID{
					ID: containerID,
				},
			},
		},
	}

	// Process the stats
	err = check.processSysprobeStats(mockSender, stats, gpuToContainersMap)
	assert.NoError(t, err)

	// Verify we sent metrics with the correct tags and values
	expectedTags := slices.Concat([]string{"pid:123"}, containerTags, gpuTags)
	slices.Sort(expectedTags) // Sort to avoid order issues
	matchTagsFunc := func(tags []string) bool {
		// Check that both slices have the same elements, regardless of order
		slices.Sort(tags)
		return slices.Equal(tags, expectedTags)
	}

	mockSender.AssertCalled(t, "Gauge", "gpu.core.usage", float64(50), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.usage", float64(1024), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.core.limit", float64(testutil.DefaultGpuCores), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.limit", float64(testutil.DefaultTotalMemory), "", mock.MatchedBy(matchTagsFunc))
	mockSender.ResetCalls()

	// Now do another run with no data
	stats = model.GPUStats{}
	err = check.processSysprobeStats(mockSender, stats, gpuToContainersMap)
	assert.NoError(t, err)

	// Verify we sent metrics with the correct tags and zero values
	mockSender.AssertCalled(t, "Gauge", "gpu.core.usage", float64(0), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.usage", float64(0), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.core.limit", float64(testutil.DefaultGpuCores), "", mock.MatchedBy(matchTagsFunc))
	mockSender.AssertCalled(t, "Gauge", "gpu.memory.limit", float64(testutil.DefaultTotalMemory), "", mock.MatchedBy(matchTagsFunc))
}

func TestEmitNvmlMetrics(t *testing.T) {
	// Create a mock sender
	mockSender := mocksender.NewMockSender("gpu")
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		testutil.GetWorkloadMetaMock(t),
	)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	device1UUID := "gpu-uuid-1"
	device2UUID := "gpu-uuid-2"

	// Create mock collectors
	for i, deviceUUID := range []string{device1UUID, device2UUID} {
		metricValueBase := 10 * i

		check.collectors = append(check.collectors, &mockCollector{
			name:       "device",
			deviceUUID: deviceUUID,
			metrics: []nvidia.Metric{
				{Name: "metric1", Value: float64(metricValueBase + 1), Type: ddmetrics.GaugeType, Priority: 0},
				{Name: "metric2", Value: float64(metricValueBase + 2), Type: ddmetrics.GaugeType, Priority: 0},
			},
		})

		check.collectors = append(check.collectors, &mockCollector{
			name:       "fields",
			deviceUUID: deviceUUID,
			metrics: []nvidia.Metric{
				{Name: "metric2", Value: float64(metricValueBase + 2), Type: ddmetrics.GaugeType, Priority: 1},
				{Name: "metric3", Value: float64(metricValueBase + 3), Type: ddmetrics.GaugeType, Priority: 1},
			},
		})
	}

	// Set device tags
	check.deviceTags = map[string][]string{
		device1UUID: {"gpu_uuid:" + device1UUID, "gpu_vendor:nvidia"},
		device2UUID: {"gpu_uuid:" + device2UUID, "gpu_vendor:nvidia"},
	}

	// Set up GPU and container tags
	containerID := "container1"
	containerTags := []string{"container_id:" + containerID}
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.ContainerID, containerID), "foo", containerTags, nil, nil, nil)

	gpuToContainersMap := map[string][]*workloadmeta.Container{
		device1UUID: {
			{
				EntityID: workloadmeta.EntityID{
					ID: containerID,
				},
			},
		},
	}

	// Process the metrics
	err := check.emitNvmlMetrics(mockSender, gpuToContainersMap)
	assert.NoError(t, err)

	// Verify metrics for each device
	for i, deviceUUID := range []string{device1UUID, device2UUID} {
		metricValueBase := 10 * i

		// Build expected tags
		var expectedTags []string
		if deviceUUID == device1UUID {
			// Device 1 has container tags
			expectedTags = append([]string{"gpu_uuid:" + deviceUUID, "gpu_vendor:nvidia"}, containerTags...)
		} else {
			// Device 2 has no container tags
			expectedTags = []string{"gpu_uuid:" + deviceUUID, "gpu_vendor:nvidia"}
		}
		slices.Sort(expectedTags)

		matchTagsFunc := func(tags []string) bool {
			slices.Sort(tags)
			return slices.Equal(tags, expectedTags)
		}

		// Verify metrics for this device
		// metric1: only from device collector (priority 0)
		mockSender.AssertCalled(t, "Gauge", "gpu.metric1", float64(metricValueBase+1), "", mock.MatchedBy(matchTagsFunc))

		// metric2: priority 1 wins (from fields collector)
		mockSender.AssertCalled(t, "Gauge", "gpu.metric2", float64(metricValueBase+2), "", mock.MatchedBy(matchTagsFunc))

		// metric3: only from fields collector (priority 1)
		mockSender.AssertCalled(t, "Gauge", "gpu.metric3", float64(metricValueBase+3), "", mock.MatchedBy(matchTagsFunc))
	}
}

// mockCollector implements the nvidia.Collector interface for testing
type mockCollector struct {
	name       nvidia.CollectorName
	deviceUUID string
	metrics    []nvidia.Metric
}

func (m *mockCollector) Collect() ([]nvidia.Metric, error) {
	return m.metrics, nil
}

func (m *mockCollector) Name() nvidia.CollectorName {
	return m.name
}

func (m *mockCollector) DeviceUUID() string {
	return m.deviceUUID
}
