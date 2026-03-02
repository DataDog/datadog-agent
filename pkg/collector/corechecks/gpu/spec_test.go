// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

func TestMetricsFollowSpec(t *testing.T) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	require.NoError(t, err)
	archFile, err := gpuspec.LoadArchitecturesSpec()
	require.NoError(t, err)

	// Build spec metric set for quick membership checks.
	specMetrics := make(map[string]struct{}, len(metricsSpec.Metrics))
	for name := range metricsSpec.Metrics {
		specMetrics[name] = struct{}{}
	}

	// XID metrics require real device events.
	notExpectedOnBasicRun := map[string]bool{
		"errors.xid.total": true,
	}

	deviceModes := []gpuspec.DeviceMode{
		gpuspec.DeviceModePhysical,
		gpuspec.DeviceModeMIG,
		gpuspec.DeviceModeVGPU,
	}

	for archName, archSpec := range archFile.Architectures {
		t.Run("arch="+archName, func(t *testing.T) {
			for _, mode := range deviceModes {
				if !gpuspec.IsModeSupportedByArchitecture(archSpec, mode) {
					continue
				}

				t.Run("mode="+string(mode), func(t *testing.T) {
					emittedMetrics, knownTagValues := collectMetricSamples(t, archName, mode, archSpec)

					t.Run("_emits_only_expected_metrics", func(t *testing.T) {
						for metricName := range emittedMetrics {
							assert.Contains(t, specMetrics, metricName, "metric emitted by check is missing from spec: %s", metricName)

							metricSpec := metricsSpec.Metrics[metricName]
							assert.False(t, notExpectedOnBasicRun[metricName], "metric should not be emitted in basic run: %s", metricName)
							assert.True(t, metricSpec.SupportsArchitecture(archName), "metric %s emitted on unsupported architecture %s", metricName, archName)
							assert.False(t, metricSpec.IsDeviceModeExplicitlyUnsupported(mode), "metric %s emitted on unsupported device mode %s", metricName, mode)
						}
					})

					for name, m := range metricsSpec.Metrics {
						if notExpectedOnBasicRun[name] || !m.SupportsArchitecture(archName) || !m.SupportsDeviceMode(mode) {
							continue
						}

						t.Run(name, func(t *testing.T) {
							metrics, found := emittedMetrics[name]
							require.True(t, found, "spec metric is not emitted by check run: %s", name)
							validateMetricTagsAgainstSpec(t, metricsSpec, name, m, metrics, knownTagValues)
						})
					}
				})
			}
		})
	}
}

// collectMetricSamples runs the GPU check with a capability-driven mock
// for the given architecture and device mode, then returns emitted metrics (without "gpu." prefix)
// and their tags.
func collectMetricSamples(t *testing.T, archName string, mode gpuspec.DeviceMode, archSpec gpuspec.ArchitectureSpec) (map[string][]metric, map[string]string) {
	t.Helper()

	collectionSetup := setupMockCheckForMetricCollection(t, archName, mode, archSpec)
	collectionSetup.runCollection()

	return getEmittedGPUMetricsWithTags(collectionSetup.mockSender), collectionSetup.knownTagValues
}

type metricCollectionSetup struct {
	mockSender     *mocksender.MockSender
	runCollection  func()
	knownTagValues map[string]string
}

