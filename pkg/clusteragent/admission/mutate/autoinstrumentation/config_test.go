// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewInstrumentationConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		expected   *InstrumentationConfig
		shouldErr  bool
	}{
		{
			name:       "valid config enabled namespaces",
			configPath: "testdata/enabled_namespaces.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:            true,
				EnabledNamespaces:  []string{"default"},
				DisabledNamespaces: []string{},
				LibVersions: map[string]string{
					"python": "default",
				},
				Version:          "v2",
				InjectorImageTag: "foo",
			},
		},
		{
			name:       "valid config disabled namespaces",
			configPath: "testdata/disabled_namespaces.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:            true,
				EnabledNamespaces:  []string{},
				DisabledNamespaces: []string{"default"},
				LibVersions: map[string]string{
					"python": "default",
				},
				Version:          "v2",
				InjectorImageTag: "foo",
			},
		},
		{
			name:       "valid targets based config",
			configPath: "testdata/targets.yaml",
			shouldErr:  false,
			expected: &InstrumentationConfig{
				Enabled:           true,
				EnabledNamespaces: []string{},
				InjectorImageTag:  "0",
				LibVersions:       map[string]string{},
				Version:           "v2",
				DisabledNamespaces: []string{
					"hacks",
				},
				Targets: []Target{
					{
						Name: "Billing Service",
						PodSelector: PodSelector{
							MatchLabels: map[string]string{
								"app": "billing-service",
							},
							MatchExpressions: []PodSelectorMatchExpression{
								{
									Key:      "env",
									Operator: "In",
									Values:   []string{"prod"},
								},
							},
						},
						NamespaceSelector: NamespaceSelector{
							MatchNames: []string{"billing"},
						},
						TracerVersions: map[string]string{
							"java": "default",
						},
					},
				},
			},
		},
		{
			name:       "both enabled and disabled namespaces",
			configPath: "testdata/both_enabled_and_disabled.yaml",
			shouldErr:  true,
		},
		{
			name:       "both enabled namespaces and targets",
			configPath: "testdata/both_enabled_and_targets.yaml",
			shouldErr:  true,
		},
		{
			name:       "both library versions and targets",
			configPath: "testdata/both_versions_and_targets.yaml",
			shouldErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.NewFromFile(t, tt.configPath)
			actual, err := NewInstrumentationConfig(mockConfig)
			if tt.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}
