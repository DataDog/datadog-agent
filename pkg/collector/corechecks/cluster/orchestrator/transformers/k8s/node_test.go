// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractNode(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	tests := map[string]struct {
		input    corev1.Node
		expected model.Node
	}{
		"full node": {
			input: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
					Name:              "node",
					CreationTimestamp: timestamp,
					Labels: map[string]string{
						"kubernetes.io/role": "data",
					},
					Annotations: map[string]string{
						"annotation": "bar",
					},
					ResourceVersion: "1234",
				},
				Spec: corev1.NodeSpec{
					PodCIDR:       "1234-5678-90",
					Unschedulable: true,
					Taints: []corev1.Taint{{
						Key:    "taint2NoTimeStamp",
						Value:  "val1",
						Effect: "effect1",
					}},
				},
				Status: corev1.NodeStatus{
					NodeInfo: corev1.NodeSystemInfo{
						KernelVersion:           "kernel1",
						OSImage:                 "os1",
						ContainerRuntimeVersion: "docker1",
						KubeletVersion:          "1.18",
						KubeProxyVersion:        "11",
						OperatingSystem:         "linux",
						Architecture:            "amd64",
					},
					Addresses: []corev1.NodeAddress{{
						Type:    "endpoint",
						Address: "1234567890",
					}},
					Images: []corev1.ContainerImage{{
						Names:     []string{"image1"},
						SizeBytes: 10,
					}},
					DaemonEndpoints: corev1.NodeDaemonEndpoints{KubeletEndpoint: corev1.DaemonEndpoint{Port: 11}},
					Capacity: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourcePods:   resource.MustParse("100"),
						corev1.ResourceCPU:    resource.MustParse("10"),
						corev1.ResourceMemory: resource.MustParse("10Gi"),
					},
					Allocatable: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourcePods:   resource.MustParse("50"),
						corev1.ResourceCPU:    resource.MustParse("5"),
						corev1.ResourceMemory: resource.MustParse("5G"),
					},
					Conditions: []corev1.NodeCondition{{
						Type:               corev1.NodeReady,
						Status:             corev1.ConditionTrue,
						LastHeartbeatTime:  timestamp,
						LastTransitionTime: timestamp,
						Reason:             "node to ready",
						Message:            "ready",
					}},
				},
			}, expected: model.Node{
				Metadata: &model.Metadata{
					Name:              "node",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"kubernetes.io/role:data"},
					Annotations:       []string{"annotation:bar"},
					ResourceVersion:   "1234",
				},
				Status: &model.NodeStatus{
					Capacity: map[string]int64{
						"pods":   100,
						"cpu":    10000,
						"memory": 10737418240, // 10 Gibibytes (Gi) are 10737418240 (base 1024)
					},
					Allocatable: map[string]int64{
						"pods":   50,
						"cpu":    5000,
						"memory": 5000000000, // 5 Gigabytes (G) are 5000000000 (base 1000)
					},
					NodeAddresses: map[string]string{"endpoint": "1234567890"},
					Status:        "Ready,SchedulingDisabled",
					Images: []*model.ContainerImage{{
						Names:     []string{"image1"},
						SizeBytes: 10,
					}},
					KernelVersion:           "kernel1",
					OsImage:                 "os1",
					ContainerRuntimeVersion: "docker1",
					KubeletVersion:          "1.18",
					KubeProxyVersion:        "11",
					OperatingSystem:         "linux",
					Architecture:            "amd64",
					Conditions: []*model.NodeCondition{{
						Type:               string(corev1.NodeReady),
						Status:             string(corev1.ConditionTrue),
						LastTransitionTime: timestamp.Unix(),
						Reason:             "node to ready",
						Message:            "ready",
					}},
				},
				PodCIDR:       "1234-5678-90",
				Unschedulable: true,
				Tags:          []string{"node_status:ready", "node_schedulable:false", "kube_node_role:data"},
				Taints: []*model.Taint{{
					Key:    "taint2NoTimeStamp",
					Value:  "val1",
					Effect: "effect1",
				}},
				Roles: []string{"data"},
			},
		},
		"empty node": {
			input: corev1.Node{},
			expected: model.Node{
				Metadata: &model.Metadata{},
				Status: &model.NodeStatus{
					Allocatable: map[string]int64{},
					Capacity:    map[string]int64{},
					Status:      "Unknown",
				},
				Tags: []string{"node_status:unknown", "node_schedulable:true"},
			}},
		"partial node with no memory": {
			input: corev1.Node{
				Status: corev1.NodeStatus{
					Capacity: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourcePods: resource.MustParse("100"),
						corev1.ResourceCPU:  resource.MustParse("10"),
					},
					Allocatable: map[corev1.ResourceName]resource.Quantity{
						corev1.ResourcePods: resource.MustParse("50"),
						corev1.ResourceCPU:  resource.MustParse("5"),
					}},
			}, expected: model.Node{
				Metadata: &model.Metadata{},
				Status: &model.NodeStatus{
					Status: "Unknown",
					Capacity: map[string]int64{
						"pods": 100,
						"cpu":  10000,
					},
					Allocatable: map[string]int64{
						"pods": 50,
						"cpu":  5000,
					},
				},
				Tags: []string{"node_status:unknown", "node_schedulable:true"},
			}},
		"node with only a condition": {
			input: corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "node",
					Namespace: "test",
				},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						}},
				},
				Spec: corev1.NodeSpec{},
			},
			expected: model.Node{
				Metadata: &model.Metadata{
					Name:      "node",
					Namespace: "test",
				},
				Status: &model.NodeStatus{
					Allocatable: map[string]int64{},
					Capacity:    map[string]int64{},
					Status:      "NotReady",
					Conditions: []*model.NodeCondition{{
						Type:   string(corev1.NodeReady),
						Status: string(corev1.ConditionFalse),
					}},
				},
				Tags: []string{"node_status:notready", "node_schedulable:true"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractNode(&tc.input))
		})
	}
}

