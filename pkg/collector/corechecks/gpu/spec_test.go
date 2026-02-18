// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package gpu

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
	mock_containers "github.com/DataDog/datadog-agent/pkg/process/util/containers/mocks"
)

// specFile is the YAML metric specification
type specFile struct {
	MetricPrefix string                `yaml:"metric_prefix"`
	Tagsets      map[string]specTagset `yaml:"tagsets"`
	Metrics      []specMetric          `yaml:"metrics"`
}

type specTagset struct {
	Tags         []string `yaml:"tags"`
	FallbackTags []string `yaml:"fallback_tags"`
}

type specMetric struct {
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type"`
	Tagsets         []string          `yaml:"tagsets"`
	CustomTags      []string          `yaml:"custom_tags"`
	MemoryLocations []string          `yaml:"memory_locations"`
	Support         metricSupportSpec `yaml:"support"`
	Deprecated      bool              `yaml:"deprecated"`
	ReplacedBy      string            `yaml:"replaced_by"`
}

type metricSupportSpec struct {
	UnsupportedArchitectures []string          `yaml:"unsupported_architectures"`
	DeviceFeatures           map[string]string `yaml:"device_features"`
	ProcessData              bool              `yaml:"process_data"`
}

type architecturesFile struct {
	Architectures map[string]architectureSpec `yaml:"architectures"`
}

type architectureSpec struct {
	Capabilities              map[string]bool `yaml:"capabilities"`
	UnsupportedDeviceFeatures []string        `yaml:"unsupported_device_features"`
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

// isArchitectureSupported returns true if the metric is supported on the given architecture.
// A metric is supported if the architecture is not in the metric's unsupported_architectures list.
func isArchitectureSupported(metric specMetric, arch string) bool {
	for _, u := range metric.Support.UnsupportedArchitectures {
		if u == arch {
			return false
		}
	}
	return true
}

// isDeviceFeatureSupported returns true if the metric's device_features explicitly allows the mode.
// "true" = supported, "false" = not supported, "unknown" or missing = treat as not required for assertion.
func isDeviceFeatureSupported(metric specMetric, mode string) bool {
	if metric.Support.DeviceFeatures == nil {
		return false
	}
	v, ok := metric.Support.DeviceFeatures[mode]
	return ok && v == "true"
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

func TestLoadSpecNotEmpty(t *testing.T) {
	spec := loadSpec(t)

	require.NotEmpty(t, spec.MetricPrefix, "metric_prefix should not be empty")
	require.NotEmpty(t, spec.Tagsets, "tagsets should not be empty")
	require.NotEmpty(t, spec.Metrics, "metrics should not be empty")
}

func TestLoadArchitecturesNotEmpty(t *testing.T) {
	arch := loadArchitectures(t)

	require.NotEmpty(t, arch.Architectures, "architectures should not be empty")
	for name, spec := range arch.Architectures {
		name := name
		spec := spec
		t.Run(name, func(t *testing.T) {
			require.NotEmpty(t, spec.Capabilities, "capabilities should not be empty")
			require.NotNil(t, spec.UnsupportedDeviceFeatures, "unsupported_device_features should be present")
		})
	}
}

func TestRunMetricsArePresentInSpec(t *testing.T) {
	spec := loadSpec(t)
	archFile := loadArchitectures(t)

	// Build spec metric set for quick membership checks.
	specMetrics := make(map[string]struct{}, len(spec.Metrics))
	for _, m := range spec.Metrics {
		specMetrics[m.Name] = struct{}{}
	}

	// Deprecated metrics are kept in spec for visibility/history but are not expected
	// from current check runs. XID metrics require real device events.
	notExpectedOnBasicRun := map[string]string{
		"errors.xid.total": "requires XID device events",
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
				emittedMetrics := runCheckAndCollectMetricNamesWithConfig(t, archName, mode, archSpec)
				emittedSet := make(map[string]struct{}, len(emittedMetrics))
				for _, name := range emittedMetrics {
					emittedSet[name] = struct{}{}
				}

				t.Run("EmittedMetricsExistInSpec", func(t *testing.T) {
					for _, metricName := range emittedMetrics {
						metricName := metricName
						t.Run(metricName, func(t *testing.T) {
							_, found := specMetrics[metricName]
							require.True(t, found, "metric emitted by check is missing from spec: %s", metricName)
						})
					}
				})

				t.Run("SpecMetricsAreEmittedByRun", func(t *testing.T) {
					for _, metric := range spec.Metrics {
						metric := metric
						t.Run(metric.Name, func(t *testing.T) {
							if metric.Deprecated {
								t.Skip("deprecated metric; not expected from current check runs")
							}
							if reason, shouldSkip := notExpectedOnBasicRun[metric.Name]; shouldSkip {
								t.Skip(reason)
							}
							if !isArchitectureSupported(metric, archName) {
								t.Skip("metric not supported on this architecture")
							}
							if !isDeviceFeatureSupported(metric, string(mode)) {
								t.Skip("metric not supported for this device feature mode")
							}
							_, found := emittedSet[metric.Name]
							require.True(t, found, "spec metric is not emitted by check run: %s", metric.Name)
						})
					}
				})
			})
		}
	}
}

// runCheckAndCollectMetricNamesWithConfig runs the GPU check with a capability-driven mock
// for the given architecture and device feature mode, then returns emitted metric names (without "gpu." prefix).
func runCheckAndCollectMetricNamesWithConfig(t *testing.T, archName string, mode testutil.DeviceFeatureMode, archSpec architectureSpec) []string {
	t.Helper()

	opts := []testutil.NvmlMockOption{
		testutil.WithArchitecture(archName),
		testutil.WithCapabilities(testutil.CapabilitiesMap(archSpec.Capabilities)),
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

	wmeta := testutil.GetWorkloadMetaMockWithDefaultGPUs(t)
	if mode == testutil.DeviceFeatureMIG {
		for _, uuids := range testutil.MIGChildrenUUIDs {
			for _, u := range uuids {
				wmeta.Set(&workloadmeta.GPU{
					EntityID: workloadmeta.EntityID{ID: u, Kind: workloadmeta.KindGPU},
				})
			}
		}
	}

	checkGeneric := newCheck(fakeTagger, testutil.GetTelemetryMock(t), wmeta)
	check, ok := checkGeneric.(*Check)
	require.True(t, ok)

	WithGPUConfigEnabled(t)
	mockContainerProvider := mock_containers.NewMockContainerProvider(gomock.NewController(t))
	mockContainerProvider.EXPECT().GetPidToCid(gomock.Any()).Return(map[int]string{}).AnyTimes()
	check.containerProvider = mockContainerProvider
	require.NoError(t, check.Configure(senderManager, integration.FakeConfigHash, []byte{}, []byte{}, "test"))
	t.Cleanup(func() { checkGeneric.Cancel() })

	require.NoError(t, checkGeneric.Run())

	return getEmittedGPUMetrics(mockSender)
}

func getEmittedGPUMetrics(mockSender *mocksender.MockSender) []string {
	metricsSet := make(map[string]struct{})

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
		metricsSet[specMetricName] = struct{}{}
	}

	metrics := make([]string, 0, len(metricsSet))
	for metric := range metricsSet {
		metrics = append(metrics, metric)
	}
	slices.Sort(metrics)

	return metrics
}
