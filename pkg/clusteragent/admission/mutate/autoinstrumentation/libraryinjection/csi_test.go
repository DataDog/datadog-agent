// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestCSIProvider_InjectInjector(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "my-app:latest"},
			},
		},
	}

	provider := libraryinjection.NewCSIProvider(libraryinjection.LibraryInjectionConfig{})
	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})

	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)
	assert.Nil(t, result.Err)

	// Verify volumes were added
	require.Len(t, pod.Spec.Volumes, 2)

	// Find volumes by name (order may vary)
	var instrVol, etcVol *corev1.Volume
	for i := range pod.Spec.Volumes {
		switch pod.Spec.Volumes[i].Name {
		case libraryinjection.InstrumentationVolumeName:
			instrVol = &pod.Spec.Volumes[i]
		case libraryinjection.EtcVolumeName:
			etcVol = &pod.Spec.Volumes[i]
		}
	}

	// Check instrumentation volume (CSI)
	require.NotNil(t, instrVol, "instrumentation volume should exist")
	require.NotNil(t, instrVol.CSI)
	assert.Equal(t, "k8s.csi.datadoghq.com", instrVol.CSI.Driver)
	assert.Equal(t, "DatadogLibrary", instrVol.CSI.VolumeAttributes["type"])
	assert.Equal(t, "apm-inject", instrVol.CSI.VolumeAttributes["dd.csi.datadog.com/library.package"])
	assert.Equal(t, "gcr.io/datadoghq", instrVol.CSI.VolumeAttributes["dd.csi.datadog.com/library.registry"])
	assert.Equal(t, "0.52.0", instrVol.CSI.VolumeAttributes["dd.csi.datadog.com/library.version"])

	// Check etc volume (CSI for ld.so.preload)
	require.NotNil(t, etcVol, "etc volume should exist")
	require.NotNil(t, etcVol.CSI)
	assert.Equal(t, "DatadogInjectorPreload", etcVol.CSI.VolumeAttributes["type"])

	// Verify volume mounts were added to container
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)

	// Find mounts by name
	var instrMount, etcMount *corev1.VolumeMount
	for i := range pod.Spec.Containers[0].VolumeMounts {
		switch pod.Spec.Containers[0].VolumeMounts[i].Name {
		case libraryinjection.InstrumentationVolumeName:
			instrMount = &pod.Spec.Containers[0].VolumeMounts[i]
		case libraryinjection.EtcVolumeName:
			etcMount = &pod.Spec.Containers[0].VolumeMounts[i]
		}
	}
	require.NotNil(t, instrMount, "instrumentation mount should exist")
	require.NotNil(t, etcMount, "etc mount should exist")
	assert.Equal(t, "/etc/ld.so.preload", etcMount.MountPath)
}

func TestCSIProvider_InjectLibrary(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "my-app:latest"},
			},
		},
	}

	provider := libraryinjection.NewCSIProvider(libraryinjection.LibraryInjectionConfig{})
	result := provider.InjectLibrary(pod, libraryinjection.LibraryConfig{
		Language: "java",
		Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-java-init:1.2.3", "1.2.3"),
	})

	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)
	assert.Nil(t, result.Err)

	// Verify volume was added
	require.Len(t, pod.Spec.Volumes, 1)
	vol := pod.Spec.Volumes[0]
	assert.Equal(t, "dd-lib-java", vol.Name)
	require.NotNil(t, vol.CSI)
	assert.Equal(t, "k8s.csi.datadoghq.com", vol.CSI.Driver)
	assert.Equal(t, "DatadogLibrary", vol.CSI.VolumeAttributes["type"])
	assert.Equal(t, "dd-lib-java-init", vol.CSI.VolumeAttributes["dd.csi.datadog.com/library.package"])
	assert.Equal(t, "gcr.io/datadoghq", vol.CSI.VolumeAttributes["dd.csi.datadog.com/library.registry"])
	assert.Equal(t, "1.2.3", vol.CSI.VolumeAttributes["dd.csi.datadog.com/library.version"])

	// Verify volume mount was added
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 1)
	mount := pod.Spec.Containers[0].VolumeMounts[0]
	assert.Equal(t, "dd-lib-java", mount.Name)
	assert.Contains(t, mount.MountPath, "java")
	assert.True(t, mount.ReadOnly)
}

func TestCSIProvider_InjectMultipleLibraries(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "my-app:latest"},
			},
		},
	}

	provider := libraryinjection.NewCSIProvider(libraryinjection.LibraryInjectionConfig{})

	// Inject Java library
	result := provider.InjectLibrary(pod, libraryinjection.LibraryConfig{
		Language: "java",
		Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-java-init:1.0.0", ""),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Inject Python library
	result = provider.InjectLibrary(pod, libraryinjection.LibraryConfig{
		Language: "python",
		Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-python-init:2.0.0", ""),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Verify both volumes were added
	require.Len(t, pod.Spec.Volumes, 2)
	assert.Equal(t, "dd-lib-java", pod.Spec.Volumes[0].Name)
	assert.Equal(t, "dd-lib-python", pod.Spec.Volumes[1].Name)

	// Verify both mounts were added
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)
}

func TestCSIProvider_ContainerFilter(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "my-app:latest"},
				{Name: "sidecar", Image: "sidecar:latest"},
			},
		},
	}

	// Filter to only inject into "app" container
	provider := libraryinjection.NewCSIProvider(libraryinjection.LibraryInjectionConfig{
		ContainerFilter: func(c *corev1.Container) bool {
			return c.Name == "app"
		},
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", ""),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Only "app" container should have volume mounts
	assert.Len(t, pod.Spec.Containers[0].VolumeMounts, 2) // app
	assert.Len(t, pod.Spec.Containers[1].VolumeMounts, 0) // sidecar
}
