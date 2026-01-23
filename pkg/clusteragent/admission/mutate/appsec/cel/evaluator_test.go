// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && cel

package cel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodEvaluator_Matches(t *testing.T) {
	evaluator := NewPodEvaluator()

	tests := []struct {
		name          string
		expression    string
		pod           *corev1.Pod
		expectedMatch bool
		expectError   bool
		errorContains string
	}{
		{
			name:       "label exists - positive",
			expression: "'app' in pod.labels",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "myapp",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "label exists - negative",
			expression: "'app' in pod.labels",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"other": "value",
					},
				},
			},
			expectedMatch: false,
		},
		{
			name:       "label equals - positive",
			expression: "'app' in pod.labels && pod.labels['app'] == 'myapp'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "myapp",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "label equals - negative",
			expression: "'app' in pod.labels && pod.labels['app'] == 'myapp'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "otherapp",
					},
				},
			},
			expectedMatch: false,
		},
		{
			name:       "complex expression with multiple labels",
			expression: "'app' in pod.labels && 'env' in pod.labels && pod.labels['env'] == 'prod'",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "myapp",
						"env": "prod",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "gateway class name label exists",
			expression: "'gateway.networking.k8s.io/gateway-class-name' in pod.labels",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"gateway.networking.k8s.io/gateway-class-name": "istio",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "OR expression - first matches",
			expression: "('app' in pod.labels) || ('env' in pod.labels)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "myapp",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "OR expression - second matches",
			expression: "('app' in pod.labels) || ('env' in pod.labels)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name:       "OR expression - neither matches",
			expression: "('app' in pod.labels) || ('env' in pod.labels)",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"other": "value",
					},
				},
			},
			expectedMatch: false,
		},
		{
			name:          "invalid expression",
			expression:    "invalid syntax {",
			pod:           &corev1.Pod{},
			expectError:   true,
			errorContains: "Syntax error",
		},
		// Note: workloadfilter's CEL doesn't enforce boolean return type at compile time
		// Non-boolean expressions will evaluate but may return Unknown result
		{
			name:       "non-boolean expression",
			expression: "pod.name",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			expectedMatch: false, // Will return Unknown, which maps to false
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := evaluator.Matches(tt.expression, tt.pod)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedMatch, matches)
			}
		})
	}
}

func TestPodEvaluator_Compile(t *testing.T) {
	evaluator := NewPodEvaluator()

	tests := []struct {
		name          string
		expression    string
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid expression",
			expression:  "'app' in pod.labels",
			expectError: false,
		},
		{
			name:          "invalid syntax",
			expression:    "invalid {",
			expectError:   true,
			errorContains: "Syntax error",
		},
		// Note: workloadfilter's CEL doesn't enforce boolean return type at compile time
		{
			name:        "non-boolean expression",
			expression:  "pod.name",
			expectError: false, // Compiles successfully, just won't match pods
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compiled, err := evaluator.Compile(tt.expression)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, compiled)
				assert.Equal(t, tt.expression, compiled.Expression())
			}
		})
	}
}

func TestCompiledMatcher_Matches(t *testing.T) {
	evaluator := NewPodEvaluator()

	expression := "'app' in pod.labels && pod.labels['app'] == 'myapp'"
	compiled, err := evaluator.Compile(expression)
	require.NoError(t, err)
	require.NotNil(t, compiled)

	tests := []struct {
		name          string
		pod           *corev1.Pod
		expectedMatch bool
	}{
		{
			name: "matches",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "myapp",
					},
				},
			},
			expectedMatch: true,
		},
		{
			name: "does not match - different value",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "otherapp",
					},
				},
			},
			expectedMatch: false,
		},
		{
			name: "does not match - missing label",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"env": "prod",
					},
				},
			},
			expectedMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches, err := compiled.Matches(tt.pod)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedMatch, matches)
		})
	}
}

func TestPodEvaluator_MultipleExpressions(t *testing.T) {
	evaluator := NewPodEvaluator()

	// Test that the evaluator can handle multiple different expressions
	expressions := []string{
		"'app' in pod.labels",
		"'env' in pod.labels",
		"pod.namespace == 'default'",
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Labels: map[string]string{
				"app": "myapp",
			},
		},
	}

	// First expression should match
	matches, err := evaluator.Matches(expressions[0], pod)
	require.NoError(t, err)
	assert.True(t, matches)

	// Second expression should not match
	matches, err = evaluator.Matches(expressions[1], pod)
	require.NoError(t, err)
	assert.False(t, matches)

	// Third expression should match
	matches, err = evaluator.Matches(expressions[2], pod)
	require.NoError(t, err)
	assert.True(t, matches)
}
