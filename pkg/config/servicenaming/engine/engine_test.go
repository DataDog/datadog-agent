// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEngine_EmptyRules(t *testing.T) {
	eng, err := NewEngine([]Rule{})
	require.NoError(t, err)
	assert.NotNil(t, eng)

	result := eng.Evaluate(CELInput{})
	assert.Nil(t, result)
}

func TestNewEngine_CompilationErrors(t *testing.T) {
	tests := []struct {
		name    string
		rules   []Rule
		wantErr string
	}{
		{
			name: "empty query",
			rules: []Rule{
				{Query: "", Value: "'service'"},
			},
			wantErr: "query cannot be empty",
		},
		{
			name: "empty value",
			rules: []Rule{
				{Query: "true", Value: ""},
			},
			wantErr: "value cannot be empty",
		},
		{
			name: "invalid query syntax",
			rules: []Rule{
				{Query: "invalid syntax!", Value: "'service'"},
			},
			wantErr: "failed to compile query",
		},
		{
			name: "query not boolean",
			rules: []Rule{
				{Query: "'not a bool'", Value: "'service'"},
			},
			wantErr: "query must return boolean",
		},
		{
			name: "invalid value syntax",
			rules: []Rule{
				{Query: "true", Value: "invalid syntax!"},
			},
			wantErr: "failed to compile value",
		},
		{
			name: "value returns non-string",
			rules: []Rule{
				{Query: "true", Value: "123"},
			},
			wantErr: "value must return string",
		},
		{
			name: "query returns non-boolean",
			rules: []Rule{
				{Query: "123", Value: "'service'"},
			},
			wantErr: "query must return boolean",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewEngine(tt.rules)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestEvaluate_FirstRuleMatches(t *testing.T) {
	eng, err := NewEngine([]Rule{
		{Query: "container['name'] == 'web'", Value: "'first-service'"},
		{Query: "container['name'] == 'web'", Value: "'second-service'"},
		{Query: "true", Value: "'third-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "web"},
	}

	result := eng.Evaluate(input)
	require.NotNil(t, result)
	assert.Equal(t, "first-service", result.ServiceName)
	assert.Equal(t, "0", result.MatchedRule)
}

func TestEvaluate_NoRuleMatches(t *testing.T) {
	eng, err := NewEngine([]Rule{
		{Query: "container['name'] == 'db'", Value: "'database-service'"},
		{Query: "container['name'] == 'cache'", Value: "'cache-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "web"},
	}

	result := eng.Evaluate(input)
	assert.Nil(t, result)
}

func TestEvaluate_RuntimeErrorInQuery_RuleSkipped(t *testing.T) {
	eng, err := NewEngine([]Rule{
		{Query: "container.name == 'web'", Value: "'first-service'"}, // Will fail: container is map
		{Query: "true", Value: "'fallback-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: nil,
	}

	result := eng.Evaluate(input)
	require.NotNil(t, result)
	assert.Equal(t, "fallback-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_ValueNotEvaluatedIfQueryFalse(t *testing.T) {
	eng, err := NewEngine([]Rule{
		{Query: "false", Value: "container.name"}, // Would fail if evaluated with nil container
		{Query: "true", Value: "'success-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: nil,
	}

	result := eng.Evaluate(input)
	require.NotNil(t, result)
	assert.Equal(t, "success-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_EmptyValueSkipped(t *testing.T) {
	eng, err := NewEngine([]Rule{
		{Query: "true", Value: "''"},           // Empty string value should be skipped
		{Query: "true", Value: "'valid-name'"}, // Should match this one
	})
	require.NoError(t, err)

	result := eng.Evaluate(CELInput{})
	require.NotNil(t, result)
	assert.Equal(t, "valid-name", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_ComplexExpressions(t *testing.T) {
	tests := []struct {
		name        string
		rules       []Rule
		input       CELInput
		wantService string
		wantMatched string
		wantNil     bool
	}{
		{
			name: "container labels",
			rules: []Rule{
				{
					Query: "container['labels']['app'] == 'redis'",
					Value: "container['labels']['service']",
				},
			},
			input: CELInput{
				Container: map[string]any{
					"labels": map[string]string{
						"app":     "redis",
						"service": "redis-cache",
					},
				},
			},
			wantService: "redis-cache",
			wantMatched: "0",
		},
		{
			name: "container image shortname",
			rules: []Rule{
				{
					Query: "container['image']['shortname'] == 'nginx'",
					Value: "container['image']['shortname']",
				},
			},
			input: CELInput{
				Container: map[string]any{
					"image": map[string]any{
						"shortname": "nginx",
						"tag":       "latest",
					},
				},
			},
			wantService: "nginx",
			wantMatched: "0",
		},
		{
			name: "port access with full structure",
			rules: []Rule{
				{Query: "size(container['ports']) == 2 && container['ports'][0]['port'] == 8080", Value: "'has-expected-ports'"},
			},
			input: CELInput{
				Container: map[string]any{
					"ports": []map[string]any{
						{"name": "http", "port": 8080, "protocol": "tcp"},
						{"name": "metrics", "port": 9090, "protocol": "tcp"},
					},
				},
			},
			wantService: "has-expected-ports",
			wantMatched: "0",
		},
		{
			name: "nil pointer access handled gracefully",
			rules: []Rule{
				{Query: "container != null && container['name'] == 'test'", Value: "'found-container'"},
				{Query: "container != null", Value: "'container-service'"},
			},
			input: CELInput{
				Container: map[string]any{"name": "other"},
			},
			wantService: "container-service",
			wantMatched: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := NewEngine(tt.rules)
			require.NoError(t, err)

			result := eng.Evaluate(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantService, result.ServiceName)
				assert.Equal(t, tt.wantMatched, result.MatchedRule)
			}
		})
	}
}

// TestEvaluate_QueryRuntimeTypeMismatch tests that when a query expression returns
// a non-boolean at runtime (via DynType), the rule is skipped.
func TestEvaluate_QueryRuntimeTypeMismatch(t *testing.T) {
	// Query compiles as DynType but returns string at runtime
	eng, err := NewEngine([]Rule{
		{
			Query: "container['name']", // Returns string, not bool
			Value: "'should-not-reach'",
		},
		{
			Query: "true",
			Value: "'fallback-service'",
		},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{
			"name": "test-container",
		},
	}

	// First rule should fail runtime type check (returns string, not bool)
	// Fallback should be returned
	result := eng.Evaluate(input)
	require.NotNil(t, result)
	assert.Equal(t, "fallback-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule) // Rule identified by index
}

// TestEvaluate_NilAndMissingFieldAccess tests handling of nil/missing field access and empty containers.
func TestEvaluate_NilAndMissingFieldAccess(t *testing.T) {
	tests := []struct {
		name        string
		rules       []Rule
		input       CELInput
		wantService string
		wantMatched string
		wantNil     bool
	}{
		{
			name: "access missing map key with fallback",
			rules: []Rule{
				{
					Query: "container['nonexistent'] == 'value'",
					Value: "'should-skip'",
				},
				{
					Query: "true",
					Value: "'fallback'",
				},
			},
			input: CELInput{
				Container: map[string]any{
					"name": "test",
				},
			},
			wantService: "fallback",
			wantMatched: "1",
		},
		{
			name: "safe navigation with null check",
			rules: []Rule{
				{
					Query: "container != null && 'labels' in container && 'app' in container['labels']",
					Value: "container['labels']['app']",
				},
				{
					Query: "true",
					Value: "'default'",
				},
			},
			input: CELInput{
				Container: map[string]any{
					"name": "test",
				},
			},
			wantService: "default",
			wantMatched: "1",
		},
		{
			name: "nil container access fails, fallback used",
			rules: []Rule{
				{
					Query: "container['name'] == 'test'",
					Value: "'should-skip'",
				},
				{
					Query: "true",
					Value: "'fallback'",
				},
			},
			input: CELInput{
				Container: nil,
			},
			wantService: "fallback",
			wantMatched: "1",
		},
		{
			name: "empty container map",
			rules: []Rule{
				{
					Query: "container != null && size(container) == 0",
					Value: "'empty-container'",
				},
			},
			input: CELInput{
				Container: map[string]any{},
			},
			wantService: "empty-container",
			wantMatched: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng, err := NewEngine(tt.rules)
			require.NoError(t, err)

			result := eng.Evaluate(tt.input)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantService, result.ServiceName)
				assert.Equal(t, tt.wantMatched, result.MatchedRule)
			}
		})
	}
}
