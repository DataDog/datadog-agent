// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestParseConfig(t *testing.T) {
	mockConfig := configmock.NewFromFile(t, "testdata/targets.yaml")
	targets, err := ParseConfig(mockConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ensure there is exactly one target.
	require.Len(t, targets, 1)
	target := targets[0]

	// Check the name.
	require.Equal(t, "Billing Service", target.Name)

	// Check the pod selector.
	require.Len(t, target.Selector.MatchLabels, 1)
	require.Equal(t, "billing-service", target.Selector.MatchLabels["app"])
	require.Len(t, target.Selector.MatchExpressions, 1)
	require.Equal(t, "env", target.Selector.MatchExpressions[0].Key)
	require.Equal(t, metav1.LabelSelectorOpIn, target.Selector.MatchExpressions[0].Operator)
	require.Len(t, target.Selector.MatchExpressions[0].Values, 1)
	require.Equal(t, "prod", target.Selector.MatchExpressions[0].Values[0])

	// Check the namespace selector.
	require.Len(t, target.NamespaceSelector.MatchNames, 1)
	require.Equal(t, "billing", target.NamespaceSelector.MatchNames[0])

	// Check the tracer versions.
	require.Len(t, target.TracerVersions, 1)
	require.Equal(t, "default", target.TracerVersions["java"])
}

func TestFilter(t *testing.T) {
	tests := map[string]struct {
		configPath string
		in         *corev1.Pod
		out        []language
	}{
		"a rule without selectors applies as a default": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			out: []language{js},
		},
		"a single service example matches rule": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "billing-service",
					Labels: map[string]string{
						"app": "billing-service",
					},
				},
			},
			out: []language{python},
		},
		"a java microservice service matches rule": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
					Labels: map[string]string{
						"language": "java",
					},
				},
			},
			out: []language{java},
		},
		"a disabled namespace gets no tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "infra",
					Labels: map[string]string{
						"language": "java",
					},
				},
			},
			out: nil,
		},
		"unset tracer versions applies all tracers": {
			configPath: "testdata/filter.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "application",
					Labels: map[string]string{
						"language": "unknown",
					},
				},
			},
			out: []language{java, js, python, dotnet, ruby},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.NewFromFile(t, test.configPath)
			f, err := NewTargetFilter(mockConfig)
			require.NoError(t, err)

			libList := f.Filter(test.in)
			languages := make([]language, 0, len(libList))
			for _, lib := range libList {
				languages = append(languages, lib.lang)
			}
			require.Equal(t, test.out, languages)
		})
	}
}
