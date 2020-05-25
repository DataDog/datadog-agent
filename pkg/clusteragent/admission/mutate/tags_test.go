// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver

package mutate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_injectTagsFromLabels(t *testing.T) {
	tests := []struct {
		name        string
		pod         *corev1.Pod
		wantPodFunc func() corev1.Pod
	}{
		{
			name: "nominal case",
			pod:  fakePodWithLabels("foo-pod", map[string]string{"tags.datadoghq.com/env": "dev", "tags.datadoghq.com/service": "dd-agent", "tags.datadoghq.com/version": "7"}),
			wantPodFunc: func() corev1.Pod {
				pod := fakePodWithLabels("foo-pod", map[string]string{"tags.datadoghq.com/env": "dev", "tags.datadoghq.com/service": "dd-agent", "tags.datadoghq.com/version": "7"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_ENV", "dev"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_SERVICE", "dd-agent"))
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_VERSION", "7"))
				return *pod
			},
		},
		{
			name: "no labels",
			pod:  fakePodWithLabels("foo-pod", map[string]string{}),
			wantPodFunc: func() corev1.Pod {
				pod := fakePodWithLabels("foo-pod", map[string]string{})
				return *pod
			},
		},
		{
			name: "env only",
			pod:  fakePodWithLabels("foo-pod", map[string]string{"tags.datadoghq.com/env": "dev"}),
			wantPodFunc: func() corev1.Pod {
				pod := fakePodWithLabels("foo-pod", map[string]string{"tags.datadoghq.com/env": "dev"})
				pod.Spec.Containers[0].Env = append(pod.Spec.Containers[0].Env, fakeEnvWithValue("DD_ENV", "dev"))
				return *pod
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			injectTagsFromLabels(tt.pod)
			assert.Len(t, tt.pod.Spec.Containers, 1)
			assert.Len(t, tt.wantPodFunc().Spec.Containers, 1)
			assert.ElementsMatch(t, tt.wantPodFunc().Spec.Containers[0].Env, tt.pod.Spec.Containers[0].Env)
		})
	}
}

func fakePodWithLabels(name string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: name + "-container",
				},
			},
		},
	}
}

func fakeEnvWithValue(name, value string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  name,
		Value: value,
	}
}
