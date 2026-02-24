// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

// specFile is the YAML metric specification
type specFile struct {
	MetricPrefix string                `yaml:"metric_prefix"`
	Tagsets      map[string]specTagset `yaml:"tagsets"`
	Metrics      metricsMap            `yaml:"metrics"`
}

type specTagset struct {
	Tags []string `yaml:"tags"`
}

// metricsMap is metrics by name (map format in YAML only).
type metricsMap map[string]specMetric

// specMetric is a metric definition without the name (name is the map key).
type specMetric struct {
	Type       string            `yaml:"type"`
	Tagsets    []string          `yaml:"tagsets"`
	CustomTags []string          `yaml:"custom_tags,omitempty"`
	Support    metricSupportSpec `yaml:"support"`
}

type metricSupportSpec struct {
	UnsupportedArchitectures []string        `yaml:"unsupported_architectures"`
	DeviceFeatures           map[string]bool `yaml:"device_features"`
	ProcessData              bool            `yaml:"process_data"`
}

type architecturesFile struct {
	Architectures map[string]architectureSpec `yaml:"architectures"`
}

type architectureCapabilities struct {
	GPM               bool     `yaml:"gpm"`
	UnsupportedFields []string `yaml:"unsupported_fields"`
}

type architectureSpec struct {
	Capabilities              architectureCapabilities `yaml:"capabilities"`
	UnsupportedDeviceFeatures []string                 `yaml:"unsupported_device_features"`
}

func (m specMetric) supportsArchitecture(arch string) bool {
	for _, u := range m.Support.UnsupportedArchitectures {
		if u == arch {
			return false
		}
	}
	return true
}

// supportsDeviceFeature returns true if the metric's device_features explicitly allows the mode.
// device_features values are expected to be booleans; missing means unsupported.
func (m specMetric) supportsDeviceFeature(mode string) bool {
	if m.Support.DeviceFeatures == nil {
		return false
	}
	v, ok := m.Support.DeviceFeatures[mode]
	return ok && v
}

func (m specMetric) isDeviceFeatureExplicitlyUnsupported(mode string) bool {
	if m.Support.DeviceFeatures == nil {
		return false
	}
	v, ok := m.Support.DeviceFeatures[mode]
	return ok && !v
}

var nvmlFieldNameToFieldID = map[string]uint32{
	"FI_DEV_MEMORY_TEMP":                       nvml.FI_DEV_MEMORY_TEMP,
	"FI_DEV_NVLINK_LINK_COUNT":                 nvml.FI_DEV_NVLINK_LINK_COUNT,
	"FI_DEV_NVLINK_THROUGHPUT_DATA_RX":         nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_RX,
	"FI_DEV_NVLINK_THROUGHPUT_DATA_TX":         nvml.FI_DEV_NVLINK_THROUGHPUT_DATA_TX,
	"FI_DEV_NVLINK_THROUGHPUT_RAW_RX":          nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_RX,
	"FI_DEV_NVLINK_THROUGHPUT_RAW_TX":          nvml.FI_DEV_NVLINK_THROUGHPUT_RAW_TX,
	"FI_DEV_NVLINK_SPEED_MBPS_COMMON":          nvml.FI_DEV_NVLINK_SPEED_MBPS_COMMON,
	"FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT":     nvml.FI_DEV_NVSWITCH_CONNECTED_LINK_COUNT,
	"FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_CRC_DATA_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_CRC_FLIT_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_ECC_DATA_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL": nvml.FI_DEV_NVLINK_RECOVERY_ERROR_COUNT_TOTAL,
	"FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL":   nvml.FI_DEV_NVLINK_REPLAY_ERROR_COUNT_TOTAL,
	"FI_DEV_PCIE_REPLAY_COUNTER":               nvml.FI_DEV_PCIE_REPLAY_COUNTER,
	"FI_DEV_PERF_POLICY_THERMAL":               nvml.FI_DEV_PERF_POLICY_THERMAL,
}

