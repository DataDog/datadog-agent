// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package cluster

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

type metricsExpected struct {
	val  float64
	name string
	tags []string
}

func TestProcessMetrics(t *testing.T) {
	tests := []struct {
		name             string
		metricsToProcess map[string][]ksmstore.DDMetricsFam
		expected         []metricsExpected
	}{
		{
			name: "single metrics",
			metricsToProcess: map[string][]ksmstore.DDMetricsFam{
				"kube_node_created": {
					{
						Type: "*v1.Node",
						Name: "kube_node_created",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"node": "gke-charly-default-pool-6948dc89-g54n", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1.588236673e+09,
							},
						},
					},
					{
						Type: "*v1.Node",
						Name: "kube_node_created",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"node": "gke-charly-default-pool-6948dc89-fs46", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1.588197278e+09,
							},
						},
					},
				},
				"kube_node_status_condition": {
					{
						Type: "*v1.Node",
						Name: "kube_node_status_condition",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "true", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "false", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-fs46", "status": "unknown", "uid": "05e99c5f-8a64-11ea-8546-42010a80022c"},
								Val:    0,
							},
						},
					},
					{
						Type: "*v1.Node",
						Name: "kube_node_status_condition",
						ListMetrics: []ksmstore.DDMetric{
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentUnregisterNetDevice", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentKubeletRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentDockerRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "FrequentContainerdRestart", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "CorruptDockerOverlay2", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "KernelDeadlock", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "ReadonlyFilesystem", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "NetworkUnavailable", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "MemoryPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "DiskPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "PIDPressure", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "true", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    1,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "false", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
							{
								Labels: map[string]string{"condition": "Ready", "node": "gke-charly-default-pool-6948dc89-g54n", "status": "unknown", "uid": "bec19172-8abf-11ea-8546-42010a80022c"},
								Val:    0,
							},
						},
					},
				},
			},
			expected: []metricsExpected{
				{
					name: "kube_node_status_condition",
					val:  0,
					tags: []string{"condition:Ready", "node:gke-charly-default-pool-6948dc89-g54n", "status:unknown", "uid:bec19172-8abf-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_status_condition",
					val:  1,
					tags: []string{"condition:Ready", "node:gke-charly-default-pool-6948dc89-fs46", "status:true", "uid:05e99c5f-8a64-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_status_condition",
					val:  1,
					tags: []string{"condition:PIDPressure", "node:gke-charly-default-pool-6948dc89-g54n", "status:false", "uid:bec19172-8abf-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_status_condition",
					val:  1,
					tags: []string{"condition:ReadonlyFilesystem", "node:gke-charly-default-pool-6948dc89-fs46", "status:false", "uid:05e99c5f-8a64-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_status_condition",
					val:  0,
					tags: []string{"condition:ReadonlyFilesystem", "node:gke-charly-default-pool-6948dc89-fs46", "status:true", "uid:05e99c5f-8a64-11ea-8546-42010a80022c"},
				},
			},
		},
	}
	for _, test := range tests {
		kubeStateMetricsSCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{})
		mocked := mocksender.NewMockSender(kubeStateMetricsSCheck.ID())
		mocked.SetupAcceptAll()

		processMetrics(mocked, test.metricsToProcess)
		t.Run(test.name, func(t *testing.T) {
			for _, expectMetric := range test.expected {
				mocked.AssertMetric(t, "Gauge", expectMetric.name, expectMetric.val, "", expectMetric.tags)
			}
		})
	}
}
