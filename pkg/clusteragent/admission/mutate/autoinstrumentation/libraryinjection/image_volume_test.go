// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestImageVolumeProvider_InjectInjector(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "my-app:latest"},
			},
		},
	}

	provider := libraryinjection.NewImageVolumeProvider(libraryinjection.LibraryInjectionConfig{})
	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})

	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)
	assert.Nil(t, result.Err)

	// Verify the injector image volume was added.
	require.Len(t, pod.Spec.Volumes, 1)
	vol := pod.Spec.Volumes[0]
	assert.Equal(t, libraryinjection.InstrumentationVolumeName, vol.Name)
	require.NotNil(t, vol.VolumeSource.Image)
	assert.Equal(t, "gcr.io/datadoghq/apm-inject:0.52.0", vol.VolumeSource.Image.Reference)
	assert.Equal(t, corev1.PullIfNotPresent, vol.VolumeSource.Image.PullPolicy)

	// Verify volume mounts were added to the application container.
	require.Len(t, pod.Spec.Containers, 1)
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)

	// Find mounts by mount path (order may vary).
	var instrMount, etcMount *corev1.VolumeMount
	for i := range pod.Spec.Containers[0].VolumeMounts {
		m := &pod.Spec.Containers[0].VolumeMounts[i]
		switch m.MountPath {
		case "/opt/datadog-packages/datadog-apm-inject":
			instrMount = m
		case "/etc/ld.so.preload":
			etcMount = m
		}
	}
	require.NotNil(t, instrMount, "injector mount should exist")
	require.NotNil(t, etcMount, "ld.so.preload mount should exist")

	assert.Equal(t, libraryinjection.InstrumentationVolumeName, instrMount.Name)
	assert.True(t, instrMount.ReadOnly)
	assert.Equal(t, "opt/datadog-packages/datadog-apm-inject", instrMount.SubPath)

	assert.Equal(t, libraryinjection.InstrumentationVolumeName, etcMount.Name)
	assert.True(t, etcMount.ReadOnly)
	assert.Equal(t, "opt/datadog-packages/datadog-apm-inject/stable/inject/ld.so.preload", etcMount.SubPath)
}