func unsupportedFieldIDsFromNames(t *testing.T, names []string) []uint32 {
	t.Helper()
	ids := make([]uint32, 0, len(names))
	for _, name := range names {
		id, found := nvmlFieldNameToFieldID[name]
		require.True(t, found, "unknown NVML field in architectures capabilities: %s", name)
		ids = append(ids, id)
	}
	return ids
}

func allConfiguredNVMLFieldValues() []nvml.FieldValue {
	ids := make([]uint32, 0, len(nvmlFieldNameToFieldID))
	for _, id := range nvmlFieldNameToFieldID {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	values := make([]nvml.FieldValue, len(ids))
	for i, id := range ids {
		values[i] = nvml.FieldValue{FieldId: id}
	}
	return values
}

func loadSpec(t *testing.T) *specFile {
	t.Helper()
	data, err := os.ReadFile("spec/gpu_metrics.yaml")
	require.NoError(t, err, "failed to read spec file")

	var spec specFile
	require.NoError(t, yaml.Unmarshal(data, &spec))
	return &spec
}

func loadArchitectures(t *testing.T) *architecturesFile {
	t.Helper()
	data, err := os.ReadFile("spec/architectures.yaml")
	require.NoError(t, err, "failed to read architectures spec file")

	var arch architecturesFile
	require.NoError(t, yaml.Unmarshal(data, &arch))
	return &arch
}

// isModeSupportedByArchitecture returns true if the architecture spec allows the device feature mode
// (i.e. mode is not in unsupported_device_features).
func isModeSupportedByArchitecture(archSpec architectureSpec, mode string) bool {
	for _, u := range archSpec.UnsupportedDeviceFeatures {
		if u == mode {
			return false
		}
	}
	return true
}

// buildMockOptionsForArchAndMode returns the same NVML mock options used by runCheckAndCollectMetricNamesWithConfig
// for the given architecture and device feature mode, so capability assertions use the same mock contract.
func buildMockOptionsForArchAndMode(t *testing.T, archName string, mode testutil.DeviceFeatureMode, archSpec architectureSpec) []testutil.NvmlMockOption {
	t.Helper()
	caps := testutil.Capabilities{
		GPM:               archSpec.Capabilities.GPM,
		UnsupportedFields: unsupportedFieldIDsFromNames(t, archSpec.Capabilities.UnsupportedFields),
	}
	opts := []testutil.NvmlMockOption{
		testutil.WithArchitecture(archName),
		testutil.WithCapabilities(caps),
		testutil.WithMockAllFunctions(),
	}
	switch mode {
	case testutil.DeviceFeaturePhysical:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceCount(1), testutil.WithMIGDisabled()}, opts...)
	case testutil.DeviceFeatureMIG:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceFeatureMode(mode)}, opts...)
	case testutil.DeviceFeatureVGPU:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceCount(1), testutil.WithDeviceFeatureMode(mode)}, opts...)
	}
	return opts
}

func TestLoadSpecNotEmpty(t *testing.T) {
	spec := loadSpec(t)

	require.NotEmpty(t, spec.MetricPrefix, "metric_prefix should not be empty")
	require.NotEmpty(t, spec.Tagsets, "tagsets should not be empty")
	require.NotEmpty(t, spec.Metrics, "metrics should not be empty")
	for name := range spec.Metrics {
		require.NotEmpty(t, name, "metric name should not be empty")
	}

	for metricName, metricSpec := range spec.Metrics {
		for featureMode := range metricSpec.Support.DeviceFeatures {
			require.Containsf(t, []string{"physical", "mig", "vgpu"}, featureMode, "metric %s has invalid device feature mode key %q", metricName, featureMode)
		}
	}
}

func TestLoadArchitecturesNotEmpty(t *testing.T) {
	arch := loadArchitectures(t)

	require.NotEmpty(t, arch.Architectures, "architectures should not be empty")
	for name, spec := range arch.Architectures {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, spec.UnsupportedDeviceFeatures, "unsupported_device_features should be present")
		})
	}
}

