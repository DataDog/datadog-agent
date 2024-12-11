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
