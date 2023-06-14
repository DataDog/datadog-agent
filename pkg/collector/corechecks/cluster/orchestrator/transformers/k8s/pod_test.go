// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func getTemplateWithResourceRequirements() v1.PodTemplateSpec {
	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	return v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name: "container-1",
					Resources: v1.ResourceRequirements{
						Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseLimits},
						Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseRequests},
					},
				},
			},
			InitContainers: []v1.Container{
				{
					Name: "container-2",
					Resources: v1.ResourceRequirements{
						Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseLimits},
						Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseRequests},
					},
				},
			},
		},
	}
}

func getExpectedModelResourceRequirements() []*model.ResourceRequirements {
	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	return []*model.ResourceRequirements{
		{
			Limits:   map[string]int64{v1.ResourceMemory.String(): parseLimits.Value()},
			Requests: map[string]int64{v1.ResourceMemory.String(): parseRequests.Value()},
			Name:     "container-1",
			Type:     model.ResourceRequirementsType_container,
		}, {
			Limits:   map[string]int64{v1.ResourceMemory.String(): parseLimits.Value()},
			Requests: map[string]int64{v1.ResourceMemory.String(): parseRequests.Value()},
			Name:     "container-2",
			Type:     model.ResourceRequirementsType_initContainer,
		},
	}
}

