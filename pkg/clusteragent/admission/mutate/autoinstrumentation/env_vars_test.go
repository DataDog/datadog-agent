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
		mutator  envVar
		expected []corev1.EnvVar
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
				key:     "INITIAL",
				valFunc: useExistingEnvValOr("new"),
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
	}

	for _, tt := range testdata {
		t.Run(tt.name, func(t *testing.T) {
			c := corev1.Container{
				Name: "container",
				Env:  []corev1.EnvVar{initial},
			}
			require.NoError(t, tt.mutator.mutateContainer(&c))
			require.Equal(t, tt.expected, c.Env)
		})
	}
}
