// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	testInjectorImage  = "registry.example/nccl-profiler-injector:latest"
	testHostSocketPath = "/var/run/datadog"
	testSocketPath     = "/var/run/datadog/nccl.socket"
)

func newTestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "training-pod",
			Namespace: "default",
			Labels:    map[string]string{EnabledLabel: "true"},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "trainer", Image: "pytorch:latest"},
			},
		},
	}
}

func TestMutatePod_HappyPath(t *testing.T) {
	pod := newTestPod()

	mutated, err := mutatePod(pod, testInjectorImage, testHostSocketPath, testSocketPath, nil)

	require.NoError(t, err)
	assert.True(t, mutated, "mutatePod should report it mutated the pod")

	// Init container prepended.
	require.Len(t, pod.Spec.InitContainers, 1)
	init := pod.Spec.InitContainers[0]
	assert.Equal(t, "datadog-nccl-profiler-inject", init.Name)
	assert.Equal(t, testInjectorImage, init.Image)

	// Two volumes added: emptyDir for .so + hostPath for socket.
	require.Len(t, pod.Spec.Volumes, 2)
	var sawSoVol, sawSocketVol bool
	for _, v := range pod.Spec.Volumes {
		switch v.Name {
		case soVolumeName:
			require.NotNil(t, v.EmptyDir, "so volume should be emptyDir")
			sawSoVol = true
		case socketVolumeName:
			require.NotNil(t, v.HostPath, "socket volume should be hostPath")
			assert.Equal(t, testHostSocketPath, v.HostPath.Path)
			sawSocketVol = true
		}
	}
	assert.True(t, sawSoVol)
	assert.True(t, sawSocketVol)

	// Trainer container has both volume mounts and 4 NCCL env vars.
	require.Len(t, pod.Spec.Containers, 1)
	trainer := pod.Spec.Containers[0]
	mountNames := map[string]bool{}
	for _, m := range trainer.VolumeMounts {
		mountNames[m.Name] = true
	}
	assert.True(t, mountNames[soVolumeName])
	assert.True(t, mountNames[socketVolumeName])

	envs := map[string]string{}
	for _, e := range trainer.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, soDestPath, envs["NCCL_PROFILER_PLUGIN"])
	assert.Equal(t, testSocketPath, envs["NCCL_DD_SOCKET_PATH"])
	assert.Equal(t, soMountPath+"/libnccl-profiler-inspector.so", envs["NCCL_DD_INSPECTOR_PATH"])
	assert.Equal(t, "1", envs["NCCL_INSPECTOR_ENABLE"])
}