func TestExtractPod(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000

	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	tests := map[string]struct {
		input    v1.Pod
		expected model.Pod
	}{
		"full pod with containers without resourceRequirements": {
			input: v1.Pod{
				Status: v1.PodStatus{
					Phase:             v1.PodRunning,
					StartTime:         &timestamp,
					NominatedNodeName: "nominated",
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
						{
							Type:               v1.PodScheduled,
							Status:             v1.ConditionTrue,
							LastTransitionTime: timestamp,
						},
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:         "container-1",
							Image:        "container-1-image",
							ContainerID:  "docker://1",
							RestartCount: 13,
							State: v1.ContainerState{
								Running: &v1.ContainerStateRunning{
									StartedAt: timestamp,
								},
							},
						},
						{
							Name:         "container-2",
							Image:        "container-2-image",
							ContainerID:  "docker://2",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
						{
							Name:         "container-3",
							Image:        "container-3-image",
							ContainerID:  "docker://3",
							RestartCount: 19,
							State: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{
									ExitCode: -1,
									Signal:   9,
									Reason:   "CLB",
									Message:  "PLS",
								},
							},
						},
					},
					QOSClass: v1.PodQOSGuaranteed,
				},
				ObjectMeta: metav1.ObjectMeta{
					UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
					Name:              "pod",
					Namespace:         "namespace",
					CreationTimestamp: timestamp,
					Labels: map[string]string{
						"label": "foo",
					},
					Annotations: map[string]string{
						"annotation": "bar",
					},
					OwnerReferences: []metav1.OwnerReference{
						{
							Name: "test-controller",
							Kind: "replicaset",
							UID:  types.UID("1234567890"),
						},
					},
					ResourceVersion: "1234",
				},
				Spec: v1.PodSpec{
					NodeName: "node",
					Containers: []v1.Container{
						{Name: "container-1"},
						{Name: "container-2"},
						{Name: "container-3"},
					},
					PriorityClassName: "high-priority",
				},
			}, expected: model.Pod{
				Metadata: &model.Metadata{
					Name:              "pod",
					Namespace:         "namespace",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"label:foo"},
					Annotations:       []string{"annotation:bar"},
					OwnerReferences: []*model.OwnerReference{
						{
							Name: "test-controller",
							Kind: "replicaset",
							Uid:  "1234567890",
						},
					},
					ResourceVersion: "1234",
				},
				Phase:             "Running",
				NominatedNodeName: "nominated",
				NodeName:          "node",
				RestartCount:      42,
				ScheduledTime:     timestamp.Unix(),
				StartTime:         timestamp.Unix(),
				Status:            "chillin",
				QOSClass:          "Guaranteed",
				PriorityClass:     "high-priority",
				ContainerStatuses: []*model.ContainerStatus{
					{
						State:        "Running",
						RestartCount: 13,
						Name:         "container-1",
						ContainerID:  "docker://1",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
					},
					{
						State:        "Terminated",
						Message:      "CLB PLS (exit: -1)",
						RestartCount: 19,
						Name:         "container-3",
						ContainerID:  "docker://3",
					},
				},
				Conditions: []*model.PodCondition{
					{
						Type:   "Ready",
						Status: "True",
					},
					{
						Type:               "PodScheduled",
						Status:             "True",
						LastTransitionTime: timestamp.Unix(),
					},
				},
				Tags: []string{"kube_condition_ready:true", "kube_condition_podscheduled:true"},
				ResourceRequirements: []*model.ResourceRequirements{
					{
						Limits:   map[string]int64{},
						Requests: map[string]int64{},
						Name:     "container-1",
						Type:     model.ResourceRequirementsType_container,
					},
					{
						Limits:   map[string]int64{},
						Requests: map[string]int64{},
						Name:     "container-2",
						Type:     model.ResourceRequirementsType_container,
					},
					{
						Limits:   map[string]int64{},
						Requests: map[string]int64{},
						Name:     "container-3",
						Type:     model.ResourceRequirementsType_container,
					},
				},
			},
		},
		"empty pod": {input: v1.Pod{}, expected: model.Pod{Metadata: &model.Metadata{}}},
		"partial pod": {
			input: v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:        "container-1",
							Image:       "container-1-image",
							ContainerID: "docker://1",
						},
						{
							Name:         "container-2",
							Image:        "container-2-image",
							ContainerID:  "docker://2",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
						{
							Name: "container-3",
						},
					},
					QOSClass: v1.PodQOSBurstable,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							UID: types.UID("1234567890"),
						},
					},
				},
			}, expected: model.Pod{
				Metadata: &model.Metadata{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []*model.OwnerReference{
						{
							Uid: "1234567890",
						},
					},
				},
				RestartCount: 10,
				Status:       "chillin",
				QOSClass:     "Burstable",
				ContainerStatuses: []*model.ContainerStatus{
					{
						Name:        "container-1",
						ContainerID: "docker://1",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
					},
					{
						Name: "container-3",
					},
				},
				Conditions: []*model.PodCondition{
					{
						Type:   "Ready",
						Status: "True",
					},
				},
				Tags: []string{"kube_condition_ready:true"},
			},
		},
		"partial pod with init container": {
			input: v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
					},
					InitContainerStatuses: []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{
									Reason:   "Completed",
									ExitCode: 0,
								},
							},
							RestartCount: 2,
						},
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:        "container-1",
							Image:       "container-1-image",
							ContainerID: "docker://1",
						},
						{
							Name:         "container-2",
							Image:        "container-2-image",
							ContainerID:  "docker://2",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
						{
							Name: "container-3",
						},
					},
					QOSClass: v1.PodQOSBestEffort,
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							UID: types.UID("1234567890"),
						},
					},
				},
			}, expected: model.Pod{
				Metadata: &model.Metadata{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []*model.OwnerReference{
						{
							Uid: "1234567890",
						},
					},
				},
				RestartCount: 12,
				Status:       "chillin",
				InitContainerStatuses: []*model.ContainerStatus{
					{
						Message:      "Completed  (exit: 0)",
						State:        "Terminated",
						Ready:        false,
						RestartCount: 2,
					},
				},
				ContainerStatuses: []*model.ContainerStatus{
					{
						Name:        "container-1",
						ContainerID: "docker://1",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
					},
					{
						Name: "container-3",
					},
				},
				Conditions: []*model.PodCondition{
					{
						Type:   "Ready",
						Status: "True",
					},
				},
				Tags:     []string{"kube_condition_ready:true"},
				QOSClass: "BestEffort",
			},
		},
		"partial pod with resourceRequirements": {
			input: v1.Pod{
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name: "aContainer",
							Resources: v1.ResourceRequirements{
								Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseLimits},
								Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseRequests},
							},
						},
					},
				},
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:   v1.PodReady,
							Status: v1.ConditionTrue,
						},
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:         "container-2",
							Image:        "container-2-image",
							ContainerID:  "docker://2",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
					},
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							UID: types.UID("1234567890"),
						},
					},
				},
			}, expected: model.Pod{
				Metadata: &model.Metadata{
					Name:      "pod",
					Namespace: "namespace",
					OwnerReferences: []*model.OwnerReference{
						{
							Uid: "1234567890",
						},
					},
				},
				RestartCount: 10,
				Status:       "chillin",
				ContainerStatuses: []*model.ContainerStatus{
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
					},
				},
				ResourceRequirements: []*model.ResourceRequirements{
					{
						Limits:   map[string]int64{v1.ResourceMemory.String(): parseLimits.Value()},
						Requests: map[string]int64{v1.ResourceMemory.String(): parseRequests.Value()},
						Name:     "aContainer",
						Type:     model.ResourceRequirementsType_container,
					},
				},
				Conditions: []*model.PodCondition{
					{
						Type:   "Ready",
						Status: "True",
					},
				},
				Tags: []string{"kube_condition_ready:true"},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractPod(&tc.input))
		})
	}
}

