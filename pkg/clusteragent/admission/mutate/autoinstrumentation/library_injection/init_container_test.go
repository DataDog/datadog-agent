// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodSumResourceRequirements(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways

	tests := []struct {
		name             string
		pod              *corev1.Pod
		expectedLimitCPU string
		expectedLimitMem string
		expectedReqCPU   string
		expectedReqMem   string
	}{
		{
			name: "single container",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
								Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
							},
						},
					},
				},
			},
			expectedLimitCPU: "500m",
			expectedLimitMem: "256Mi",
			expectedReqCPU:   "100m",
			expectedReqMem:   "128Mi",
		},
		{
			name: "multiple containers sum",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app1",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
							},
						},
						{
							Name: "app2",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("300m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
							},
						},
					},
				},
			},
			expectedLimitCPU: "800m",
			expectedLimitMem: "384Mi",
		},
		{
			name: "init container with higher limit takes precedence",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name: "init",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("1Gi")},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
							},
						},
					},
				},
			},
			expectedLimitCPU: "2",
			expectedLimitMem: "1Gi",
		},
		{
			name: "sidecar init container adds to container sum",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:          "sidecar",
							RestartPolicy: &restartAlways,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
							},
						},
					},
				},
			},
			expectedLimitCPU: "700m",  // 500m + 200m
			expectedLimitMem: "320Mi", // 256Mi + 64Mi
		},
		{
			name: "request cannot exceed limit",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
								Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")},
							},
						},
					},
				},
			},
			expectedLimitCPU: "500m", // limit is adjusted to match request
			expectedReqCPU:   "500m",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := podSumResourceRequirements(tt.pod)

			if tt.expectedLimitCPU != "" {
				assert.Equal(t, tt.expectedLimitCPU, result.Limits.Cpu().String())
			}
			if tt.expectedLimitMem != "" {
				assert.Equal(t, tt.expectedLimitMem, result.Limits.Memory().String())
			}
			if tt.expectedReqCPU != "" {
				assert.Equal(t, tt.expectedReqCPU, result.Requests.Cpu().String())
			}
			if tt.expectedReqMem != "" {
				assert.Equal(t, tt.expectedReqMem, result.Requests.Memory().String())
			}
		})
	}
}

func TestComputeResourceRequirements_Skip(t *testing.T) {
	tests := []struct {
		name       string
		pod        *corev1.Pod
		shouldSkip bool
	}{
		{
			name: "skip when CPU limit too low",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("10m"), // < 50m minimum
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
			shouldSkip: true,
		},
		{
			name: "skip when memory limit too low",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("50Mi"), // < 100Mi minimum
								},
							},
						},
					},
				},
			},
			shouldSkip: true,
		},
		{
			name: "do not skip when resources are sufficient",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "app",
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
			shouldSkip: false,
		},
		{
			name: "do not skip when no limits are set",
			pod: &corev1.Pod{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "app"},
					},
				},
			},
			shouldSkip: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := newInitContainerProvider(LibraryInjectionConfig{})
			_, shouldSkip, _ := provider.computeResourceRequirements(tt.pod)
			assert.Equal(t, tt.shouldSkip, shouldSkip)
		})
	}
}

func TestComputeResourceRequirements_UsesConfigDefaults(t *testing.T) {
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "app",
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
		},
	}

	// Provide default resource requirements in config
	provider := newInitContainerProvider(LibraryInjectionConfig{
		DefaultResourceRequirements: map[corev1.ResourceName]resource.Quantity{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
	})

	requirements, shouldSkip, _ := provider.computeResourceRequirements(pod)

	assert.False(t, shouldSkip)
	assert.Equal(t, "100m", requirements.Limits.Cpu().String())
	assert.Equal(t, "128Mi", requirements.Limits.Memory().String())
}

func TestInjectInjector(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app"},
			},
		},
	}

	provider := newInitContainerProvider(LibraryInjectionConfig{})
	result := provider.injectInjector(pod, InjectorConfig{
		ResolvedImage: ResolvedImage{Image: "gcr.io/datadoghq/apm-inject:latest"},
	})

	require.Equal(t, mutationStatusInjected, result.status)

	// Check init container was added
	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, injectorInitContainerName, pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "gcr.io/datadoghq/apm-inject:latest", pod.Spec.InitContainers[0].Image)

	// Check volumes were added
	require.Len(t, pod.Spec.Volumes, 2)
	assert.Equal(t, instrumentationVolumeName, pod.Spec.Volumes[0].Name)
	assert.Equal(t, etcVolumeName, pod.Spec.Volumes[1].Name)

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

	provider := newInitContainerProvider(LibraryInjectionConfig{})
	result := provider.injectLibrary(pod, LibraryConfig{
		Language:      "java",
		ResolvedImage: ResolvedImage{Image: "gcr.io/datadoghq/dd-lib-java-init:latest"},
	})

	require.Equal(t, mutationStatusInjected, result.status)

	// Check init container was added
	require.Len(t, pod.Spec.InitContainers, 1)
	assert.Equal(t, "datadog-lib-java-init", pod.Spec.InitContainers[0].Name)
	assert.Equal(t, "gcr.io/datadoghq/dd-lib-java-init:latest", pod.Spec.InitContainers[0].Image)
}

func TestInjectLibrary_UnsupportedLanguage(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	provider := newInitContainerProvider(LibraryInjectionConfig{})
	result := provider.injectLibrary(pod, LibraryConfig{
		Language:      "cobol",
		ResolvedImage: ResolvedImage{Image: "some-image"},
	})

	assert.Equal(t, mutationStatusError, result.status)
	assert.Contains(t, result.err.Error(), "cobol")
}

func TestInitContainerIsSidecar(t *testing.T) {
	restartAlways := corev1.ContainerRestartPolicyAlways
	restartOnFailure := corev1.ContainerRestartPolicyOnFailure

	tests := []struct {
		name      string
		container *corev1.Container
		isSidecar bool
	}{
		{
			name:      "nil restart policy is not sidecar",
			container: &corev1.Container{Name: "init"},
			isSidecar: false,
		},
		{
			name:      "restartPolicy Always is sidecar",
			container: &corev1.Container{Name: "sidecar", RestartPolicy: &restartAlways},
			isSidecar: true,
		},
		{
			name:      "restartPolicy OnFailure is not sidecar",
			container: &corev1.Container{Name: "init", RestartPolicy: &restartOnFailure},
			isSidecar: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.isSidecar, initContainerIsSidecar(tt.container))
		})
	}
}

func TestIsLanguageSupported(t *testing.T) {
	supportedLangs := []string{"java", "js", "python", "dotnet", "ruby", "php"}
	for _, lang := range supportedLangs {
		assert.True(t, isLanguageSupported(lang), "expected %s to be supported", lang)
	}

	unsupportedLangs := []string{"cobol", "fortran", "go", "rust", ""}
	for _, lang := range unsupportedLangs {
		assert.False(t, isLanguageSupported(lang), "expected %s to be unsupported", lang)
	}
}
