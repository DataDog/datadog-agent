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

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestTargetFilter(t *testing.T) {
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
		"a pod that matches no targets gets no values": {
			configPath: "testdata/filter_no_default.yaml",
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
					Labels: map[string]string{
						"app": "frontend",
					},
				},
			},
			out: nil,
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
			// Load the config.
			mockConfig := configmock.NewFromFile(t, test.configPath)
			cfg, err := NewInstrumentationConfig(mockConfig)
			require.NoError(t, err)

			// Create the filter.
			f, err := NewTargetFilter(cfg.Targets, cfg.DisabledNamespaces, "registry")
			require.NoError(t, err)

			// Filter the pod.
			libList := f.filter(test.in)

			// Validate the output.
			languages := convertLibList(libList)
			require.Equal(t, test.out, languages)
		})
	}
}

func convertLibList(libs []libInfo) []language {
	if len(libs) == 0 {
		return nil
	}

	languages := make([]language, len(libs))
	for i, lib := range libs {
		languages[i] = lib.lang
	}

	return languages
}