func TestConvertResourceRequirements(t *testing.T) {
	tests := map[string]struct {
		input    v1.Container
		expected *model.ResourceRequirements
	}{
		"no ResourceRequirements set": {
			input: v1.Container{
				Name: "test",
			},
			expected: &model.ResourceRequirements{
				Limits:   map[string]int64{},
				Requests: map[string]int64{},
				Name:     "test",
				Type:     model.ResourceRequirementsType_container,
			},
		},
		"0 ResourceRequirement explicitly set": {
			input: v1.Container{
				Name: "test",
				Resources: v1.ResourceRequirements{
					// 1024 = 1Ki
					Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: resource.MustParse("0"), v1.ResourceCPU: resource.MustParse("0.5")},
					Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: resource.MustParse("0")}, // explicitly set the "0" value, that means if not set, it will not be in the map
				},
			},
			expected: &model.ResourceRequirements{
				Limits:   map[string]int64{v1.ResourceCPU.String(): 500, v1.ResourceMemory.String(): 0},
				Requests: map[string]int64{v1.ResourceMemory.String(): 0},
				Name:     "test",
				Type:     model.ResourceRequirementsType_container,
			},
		},
		"only mem set": {
			input: v1.Container{
				Name: "test",
				Resources: v1.ResourceRequirements{
					// 1024 = 1Ki
					Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: resource.MustParse("550Mi")},
					Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: resource.MustParse("250Mi")},
				},
			},
			expected: &model.ResourceRequirements{
				Limits:   map[string]int64{v1.ResourceMemory.String(): 576716800},
				Requests: map[string]int64{v1.ResourceMemory.String(): 262144000},
				Name:     "test",
				Type:     model.ResourceRequirementsType_container,
			},
		},
		"only cpu set": {
			input: v1.Container{
				Name: "test",
				Resources: v1.ResourceRequirements{
					Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceCPU: resource.MustParse("1")},
					Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceCPU: resource.MustParse("0.5")},
				},
			},
			expected: &model.ResourceRequirements{
				Limits:   map[string]int64{v1.ResourceCPU.String(): 1000},
				Requests: map[string]int64{v1.ResourceCPU.String(): 500},
				Name:     "test",
				Type:     model.ResourceRequirementsType_container,
			},
		},
		"only cpu request set": {
			input: v1.Container{
				Name: "test",
				Resources: v1.ResourceRequirements{
					Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceCPU: resource.MustParse("0.5")},
				},
			},
			expected: &model.ResourceRequirements{
				Requests: map[string]int64{v1.ResourceCPU.String(): 500},
				Limits:   map[string]int64{},
				Name:     "test",
				Type:     model.ResourceRequirementsType_container,
			},
		},
		"mem and cpu set": {
			input: v1.Container{
				Name: "test",
				Resources: v1.ResourceRequirements{
					Limits: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("1"),
						v1.ResourceMemory: resource.MustParse("550Mi"),
					},
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU:    resource.MustParse("0.5"),
						v1.ResourceMemory: resource.MustParse("250Mi"),
					},
				},
			},
			expected: &model.ResourceRequirements{
				Limits: map[string]int64{
					v1.ResourceCPU.String():    1000,
					v1.ResourceMemory.String(): 576716800,
				},
				Requests: map[string]int64{
					v1.ResourceCPU.String():    500,
					v1.ResourceMemory.String(): 262144000,
				},
				Name: "test",
				Type: model.ResourceRequirementsType_container,
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := convertResourceRequirements(tc.input.Resources, tc.input.Name, model.ResourceRequirementsType_container)
			assert.Equal(t, tc.expected, actual)
		})
	}
}

