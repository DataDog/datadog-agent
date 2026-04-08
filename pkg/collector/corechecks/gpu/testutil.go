// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && nvml && test

package gpu

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
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
func GetEmittedGPUMetrics(mockSender *mocksender.MockSender) map[string][]gpuspec.EmittedMetric {
	metricsByName := make(map[string][]gpuspec.EmittedMetric)

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

		metricsByName[specMetricName] = append(metricsByName[specMetricName], gpuspec.EmittedMetric{
			Name: specMetricName,
			Tags: tags,
		})
	}

	return metricsByName
}
