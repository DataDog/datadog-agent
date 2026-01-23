// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package appsec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

func TestNewWebhook(t *testing.T) {
	tests := []struct {
		name           string
		config         map[string]any
		expectNil      bool
		expectEnabled  bool
		expectEndpoint string
	}{
		{
			name: "webhook nil when mode is external",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "external",
			},
			expectNil: true,
		},
		{
			name: "webhook nil when mode is not sidecar",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "invalid",
			},
			expectNil: true,
		},
		{
			name: "webhook created for sidecar mode",
			config: map[string]any{
				"cluster_agent.appsec.injector.mode": "sidecar",
			},
			expectNil:      false,
			expectEnabled:  false, // Enabled depends on patterns available
			expectEndpoint: "/appsec-proxies",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := common.FakeConfigWithValues(t, tt.config)
			webhook := NewWebhook(mockConfig)

			if tt.expectNil {
				assert.Nil(t, webhook)
			} else {
				require.NotNil(t, webhook)
				assert.Equal(t, tt.expectEndpoint, webhook.Endpoint())
				assert.Equal(t, webhookName, webhook.Name())
			}
		})
	}
}

func TestWebhookMatchConditions(t *testing.T) {
	mockConfig := common.FakeConfigWithValues(t, map[string]any{
		"cluster_agent.appsec.injector.mode": "sidecar",
	})

	webhook := NewWebhook(mockConfig)
	if webhook == nil {
		t.Skip("Webhook is nil, likely no patterns available")
	}

	conditions := webhook.MatchConditions()

	// Should return at least one condition
	require.NotEmpty(t, conditions)

	// The condition should have a name and expression
	assert.Equal(t, webhookName, conditions[0].Name)
	assert.NotEmpty(t, conditions[0].Expression)

	// The expression should be valid CEL (at minimum it should not be empty)
	t.Logf("Generated CEL expression: %s", conditions[0].Expression)
}
