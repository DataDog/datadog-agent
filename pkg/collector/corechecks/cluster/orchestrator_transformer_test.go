// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package cluster

import (
	"fmt"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/process"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestExtractDeployment(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)
	testIntorStr := intstr.FromString("1%")
	tests := map[string]struct {
		input    v1.Deployment
		expected model.Deployment
	}{
		"full deploy": {
			input: v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
					Name:              "deploy",
					Namespace:         "namespace",
					CreationTimestamp: timestamp,
					Labels: map[string]string{
						"label": "foo",
					},
					Annotations: map[string]string{
						"annotation": "bar",
					},
					ResourceVersion: "1234",
				},
				Spec: v1.DeploymentSpec{
					MinReadySeconds:         600,
					ProgressDeadlineSeconds: &testInt32,
					Replicas:                &testInt32,
					RevisionHistoryLimit:    &testInt32,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-deploy",
						},
					},
					Strategy: v1.DeploymentStrategy{
						Type: v1.DeploymentStrategyType("RollingUpdate"),
						RollingUpdate: &v1.RollingUpdateDeployment{
							MaxSurge:       &testIntorStr,
							MaxUnavailable: &testIntorStr,
						},
					},
				},
				Status: v1.DeploymentStatus{
					AvailableReplicas:  2,
					ObservedGeneration: 3,
					ReadyReplicas:      2,
					Replicas:           2,
					UpdatedReplicas:    2,
					Conditions: []v1.DeploymentCondition{
						{
							Type:    v1.DeploymentAvailable,
							Status:  corev1.ConditionFalse,
							Reason:  "MinimumReplicasAvailable",
							Message: "Deployment has minimum availability.",
						},
						{
							Type:    v1.DeploymentProgressing,
							Status:  corev1.ConditionFalse,
							Reason:  "NewReplicaSetAvailable",
							Message: `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
						},
					},
				},
			}, expected: model.Deployment{
				Metadata: &model.Metadata{
					Name:              "deploy",
					Namespace:         "namespace",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"label:foo"},
					Annotations:       []string{"annotation:bar"},
					ResourceVersion:   "1234",
				},
				ReplicasDesired:    2,
				DeploymentStrategy: "RollingUpdate",
				MaxUnavailable:     "1%",
				MaxSurge:           "1%",
				Paused:             false,
				Selectors: []*model.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "In",
						Values:   []string{"test-deploy"},
					},
				},
				Replicas:            2,
				UpdatedReplicas:     2,
				ReadyReplicas:       2,
				AvailableReplicas:   2,
				UnavailableReplicas: 0,
				ConditionMessage:    `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
			},
		},
		"empty deploy": {input: v1.Deployment{}, expected: model.Deployment{Metadata: &model.Metadata{}, ReplicasDesired: 1}},
		"partial deploy": {
			input: v1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy",
					Namespace: "namespace",
				},
				Spec: v1.DeploymentSpec{
					MinReadySeconds: 600,
					Strategy: v1.DeploymentStrategy{
						Type: v1.DeploymentStrategyType("RollingUpdate"),
					},
				},
			}, expected: model.Deployment{
				ReplicasDesired: 1,
				Metadata: &model.Metadata{
					Name:      "deploy",
					Namespace: "namespace",
				},
				DeploymentStrategy: "RollingUpdate",
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractDeployment(&tc.input))
		})
	}
}

func TestExtractDeploymentConditionMessage(t *testing.T) {
	for nb, tc := range []struct {
		conditions []v1.DeploymentCondition
		message    string
	}{
		{
			conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentReplicaFailure,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				},
			},
			message: "foo",
		}, {
			conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				}, {
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Message: "bar",
				},
			},
			message: "bar",
		}, {
			conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Message: "foo",
				}, {
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionTrue,
					Message: "bar",
				},
			},
			message: "foo",
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.message, extractDeploymentConditionMessage(tc.conditions))
		})
	}
}

