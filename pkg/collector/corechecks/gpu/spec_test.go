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

func loadSpec(t *testing.T) *specFile {
	t.Helper()
	data, err := os.ReadFile("spec/gpu_metrics.yaml")
	require.NoError(t, err, "failed to read spec file")

	var spec specFile
	require.NoError(t, yaml.Unmarshal(data, &spec))
	return &spec
}

func TestLoadSpecNotEmpty(t *testing.T) {
	spec := loadSpec(t)

	require.NotEmpty(t, spec.MetricPrefix, "metric_prefix should not be empty")
	require.NotEmpty(t, spec.Tagsets, "tagsets should not be empty")
	require.NotEmpty(t, spec.Metrics, "metrics should not be empty")
}

func TestRunMetricsArePresentInSpec(t *testing.T) {
	spec := loadSpec(t)

	// Build spec metric set for quick membership checks.
	specMetrics := make(map[string]struct{}, len(spec.Metrics))
	for _, m := range spec.Metrics {
		specMetrics[m.Name] = struct{}{}
	}

	emittedMetrics := runCheckAndCollectMetricNames(t)
	require.NotEmpty(t, emittedMetrics, "expected check run to emit gpu metrics")
	emittedSet := make(map[string]struct{}, len(emittedMetrics))
	for _, metricName := range emittedMetrics {
		emittedSet[metricName] = struct{}{}
	}

	// Deprecated metrics are kept in spec for visibility/history but are not expected
	// from current check runs. XID metrics require real device events.
	notExpectedOnBasicRun := map[string]string{
		"errors.xid.total": "requires XID device events",
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

				_, found := emittedSet[metric.Name]
				require.True(t, found, "spec metric is not emitted by check run: %s", metric.Name)
			})
		}
	})
}

func runCheckAndCollectMetricNames(t *testing.T) []string {
	t.Helper()

	senderManager := mocksender.CreateDefaultDemultiplexer()
	mockSender := mocksender.NewMockSenderWithSenderManager("gpu", senderManager)
	mockSender.SetupAcceptAll()

	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	ddnvml.WithMockNVML(t, testutil.GetBasicNvmlMockWithOptions(testutil.WithMockAllFunctions()))

	checkGeneric := newCheck(fakeTagger, testutil.GetTelemetryMock(t), testutil.GetWorkloadMetaMockWithDefaultGPUs(t))
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
