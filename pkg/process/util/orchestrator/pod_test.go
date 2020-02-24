// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build orchestrator

package orchestrator

import (
	"fmt"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/config"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestExtractPodMessage(t *testing.T) {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	tests := map[string]struct {
		input    v1.Pod
		expected model.Pod
	}{
		"full pod": {
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
					},
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:         "fooName",
							Image:        "fooImage",
							ContainerID:  "docker://fooID",
							RestartCount: 13,
							State: v1.ContainerState{
								Running: &v1.ContainerStateRunning{
									StartedAt: timestamp,
								},
							},
						},
						{
							Name:         "barName",
							Image:        "barImage",
							ContainerID:  "docker://barID",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
						{
							Name:         "bazName",
							Image:        "bazImage",
							ContainerID:  "docker://bazID",
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
				},
				Spec: v1.PodSpec{
					NodeName:   "node",
					Containers: []v1.Container{{}, {}},
				},
			}, expected: model.Pod{
				Name:              "pod",
				Namespace:         "namespace",
				Uid:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
				CreationTimestamp: 1389744000,
				Phase:             "Running",
				NominatedNodeName: "nominated",
				NodeName:          "node",
				RestartCount:      42,
				Labels:            []string{"label:foo"},
				Annotations:       []string{"annotation:bar"},
				Status:            "chillin",
				OwnerReferences: []*model.OwnerReference{
					{
						Name: "test-controller",
						Kind: "replicaset",
						Uid:  "1234567890",
					},
				},
				ContainerStatuses: []*model.ContainerStatus{
					{
						State:        "Running",
						RestartCount: 13,
						Name:         "fooName",
						ContainerID:  "docker://fooID",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "barName",
						ContainerID:  "docker://barID",
					},
					{
						State:        "Terminated",
						Message:      "CLB PLS (exit: -1)",
						RestartCount: 19,
						Name:         "bazName",
						ContainerID:  "docker://bazID",
					},
				},
			},
		},
		"empty pod": {input: v1.Pod{}, expected: model.Pod{}},
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
							Name:        "fooName",
							Image:       "fooImage",
							ContainerID: "docker://fooID",
						},
						{
							Name:         "barName",
							Image:        "barImage",
							ContainerID:  "docker://barID",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{
									Reason:  "chillin",
									Message: "testin",
								},
							},
						},
						{
							Name: "bazName",
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
				Name:         "pod",
				Namespace:    "namespace",
				RestartCount: 10,
				OwnerReferences: []*model.OwnerReference{
					{
						Uid: "1234567890",
					},
				},
				Status: "chillin",
				ContainerStatuses: []*model.ContainerStatus{
					{
						Name:        "fooName",
						ContainerID: "docker://fooID",
					},
					{
						State:        "Waiting",
						Message:      "chillin testin",
						RestartCount: 10,
						Name:         "barName",
						ContainerID:  "docker://barID",
					},
					{
						Name: "bazName",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, extractPodMessage(&tc.input))
		})
	}
}

func TestScrubContainer(t *testing.T) {
	cfg := config.NewDefaultAgentConfig(true)
	tests := map[string]struct {
		input    v1.Container
		expected v1.Container
	}{
		"sensitive CLI": {
			input: v1.Container{
				Command: []string{"mysql", "--password", "afztyerbzio1234"},
			},
			expected: v1.Container{
				Command: []string{"mysql", "--password", "********"},
			},
		},
		"sensitive env var": {
			input: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "kqhkiG9w0BAQEFAASCAl8wggJbAgEAAoGBAOLJ"}},
			},
			expected: v1.Container{
				Env: []v1.EnvVar{{Name: "password", Value: "********"}},
			},
		},
		"sensitive container": {
			input: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "afztyerbzio1234", "--access_token", "yolo123"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "yolo"},
				},
			},
			expected: v1.Container{
				Name:    "test container",
				Image:   "random",
				Command: []string{"decrypt", "--password", "********", "--access_token", "********"},
				Env: []v1.EnvVar{
					{Name: "hostname", Value: "password"},
					{Name: "pwd", Value: "********"},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scrubContainer(&tc.input, cfg)
			assert.Equal(t, tc.expected, tc.input)
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
			assert.EqualValues(t, tc.status, ComputeStatus(tc.pod))
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
		},
	} {
		t.Run(fmt.Sprintf("case %d", nb), func(t *testing.T) {
			assert.EqualValues(t, tc.message, GetConditionMessage(tc.pod))
		})
	}
}
