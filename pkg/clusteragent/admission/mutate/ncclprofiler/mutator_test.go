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
	testHostSocketDir  = "/var/run/datadog"
	testClientDir      = "/var/run/datadog"
	testSocketFilename = "nccl.socket"
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

	mutated, err := mutatePod(pod, testInjectorImage, testHostSocketDir, testClientDir, testSocketFilename, nil)

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
			assert.Equal(t, testHostSocketDir, v.HostPath.Path,
				"socket volume mounts the host socket DIRECTORY, not the file")
			require.NotNil(t, v.HostPath.Type)
			assert.Equal(t, corev1.HostPathDirectoryOrCreate, *v.HostPath.Type)
			sawSocketVol = true
		}
	}
	assert.True(t, sawSoVol)
	assert.True(t, sawSocketVol)

	// Trainer container has both volume mounts and 4 NCCL env vars.
	require.Len(t, pod.Spec.Containers, 1)
	trainer := pod.Spec.Containers[0]
	mounts := map[string]string{}
	for _, m := range trainer.VolumeMounts {
		mounts[m.Name] = m.MountPath
	}
	assert.Contains(t, mounts, soVolumeName)
	assert.Contains(t, mounts, socketVolumeName)
	assert.Equal(t, testClientDir, mounts[socketVolumeName],
		"socket mount destination is the in-pod socket DIRECTORY")

	envs := map[string]string{}
	for _, e := range trainer.Env {
		envs[e.Name] = e.Value
	}
	assert.Equal(t, soDestPath, envs["NCCL_PROFILER_PLUGIN"])
	assert.Equal(t, testSocketPath, envs["NCCL_DD_SOCKET_PATH"])
	assert.Equal(t, soMountPath+"/libnccl-profiler-inspector.so", envs["NCCL_DD_INSPECTOR_PATH"])
	assert.Equal(t, "1", envs["NCCL_INSPECTOR_ENABLE"])
}

// TestMutatePod_DecoupledHostAndContainerPaths: host and in-container
// directories can differ. Host file = hostDir+/+filename, in-workload
// file = clientDir+/+filename, NCCL_DD_SOCKET_PATH = in-workload file.
func TestMutatePod_DecoupledHostAndContainerPaths(t *testing.T) {
	pod := newTestPod()
	hostDir := "/var/run/datadog-agent"
	clientDir := "/var/run/datadog"
	filename := "nccl.socket"

	mutated, err := mutatePod(pod, testInjectorImage, hostDir, clientDir, filename, nil)
	require.NoError(t, err)
	assert.True(t, mutated)

	var socketVol *corev1.Volume
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == socketVolumeName {
			socketVol = &pod.Spec.Volumes[i]
		}
	}
	require.NotNil(t, socketVol)
	require.NotNil(t, socketVol.HostPath)
	assert.Equal(t, "/var/run/datadog-agent", socketVol.HostPath.Path)
	require.NotNil(t, socketVol.HostPath.Type)
	assert.Equal(t, corev1.HostPathDirectoryOrCreate, *socketVol.HostPath.Type)

	trainer := pod.Spec.Containers[0]
	var mountPath string
	for _, m := range trainer.VolumeMounts {
		if m.Name == socketVolumeName {
			mountPath = m.MountPath
		}
	}
	assert.Equal(t, "/var/run/datadog", mountPath)

	for _, e := range trainer.Env {
		if e.Name == "NCCL_DD_SOCKET_PATH" {
			assert.Equal(t, "/var/run/datadog/nccl.socket", e.Value)
		}
	}
}
