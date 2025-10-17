// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestExtractClusterNodeInfo(t *testing.T) {
	type testCase struct {
		name     string
		node     *corev1.Node
		expected *model.ClusterNodeInfo
	}
	for _, testCase := range []testCase{
		{
			name: "deprecated labels",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"beta.kubernetes.io/instance-type":         "c4a-standard-1",
						"failure-domain.beta.kubernetes.io/region": "us-east1",
					},
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						Architecture:            "amd64",
						ContainerRuntimeVersion: "containerd://1.7.25",
						KernelVersion:           "6.8.0-1020-gcp",
						KubeletVersion:          "v1.31.2",
						OperatingSystem:         "linux",
						OSImage:                 "Ubuntu 22.04.5 LTS",
					},
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("14Gi"),
					},
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("12"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			expected: &model.ClusterNodeInfo{
				Architecture:            "amd64",
				ContainerRuntimeVersion: "containerd://1.7.25",
				KernelVersion:           "6.8.0-1020-gcp",
				KubeletVersion:          "v1.31.2",
				InstanceType:            "c4a-standard-1",
				Name:                    "node-1",
				OperatingSystem:         "linux",
				OperatingSystemImage:    "Ubuntu 22.04.5 LTS",
				Region:                  "us-east1",
				ResourceAllocatable: map[string]string{
					"cpu":    "10",
					"memory": "14Gi",
				},
				ResourceCapacity: map[string]string{
					"cpu":    "12",
					"memory": "16Gi",
				},
			},
		},
		{
			name: "current labels",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"node.kubernetes.io/instance-type": "c4a-standard-1",
						"topology.kubernetes.io/region":    "us-east1",
					},
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						Architecture:            "amd64",
						ContainerRuntimeVersion: "containerd://1.7.25",
						KernelVersion:           "6.8.0-1020-gcp",
						KubeletVersion:          "v1.31.2",
						OperatingSystem:         "linux",
						OSImage:                 "Ubuntu 22.04.5 LTS",
					},
					Allocatable: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("14Gi"),
					},
					Capacity: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("12"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			expected: &model.ClusterNodeInfo{
				Architecture:            "amd64",
				ContainerRuntimeVersion: "containerd://1.7.25",
				KernelVersion:           "6.8.0-1020-gcp",
				KubeletVersion:          "v1.31.2",
				InstanceType:            "c4a-standard-1",
				Name:                    "node-1",
				OperatingSystem:         "linux",
				OperatingSystemImage:    "Ubuntu 22.04.5 LTS",
				Region:                  "us-east1",
				ResourceAllocatable: map[string]string{
					"cpu":    "10",
					"memory": "14Gi",
				},
				ResourceCapacity: map[string]string{
					"cpu":    "12",
					"memory": "16Gi",
				},
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			actual := ExtractClusterNodeInfo(testCase.node)
			assert.Equal(t, testCase.expected, actual)
		})
	}
}
