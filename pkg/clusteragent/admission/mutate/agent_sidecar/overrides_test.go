// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestWithEnvOverrides(t *testing.T) {
	tests := []struct {
		name                   string
		baseContainer          *corev1.Container
		extraEnvs              []corev1.EnvVar
		expectError            bool
		expectMutated          bool
		containerAfterOverride *corev1.Container
	}{
		{
			name:          "Nil container",
			baseContainer: nil,
			extraEnvs:     []corev1.EnvVar{{Name: "Foo", Value: "Bar"}},
			expectError:   true,
			expectMutated: false,
		},
		{
			name: "Happy path - override existing environment variable",
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "Foo", Value: "Bar"}},
			},
			extraEnvs:     []corev1.EnvVar{{Name: "Foo", Value: "NewBar"}},
			expectError:   false,
			expectMutated: true,
			containerAfterOverride: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "Foo", Value: "NewBar"}},
			},
		},
		{
			name: "Happy path - add a new environment variable",
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "Foo", Value: "Bar"}},
			},
			extraEnvs:     []corev1.EnvVar{{Name: "NewFoo", Value: "Bar"}},
			expectError:   false,
			expectMutated: true,
			containerAfterOverride: &corev1.Container{
				Env: []corev1.EnvVar{{Name: "Foo", Value: "Bar"}, {Name: "NewFoo", Value: "Bar"}},
			},
		},
		{
			name: "No overrides",
			baseContainer: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "Foo", Value: "Bar"},
				},
			},
			extraEnvs: []corev1.EnvVar{
				{Name: "Foo", Value: "Bar"},
			},
			expectError:   false,
			expectMutated: false,
			containerAfterOverride: &corev1.Container{
				Env: []corev1.EnvVar{
					{Name: "Foo", Value: "Bar"},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mutated, err := withEnvOverrides(test.baseContainer, test.extraEnvs...)
			assert.Equal(tt, test.expectMutated, mutated)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
				assert.Equal(tt, test.containerAfterOverride, test.baseContainer)
			}

		})
	}
}

func TestWithResourceLimits(t *testing.T) {
	tests := []struct {
		name                 string
		baseContainer        *corev1.Container
		resourceLimits       corev1.ResourceRequirements
		expectError          bool
		containerAfterLimits *corev1.Container
	}{
		{
			name:          "Nil container",
			baseContainer: nil,
			resourceLimits: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
				Requests: corev1.ResourceList{"cpu": resource.MustParse("50m")},
			},
			expectError: true,
		},
		{
			name: "Happy path - apply resource limits",
			baseContainer: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("100m")},
				},
			},
			resourceLimits: corev1.ResourceRequirements{
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
				Requests: corev1.ResourceList{"cpu": resource.MustParse("50m")},
			},
			expectError: false,
			containerAfterLimits: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("100m")},
					Requests: corev1.ResourceList{"cpu": resource.MustParse("50m")},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			err := withResourceLimits(test.baseContainer, test.resourceLimits)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
				assert.Equal(tt, test.containerAfterLimits, test.baseContainer)
			}

		})
	}
}

func TestParseAnnotationResourceOverrides(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    *corev1.ResourceRequirements
		expectError bool
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    nil,
			expectError: false,
		},
		{
			name:        "no resource annotations",
			annotations: map[string]string{"some-other-annotation": "value"},
			expected:    nil,
			expectError: false,
		},
		{
			name: "all four annotations set",
			annotations: map[string]string{
				annotationSidecarCPURequest:    "100m",
				annotationSidecarCPULimit:      "500m",
				annotationSidecarMemoryRequest: "256Mi",
				annotationSidecarMemoryLimit:   "1Gi",
			},
			expected: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("100m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expectError: false,
		},
		{
			name: "memory limit only - request should equal limit",
			annotations: map[string]string{
				annotationSidecarMemoryLimit: "1Gi",
			},
			expected: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expectError: false,
		},
		{
			name: "cpu request only - limit should equal request",
			annotations: map[string]string{
				annotationSidecarCPURequest: "200m",
			},
			expected: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("200m"),
				},
			},
			expectError: false,
		},
		{
			name: "memory only - cpu untouched",
			annotations: map[string]string{
				annotationSidecarMemoryRequest: "512Mi",
				annotationSidecarMemoryLimit:   "1Gi",
			},
			expected: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("512Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expectError: false,
		},
		{
			name: "cpu request exceeds limit - error",
			annotations: map[string]string{
				annotationSidecarCPURequest: "1",
				annotationSidecarCPULimit:   "500m",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "memory request exceeds limit - error",
			annotations: map[string]string{
				annotationSidecarMemoryRequest: "2Gi",
				annotationSidecarMemoryLimit:   "1Gi",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid cpu value - error",
			annotations: map[string]string{
				annotationSidecarCPURequest: "abc",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid memory value - error",
			annotations: map[string]string{
				annotationSidecarMemoryLimit: "not-a-quantity",
			},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "negative cpu value - error",
			annotations: map[string]string{annotationSidecarCPURequest: "-100m"},
			expected:    nil,
			expectError: true,
		},
		{
			name:        "negative memory value - error",
			annotations: map[string]string{annotationSidecarMemoryLimit: "-256Mi"},
			expected:    nil,
			expectError: true,
		},
		{
			name: "request equals limit - valid",
			annotations: map[string]string{
				annotationSidecarCPURequest:    "500m",
				annotationSidecarCPULimit:      "500m",
				annotationSidecarMemoryRequest: "1Gi",
				annotationSidecarMemoryLimit:   "1Gi",
			},
			expected: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			},
			expectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			result, err := parseAnnotationResourceOverrides(test.annotations)

			if test.expectError {
				assert.Error(tt, err)
				assert.Nil(tt, result)
			} else {
				assert.NoError(tt, err)
				assert.Equal(tt, test.expected, result)
			}
		})
	}
}

