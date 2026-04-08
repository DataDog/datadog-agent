// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package gpu

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	taggercollectors "github.com/DataDog/datadog-agent/comp/core/tagger/collectors"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taglist"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// WithGPUConfigEnabled enables the GPU check configuration for testing
// and registers a cleanup to disable it after the test completes.
func WithGPUConfigEnabled(t testing.TB) {
	t.Helper()
	pkgconfigsetup.Datadog().SetWithoutSource("gpu.enabled", true)
	t.Cleanup(func() {
		pkgconfigsetup.Datadog().SetWithoutSource("gpu.enabled", false)
	})
}

// GetEmittedGPUMetrics returns emitted GPU metrics from a mock sender keyed by
// spec metric name without the "gpu." prefix.
func GetEmittedGPUMetrics(mockSender *mocksender.MockSender) map[string][]gpuspec.MetricObservation {
	metricsByName := make(map[string][]gpuspec.MetricObservation)

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
		var value *float64
		if len(call.Arguments) > 1 {
			if metricValue, ok := call.Arguments.Get(1).(float64); ok {
				value = &metricValue
			}
		}

		metricsByName[specMetricName] = append(metricsByName[specMetricName], gpuspec.MetricObservation{
			Name:  specMetricName,
			Tags:  tags,
			Value: value,
		})
	}

	return metricsByName
}

// ValidateEmittedMetricsAgainstSpec validates emitted metrics against the spec for a given GPU config.
func ValidateEmittedMetricsAgainstSpec(t *testing.T, metricsSpec *gpuspec.MetricsSpec, config gpuspec.GPUConfig, emittedMetrics map[string][]gpuspec.MetricObservation, knownTagValues map[string]string) {
	results, err := gpuspec.ValidateEmittedMetricsAgainstSpec(metricsSpec, config, emittedMetrics, knownTagValues)
	require.NoError(t, err, "internal failure validating emitted metrics, likely a bug or inconsistency in the spec")

	for metricName, status := range results.Metrics {
		t.Run(metricName, func(t *testing.T) {
			assert.Empty(t, status.Errors, "metric %s has errors: %s", metricName, status.Errors)
			for tag, tagResult := range status.TagResults {
				assert.Zero(t, tagResult.Missing, "metric %s: tag %s missing in %d cases", metricName, tag, tagResult.Missing)
				assert.Zero(t, tagResult.Unknown, "metric %s: tag %s unknown in %d cases", metricName, tag, tagResult.Unknown)
				assert.Zero(t, tagResult.InvalidValue, "metric %s: tag %s invalid in %d cases", metricName, tag, tagResult.InvalidValue)
			}
		})
	}
}

func SetupWorkloadmetaGPUs(t *testing.T, wmetaMock workloadmetamock.Mock, fakeTagger taggermock.Mock, mode gpuspec.DeviceMode, validateDeviceCount bool) {
	// Create the NVML collector to ensure we get the data in the same way as with real checks
	cfg := config.NewMockWithOverrides(t, map[string]interface{}{
		"gpu.integrate_with_workloadmeta_processes": false,
	})
	nvmlCollector := collectors.GetNvmlCollector(t, cfg)
	ctx := t.Context()
	require.NoError(t, nvmlCollector.Start(ctx, wmetaMock), "failed to start NVML collector")
	require.NoError(t, nvmlCollector.Pull(ctx), "failed to pull NVML collector")

	// Iterate all GPUs from workloadmeta and set the tags in the tagger
	wmetaGpus := wmetaMock.ListGPUs()

	if validateDeviceCount {
		if mode == gpuspec.DeviceModeMIG {
			require.Len(t, wmetaGpus, 2, "expected 2 GPUs in MIG mode (parent + child)")
		} else {
			require.Len(t, wmetaGpus, 1, "expected 1 GPU in non-MIG mode")
		}
	}
	for _, gpu := range wmetaGpus {
		tags := taglist.NewTagList()
		taggercollectors.ExtractGPUTags(gpu, tags)
		low, orch, high, standard := tags.Compute()
		fakeTagger.SetTags(
			taggertypes.NewEntityID(taggertypes.GPU, gpu.ID),
			"spec-test",
			low,
			orch,
			high,
			standard,
		)
	}
}
