// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestFailingInjectionConfig(t *testing.T) {
	tests := []struct {
		name string

		instrumentationEnabled                bool
		enabledNamespaces, disabledNamespaces []string

		expectedFilterError, expectedWebhookError bool
		expectedNamespaces                        map[string]bool
	}{
		{
			name: "disabled",
			expectedNamespaces: map[string]bool{
				"enabled-ns":   false,
				"disabled-ns":  false,
				"any-other-ns": false,
			},
		},
		{
			name:                   "enabled no namespaces",
			instrumentationEnabled: true,
			expectedNamespaces: map[string]bool{
				"enabled-ns":   true,
				"disabled-ns":  true,
				"any-other-ns": true,
			},
		},
		{
			name:                   "enabled with enabled namespace",
			instrumentationEnabled: true,
			enabledNamespaces:      []string{"enabled-ns"},
			expectedNamespaces: map[string]bool{
				"enabled-ns":   true,
				"disabled-ns":  false,
				"any-other-ns": false,
			},
		},
		{
			name:                   "enabled with disabled namespace",
			instrumentationEnabled: true,
			disabledNamespaces:     []string{"disabled-ns"},
			expectedNamespaces: map[string]bool{
				"enabled-ns":   true,
				"disabled-ns":  false,
				"any-other-ns": true,
			},
		},
		{
			name:                   "both enabled and disabled errors, fail closed",
			instrumentationEnabled: true,
			enabledNamespaces:      []string{"enabled-ns"},
			disabledNamespaces:     []string{"disabled-ns"},
			expectedFilterError:    true,
			expectedWebhookError:   true,
			expectedNamespaces: map[string]bool{
				"enabled-ns":   false,
				"disabled-ns":  false,
				"any-other-ns": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			wmeta := common.FakeStoreWithDeployment(t, nil)

			c := mockconfig.New(t)
			c.SetWithoutSource("apm_config.instrumentation.enabled", tt.instrumentationEnabled)
			c.SetWithoutSource("apm_config.instrumentation.enabled_namespaces", tt.enabledNamespaces)
			c.SetWithoutSource("apm_config.instrumentation.disabled_namespaces", tt.disabledNamespaces)

			nsFilter, _ := NewInjectionFilter(c)
			require.NotNil(t, nsFilter, "we should always get a filter")

			_, err := NewWebhook(wmeta, c, nsFilter)
			if tt.expectedWebhookError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			checkedNamespaces := map[string]bool{}
			for ns := range tt.expectedNamespaces {
				checkedNamespaces[ns] = nsFilter.IsNamespaceEligible(ns)
			}

			require.Equal(t, tt.expectedNamespaces, checkedNamespaces)
		})
	}
}