// TestMockCapabilitiesMatchArchitectureSpec ensures that for each architecture and supported device mode,
// the NVML mock configured from architectures.yaml returns API behavior that matches the capability flags
// (gpm, unsupported_fields). This validates that the mock actually applies the spec.
func TestMockCapabilitiesMatchArchitectureSpec(t *testing.T) {
	archFile := loadArchitectures(t)
	deviceModes := []testutil.DeviceFeatureMode{
		testutil.DeviceFeaturePhysical,
		testutil.DeviceFeatureMIG,
		testutil.DeviceFeatureVGPU,
	}

	for archName, archSpec := range archFile.Architectures {
		for _, mode := range deviceModes {
			if !isModeSupportedByArchitecture(archSpec, string(mode)) {
				continue
			}

			subtestName := "arch=" + archName + "/mode=" + string(mode)
			t.Run(subtestName, func(t *testing.T) {
				opts := buildMockOptionsForArchAndMode(t, archName, mode, archSpec)
				ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(opts...))

				lib, err := ddnvml.GetSafeNvmlLib()
				require.NoError(t, err, "should be able to get NVML lib", archName, mode)
				dev, err := lib.DeviceGetHandleByIndex(0)
				require.NoError(t, err, "should be able to get device 0", archName, mode)

				caps := archSpec.Capabilities

				// gpm -> GpmQueryDeviceSupport(): IsSupportedDevice 1 when enabled, 0 when disabled
				support, err := dev.GpmQueryDeviceSupport()
				require.NoError(t, err, "GpmQueryDeviceSupport should not report an error")
				expected := uint32(0)
				if caps.GPM {
					expected = 1
				}
				assert.Equal(t, expected, support.IsSupportedDevice, "GpmQueryDeviceSupport.IsSupportedDevice should be %d when gpm=%v", expected, caps.GPM)

				unsupportedIDs := unsupportedFieldIDsFromNames(t, caps.UnsupportedFields)
				unsupportedSet := make(map[uint32]struct{}, len(unsupportedIDs))
				for _, id := range unsupportedIDs {
					unsupportedSet[id] = struct{}{}
				}

				fieldValues := allConfiguredNVMLFieldValues()
				err = dev.GetFieldValues(fieldValues)
				if (mode == testutil.DeviceFeatureVGPU || mode == testutil.DeviceFeatureMIG) && err != nil && ddnvml.IsUnsupported(err) {
					// vGPU and MIG modes can report field APIs as unsupported.
					return
				}
				require.NoError(t, err, "GetFieldValues should not return an error")
				for _, fv := range fieldValues {
					_, isUnsupported := unsupportedSet[fv.FieldId]
					if isUnsupported {
						require.Equal(t, uint32(nvml.ERROR_NOT_SUPPORTED), fv.NvmlReturn, "field id %d should be unsupported", fv.FieldId)
					} else {
						require.Equal(t, uint32(nvml.SUCCESS), fv.NvmlReturn, "field id %d should be supported", fv.FieldId)
					}
				}
			})
		}
	}
}

