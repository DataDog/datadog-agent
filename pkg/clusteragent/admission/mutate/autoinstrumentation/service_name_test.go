// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestServiceNameMutatorForMetaAsTags(t *testing.T) {
	testData := []struct {
		name            string
		pod             *corev1.Pod
		podMetaAsTags   podMetaAsTags
		expectedMutator *serviceNameMutator
	}{
		{
			name: "no meta",
			pod:  &corev1.Pod{},
		},
		{
			name: "match label app=service",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "banana"},
				},
			},
			podMetaAsTags: podMetaAsTags{
				Labels: map[string]string{"app": "service"},
			},
			expectedMutator: &serviceNameMutator{
				EnvVar: corev1.EnvVar{
					Name: "DD_SERVICE",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "metadata.labels['app']",
						},
					},
				},
				Source: serviceNameSourceLabelsAsTags,
			},
		},
	}
	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			mutator := serviceNameMutatorForMetaAsTags(tt.pod, tt.podMetaAsTags)
			require.Equal(t, tt.expectedMutator, mutator)
		})
	}
}

func TestFindServiceNameInPod(t *testing.T) {
	envVar := func(k, v string) corev1.EnvVar {
		return corev1.EnvVar{Name: k, Value: v}
	}

	envValueFrom := func(k, fieldPath string) corev1.EnvVar {
		return corev1.EnvVar{
			Name: k,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: fieldPath,
				},
			},
		}
	}

	containerWithEnv := func(name string, env ...corev1.EnvVar) corev1.Container {
		return corev1.Container{Name: name, Env: env}
	}

	makePod := func(cs ...corev1.Container) *corev1.Pod {
		return &corev1.Pod{
			Spec: corev1.PodSpec{
				Containers: cs,
			},
		}
	}

	testData := []struct {
		name     string
		pod      *corev1.Pod
		expected []corev1.EnvVar
	}{
		{
			name:     "one container, no env",
			pod:      makePod(containerWithEnv("c-1")),
			expected: []corev1.EnvVar{},
		},
		{
			name: "one container one env",
			pod: makePod(
				containerWithEnv("c-1", envVar("DD_SERVICE", "banana")),
			),
			expected: []corev1.EnvVar{
				{Name: "DD_SERVICE", Value: "banana"},
			},
		},
		{
			name: "two containers one env",
			pod: makePod(
				containerWithEnv("c-1", envVar("DD_SERVICE", "banana")),
				containerWithEnv("c-2", envVar("DD_SERVICE", "banana")),
			),
			expected: []corev1.EnvVar{
				{Name: "DD_SERVICE", Value: "banana"},
			},
		},
		{
			name: "env from",
			pod: makePod(
				containerWithEnv("c-1", envValueFrom("DD_SERVICE", "some-field")),
				containerWithEnv("c-2", envValueFrom("DD_SERVICE", "some-field")),
			),
			expected: []corev1.EnvVar{
				envValueFrom("DD_SERVICE", "some-field"),
			},
		},
		{
			name: "multiple different sources",
			pod: makePod(
				containerWithEnv("c-1", envValueFrom("DD_SERVICE", "some-field")),
				containerWithEnv("c-2", envVar("DD_SERVICE", "some-name")),
			),
			expected: []corev1.EnvVar{
				envValueFrom("DD_SERVICE", "some-field"),
				envVar("DD_SERVICE", "some-name"),
			},
		},
	}

	for _, tt := range testData {
		t.Run(tt.name, func(t *testing.T) {
			out := findServiceNameEnvVarsInPod(tt.pod)
			require.ElementsMatch(t, tt.expected, out)
		})
	}
}
