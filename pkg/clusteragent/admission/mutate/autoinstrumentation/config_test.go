// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoinstrumentation_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestNewInstrumentationConfig(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		expected   *autoinstrumentation.InstrumentationConfig
		shouldErr  bool
	}{
		{
			name:       "valid config enabled namespaces",
			configPath: "testdata/enabled_namespaces.yaml",
			shouldErr:  false,
			expected: &autoinstrumentation.InstrumentationConfig{
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
			expected: &autoinstrumentation.InstrumentationConfig{
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
			name:       "both enabled and disabled namespaces",
			configPath: "testdata/both_enabled_and_disabled.yaml",
			shouldErr:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.NewFromFile(t, tt.configPath)
			actual, err := autoinstrumentation.NewInstrumentationConfig(mockConfig)
			if tt.shouldErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}
