// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	taggerUtils "github.com/DataDog/datadog-agent/pkg/tagger/utils"
	"github.com/DataDog/datadog-agent/pkg/util/containers/v2/metrics"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

func TestContainerdCheckGenericPart(t *testing.T) {
	// Creating mocks
	containersMeta := []*workloadmeta.Container{
		// Container with full stats
		generic.CreateContainerMeta("containerd", "cID100"),
		// Should never been called as we are in the Docker check
		generic.CreateContainerMeta("docker", "cID101"),
	}

	containersStats := map[string]metrics.MockContainerEntry{
		"cID100": metrics.GetFullSampleContainerEntry(),
		"cID101": metrics.GetFullSampleContainerEntry(),
	}

	// Inject mock processor in check
	mockSender, processor, _ := generic.CreateTestProcessor(containersMeta, nil, containersStats, metricsAdapter{}, getProcessorFilter(nil))
	processor.RegisterExtension("containerd-custom-metrics", &containerdCustomMetricsExtension{})

	// Create Docker check
	check := ContainerdCheck{
		instance: &ContainerdConfig{
			CollectEvents: true,
		},
		processor: *processor,
	}

	err := check.runProcessor(mockSender)
	assert.NoError(t, err)

	expectedTags := []string{"runtime:containerd"}
	mockSender.AssertNumberOfCalls(t, "Rate", 13)
	mockSender.AssertNumberOfCalls(t, "Gauge", 10)

	mockSender.AssertMetricInRange(t, "Gauge", "containerd.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.total", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.user", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.system", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.throttled.time", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "containerd.cpu.throttled.periods", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.cpu.limit", 5e8, "", expectedTags)

	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.usage", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.kernel", 40, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.limit", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.rss", 300, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.cache", 200, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.swap.usage", 0, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "containerd.mem.current.failcnt", 10, "", expectedTags)

	fooReadTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo", "operation:read")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 100, "", fooReadTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 10, "", fooReadTags)
	fooWriteTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/foo", "device_name:/dev/foo", "operation:write")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 200, "", fooWriteTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 20, "", fooWriteTags)

	barReadTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar", "operation:read")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 100, "", barReadTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 10, "", barReadTags)
	barWriteTags := taggerUtils.ConcatenateStringTags(expectedTags, "device:/dev/bar", "device_name:/dev/bar", "operation:write")
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.service_recursive_bytes", 200, "", barWriteTags)
	mockSender.AssertMetric(t, "Rate", "containerd.blkio.serviced_recursive", 20, "", barWriteTags)

	mockSender.AssertMetric(t, "Gauge", "containerd.proc.open_fds", 200, "", expectedTags)
}
