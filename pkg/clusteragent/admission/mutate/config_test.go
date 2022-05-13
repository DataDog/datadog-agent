// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver
// +build kubeapiserver

package mutate

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func Test_shouldInjectConf(t *testing.T) {
	mockConfig := config.Mock()
	tests := []struct {
		name        string
		pod         *corev1.Pod
		setupConfig func()
		want        bool
	}{
		{
			name:        "mutate unlabelled, no label",
			pod:         fakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "mutate unlabelled, label enabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        true,
		},
		{
			name:        "mutate unlabelled, label disabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", true) },
			want:        false,
		},
		{
			name:        "no mutate unlabelled, no label",
			pod:         fakePodWithLabel("", ""),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
		{
			name:        "no mutate unlabelled, label enabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "true"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        true,
		},
		{
			name:        "no mutate unlabelled, label disabled",
			pod:         fakePodWithLabel("admission.datadoghq.com/enabled", "false"),
			setupConfig: func() { mockConfig.Set("admission_controller.mutate_unlabelled", false) },
			want:        false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupConfig()
			if got := shouldInjectConf(tt.pod); got != tt.want {
				t.Errorf("shouldInjectConf() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_injectionMode(t *testing.T) {
	tests := []struct {
		name       string
		pod        *corev1.Pod
		globalMode string
		want       string
	}{
		{
			name:       "nominal case",
			pod:        fakePod("foo"),
			globalMode: "hostip",
			want:       "hostip",
		},
		{
			name:       "custom mode #1",
			pod:        fakePodWithLabel("admission.datadoghq.com/config.mode", "service"),
			globalMode: "hostip",
			want:       "service",
		},
		{
			name:       "custom mode #2",
			pod:        fakePodWithLabel("admission.datadoghq.com/config.mode", "socket"),
			globalMode: "hostip",
			want:       "socket",
		},
		{
			name:       "invalid",
			pod:        fakePodWithLabel("admission.datadoghq.com/config.mode", "wrong"),
			globalMode: "hostip",
			want:       "hostip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, injectionMode(tt.pod, tt.globalMode))
		})
	}
}

func TestInjectHostIP(t *testing.T) {
	pod := fakePodWithContainer("foo-pod", corev1.Container{})
	pod = withLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true"})
	err := injectConfig(pod, "", nil)
	assert.Nil(t, err)
	assert.Equal(t, pod.Spec.Containers[0].Env[0].Name, "DD_AGENT_HOST")
	assert.Equal(t, pod.Spec.Containers[0].Env[0].ValueFrom.FieldRef.FieldPath, "status.hostIP")
}

func TestInjectService(t *testing.T) {
	pod := fakePodWithContainer("foo-pod", corev1.Container{})
	pod = withLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true", "admission.datadoghq.com/config.mode": "service"})
	err := injectConfig(pod, "", nil)
	assert.Nil(t, err)
	assert.Equal(t, pod.Spec.Containers[0].Env[0].Name, "DD_AGENT_HOST")
	assert.Equal(t, pod.Spec.Containers[0].Env[0].Value, "datadog.default.svc.cluster.local")
}

func TestInjectSocket(t *testing.T) {
	pod := fakePodWithContainer("foo-pod", corev1.Container{})
	pod = withLabels(pod, map[string]string{"admission.datadoghq.com/enabled": "true", "admission.datadoghq.com/config.mode": "socket"})
	err := injectConfig(pod, "", nil)
	assert.Nil(t, err)
	assert.Equal(t, pod.Spec.Containers[0].Env[0].Name, "DD_TRACE_AGENT_URL")
	assert.Equal(t, pod.Spec.Containers[0].Env[0].Value, "unix:///var/run/datadog/apm.socket")
	assert.Equal(t, pod.Spec.Containers[0].VolumeMounts[0].MountPath, "/var/run/datadog")
	assert.Equal(t, pod.Spec.Containers[0].VolumeMounts[0].Name, "datadog")
	assert.Equal(t, pod.Spec.Volumes[0].Name, "datadog")
	assert.Equal(t, pod.Spec.Volumes[0].VolumeSource.HostPath.Path, "/var/run/datadog")
	assert.Equal(t, *pod.Spec.Volumes[0].VolumeSource.HostPath.Type, corev1.HostPathDirectoryOrCreate)
}
