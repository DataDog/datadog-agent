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
	"github.com/stretchr/testify/assert"
)

type metricsExpected struct {
	val  float64
	name string
	tags []string
}

func TestProcessMetrics(t *testing.T) {
	tests := []struct {
		name             string
		config           *KSMConfig
		metricsToProcess map[string][]ksmstore.DDMetricsFam
		metricsToGet     []ksmstore.DDMetricsFam
		expected         []metricsExpected
	}{
		{
			name:   "single metrics",
			config: &KSMConfig{},
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
			metricsToGet: []ksmstore.DDMetricsFam{},
			expected: []metricsExpected{
				{
					name: "kube_node_created",
					val:  1.588236673e+09,
					tags: []string{"node:gke-charly-default-pool-6948dc89-g54n", "uid:bec19172-8abf-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_created",
					val:  1.588197278e+09,
					tags: []string{"node:gke-charly-default-pool-6948dc89-fs46", "uid:05e99c5f-8a64-11ea-8546-42010a80022c"},
				},
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
		{
			name:   "with label joins",
			config: &KSMConfig{LabelJoins: map[string]*JoinsConfig{"kube_node_info": {LabelsToMatch: []string{"node"}, LabelsToGet: []string{"kernel_version"}}}},
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
			},
			metricsToGet: []ksmstore.DDMetricsFam{
				{
					Name:        "kube_node_info",
					ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"node": "gke-charly-default-pool-6948dc89-fs46", "kernel_version": "4.14.138"}}},
				},
			},
			expected: []metricsExpected{
				{
					name: "kube_node_created",
					val:  1.588236673e+09,
					tags: []string{"node:gke-charly-default-pool-6948dc89-g54n", "uid:bec19172-8abf-11ea-8546-42010a80022c"},
				},
				{
					name: "kube_node_created",
					val:  1.588197278e+09,
					tags: []string{"node:gke-charly-default-pool-6948dc89-fs46", "uid:05e99c5f-8a64-11ea-8546-42010a80022c", "kernel_version:4.14.138"},
				},
			},
		},
	}
	for _, test := range tests {
		kubeStateMetricsSCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), test.config)
		mocked := mocksender.NewMockSender(kubeStateMetricsSCheck.ID())
		mocked.SetupAcceptAll()

		kubeStateMetricsSCheck.processMetrics(mocked, test.metricsToProcess, test.metricsToGet)
		t.Run(test.name, func(t *testing.T) {
			for _, expectMetric := range test.expected {
				mocked.AssertMetric(t, "Gauge", expectMetric.name, expectMetric.val, "", expectMetric.tags)
			}
		})
	}
}

func Test_isMatching(t *testing.T) {
	type args struct {
		config     *JoinsConfig
		destLabels map[string]string
		srcLabels  map[string]string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "match",
			args: args{
				config:     &JoinsConfig{LabelsToMatch: []string{"foo"}},
				destLabels: map[string]string{"foo": "bar", "baz": "bar"},
				srcLabels:  map[string]string{"foo": "bar"},
			},
			want: true,
		},
		{
			name: "no match",
			args: args{
				config:     &JoinsConfig{LabelsToMatch: []string{"foo"}},
				destLabels: map[string]string{"foo": "bar", "baz": "bar"},
				srcLabels:  map[string]string{"baz": "bar"},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMatching(tt.args.config, tt.args.srcLabels, tt.args.destLabels); got != tt.want {
				t.Errorf("isMatching() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKSMCheck_joinLabels(t *testing.T) {
	type args struct {
		labels       map[string]string
		metricsToGet []ksmstore.DDMetricsFam
	}
	tests := []struct {
		name       string
		labelJoins map[string]*JoinsConfig
		args       args
		wantTags   []string
	}{
		{
			name: "join labels, multiple match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label", "bar_label"},
					LabelsToGet:   []string{"baz_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
		{
			name: "join labels, multiple get",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label", "baz_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
		{
			name: "no label match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label"},
				},
			},
			args: args{
				labels: map[string]string{"baz_label": "baz_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"baz_label:baz_value"},
		},
		{
			name: "no metric name match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					LabelsToGet:   []string{"bar_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "bar",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value"},
		},
		{
			name: "join labels, multiple metric match",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label", "bar_label"},
					LabelsToGet:   []string{"baz_label"},
				},
				"bar": {
					LabelsToMatch: []string{"bar_label"},
					LabelsToGet:   []string{"baf_label"},
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
					{
						Name:        "bar",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"bar_label": "bar_value", "baf_label": "baf_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value", "baf_label:baf_value"},
		},
		{
			name: "join all labels",
			labelJoins: map[string]*JoinsConfig{
				"foo": {
					LabelsToMatch: []string{"foo_label"},
					GetAllLabels:  true,
				},
			},
			args: args{
				labels: map[string]string{"foo_label": "foo_value"},
				metricsToGet: []ksmstore.DDMetricsFam{
					{
						Name:        "foo",
						ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{"foo_label": "foo_value", "bar_label": "bar_value", "baz_label": "baz_value"}}},
					},
				},
			},
			wantTags: []string{"foo_label:foo_value", "foo_label:foo_value", "bar_label:bar_value", "baz_label:baz_value"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kubeStateMetricsSCheck := newKSMCheck(core.NewCheckBase(kubeStateMetricsCheckName), &KSMConfig{LabelJoins: tt.labelJoins})
			assert.ElementsMatch(t, tt.wantTags, kubeStateMetricsSCheck.joinLabels(tt.args.labels, tt.args.metricsToGet))
		})
	}
}
