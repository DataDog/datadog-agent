// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func Test_injectTagsFromLabels(t *testing.T) {
	tests := []struct {
		name        string
		labels      map[string]string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
		found       bool
		injected    bool
	}{
		{
			name:   "nominal case",
			labels: map[string]string{"tags.datadoghq.com/env": "dev", "tags.datadoghq.com/service": "dd-agent", "tags.datadoghq.com/version": "7"},
			pod:    fakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := fakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "no labels",
			labels: map[string]string{},
			pod:    fakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := fakePod("foo-pod")
				return *pod
			},
			found:    false,
			injected: false,
		},
		{
			name:   "env only",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    fakePod("foo-pod"),
			wantPodFunc: func() corev1.Pod {
				pod := fakePod("foo-pod")
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
			found:    true,
			injected: true,
		},
		{
			name:   "label found but not injected",
			labels: map[string]string{"tags.datadoghq.com/env": "dev"},
			pod:    fakePodWithEnv("foo-pod", "DD_ENV"),
			wantPodFunc: func() corev1.Pod {
				pod := fakePodWithEnv("foo-pod", "DD_ENV")
				return *pod
			},
			found:    true,
			injected: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, injected := injectTagsFromLabels(tt.labels, tt.pod)
			assert.Equal(t, tt.found, found)
			assert.Equal(t, tt.injected, injected)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
}

func Test_shouldInjectTags(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		want bool
	}{
		{
			name: "no admission label",
			pod:  fakePodWithLabel("k", "v"),
			want: true,
		},
		{
			name: "admission label enabled",
			pod:  fakePodWithLabel("k", "v"),
			want: true,
		},
		{
			name: "admission label disabled",
			pod:  fakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldInjectTags(tt.pod); got != tt.want {
				t.Errorf("shouldInjectTags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getOwnerInfo(t *testing.T) {
	tests := []struct {
		name    string
		owner   metav1.OwnerReference
		want    *ownerInfo
		wantErr bool
	}{
		{
			name: "replicaset",
			owner: metav1.OwnerReference{
				APIVersion:         "apps/v1",
				BlockOwnerDeletion: boolPointer(true),
				Controller:         boolPointer(true),
				Kind:               "ReplicaSet",
				Name:               "my-app-547c56f566",
				UID:                "2dfa7d22-245f-4769-8854-bc3b056cd224",
			},
			want: &ownerInfo{
				name: "my-app-547c56f566",
				gvr: schema.GroupVersionResource{
					Group:    "apps",
					Version:  "v1",
					Resource: "replicasets",
				},
			},
			wantErr: false,
		},
		{
			name: "job",
			owner: metav1.OwnerReference{
				APIVersion:         "batch/v1",
				BlockOwnerDeletion: boolPointer(true),
				Controller:         boolPointer(true),
				Kind:               "Job",
				Name:               "my-job",
				UID:                "89e8148c-8601-4c69-b8a6-3fbb176547d0",
			},
			want: &ownerInfo{
				name: "my-job",
				gvr: schema.GroupVersionResource{
					Group:    "batch",
					Version:  "v1",
					Resource: "jobs",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid APIVersion",
			owner: metav1.OwnerReference{
				APIVersion:         "batch/v1/",
				BlockOwnerDeletion: boolPointer(true),
				Controller:         boolPointer(true),
				Kind:               "Job",
				Name:               "my-job",
				UID:                "89e8148c-8601-4c69-b8a6-3fbb176547d0",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getOwnerInfo(tt.owner)
			if (err != nil) != tt.wantErr {
				t.Errorf("getOwnerInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getOwnerInfo() = %v, want %v", got, tt.want)
			}
		})
	}
}