func setupMockCheckForMetricCollection(t *testing.T, archName string, mode gpuspec.DeviceMode, archSpec gpuspec.ArchitectureSpec) metricCollectionSetup {
	t.Helper()
	opts := gpuspec.BuildMockOptionsForArchAndMode(t, archName, mode, archSpec)

	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager("gpu", senderManager)
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(opts...))

	wmeta := testutil.GetWorkloadMetaMock(t)
	for idx, uuid := range testutil.GPUUUIDs {
		gpu := newMockWorkloadMetaGPU(uuid, idx, workloadmeta.GPUDeviceTypePhysical, "")
		wmeta.Set(gpu)
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.GPU, uuid),
			"spec-test",
			gpuTagsFromWorkloadMetaGPU(gpu),
			nil,
			nil,
			nil,
		)
	}
	if mode == gpuspec.DeviceModeMIG {
		for parentIdx, uuids := range testutil.MIGChildrenUUIDs {
			parentUUID := testutil.GPUUUIDs[parentIdx]
			for migIdx, uuid := range uuids {
				gpu := newMockWorkloadMetaGPU(uuid, migIdx, workloadmeta.GPUDeviceTypeMIG, parentUUID)
				wmeta.Set(gpu)
				fakeTagger.SetTags(
					taggertypes.NewEntityID(taggertypes.GPU, uuid),
					"spec-test",
					gpuTagsFromWorkloadMetaGPU(gpu),
					nil,
					nil,
					nil,
				)
			}
		}
	}

	pidToContainerID := map[int]string{
		1:    "container-1",
		1234: "container-1234",
		5678: "container-5678",
	}
	for _, containerID := range pidToContainerID {
		wmeta.Set(&workloadmeta.Container{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   containerID,
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
		})
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.ContainerID, containerID),
			"spec-test",
			nil,
			nil,
			[]string{
				"kube_container_name:name-" + containerID,
			},
			nil,
		)
	}

	checkGeneric := newCheck(fakeTagger, testutil.GetTelemetryMock(t), wmeta)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	WithGPUConfigEnabled(t)
	pkgconfigsetup.Datadog().SetWithoutSource("gpu.disabled_collectors", []string{"device_events"})
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetWithoutSource("gpu.disabled_collectors", []string{})
	})
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(pidToContainerID).AnyTimes()
	check.containerProvider = mockContainerProvider
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	t.Cleanup(func() { checkGeneric.Cancel() })

	// process.core.usage/core.limit come from system-probe/eBPF collector. Provide deterministic
	// cache data for every device shape used by the mode.
	cacheDeviceUUIDs := []string{testutil.DefaultGpuUUID}
	if mode == gpuspec.DeviceModeMIG {
		parentIdx := testutil.DevicesWithMIGChildren[0]
		cacheDeviceUUIDs = append([]string{testutil.GPUUUIDs[parentIdx]}, testutil.MIGChildrenUUIDs[parentIdx]...)
	}
	processMetrics := make([]model.ProcessStatsTuple, 0, len(cacheDeviceUUIDs))
	deviceMetrics := make([]model.DeviceStatsTuple, 0, len(cacheDeviceUUIDs))
	for _, uuid := range cacheDeviceUUIDs {
		processMetrics = append(processMetrics, model.ProcessStatsTuple{
			Key: model.ProcessStatsKey{
				PID:        1234,
				DeviceUUID: uuid,
			},
			UtilizationMetrics: model.UtilizationMetrics{
				UsedCores: 42,
				Memory: model.MemoryMetrics{
					CurrentBytes: 100,
					MaxBytes:     200,
				},
				ActiveTimePct: 50,
			},
		})
		deviceMetrics = append(deviceMetrics, model.DeviceStatsTuple{
			DeviceUUID: uuid,
			Metrics: model.DeviceUtilizationMetrics{
				ActiveTimePct: 50,
			},
		})
	}
	spCache := &nvidia.SystemProbeCache{}
	spCache.SetStatsForTest(&model.GPUStats{
		ProcessMetrics: processMetrics,
		DeviceMetrics:  deviceMetrics,
	})
	check.spCache = spCache

	runCollection := func() {
		// Some metrics require a second run to be collected, so we run it twice and clear
		// the mock sender between runs.
		require.NoError(t, checkGeneric.Run())
		mockSender.ResetCalls()
		require.NoError(t, checkGeneric.Run())
	}

	// Known values are defined at the same place where the mock behavior/data is configured.
	knownTagValues := map[string]string{
		"gpu_device":         strings.ToLower(strings.ReplaceAll(testutil.DefaultGPUName, " ", "_")),
		"gpu_vendor":         "nvidia",
		"gpu_driver_version": testutil.DefaultNvidiaDriverVersion,
	}

	return metricCollectionSetup{
		mockSender:     mockSender,
		runCollection:  runCollection,
		knownTagValues: knownTagValues,
	}
}

