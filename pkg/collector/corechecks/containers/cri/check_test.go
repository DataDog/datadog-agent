// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cri

package cri

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri"
	"github.com/DataDog/datadog-agent/pkg/util/containers/cri/crimock"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"

	criTypes "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/stretchr/testify/assert"
)

func TestCriCheck(t *testing.T) {
	// Creating mocks
	fullStatContainer := generic.CreateContainerMeta("containerd", "cID100")
	fullStatContainer.Labels = map[string]string{"io.kubernetes.pod.namespace": "default"}
	containersMeta := []*workloadmeta.Container{
		// Container with full stats
		fullStatContainer,
		// Should never been called as it does not have the kubernetes label
		generic.CreateContainerMeta("docker", "cID101"),
	}

	containersStats := map[string]mock.ContainerEntry{
		"cID100": mock.GetFullSampleContainerEntry(),
		"cID101": mock.GetFullSampleContainerEntry(),
	}

	// Inject mock processor in check
	mockCri := &crimock.MockCRIClient{}
	mockSender, processor, _ := generic.CreateTestProcessor(containersMeta, containersStats, metricsAdapter{}, getProcessorFilter(nil))
	processor.RegisterExtension("cri-custom-metrics", &criCustomMetricsExtension{criGetter: func() (cri.CRIClient, error) { return mockCri, nil }})

	mockCri.On("ListContainerStats").Return(map[string]*criTypes.ContainerStats{
		"cID100": {
			WritableLayer: &criTypes.FilesystemUsage{
				UsedBytes: &criTypes.UInt64Value{
					Value: 10,
				},
				InodesUsed: &criTypes.UInt64Value{
					Value: 20,
				},
			},
		},
		"cID101": {
			WritableLayer: &criTypes.FilesystemUsage{
				UsedBytes: &criTypes.UInt64Value{
					Value: 30,
				},
				InodesUsed: &criTypes.UInt64Value{
					Value: 40,
				},
			},
		},
	}, nil)

	// Create CRI check
	check := CRICheck{
		instance: &CRIConfig{
			CollectDisk: true,
		},
		processor: *processor,
	}

	err := check.runProcessor(mockSender)
	assert.NoError(t, err)

	expectedTags := []string{"runtime:containerd"}
	mockSender.AssertNumberOfCalls(t, "Rate", 1)
	mockSender.AssertNumberOfCalls(t, "Gauge", 4)

	mockSender.AssertMetricInRange(t, "Gauge", "cri.uptime", 0, 600, "", expectedTags)
	mockSender.AssertMetric(t, "Rate", "cri.cpu.usage", 100, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "cri.mem.rss", 42000, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "cri.disk.used", 10, "", expectedTags)
	mockSender.AssertMetric(t, "Gauge", "cri.disk.inodes", 20, "", expectedTags)
}