func TestFindNodeRoles(t *testing.T) {
	tests := map[string]struct {
		input    map[string]string
		expected []string
	}{
		"kubernetes.io/role role": {
			input: map[string]string{
				"label":                    "foo",
				"node-role.kubernetes.io/": "master",
				"kubernetes.io/role":       "data",
			},
			expected: []string{"data"},
		},
		"node-role.kubernetes.io roles": {
			input: map[string]string{
				"node-role.kubernetes.io/compute":                              "",
				"node-role.kubernetes.io/ingress-haproxy-metrics-agent-public": "",
			},
			expected: []string{"compute", "ingress-haproxy-metrics-agent-public"},
		}, "node-role.kubernetes.io roles and kubernetes.io/role role": {
			input: map[string]string{
				"node-role.kubernetes.io/compute":                              "",
				"node-role.kubernetes.io/ingress-haproxy-metrics-agent-public": "",
				"kubernetes.io/role":                                           "master",
			},
			expected: []string{"compute", "ingress-haproxy-metrics-agent-public", "master"},
		},
		"incorrect label": {
			input: map[string]string{
				"node-role.kubernetes.io/": "master",
			},
			expected: []string{},
		},
		"no labels": {
			input:    map[string]string{},
			expected: []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, findNodeRoles(tc.input))
		})
	}
}

func TestComputeNodeStatus(t *testing.T) {
	tests := map[string]struct {
		input    corev1.Node
		expected string
	}{
		"Ready": {
			input: corev1.Node{
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				}},
			},
			expected: "Ready",
		},
		"Ready,SchedulingDisabled": {
			input: corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionTrue,
					},
				}},
			},
			expected: "Ready,SchedulingDisabled",
		},
		"Unknown": {
			input: corev1.Node{
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{}},
			},
			expected: "Unknown",
		},
		"Unknown,SchedulingDisabled": {
			input: corev1.Node{
				Spec:   corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{}},
			},
			expected: "Unknown,SchedulingDisabled",
		},
		"NotReady": {
			input: corev1.Node{
				Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{
					{
						Type:   corev1.NodeReady,
						Status: corev1.ConditionFalse,
					},
				}},
			},
			expected: "NotReady",
		}, "NotReady,SchedulingDisabled": {
			input: corev1.Node{
				Spec: corev1.NodeSpec{Unschedulable: true},
				Status: corev1.NodeStatus{
					Conditions: []corev1.NodeCondition{
						{
							Type:   corev1.NodeReady,
							Status: corev1.ConditionFalse,
						},
					}},
			},
			expected: "NotReady,SchedulingDisabled",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.expected, computeNodeStatus(&tc.input))
		})
	}
}

func TestConvertNodeStatusToTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Ready,SchedulingDisabled",
			input:    "Ready,SchedulingDisabled",
			expected: []string{"node_status:ready", "node_schedulable:false"},
		}, {
			name:     "Ready",
			input:    "Ready",
			expected: []string{"node_status:ready", "node_schedulable:true"},
		}, {
			name:     "Unknown",
			input:    "Unknown",
			expected: []string{"node_status:unknown", "node_schedulable:true"},
		}, {
			name:     "",
			input:    "",
			expected: []string{"node_schedulable:true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertNodeStatusToTags(tt.input))
		})
	}
}