func TestExtractReplicaSet(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)
	tests := map[string]struct {
		input    v1.ReplicaSet
		expected model.ReplicaSet
	}{
		"full rs": {
			input: v1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
					Name:              "replicaset",
					Namespace:         "namespace",
					CreationTimestamp: timestamp,
					Labels: map[string]string{
						"label": "foo",
					},
					Annotations: map[string]string{
						"annotation": "bar",
					},
					ResourceVersion: "1234",
				},
				Spec: v1.ReplicaSetSpec{
					Replicas: &testInt32,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "test-deploy",
						},
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "cluster",
								Operator: "NotIn",
								Values:   []string{"staging", "prod"},
							},
						},
					},
				},
				Status: v1.ReplicaSetStatus{
					Replicas:             2,
					FullyLabeledReplicas: 2,
					ReadyReplicas:        1,
					AvailableReplicas:    1,
				},
			}, expected: model.ReplicaSet{
				Metadata: &model.Metadata{
					Name:              "replicaset",
					Namespace:         "namespace",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"label:foo"},
					Annotations:       []string{"annotation:bar"},
					ResourceVersion:   "1234",
				},
				Selectors: []*model.LabelSelectorRequirement{
					{
						Key:      "app",
						Operator: "In",
						Values:   []string{"test-deploy"},
					},
					{
						Key:      "cluster",
						Operator: "NotIn",
						Values:   []string{"staging", "prod"},
					},
				},
				ReplicasDesired:      2,
				Replicas:             2,
				FullyLabeledReplicas: 2,
				ReadyReplicas:        1,
				AvailableReplicas:    1,
			},
		},
		"empty rs": {input: v1.ReplicaSet{}, expected: model.ReplicaSet{Metadata: &model.Metadata{}, ReplicasDesired: 1}},
		"partial rs": {
			input: v1.ReplicaSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deploy",
					Namespace: "namespace",
				},
				Status: v1.ReplicaSetStatus{
					ReadyReplicas:     1,
					AvailableReplicas: 0,
				},
			}, expected: model.ReplicaSet{
				Metadata: &model.Metadata{
					Name:      "deploy",
					Namespace: "namespace",
				},
				ReplicasDesired:   1,
				ReadyReplicas:     1,
				AvailableReplicas: 0,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractReplicaSet(&tc.input))
		})
	}
}