func TestComputeStatus(t *testing.T) {
	for nb, tc := range []struct {
		pod    *v1.Pod
		status string
	}{
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: "Running",
				},
			},
			status: "Running",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: "Succeeded",
					ContainerStatuses: []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{
									Reason: "Completed",
								},
							},
						},
					},
				},
			},
			status: "Completed",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: "Failed",
					InitContainerStatuses: []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{
									Reason:   "Error",
									ExitCode: 52,
								},
							},
						},
					},
				},
			},
			status: "Init:Error",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Phase: "Running",
					ContainerStatuses: []v1.ContainerStatus{
						{
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason: "CrashLoopBackoff",
								},
							},
						},
					},
				},
			},
			status: "CrashLoopBackoff",
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.status, computeStatus(tc.pod))
		})
	}
}

func TestExtractPodConditions(t *testing.T) {
	p := &v1.Pod{
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodInitialized,
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Date(2023, 01, 06, 11, 24, 46, 0, time.UTC)),
				},
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: metav1.NewTime(time.Date(2023, 01, 06, 11, 24, 44, 0, time.UTC)),
				},
				{
					Type:               v1.ContainersReady,
					Status:             v1.ConditionFalse,
					Message:            "containers with unready status: [trace-query]",
					Reason:             "ContainersNotReady",
					LastTransitionTime: metav1.NewTime(time.Date(2023, 02, 07, 13, 06, 38, 0, time.UTC)),
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionUnknown,
					Message:            "Unknown",
					Reason:             "Unknown",
					LastProbeTime:      metav1.NewTime(time.Date(2023, 02, 07, 13, 06, 52, 0, time.UTC)),
					LastTransitionTime: metav1.NewTime(time.Date(2023, 02, 07, 13, 06, 40, 0, time.UTC)),
				},
			},
		},
	}

	expectedConditions := []*model.PodCondition{
		{
			Type:               "Initialized",
			Status:             "True",
			LastTransitionTime: time.Date(2023, 01, 06, 11, 24, 46, 0, time.UTC).Unix(),
		},
		{
			Type:               "PodScheduled",
			Status:             "True",
			LastTransitionTime: time.Date(2023, 01, 06, 11, 24, 44, 0, time.UTC).Unix(),
		},
		{
			Type:               "ContainersReady",
			Status:             "False",
			Message:            "containers with unready status: [trace-query]",
			Reason:             "ContainersNotReady",
			LastTransitionTime: time.Date(2023, 02, 07, 13, 06, 38, 0, time.UTC).Unix(),
		},
		{
			Type:               "Ready",
			Status:             "Unknown",
			Reason:             "Unknown",
			Message:            "Unknown",
			LastProbeTime:      time.Date(2023, 02, 07, 13, 06, 52, 0, time.UTC).Unix(),
			LastTransitionTime: time.Date(2023, 02, 07, 13, 06, 40, 0, time.UTC).Unix(),
		},
	}
	expectedTags := []string{
		"kube_condition_initialized:true",
		"kube_condition_podscheduled:true",
		"kube_condition_containersready:false",
		"kube_condition_ready:unknown",
	}

	conditions, conditionTags := extractPodConditions(p)
	assert.Equal(t, expectedConditions, conditions)
	assert.Equal(t, expectedTags, conditionTags)
}

