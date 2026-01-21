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
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"

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

func TestSelectorToCEL(t *testing.T) {
	tests := []struct {
		name           string
		selector       labels.Selector
		labelsExpr     string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "empty selector (matches everything)",
			selector:       labels.NewSelector(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: "true",
			expectError:    false,
		},
		{
			name: "exists operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("app", selection.Exists, nil)
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `("app" in object.metadata.labels)`,
			expectError:    false,
		},
		{
			name: "does not exist operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("app", selection.DoesNotExist, nil)
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(!("app" in object.metadata.labels))`,
			expectError:    false,
		},
		{
			name: "equals operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("app", selection.Equals, []string{"myapp"})
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(("app" in object.metadata.labels) && object.metadata.labels["app"] == "myapp")`,
			expectError:    false,
		},
		{
			name: "not equals operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("app", selection.NotEquals, []string{"myapp"})
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(!("app" in object.metadata.labels) || object.metadata.labels["app"] != "myapp")`,
			expectError:    false,
		},
		{
			name: "in operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("env", selection.In, []string{"prod", "staging"})
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:  "object.metadata.labels",
			expectError: false,
			// Note: The exact order of values in the CEL expression may vary
		},
		{
			name: "multiple requirements (AND)",
			selector: func() labels.Selector {
				req1, _ := labels.NewRequirement("app", selection.Exists, nil)
				req2, _ := labels.NewRequirement("env", selection.Equals, []string{"prod"})
				return labels.NewSelector().Add(*req1, *req2)
			}(),
			labelsExpr:  "object.metadata.labels",
			expectError: false,
		},
		{
			name: "greater than operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("version", selection.GreaterThan, []string{"1"})
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(("version" in object.metadata.labels) && object.metadata.labels["version"].matches('^[0-9]+$') && int(object.metadata.labels["version"]) > 1)`,
			expectError:    false,
		},
		{
			name: "less than operator",
			selector: func() labels.Selector {
				req, _ := labels.NewRequirement("version", selection.LessThan, []string{"10"})
				return labels.NewSelector().Add(*req)
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(("version" in object.metadata.labels) && object.metadata.labels["version"].matches('^[0-9]+$') && int(object.metadata.labels["version"]) < 10)`,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SelectorToCEL(tt.selector, tt.labelsExpr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedResult != "" {
					assert.Equal(t, tt.expectedResult, result)
				} else {
					// At least verify it's not empty and doesn't error
					assert.NotEmpty(t, result)
				}
			}
		})
	}
}

func TestSelectorsToCEL(t *testing.T) {
	tests := []struct {
		name           string
		selectors      []labels.Selector
		labelsExpr     string
		expectedResult string
		expectError    bool
	}{
		{
			name:           "empty selectors list",
			selectors:      []labels.Selector{},
			labelsExpr:     "object.metadata.labels",
			expectedResult: "false",
			expectError:    false,
		},
		{
			name:           "nil selectors in list are skipped",
			selectors:      []labels.Selector{nil, nil},
			labelsExpr:     "object.metadata.labels",
			expectedResult: "false",
			expectError:    false,
		},
		{
			name: "single selector",
			selectors: func() []labels.Selector {
				req, _ := labels.NewRequirement("app", selection.Exists, nil)
				return []labels.Selector{labels.NewSelector().Add(*req)}
			}(),
			labelsExpr:     "object.metadata.labels",
			expectedResult: `(("app" in object.metadata.labels))`,
			expectError:    false,
		},
		{
			name: "multiple selectors (OR)",
			selectors: func() []labels.Selector {
				req1, _ := labels.NewRequirement("app", selection.Equals, []string{"app1"})
				req2, _ := labels.NewRequirement("app", selection.Equals, []string{"app2"})
				return []labels.Selector{
					labels.NewSelector().Add(*req1),
					labels.NewSelector().Add(*req2),
				}
			}(),
			labelsExpr:  "object.metadata.labels",
			expectError: false,
			// Should be OR'ed together
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SelectorsToCEL(tt.selectors, tt.labelsExpr)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.expectedResult != "" {
					assert.Equal(t, tt.expectedResult, result)
				} else {
					// At least verify it's not empty
					assert.NotEmpty(t, result)
				}
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
