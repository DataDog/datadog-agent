// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompileRuleFromJSON(t *testing.T) {
	tests := []struct {
		name        string
		ruleJSON    map[string]interface{}
		expectError bool
	}{
		{
			name: "simple_mask_rule",
			ruleJSON: map[string]interface{}{
				"type":                "mask_sequences",
				"name":                "mask_ssn",
				"token_pattern":       []string{"D3", "Dash", "D2", "Dash", "D4"},
				"prefilter_keywords":  []string{"-"},
				"replace_placeholder": "[SSN_REDACTED]",
			},
			expectError: false,
		},
		{
			name: "rule_with_length_constraints",
			ruleJSON: map[string]interface{}{
				"type":               "mask_sequences",
				"name":               "mask_ipv4",
				"token_pattern":      []string{"DAny", "Period", "DAny", "Period", "DAny", "Period", "DAny"},
				"prefilter_keywords": []string{"."},
				"length_constraints": []map[string]interface{}{
					{"token_index": 0, "min_length": 1, "max_length": 3},
					{"token_index": 2, "min_length": 1, "max_length": 3},
					{"token_index": 4, "min_length": 1, "max_length": 3},
					{"token_index": 6, "min_length": 1, "max_length": 3},
				},
				"replace_placeholder": "[IP_REDACTED]",
			},
			expectError: false,
		},
		{
			name: "exclude_rule",
			ruleJSON: map[string]interface{}{
				"type":               "exclude_at_match",
				"name":               "exclude_healthcheck",
				"token_pattern":      []string{"Fslash", "C6"},
				"prefilter_keywords": []string{"/health"},
			},
			expectError: false,
		},
		{
			name: "real_world_api_key_rule",
			ruleJSON: map[string]interface{}{
				"type":               "mask_sequences",
				"name":               "mask_api_keys",
				"token_pattern":      []string{"C3", "Underscore", "C3", "Equal", "CAny"},
				"prefilter_keywords": []string{"api_key="},
				"length_constraints": []map[string]interface{}{
					{"token_index": 4, "min_length": 26, "max_length": 26},
				},
				"replace_placeholder": "api_key=**************************",
			},
			expectError: false,
		},
		{
			name: "real_world_k8s_filter",
			ruleJSON: map[string]interface{}{
				"type":               "exclude_at_match",
				"name":               "k8s_filter1",
				"token_pattern":      []string{"C8", "Space", "C5", "Space", "C4", "Space", "C6", "Space", "C7", "Space", "CAny"},
				"prefilter_keywords": []string{"mutation"},
				"length_constraints": []map[string]interface{}{
					{"token_index": 10, "min_length": 12, "max_length": 12},
				},
			},
			expectError: false,
		},
		{
			name: "missing_type",
			ruleJSON: map[string]interface{}{
				"name":                "invalid_rule",
				"token_pattern":       []string{"D3"},
				"replace_placeholder": "[REDACTED]",
			},
			expectError: true,
		},
		{
			name: "missing_name",
			ruleJSON: map[string]interface{}{
				"type":                "mask_sequences",
				"token_pattern":       []string{"D3"},
				"replace_placeholder": "[REDACTED]",
			},
			expectError: true,
		},
		{
			name: "missing_token_pattern",
			ruleJSON: map[string]interface{}{
				"type":                "mask_sequences",
				"name":                "invalid_rule",
				"replace_placeholder": "[REDACTED]",
			},
			expectError: true,
		},
		{
			name: "mask_without_placeholder",
			ruleJSON: map[string]interface{}{
				"type":          "mask_sequences",
				"name":          "invalid_mask",
				"token_pattern": []string{"D3"},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule, err := CompileRuleFromJSON(tt.ruleJSON)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, rule)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rule)

				// Verify basic fields
				assert.Equal(t, tt.ruleJSON["type"].(string), rule.Type)
				assert.Equal(t, tt.ruleJSON["name"].(string), rule.Name)

				// Verify token pattern was compiled
				assert.NotEmpty(t, rule.TokenPattern)
				assert.Equal(t, len(tt.ruleJSON["token_pattern"].([]string)), len(rule.TokenPattern))

				// Verify prefilter keywords were compiled
				if pk, ok := tt.ruleJSON["prefilter_keywords"].([]string); ok {
					assert.Equal(t, len(pk), len(rule.PrefilterKeywordsRaw))
				}

				// Verify length constraints were compiled
				if lc, ok := tt.ruleJSON["length_constraints"].([]map[string]interface{}); ok {
					assert.Equal(t, len(lc), len(rule.LengthConstraints))
				}

				// Verify placeholder for mask_sequences
				if rule.Type == "mask_sequences" {
					assert.NotEmpty(t, rule.Placeholder)
				}
			}
		})
	}
}

func TestCompileRuleFromJSON_InterfaceSlices(t *testing.T) {
	// Test with []interface{} (simulating JSON unmarshaling)
	ruleJSON := map[string]interface{}{
		"type": "mask_sequences",
		"name": "test_rule",
		"token_pattern": []interface{}{
			"D3", "Dash", "D2", "Dash", "D4",
		},
		"prefilter_keywords": []interface{}{
			"-",
		},
		"length_constraints": []interface{}{
			map[string]interface{}{
				"token_index": 0,
				"min_length":  1,
				"max_length":  3,
			},
		},
		"replace_placeholder": "[REDACTED]",
	}

	rule, err := CompileRuleFromJSON(ruleJSON)
	require.NoError(t, err)
	require.NotNil(t, rule)

	assert.Equal(t, "mask_sequences", rule.Type)
	assert.Equal(t, "test_rule", rule.Name)
	assert.Equal(t, 5, len(rule.TokenPattern))
	assert.Equal(t, 1, len(rule.PrefilterKeywordsRaw))
	assert.Equal(t, 1, len(rule.LengthConstraints))
}
