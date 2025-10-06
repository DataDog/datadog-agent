// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"slices"
	"testing"
	"time"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	nvmlmock "github.com/NVIDIA/go-nvml/pkg/nvml/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	ddmetrics "github.com/DataDog/datadog-agent/pkg/metrics"
)

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

	// create mock library returning just the 2 test devices
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(testutil.WithMIGDisabled())
	device1 := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions(), func(d *nvmlmock.Device) {
		d.GetUUIDFunc = func() (string, nvml.Return) { return device1UUID, nvml.SUCCESS }
	})
	device2 := testutil.GetDeviceMock(0, testutil.WithMockAllDeviceFunctions(), func(d *nvmlmock.Device) {
		d.GetUUIDFunc = func() (string, nvml.Return) { return device2UUID, nvml.SUCCESS }
	})
	ddnvml.WithMockNVML(t, nvmlMock)
	nvmlMock.DeviceGetHandleByIndexFunc = func(index int) (nvml.Device, nvml.Return) {
		switch index {
		case 0:
			return device1, nvml.SUCCESS
		case 1:
			return device2, nvml.SUCCESS
		default:
			return nil, nvml.ERROR_INVALID_ARGUMENT
		}
	}

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

	gpuToContainersMap := map[string]*workloadmeta.Container{
		device1UUID: {
			EntityID: workloadmeta.EntityID{
				ID: containerID,
			},
		},
	}

	// Process the metrics
	metricTime := time.Now()
	metricTimestamp := float64(metricTime.UnixNano()) / float64(time.Second)
	require.NoError(t, check.deviceCache.Refresh())
	require.NoError(t, check.emitMetrics(mockSender, gpuToContainersMap, metricTime))

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
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric1", float64(metricValueBase+1), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)

		// metric2: priority 1 wins (from fields collector)
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric2", float64(metricValueBase+2), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)

		// metric3: only from fields collector (priority 1)
		mockSender.AssertCalled(t, "GaugeWithTimestamp", "gpu.metric3", float64(metricValueBase+3), "", mock.MatchedBy(matchTagsFunc), metricTimestamp)
	}
}

func TestRunDoesNotError(t *testing.T) {
	// Tests for the specific output are above, this only ensures that the run function does not error
	// even if things are not correctly setup

	senderManager := mocksender.CreateDefaultDemultiplexer()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMockAllFunctions()))
	wmetaMock := testutil.GetWorkloadMetaMock(t)

	// Create check instance using mocks
	checkGeneric := newCheck(
		fakeTagger,
		testutil.GetTelemetryMock(t),
		wmetaMock,
	)

	// Add a container to the workload meta mock with GPU devices
	wmetaMock.Set(&workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			ID:   "container1",
			Kind: workloadmeta.KindContainer,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "container1",
		},
		ResolvedAllocatedResources: []workloadmeta.ContainerAllocatedResource{
			{
				Name: "nvidia.com/gpu",
				ID:   testutil.DefaultGpuUUID,
			},
		},
	})

	// Enable GPU check in configuration right before Configure
	pkgconfigsetup.Datadog().SetWithoutSource("gpu.enabled", true)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetWithoutSource("gpu.enabled", false)
	})

	err := checkGeneric.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test")
	require.NoError(t, err)

	require.NoError(t, checkGeneric.Run())

	// we need to cancel the check to make sure all resources and async workers are released
	// before deinitializing the mock library at test cleanup
	t.Cleanup(func() { checkGeneric.Cancel() })
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