func TestFillPodResourceVersion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input *model.Pod
	}{
		{
			name: "ordered",
			input: &model.Pod{
				Metadata: &model.Metadata{
					Name:        "pod",
					Namespace:   "default",
					Labels:      []string{"app:my-app", "chart_name:webscale-app", "team:one-team"},
					Annotations: []string{"kubernetes.io/config.seen:2021-03-01T03:22:49.057675874Z", "kubernetes.io/config.source:api"},
				},
				RestartCount: 5,
				Status:       "running",
				Tags:         []string{"kube_namespace:default", "kube_service:my-app", "pod_name:name"},
			},
		},
		{
			name: "unordered",
			input: &model.Pod{
				Metadata: &model.Metadata{
					Name:        "pod",
					Namespace:   "default",
					Labels:      []string{"chart_name:webscale-app", "team:one-team", "app:my-app"},
					Annotations: []string{"kubernetes.io/config.source:api", "kubernetes.io/config.seen:2021-03-01T03:22:49.057675874Z"},
				},
				RestartCount: 5,
				Status:       "running",
				Tags:         []string{"pod_name:name", "kube_service:my-app", "kube_namespace:default"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := FillK8sPodResourceVersion(tc.input)
			assert.NoError(t, err)
			assert.Equal(t, "4669378970017265057", tc.input.Metadata.ResourceVersion)
		})
	}
}

func TestGetConditionMessage(t *testing.T) {
	for nb, tc := range []struct {
		pod     *v1.Pod
		message string
	}{
		{
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    v1.PodScheduled,
							Status:  v1.ConditionFalse,
							Message: "foo",
						},
					},
				},
			},
			message: "foo",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    v1.PodScheduled,
							Status:  v1.ConditionFalse,
							Message: "foo",
						}, {
							Type:    v1.PodInitialized,
							Status:  v1.ConditionFalse,
							Message: "bar",
						},
					},
				},
			},
			message: "foo",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{
						{
							Type:    v1.PodScheduled,
							Status:  v1.ConditionTrue,
							Message: "foo",
						}, {
							Type:    v1.PodInitialized,
							Status:  v1.ConditionFalse,
							Message: "bar",
						},
					},
				},
			},
			message: "bar",
		}, {
			pod: &v1.Pod{
				Status: v1.PodStatus{
					Conditions: []v1.PodCondition{},
					Message:    "Pod The node was low on resource: [DiskPressure]",
				},
			},
			message: "Pod The node was low on resource: [DiskPressure]",
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.message, getConditionMessage(tc.pod))
		})
	}
}

func TestGenerateUniqueStaticPodHash(t *testing.T) {
	hostName := "agent-dev-tim"
	podName := "nginxP"
	namespace := "kube-system"
	clusterName := "something"

	uniqueHash := GenerateUniqueK8sStaticPodHash(hostName, podName, namespace, clusterName)
	uniqueHashAgain := GenerateUniqueK8sStaticPodHash(hostName, podName, namespace, clusterName)

	assert.Equal(t, uniqueHash, uniqueHashAgain)
}

func TestGenerateUniqueStaticPodHashHardCoded(t *testing.T) {
	hostName := "agent-dev-tim"
	podName := "nginxP"
	namespace := "kube-system"
	clusterName := "something"

	uniqueHash := GenerateUniqueK8sStaticPodHash(hostName, podName, namespace, clusterName)
	expectedHash := "b9d79449507ade06"

	assert.Equal(t, uniqueHash, expectedHash)
}

func TestMapToTags(t *testing.T) {
	labels := map[string]string{}
	labels["foo"] = "bar"
	labels["node-role.kubernetes.io/nodeless"] = ""

	tags := mapToTags(labels)

	assert.ElementsMatch(t, []string{"foo:bar", "node-role.kubernetes.io/nodeless"}, tags)
	assert.Len(t, tags, 2)
}