type metric struct {
	name string
	tags []string
}

func getEmittedGPUMetricsWithTags(mockSender *mocksender.MockSender) map[string][]metric {
	metricsByName := make(map[string][]metric)

	for _, call := range mockSender.Mock.Calls {
		if call.Method != "GaugeWithTimestamp" && call.Method != "CountWithTimestamp" {
			continue
		}

		if len(call.Arguments) == 0 {
			continue
		}

		metricName, ok := call.Arguments.Get(0).(string)
		if !ok || !strings.HasPrefix(metricName, "gpu.") {
			continue
		}

		specMetricName := strings.TrimPrefix(metricName, "gpu.")
		tags := []string{}
		if len(call.Arguments) > 3 {
			if callTags, ok := call.Arguments.Get(3).([]string); ok {
				tags = append([]string(nil), callTags...)
			}
		}

		metricsByName[specMetricName] = append(metricsByName[specMetricName], metric{
			name: specMetricName,
			tags: tags,
		})
	}

	return metricsByName
}

func newMockWorkloadMetaGPU(uuid string, index int, deviceType workloadmeta.GPUDeviceType, parentUUID string) *workloadmeta.GPU {
	gpu := &workloadmeta.GPU{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindGPU,
			ID:   uuid,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: testutil.DefaultGPUName,
		},
		Vendor:             "nvidia",
		Device:             testutil.DefaultGPUName,
		DriverVersion:      testutil.DefaultNvidiaDriverVersion,
		Index:              index,
		DeviceType:         deviceType,
		VirtualizationMode: "none",
	}

	if parentUUID != "" {
		gpu.ParentGPUUUID = parentUUID
	}

	return gpu
}

func gpuTagsFromWorkloadMetaGPU(gpu *workloadmeta.GPU) []string {
	return []string{
		"gpu_uuid:" + gpu.ID,
		"gpu_device:" + strings.ToLower(strings.ReplaceAll(gpu.Device, " ", "_")),
		"gpu_vendor:" + strings.ToLower(gpu.Vendor),
		"gpu_driver_version:" + gpu.DriverVersion,
	}
}

func validateMetricTagsAgainstSpec(t *testing.T, spec *gpuspec.MetricsSpec, metricName string, metricSpec gpuspec.MetricSpec, emittedMetrics []metric, knownTagValues map[string]string) {
	t.Helper()
	require.NotEmpty(t, emittedMetrics, "metric %s has no emitted samples to validate tags", metricName)

	requiredTags := make(map[string]struct{})
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, ok := spec.Tagsets[tagsetName]
		require.True(t, ok, "metric %s references unknown tagset %s", metricName, tagsetName)
		for _, tag := range tagsetSpec.Tags {
			requiredTags[tag] = struct{}{}
		}
	}
	for _, tag := range metricSpec.CustomTags {
		requiredTags[tag] = struct{}{}
	}

	for _, emittedMetric := range emittedMetrics {
		tagsByKey := tagsToKeyValues(emittedMetric.tags)

		// check that all required tags are present
		for tag := range requiredTags {
			require.Contains(t, tagsByKey, tag, "metric %s missing required tag key %s", metricName, tag)
		}

		// check that no unknown tags are present, and that all known tags have non-empty values. If the tag should have
		// a specific value, check that the value is as expected.
		for key, values := range tagsByKey {
			_, allowed := requiredTags[key]
			require.True(t, allowed, "metric %s has unknown tag key %s", metricName, key)

			for _, value := range values {
				require.NotEmpty(t, value, "metric %s has empty value for tag %s", metricName, key)
				if expectedValue, ok := knownTagValues[key]; ok {
					require.Equal(t, expectedValue, value, "metric %s has unexpected value for tag %s", metricName, key)
				}
			}
		}
	}
}

func tagsToKeyValues(tags []string) map[string][]string {
	result := make(map[string][]string, len(tags))
	for _, tag := range tags {
		key, value, ok := strings.Cut(tag, ":")
		if !ok || key == "" || value == "" {
			continue
		}
		result[key] = append(result[key], value)
	}
	return result
}