func TestMetricsFollowSpec(t *testing.T) {
	spec := loadSpec(t)
	archFile := loadArchitectures(t)

	// Build spec metric set for quick membership checks.
	specMetrics := make(map[string]struct{}, len(spec.Metrics))
	for name := range spec.Metrics {
		specMetrics[name] = struct{}{}
	}

	// XID metrics require real device events.
	notExpectedOnBasicRun := map[string]bool{
		"errors.xid.total": true,
	}

	deviceModes := []testutil.DeviceFeatureMode{
		testutil.DeviceFeaturePhysical,
		testutil.DeviceFeatureMIG,
		testutil.DeviceFeatureVGPU,
	}

	for archName, archSpec := range archFile.Architectures {
		for _, mode := range deviceModes {
			if !isModeSupportedByArchitecture(archSpec, string(mode)) {
				continue
			}
			archName := archName
			archSpec := archSpec
			mode := mode
			subtestName := "arch=" + archName + "/mode=" + string(mode)
			t.Run(subtestName, func(t *testing.T) {
				emittedTagsByMetric, knownTagValues := collectMetricSamples(t, archName, mode, archSpec)

				t.Run("_emits_only_expected_metrics", func(t *testing.T) {
					for metricName := range emittedTagsByMetric {
						assert.Contains(t, specMetrics, metricName, "metric emitted by check is missing from spec: %s", metricName)

						metricSpec := spec.Metrics[metricName]
						assert.False(t, notExpectedOnBasicRun[metricName], "metric should not be emitted in basic run: %s", metricName)
						assert.True(t, metricSpec.supportsArchitecture(archName), "metric %s emitted on unsupported architecture %s", metricName, archName)
						assert.False(t, metricSpec.isDeviceFeatureExplicitlyUnsupported(string(mode)), "metric %s emitted on unsupported device mode %s", metricName, mode)
					}
				})

				for name, m := range spec.Metrics {
					if notExpectedOnBasicRun[name] || !m.supportsArchitecture(archName) || !m.supportsDeviceFeature(string(mode)) {
						continue
					}

					t.Run(name, func(t *testing.T) {
						_, found := emittedTagsByMetric[name]
						require.True(t, found, "spec metric is not emitted by check run: %s", name)
						validateMetricTagsAgainstSpec(t, spec, name, m, emittedTagsByMetric[name], knownTagValues)
					})
				}
			})
		}
	}
}

// collectMetricSamples runs the GPU check with a capability-driven mock
// for the given architecture and device feature mode, then returns emitted metrics (without "gpu." prefix)
// and their tags.
func collectMetricSamples(t *testing.T, archName string, mode testutil.DeviceFeatureMode, archSpec architectureSpec) (map[string][][]string, map[string]string) {
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

func setupMockCheckForMetricCollection(t *testing.T, archName string, mode testutil.DeviceFeatureMode, archSpec architectureSpec) metricCollectionSetup {
	t.Helper()

	opts := []testutil.NvmlMockOption{
		testutil.WithArchitecture(archName),
		testutil.WithCapabilities(testutil.Capabilities{
			GPM:               archSpec.Capabilities.GPM,
			UnsupportedFields: unsupportedFieldIDsFromNames(t, archSpec.Capabilities.UnsupportedFields),
		}),
		testutil.WithMockAllFunctions(),
	}
	switch mode {
	case testutil.DeviceFeaturePhysical:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceCount(1), testutil.WithMIGDisabled()}, opts...)
	case testutil.DeviceFeatureMIG:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceFeatureMode(mode)}, opts...)
	case testutil.DeviceFeatureVGPU:
		opts = append([]testutil.NvmlMockOption{testutil.WithDeviceCount(1), testutil.WithDeviceFeatureMode(mode)}, opts...)
	}

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
	if mode == testutil.DeviceFeatureMIG {
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
				"kube_container_name:" + fmt.Sprintf("name-%s", containerID),
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
	if mode == testutil.DeviceFeatureMIG {
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
		"gpu_vendor":         "nvidia",
		"gpu_driver_version": testutil.DefaultNvidiaDriverVersion,
	}

	return metricCollectionSetup{
		mockSender:     mockSender,
		runCollection:  runCollection,
		knownTagValues: knownTagValues,
	}
}

func getEmittedGPUMetricsWithTags(mockSender *mocksender.MockSender) map[string][][]string {
	metricsByName := make(map[string][][]string)

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

		metricsByName[specMetricName] = append(metricsByName[specMetricName], tags)
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

func validateMetricTagsAgainstSpec(t *testing.T, spec *specFile, metricName string, metricSpec specMetric, samples [][]string, knownTagValues map[string]string) {
	t.Helper()
	require.NotEmpty(t, samples, "metric %s has no emitted samples to validate tags", metricName)

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
	for _, sampleTags := range samples {
		tagsByKey := tagsToKeyValues(sampleTags)

		for tag := range requiredTags {
			require.Contains(t, tagsByKey, tag, "metric %s missing required tag key %s", metricName, tag)
		}
		for tag := range tagsByKey {
			_, allowed := requiredTags[tag]
			require.True(t, allowed, "metric %s has unknown tag key %s", metricName, tag)
		}

		for key, values := range tagsByKey {
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