func TestApplyAnnotationResourceOverrides(t *testing.T) {
	tests := []struct {
		name                   string
		pod                    *corev1.Pod
		container              *corev1.Container
		expectMutated          bool
		expectError            bool
		containerAfterOverride *corev1.Container
	}{
		{
			name:        "nil pod",
			pod:         nil,
			container:   &corev1.Container{},
			expectError: true,
		},
		{
			name: "nil container",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			container:   nil,
			expectError: true,
		},
		{
			name: "no annotations - no mutation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "test"},
			},
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expectMutated: false,
			expectError:   false,
			containerAfterOverride: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
		},
		{
			name: "memory annotation overrides memory, cpu unchanged",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						annotationSidecarMemoryLimit: "1Gi",
					},
				},
			},
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expectMutated: true,
			expectError:   false,
			containerAfterOverride: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		},
		{
			name: "invalid annotation - no mutation, no error (graceful fallback)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						annotationSidecarMemoryRequest: "2Gi",
						annotationSidecarMemoryLimit:   "1Gi",
					},
				},
			},
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expectMutated: false,
			expectError:   false,
			containerAfterOverride: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
		},
		{
			name: "all four annotations override everything",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
					Annotations: map[string]string{
						annotationSidecarCPURequest:    "100m",
						annotationSidecarCPULimit:      "1",
						annotationSidecarMemoryRequest: "512Mi",
						annotationSidecarMemoryLimit:   "2Gi",
					},
				},
			},
			container: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expectMutated: true,
			expectError:   false,
			containerAfterOverride: &corev1.Container{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mutated, err := applyAnnotationResourceOverrides(test.pod, test.container)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
				assert.Equal(tt, test.expectMutated, mutated)
				assert.Equal(tt, test.containerAfterOverride, test.container)
			}
		})
	}
}

func TestWithSecurityContextOverrides(t *testing.T) {
	tests := []struct {
		name                   string
		baseContainer          *corev1.Container
		securityOverride       *corev1.SecurityContext
		expectError            bool
		expectMutated          bool
		containerAfterOverride *corev1.Container
	}{
		{
			name:                   "nil container",
			baseContainer:          nil,
			securityOverride:       &corev1.SecurityContext{},
			expectError:            true,
			expectMutated:          false,
			containerAfterOverride: nil,
		},
		{
			name: "no overrides",
			baseContainer: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:              pointer.Ptr(int64(1000)),
					ReadOnlyRootFilesystem: pointer.Ptr(true),
				},
			},
			securityOverride: nil,
			expectError:      false,
			expectMutated:    false,
			containerAfterOverride: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:              pointer.Ptr(int64(1000)),
					ReadOnlyRootFilesystem: pointer.Ptr(true),
				},
			},
		},
		{
			name: "apply overrides",
			baseContainer: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem: pointer.Ptr(true),
				},
			},
			securityOverride: &corev1.SecurityContext{
				RunAsUser:              pointer.Ptr(int64(1000)),
				ReadOnlyRootFilesystem: pointer.Ptr(false),
			},
			expectError:   false,
			expectMutated: true,
			containerAfterOverride: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:              pointer.Ptr(int64(1000)),
					ReadOnlyRootFilesystem: pointer.Ptr(false),
				},
			},
		},
		{
			name: "apply blank overrides",
			baseContainer: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{
					RunAsUser:              pointer.Ptr(int64(1000)),
					ReadOnlyRootFilesystem: pointer.Ptr(true),
				},
			},
			securityOverride: &corev1.SecurityContext{},
			expectError:      false,
			expectMutated:    true,
			containerAfterOverride: &corev1.Container{
				SecurityContext: &corev1.SecurityContext{},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			mutated, err := withSecurityContextOverrides(test.baseContainer, test.securityOverride)

			assert.Equal(tt, test.expectMutated, mutated)
			assert.Equal(tt, test.containerAfterOverride, test.baseContainer)

			if test.expectError {
				assert.Error(tt, err)
			} else {
				assert.NoError(tt, err)
			}

		})
	}
}
