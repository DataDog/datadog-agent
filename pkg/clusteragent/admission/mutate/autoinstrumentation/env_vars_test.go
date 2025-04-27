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
)

func TestEnvVars(t *testing.T) {
	initial := corev1.EnvVar{
		Name:  "INITIAL",
		Value: "initial",
	}
	testdata := []struct {
		name     string
		mutator  containerMutator
		expected []corev1.EnvVar
		err      bool
	}{
		{
			name: "raw env var works as prepend",
			mutator: envVarMutator(corev1.EnvVar{
				Name: "PREPEND_ME",
			}),
			expected: []corev1.EnvVar{
				{Name: "PREPEND_ME"},
				initial,
			},
		},
		{
			name: "legacy env var appends",
			mutator: envVar{
				key:     "APPEND_ME",
				valFunc: identityValFunc(""),
			},
			expected: []corev1.EnvVar{
				initial,
				{Name: "APPEND_ME"},
			},
		},
		{
			name: "prefer initial values",
			mutator: envVar{
				key:           "INITIAL",
				valFunc:       identityValFunc("new"),
				dontOverwrite: true,
			},
			expected: []corev1.EnvVar{
				initial,
			},
		},
		{
			name: "we can overwrite",
			mutator: envVar{
				key:     "INITIAL",
				valFunc: identityValFunc("new"),
			},
			expected: []corev1.EnvVar{
				{Name: "INITIAL", Value: "new"},
			},
		},
		{
			name: "we can do the default behavior",
			mutator: containerMutators{
				envVarMutator(corev1.EnvVar{
					Name:  "INITIAL",
					Value: "updated",
				}),
				envVarMutator(corev1.EnvVar{
					Name:  "PREPENDED",
					Value: "new",
				}),
			},
			expected: []corev1.EnvVar{
				{Name: "PREPENDED", Value: "new"},
				initial,
			},
		},
		{
			name: "rawEnvVar can overwrite",
			mutator: envVar{
				key: "INITIAL",
				rawEnvVar: &corev1.EnvVar{
					Name: "INITIAL",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "banana",
						},
					},
				},
			},
			expected: []corev1.EnvVar{
				{
					Name: "INITIAL",
					ValueFrom: &corev1.EnvVarSource{
						FieldRef: &corev1.ObjectFieldSelector{
							FieldPath: "banana",
						},
					},
				},
			},
		},
		{
			name: "empty mutation",
			mutator: envVar{
				key: "INITIAL",
			},
			expected: []corev1.EnvVar{
				initial,
			},
		},
	}

	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			c := corev1.Container{
				Name: "container",
				Env:  []corev1.EnvVar{initial},
			}

			err := tt.mutator.mutateContainer(&c)
			require.NoError(t, err)
			require.Equal(t, tt.expected, c.Env)
		})
	}
}
