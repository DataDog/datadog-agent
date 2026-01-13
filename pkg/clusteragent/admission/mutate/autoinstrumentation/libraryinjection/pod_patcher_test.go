// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestPodPatcher_AddVolume(t *testing.T) {
	tests := []struct {
		name            string
		existingVolumes []corev1.Volume
		volumeToAdd     corev1.Volume
		expectedVolumes []corev1.Volume
	}{
		{
			name:            "add volume to empty pod",
			existingVolumes: nil,
			volumeToAdd:     corev1.Volume{Name: "vol1"},
			expectedVolumes: []corev1.Volume{{Name: "vol1"}},
		},
		{
			name:            "add volume when different volume exists",
			existingVolumes: []corev1.Volume{{Name: "existing"}},
			volumeToAdd:     corev1.Volume{Name: "vol1"},
			expectedVolumes: []corev1.Volume{{Name: "existing"}, {Name: "vol1"}},
		},
		{
			name: "replace volume with same name",
			existingVolumes: []corev1.Volume{
				{Name: "vol1", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/old"}}},
			},
			volumeToAdd: corev1.Volume{Name: "vol1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			expectedVolumes: []corev1.Volume{
				{Name: "vol1", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{Volumes: tt.existingVolumes},
			}
			patcher := libraryinjection.NewPodPatcher(pod, nil)
			patcher.AddVolume(tt.volumeToAdd)

			assert.Equal(t, len(tt.expectedVolumes), len(pod.Spec.Volumes))
			for i, expected := range tt.expectedVolumes {
				assert.Equal(t, expected.Name, pod.Spec.Volumes[i].Name)
			}
		})
	}
}

func TestPodPatcher_AddVolumeMount_WithFilter(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
				{Name: "istio-proxy"},
				{Name: "other-app"},
			},
		},
	}

	// Filter that excludes istio-proxy
	filter := func(c *corev1.Container) bool {
		return c.Name != "istio-proxy"
	}

	patcher := libraryinjection.NewPodPatcher(pod, filter)
	mount := corev1.VolumeMount{Name: "test-vol", MountPath: "/test"}
	patcher.AddVolumeMount(mount)

	// app should have the mount
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
	assert.Equal(t, "test-vol", pod.Spec.Containers[0].VolumeMounts[0].Name)

	// istio-proxy should NOT have the mount
	assert.Len(t, pod.Spec.Containers[1].VolumeMounts, 0)

	// other-app should have the mount
	assert.Len(t, pod.Spec.Containers[2].VolumeMounts, 1)
}

func TestPodPatcher_AddVolumeMount_Replaces(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					VolumeMounts: []corev1.VolumeMount{
						{Name: "vol1", MountPath: "/path1", ReadOnly: false},
					},
				},
			},
		},
	}

	patcher := libraryinjection.NewPodPatcher(pod, nil)
	// Add mount with same name and path but different ReadOnly
	mount := corev1.VolumeMount{Name: "vol1", MountPath: "/path1", ReadOnly: true}
	patcher.AddVolumeMount(mount)

	// Should replace, not append
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
	assert.True(t, pod.Spec.Containers[0].VolumeMounts[0].ReadOnly)
}

func TestPodPatcher_AddEnvVar_DoesNotOverwrite(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Env: []corev1.EnvVar{
						{Name: "EXISTING", Value: "original"},
					},
				},
			},
		},
	}

	patcher := libraryinjection.NewPodPatcher(pod, nil)
	patcher.AddEnvVar(corev1.EnvVar{Name: "EXISTING", Value: "new"})

	// Should NOT overwrite
	assert.Len(t, pod.Spec.Containers[0].Env, 1)
	assert.Equal(t, "original", pod.Spec.Containers[0].Env[0].Value)
}

func TestPodPatcher_AddEnvVar_Prepends(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Env: []corev1.EnvVar{
						{Name: "EXISTING", Value: "val"},
					},
				},
			},
		},
	}

	patcher := libraryinjection.NewPodPatcher(pod, nil)
	patcher.AddEnvVar(corev1.EnvVar{Name: "NEW", Value: "new-val"})

	// New env var should be prepended
	assert.Len(t, pod.Spec.Containers[0].Env, 2)
	assert.Equal(t, "NEW", pod.Spec.Containers[0].Env[0].Name)
	assert.Equal(t, "EXISTING", pod.Spec.Containers[0].Env[1].Name)
}

func TestPodPatcher_AddEnvVarWithJoin(t *testing.T) {
	tests := []struct {
		name          string
		existingEnv   []corev1.EnvVar
		envName       string
		envValue      string
		separator     string
		expectedValue string
	}{
		{
			name:          "add new env var",
			existingEnv:   nil,
			envName:       "LD_PRELOAD",
			envValue:      "/lib/new.so",
			separator:     ":",
			expectedValue: "/lib/new.so",
		},
		{
			name: "join with existing value",
			existingEnv: []corev1.EnvVar{
				{Name: "LD_PRELOAD", Value: "/lib/existing.so"},
			},
			envName:       "LD_PRELOAD",
			envValue:      "/lib/new.so",
			separator:     ":",
			expectedValue: "/lib/existing.so:/lib/new.so",
		},
		{
			name: "join with different separator",
			existingEnv: []corev1.EnvVar{
				{Name: "PATH", Value: "/usr/bin"},
			},
			envName:       "PATH",
			envValue:      "/custom/bin",
			separator:     ";",
			expectedValue: "/usr/bin;/custom/bin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app", Env: tt.existingEnv},
					},
				},
			}

			patcher := libraryinjection.NewPodPatcher(pod, nil)
			patcher.AddEnvVarWithJoin(tt.envName, tt.envValue, tt.separator)

			assert.Len(t, pod.Spec.Containers[0].Env, 1)
			assert.Equal(t, tt.envName, pod.Spec.Containers[0].Env[0].Name)
			assert.Equal(t, tt.expectedValue, pod.Spec.Containers[0].Env[0].Value)
		})
	}
}

func TestPodPatcher_AddInitContainer(t *testing.T) {
	tests := []struct {
		name           string
		existingInits  []corev1.Container
		initToAdd      corev1.Container
		expectedNames  []string
		expectedImages []string
	}{
		{
			name:           "add to empty pod",
			existingInits:  nil,
			initToAdd:      corev1.Container{Name: "init1", Image: "image1"},
			expectedNames:  []string{"init1"},
			expectedImages: []string{"image1"},
		},
		{
			name:           "prepend init container",
			existingInits:  []corev1.Container{{Name: "existing", Image: "existing-img"}},
			initToAdd:      corev1.Container{Name: "init1", Image: "image1"},
			expectedNames:  []string{"init1", "existing"},
			expectedImages: []string{"image1", "existing-img"},
		},
		{
			name:           "replace init container with same name",
			existingInits:  []corev1.Container{{Name: "init1", Image: "old-image"}},
			initToAdd:      corev1.Container{Name: "init1", Image: "new-image"},
			expectedNames:  []string{"init1"},
			expectedImages: []string{"new-image"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
				Spec:       corev1.PodSpec{InitContainers: tt.existingInits},
			}

			patcher := libraryinjection.NewPodPatcher(pod, nil)
			patcher.AddInitContainer(tt.initToAdd)

			assert.Len(t, pod.Spec.InitContainers, len(tt.expectedNames))
			for i, name := range tt.expectedNames {
				assert.Equal(t, name, pod.Spec.InitContainers[i].Name)
				assert.Equal(t, tt.expectedImages[i], pod.Spec.InitContainers[i].Image)
			}
		})
	}
}
