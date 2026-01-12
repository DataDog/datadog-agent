// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"

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

	restartPolicyAlways := v1.ContainerRestartPolicyAlways
	parseRequests := resource.MustParse("250M")
	parseLimits := resource.MustParse("550M")
	tests := map[string]struct {
		input             v1.Pod
		labelsAsTags      map[string]string
		annotationsAsTags map[string]string
		expected          model.Pod
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
							ImageID:      "container-1-image:latest",
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
							ImageID:      "container-2-image:latest",
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
							ImageID:      "container-3-image:latest",
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
						"app": "my-app",
					},
					Annotations: map[string]string{
						"annotation": "my-annotation",
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
			},
			labelsAsTags: map[string]string{
				"app": "application",
			},
			annotationsAsTags: map[string]string{
				"annotation": "annotation_key",
			},
			expected: model.Pod{
				Metadata: &model.Metadata{
					Name:              "pod",
					Namespace:         "namespace",
					Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
					CreationTimestamp: 1389744000,
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
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
						Image:        "container-1-image",
						ImageID:      "container-1-image:latest",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
						Image:        "container-2-image",
						ImageID:      "container-2-image:latest",
					},
					{
						State:        "Terminated",
						Message:      "CLB PLS (exit: -1)",
						RestartCount: 19,
						Name:         "container-3",
						ContainerID:  "docker://3",
						Image:        "container-3-image",
						ImageID:      "container-3-image:latest",
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
				Tags: []string{
					"kube_condition_ready:true",
					"kube_condition_podscheduled:true",
					"application:my-app",
					"annotation_key:my-annotation",
				},
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
						Image:       "container-1-image",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
						Image:        "container-2-image",
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
						Image:       "container-1-image",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "container-2",
						ContainerID:  "docker://2",
						Image:        "container-2-image",
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
						Image:        "container-2-image",
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
		"sidecar pod": {
			input: v1.Pod{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{
							Name:          "sidecar-container",
							RestartPolicy: &restartPolicyAlways,
							Resources: v1.ResourceRequirements{
								Limits:   map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseLimits},
								Requests: map[v1.ResourceName]resource.Quantity{v1.ResourceMemory: parseRequests},
							},
						},
					},
				},
			},
			expected: model.Pod{
				Metadata: &model.Metadata{},
				ResourceRequirements: []*model.ResourceRequirements{
					{
						Name:     "sidecar-container",
						Type:     model.ResourceRequirementsType_nativeSidecar,
						Limits:   map[string]int64{v1.ResourceMemory.String(): parseLimits.Value()},
						Requests: map[string]int64{v1.ResourceMemory.String(): parseRequests.Value()},
					},
				},
			},
		},
		"pod with unified tags": {
			input: v1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"tags.datadoghq.com/env":     "production",
						"tags.datadoghq.com/version": "7.70.0",
						"tags.datadoghq.com/service": "my-service",
					},
				},
			},
			expected: model.Pod{
				Metadata: &model.Metadata{
					Labels: []string{
						"tags.datadoghq.com/env:production",
						"tags.datadoghq.com/version:7.70.0",
						"tags.datadoghq.com/service:my-service",
					},
				},
				Tags: []string{
					"env:production",
					"version:7.70.0",
					"service:my-service",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			pctx := &processors.K8sProcessorContext{
				LabelsAsTags:      tc.labelsAsTags,
				AnnotationsAsTags: tc.annotationsAsTags,
			}
			actual := ExtractPod(pctx, &tc.input)
			sort.Strings(actual.Tags)
			sort.Strings(tc.expected.Tags)
			sort.Strings(actual.Metadata.Labels)
			sort.Strings(tc.expected.Metadata.Labels)
			assert.Equal(t, &tc.expected, actual)
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
func boolPointer(b bool) *bool {
	return &b
}

func TestComputeStatus(t *testing.T) {
	restartPolicyAlways := v1.ContainerRestartPolicyAlways
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
		}, {
			pod: &v1.Pod{
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						{Name: "restartable-init-1", RestartPolicy: &restartPolicyAlways},
					},
				},
				Status: v1.PodStatus{
					Phase: "Running",
					InitContainerStatuses: []v1.ContainerStatus{
						{
							Started: boolPointer(true),
							Name:    "restartable-init-1",
							Ready:   true,
							State: v1.ContainerState{
								Running: &v1.ContainerStateRunning{
									StartedAt: metav1.NewTime(time.Now()),
								},
							},
						},
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Started: boolPointer(true),
							Ready:   true,
							State: v1.ContainerState{
								Running: &v1.ContainerStateRunning{
									StartedAt: metav1.NewTime(time.Now()),
								},
							},
						},
					},
				},
			},
			status: "Running",
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

