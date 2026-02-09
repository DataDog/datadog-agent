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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestInjectInjector(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	provider := libraryinjection.NewInitContainerProvider(libraryinjection.LibraryInjectionConfig{})
	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:latest", ""),
	})

	require.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Check init container was added
	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, libraryinjection.InjectorInitContainerName, pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "gcr.io/datadoghq/apm-inject:latest", pod.Spec.InitContainers[0].Image)

	// Check volumes were added
	require.Len(t, pod.Spec.Volumes, 2)
	assert.Equal(t, libraryinjection.InstrumentationVolumeName, pod.Spec.Volumes[0].Name)
	assert.Equal(t, libraryinjection.EtcVolumeName, pod.Spec.Volumes[1].Name)

	// Check volume mounts were added to app container
	require.Len(t, pod.Spec.Containers[0].VolumeMounts, 2)
}

func TestInjectLibrary(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	provider := libraryinjection.NewInitContainerProvider(libraryinjection.LibraryInjectionConfig{})
	result := provider.InjectLibrary(pod, libraryinjection.LibraryConfig{
		Language: "java",
		Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-java-init:latest", ""),
	})

	require.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Check init container was added
	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "datadog-lib-java-init", pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "gcr.io/datadoghq/dd-lib-java-init:latest", pod.Spec.InitContainers[0].Image)
}

func TestInjectInjector_SkipsWhenInsufficientResources(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}

	// Create a pod with resources below minimum
	podLowResources := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod-low", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("10m"), // below minimum
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}

	provider := libraryinjection.NewInitContainerProvider(libraryinjection.LibraryInjectionConfig{})

	// Should succeed with sufficient resources
	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("test-image", ""),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// Should skip with insufficient resources
	resultLow := provider.InjectInjector(podLowResources, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("test-image", ""),
	})
	assert.Equal(t, libraryinjection.MutationStatusSkipped, resultLow.Status)
	assert.NotNil(t, resultLow.Err)
}

func TestInjectInjector_UsesConfiguredInitSecurityContext(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	sc := &corev1.SecurityContext{
		RunAsNonRoot: ptr.To(true),
	}
	provider := libraryinjection.NewInitContainerProvider(libraryinjection.LibraryInjectionConfig{
		InitSecurityContext: sc,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:latest", ""),
	})

	require.Equal(t, libraryinjection.MutationStatusInjected, result.Status)
	require.Len(t, pod.Spec.InitContainers, 1)
	require.Same(t, sc, pod.Spec.InitContainers[0].SecurityContext)
}
