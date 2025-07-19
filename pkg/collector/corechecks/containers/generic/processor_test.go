// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggerUtils "github.com/DataDog/datadog-agent/comp/core/tagger/utils"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
)

func TestProcessorRunFullStatsLinux(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	containersMeta := []*workloadmeta.Container{
		// Container with full stats
		CreateContainerMeta("docker", "cID100"),
		// Container with no stats (returns nil)
		CreateContainerMeta("docker", "cID101"),
	}

	containersStats := map[string]mock.ContainerEntry{
		"cID100": mock.GetFullSampleContainerEntry(),
		"cID101": {
			ContainerStats: nil,
		},
	}

	tests := []struct {
		name            string
		extendedMetrics bool
	}{
		{
			name:            "ExtendedMetricsNotSent",
			extendedMetrics: false,
		},
		{
			name:            "ExtendedMetricsSent",
			extendedMetrics: true,
		},
	}

	missingMetrics := []struct {
		metricType string
		metricName string
	}{
		{"Gauge", "container.memory.page_tables"},
		{"Gauge", "container.memory.active_anon"},
		{"Gauge", "container.memory.inactive_anon"},
		{"Gauge", "container.memory.active_file"},
		{"Gauge", "container.memory.inactive_file"},
		{"Gauge", "container.memory.unevictable"},
		{"Gauge", "container.memory.shmem"},
		{"Gauge", "container.memory.file_mapped"},
		{"Gauge", "container.memory.file_dirty"},
		{"Gauge", "container.memory.file_writeback"},
		{"MonotonicCount", "container.memory.refault_anon"},
		{"MonotonicCount", "container.memory.refault_file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockSender, processor, _ := CreateTestProcessor(containersMeta, containersStats, GenericMetricsAdapter{}, nil, fakeTagger, tt.extendedMetrics)
			err := processor.Run(mockSender, 0)
			assert.NoError(t, err)

			expectedTags := []string{"runtime:docker"}
			mockSender.AssertNumberOfCalls(t, "Rate", 20)
			mockSender.AssertNumberOfCalls(t, "Gauge", 17+func() int {
				if tt.extendedMetrics {
					return 10 // 10 comes from extended set
				}
				return 0
			}())

			mockSender.AssertNumberOfCalls(t, "MonotonicCount", 2+func() int {
				if tt.extendedMetrics {
					return 2 // 2 comes from extended set
				}
				return 0
			}())

			mockSender.AssertMetricInRange(t, "Gauge", "container.uptime", 0, 600, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.usage", 100, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.user", 300, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.system", 200, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.throttled", 100, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.throttled.periods", 0, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.cpu.partial_stall", 96000, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.cpu.limit", 500000000, "", expectedTags)

			mockSender.AssertMetric(t, "Gauge", "container.memory.usage", 42000, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.kernel", 40, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.limit", 42000, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.soft_limit", 40000, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.rss", 300, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.cache", 200, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.working_set", 350, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.swap", 0, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.oom_events", 10, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.memory.usage.peak", 50000, "", expectedTags)
			mockSender.AssertMetric(t, "Rate", "container.memory.partial_stall", 97000, "", expectedTags)

			mockSender.AssertMetric(t, "MonotonicCount", "container.memory.page_faults", 97001, "", expectedTags)
			mockSender.AssertMetric(t, "MonotonicCount", "container.memory.major_page_faults", 50002, "", expectedTags)

			if tt.extendedMetrics {
				mockSender.AssertMetric(t, "Gauge", "container.memory.page_tables", 661, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.active_anon", 662, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.inactive_anon", 663, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.active_file", 664, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.inactive_file", 665, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.unevictable", 666, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.shmem", 1481, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.file_mapped", 1482, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.file_dirty", 1483, "", expectedTags)
				mockSender.AssertMetric(t, "Gauge", "container.memory.file_writeback", 1484, "", expectedTags)
				mockSender.AssertMetric(t, "MonotonicCount", "container.memory.refault_anon", 1485, "", expectedTags)
				mockSender.AssertMetric(t, "MonotonicCount", "container.memory.refault_file", 1486, "", expectedTags)

			} else { // if extendedMetrics processor should not emit those metrics
				for _, m := range missingMetrics {
					mockSender.AssertMetricMissing(t, m.metricType, m.metricName)
				}
			}

			mockSender.AssertMetric(t, "Gauge", "container.restarts", 42, "", expectedTags)

			mockSender.AssertMetric(t, "Rate", "container.io.partial_stall", 98000, "", expectedTags)
			expectedFooTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo")
			mockSender.AssertMetric(t, "Rate", "container.io.read", 100, "", expectedFooTags)
			mockSender.AssertMetric(t, "Rate", "container.io.read.operations", 10, "", expectedFooTags)
			mockSender.AssertMetric(t, "Rate", "container.io.write", 200, "", expectedFooTags)
			mockSender.AssertMetric(t, "Rate", "container.io.write.operations", 20, "", expectedFooTags)
			expectedBarTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar")
			mockSender.AssertMetric(t, "Rate", "container.io.read", 100, "", expectedBarTags)
			mockSender.AssertMetric(t, "Rate", "container.io.read.operations", 10, "", expectedBarTags)
			mockSender.AssertMetric(t, "Rate", "container.io.write", 200, "", expectedBarTags)
			mockSender.AssertMetric(t, "Rate", "container.io.write.operations", 20, "", expectedBarTags)

			mockSender.AssertMetric(t, "Gauge", "container.pid.thread_count", 10, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.pid.thread_limit", 20, "", expectedTags)
			mockSender.AssertMetric(t, "Gauge", "container.pid.open_files", 200, "", expectedTags)

			// Produced by default NetworkExtension
			expectedEth42Tags := taggerUtils.ConcatenateStringTags(expectedTags, "interface:eth42")
			mockSender.AssertMetric(t, "Rate", "container.net.sent", 42, "", expectedEth42Tags)
			mockSender.AssertMetric(t, "Rate", "container.net.sent.packets", 420, "", expectedEth42Tags)
			mockSender.AssertMetric(t, "Rate", "container.net.rcvd", 43, "", expectedEth42Tags)
			mockSender.AssertMetric(t, "Rate", "container.net.rcvd.packets", 421, "", expectedEth42Tags)
		})
	}
}

func TestProcessorRunPartialStats(t *testing.T) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	containersMeta := []*workloadmeta.Container{
		// Container without stats
		CreateContainerMeta("containerd", "cID201"),
		// Container with explicit error
		CreateContainerMeta("containerd", "cID202"),
	}

	containersStats := map[string]mock.ContainerEntry{
		"cID202": {
			Error: fmt.Errorf("Unable to read some stuff"),
		},
	}

	mockSender, processor, _ := CreateTestProcessor(containersMeta, containersStats, GenericMetricsAdapter{}, nil, fakeTagger, false)
	err := processor.Run(mockSender, 0)
	assert.NoError(t, err)

	mockSender.AssertNumberOfCalls(t, "Rate", 0)
	mockSender.AssertNumberOfCalls(t, "Gauge", 0)
}