func TestConvertNodeSelector(t *testing.T) {
	tests := []struct {
		name  string
		input *v1.NodeSelector
		want  *model.NodeSelector
	}{
		{
			name:  "nil input",
			input: nil,
			want:  nil,
		},
		{
			name: "empty NodeSelector",
			input: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{},
			},
			want: &model.NodeSelector{NodeSelectorTerms: nil},
		},
		{
			name: "with MatchExpressions and MatchFields",
			input: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{Key: "key1", Operator: v1.NodeSelectorOpIn, Values: []string{"v1", "v2"}},
						},
						MatchFields: []v1.NodeSelectorRequirement{
							{Key: "field1", Operator: v1.NodeSelectorOpNotIn, Values: []string{"v3"}},
						},
					},
				},
			},
			want: &model.NodeSelector{
				NodeSelectorTerms: []*model.NodeSelectorTerm{
					{
						MatchExpressions: []*model.LabelSelectorRequirement{
							{Key: "key1", Operator: "In", Values: []string{"v1", "v2"}},
						},
						MatchFields: []*model.LabelSelectorRequirement{
							{Key: "field1", Operator: "NotIn", Values: []string{"v3"}},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertNodeSelector(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertNodeSelector() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConvertPreferredSchedulingTerm(t *testing.T) {
	tests := []struct {
		name  string
		input []v1.PreferredSchedulingTerm
		want  []*model.PreferredSchedulingTerm
	}{
		{
			name:  "empty terms",
			input: []v1.PreferredSchedulingTerm{},
			want:  nil,
		},
		{
			name: "single preferred scheduling term",
			input: []v1.PreferredSchedulingTerm{
				{
					Preference: v1.NodeSelectorTerm{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{Key: "k", Operator: v1.NodeSelectorOpExists},
						},
					},
					Weight: 10,
				},
			},
			want: []*model.PreferredSchedulingTerm{
				{
					Preference: &model.NodeSelectorTerm{
						MatchExpressions: []*model.LabelSelectorRequirement{
							{Key: "k", Operator: "Exists", Values: nil},
						},
						MatchFields: nil,
					},
					Weight: 10,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertPreferredSchedulingTerm(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertPreferredSchedulingTerm() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConvertNodeSelectorTerms(t *testing.T) {
	tests := []struct {
		name  string
		input []v1.NodeSelectorTerm
		want  []*model.NodeSelectorTerm
	}{
		{
			name:  "empty terms",
			input: []v1.NodeSelectorTerm{},
			want:  nil,
		},
		{
			name: "multiple NodeSelectorTerms",
			input: []v1.NodeSelectorTerm{
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{Key: "k1", Operator: v1.NodeSelectorOpIn, Values: []string{"v1"}},
					},
				},
				{
					MatchExpressions: []v1.NodeSelectorRequirement{
						{Key: "k2", Operator: v1.NodeSelectorOpNotIn, Values: []string{"v2"}},
					},
				},
			},
			want: []*model.NodeSelectorTerm{
				{
					MatchExpressions: []*model.LabelSelectorRequirement{
						{Key: "k1", Operator: "In", Values: []string{"v1"}},
					},
					MatchFields: nil,
				},
				{
					MatchExpressions: []*model.LabelSelectorRequirement{
						{Key: "k2", Operator: "NotIn", Values: []string{"v2"}},
					},
					MatchFields: nil,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertNodeSelectorTerms(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertNodeSelectorTerms() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConvertNodeSelectorTerm(t *testing.T) {
	tests := []struct {
		name  string
		input v1.NodeSelectorTerm
		want  *model.NodeSelectorTerm
	}{
		{
			name:  "empty term",
			input: v1.NodeSelectorTerm{},
			want: &model.NodeSelectorTerm{
				MatchExpressions: nil,
				MatchFields:      nil,
			},
		},
		{
			name: "with match expressions and fields",
			input: v1.NodeSelectorTerm{
				MatchExpressions: []v1.NodeSelectorRequirement{
					{Key: "k1", Operator: v1.NodeSelectorOpExists},
				},
				MatchFields: []v1.NodeSelectorRequirement{
					{Key: "f1", Operator: v1.NodeSelectorOpDoesNotExist},
				},
			},
			want: &model.NodeSelectorTerm{
				MatchExpressions: []*model.LabelSelectorRequirement{
					{Key: "k1", Operator: "Exists"},
				},
				MatchFields: []*model.LabelSelectorRequirement{
					{Key: "f1", Operator: "DoesNotExist"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertNodeSelectorTerm(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertNodeSelectorTerm() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestConvertNodeSelectorRequirements(t *testing.T) {
	tests := []struct {
		name  string
		input []v1.NodeSelectorRequirement
		want  []*model.LabelSelectorRequirement
	}{
		{
			name:  "no requirements",
			input: []v1.NodeSelectorRequirement{},
			want:  nil,
		},
		{
			name: "with multiple requirements",
			input: []v1.NodeSelectorRequirement{
				{Key: "k1", Operator: v1.NodeSelectorOpIn, Values: []string{"v1", "v2"}},
				{Key: "k2", Operator: v1.NodeSelectorOpNotIn, Values: []string{"v3"}},
			},
			want: []*model.LabelSelectorRequirement{
				{Key: "k1", Operator: "In", Values: []string{"v1", "v2"}},
				{Key: "k2", Operator: "NotIn", Values: []string{"v3"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := convertNodeSelectorRequirements(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("convertNodeSelectorRequirements() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestExtractPodResourceRequirementsSidecar(t *testing.T) {
	restartAlways := v1.ContainerRestartPolicy("Always")
	tests := map[string]struct {
		input    []v1.Container
		expected []*model.ResourceRequirements
	}{
		"sidecar pod": {
			input: []v1.Container{
				{
					Name:          "sidecar",
					RestartPolicy: &restartAlways,
					Resources: v1.ResourceRequirements{
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("200Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("50m"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
				{
					Name: "main",
					Resources: v1.ResourceRequirements{
						Limits: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("100m"),
							v1.ResourceMemory: resource.MustParse("200Mi"),
						},
						Requests: map[v1.ResourceName]resource.Quantity{
							v1.ResourceCPU:    resource.MustParse("50m"),
							v1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			expected: []*model.ResourceRequirements{
				{
					Name: "sidecar",
					Type: model.ResourceRequirementsType_nativeSidecar,
					Limits: map[string]int64{
						v1.ResourceCPU.String():    100,
						v1.ResourceMemory.String(): 209715200,
					},
					Requests: map[string]int64{
						v1.ResourceCPU.String():    50,
						v1.ResourceMemory.String(): 104857600,
					},
				},
				{
					Name: "main",
					Type: model.ResourceRequirementsType_initContainer,
					Limits: map[string]int64{
						v1.ResourceCPU.String():    100,
						v1.ResourceMemory.String(): 209715200,
					},
					Requests: map[string]int64{
						v1.ResourceCPU.String():    50,
						v1.ResourceMemory.String(): 104857600,
					},
				},
			},
		},
		"sidecar pod with no resources": {
			input: []v1.Container{
				{
					Name:          "sidecar",
					RestartPolicy: &restartAlways,
				},
			},
			expected: []*model.ResourceRequirements{
				{
					Name:     "sidecar",
					Type:     model.ResourceRequirementsType_nativeSidecar,
					Limits:   map[string]int64{},
					Requests: map[string]int64{},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			actual := extractPodResourceRequirements(nil, tc.input)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