func TestExtractService(t *testing.T) {
	tests := map[string]struct {
		input    corev1.Service
		expected model.Service
	}{
		"ClusterIP": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "002631fc-4c10-11ea-8f60-02ad5c77d02b",
					Labels: map[string]string{
						"app": "app-1",
					},
					Name:            "cluster-ip-service",
					Namespace:       "project",
					ResourceVersion: "1234",
				},
				Spec: corev1.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []corev1.ServicePort{
						{
							Name:       "port-1",
							Port:       1,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(1),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-1"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeClusterIP,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-1"},
					Name:              "cluster-ip-service",
					Namespace:         "project",
					Uid:               "002631fc-4c10-11ea-8f60-02ad5c77d02b",
					ResourceVersion:   "1234",
				},
				Spec: &model.ServiceSpec{
					ClusterIP: "10.0.0.1",
					Ports: []*model.ServicePort{
						{
							Name:       "port-1",
							Port:       1,
							Protocol:   "TCP",
							TargetPort: "1",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-1"},
						},
					},
					SessionAffinity: "None",
					Type:            "ClusterIP",
				},
				Status: &model.ServiceStatus{},
			},
		},
		"ExternalName": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "a4e8d7ef-224d-11ea-bfe5-02da21d58a25",
					Labels: map[string]string{
						"app": "app-2",
					},
					Name:      "external-name-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					ExternalName: "my.service.example.com",
					Ports: []corev1.ServicePort{
						{
							Name:       "port-2",
							Port:       2,
							Protocol:   corev1.ProtocolTCP,
							TargetPort: intstr.FromInt(2),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-2"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeExternalName,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-2"},
					Name:              "external-name-service",
					Namespace:         "project",
					Uid:               "a4e8d7ef-224d-11ea-bfe5-02da21d58a25",
				},
				Spec: &model.ServiceSpec{
					ExternalName: "my.service.example.com",
					Ports: []*model.ServicePort{
						{
							Name:       "port-2",
							Port:       2,
							Protocol:   "TCP",
							TargetPort: "2",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-2"},
						},
					},
					SessionAffinity: "None",
					Type:            "ExternalName",
				},
				Status: &model.ServiceStatus{},
			},
		},
		"LoadBalancer": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "77b66dc1-6d14-11ea-a6ec-12daacdf7c55",
					Labels: map[string]string{
						"app": "app-3",
					},
					Name:      "loadbalancer-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "port-3",
							Port:       3,
							Protocol:   "TCP",
							TargetPort: intstr.FromInt(3),
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-3"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeLoadBalancer,
				},
				Status: corev1.ServiceStatus{
					LoadBalancer: corev1.LoadBalancerStatus{
						Ingress: []corev1.LoadBalancerIngress{
							{
								IP: "192.0.2.127",
							},
						},
					},
				},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-3"},
					Name:              "loadbalancer-service",
					Namespace:         "project",
					Uid:               "77b66dc1-6d14-11ea-a6ec-12daacdf7c55",
				},
				Spec: &model.ServiceSpec{
					Ports: []*model.ServicePort{
						{
							Name:       "port-3",
							Port:       3,
							Protocol:   "TCP",
							TargetPort: "3",
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-3"},
						},
					},
					SessionAffinity: "None",
					Type:            "LoadBalancer",
				},
				Status: &model.ServiceStatus{
					LoadBalancerIngress: []string{"192.0.2.127"},
				},
			},
		},
		"NodePort": {
			input: corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"prefix/name": "annotation-value",
					},
					CreationTimestamp: metav1.NewTime(time.Date(2020, time.July, 16, 0, 0, 0, 0, time.UTC)),
					UID:               "dfd0172f-1124-11ea-9888-02e48d9f4c6f",
					Labels: map[string]string{
						"app": "app-4",
					},
					Name:      "nodeport-service",
					Namespace: "project",
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Name:       "port-4",
							Port:       4,
							Protocol:   "TCP",
							TargetPort: intstr.FromInt(4),
							NodePort:   30004,
						},
					},
					PublishNotReadyAddresses: false,
					Selector:                 map[string]string{"app": "app-4"},
					SessionAffinity:          corev1.ServiceAffinityNone,
					Type:                     corev1.ServiceTypeNodePort,
				},
				Status: corev1.ServiceStatus{},
			},
			expected: model.Service{
				Metadata: &model.Metadata{
					Annotations:       []string{"prefix/name:annotation-value"},
					CreationTimestamp: 1594857600,
					Labels:            []string{"app:app-4"},
					Name:              "nodeport-service",
					Namespace:         "project",
					Uid:               "dfd0172f-1124-11ea-9888-02e48d9f4c6f",
				},
				Spec: &model.ServiceSpec{
					Ports: []*model.ServicePort{
						{
							Name:       "port-4",
							Port:       4,
							Protocol:   "TCP",
							TargetPort: "4",
							NodePort:   30004,
						},
					},
					PublishNotReadyAddresses: false,
					Selectors: []*model.LabelSelectorRequirement{
						{
							Key:      "app",
							Operator: "In",
							Values:   []string{"app-4"},
						},
					},
					SessionAffinity: "None",
					Type:            "NodePort",
				},
				Status: &model.ServiceStatus{},
			},
		},
	}
	for _, test := range tests {
		assert.Equal(t, &test.expected, extractService(&test.input))
	}
}

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
				}}},
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
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractNode(&tc.input))
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
