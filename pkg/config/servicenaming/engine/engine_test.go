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
	engine, err := NewEngine([]Rule{})
	require.NoError(t, err)
	assert.NotNil(t, engine)

	result, err := engine.Evaluate(CELInput{})
	require.NoError(t, err)
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
	engine, err := NewEngine([]Rule{
		{Query: "container['name'] == 'web'", Value: "'first-service'"},
		{Query: "container['name'] == 'web'", Value: "'second-service'"},
		{Query: "true", Value: "'third-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "web"},
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "first-service", result.ServiceName)
	assert.Equal(t, "cel", result.SourceName)
	assert.Equal(t, "0", result.MatchedRule)
}

func TestEvaluate_FirstRuleFails_SecondMatches(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{Query: "container['name'] == 'db'", Value: "'database-service'"},
		{Query: "container['name'] == 'web'", Value: "'web-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "web"},
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "web-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_NoRuleMatches(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{Query: "container['name'] == 'db'", Value: "'database-service'"},
		{Query: "container['name'] == 'cache'", Value: "'cache-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "web"},
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestEvaluate_RuntimeErrorInQuery_RuleSkipped(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{Query: "container.name == 'web'", Value: "'first-service'"}, // Will fail: container is map
		{Query: "true", Value: "'fallback-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: nil,
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "fallback-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_RuntimeErrorInValue_RuleSkipped(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{Query: "true", Value: "container.name"}, // Will fail: container is nil
		{Query: "true", Value: "'fallback-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: nil,
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "fallback-service", result.ServiceName)
	assert.Equal(t, "1", result.MatchedRule)
}

func TestEvaluate_ValueNotEvaluatedIfQueryFalse(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{Query: "false", Value: "container.name"}, // Would fail if evaluated with nil container
		{Query: "true", Value: "'success-service'"},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: nil,
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "success-service", result.ServiceName)
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
			name: "pod owner ref matching",
			rules: []Rule{
				{Query: "pod['ownerref']['kind'] == 'Deployment'", Value: "pod['ownerref']['name']"},
			},
			input: CELInput{
				Pod: map[string]any{
					"ownerref": map[string]any{
						"kind": "Deployment",
						"name": "my-deployment",
					},
				},
			},
			wantService: "my-deployment",
			wantMatched: "0",
		},
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
			name: "process binary name with startsWith",
			rules: []Rule{
				{
					Query: "process['binary']['name'].startsWith('java')",
					Value: "'java-service'",
				},
			},
			input: CELInput{
				Process: map[string]any{
					"binary": map[string]any{
						"name": "java",
						"path": "/usr/bin/java",
					},
				},
			},
			wantService: "java-service",
			wantMatched: "0",
		},
		{
			name: "list size function",
			rules: []Rule{
				{Query: "size(container['ports']) == 2", Value: "'has-two-ports'"},
			},
			input: CELInput{
				Container: map[string]any{
					"ports": []int{8080, 9090},
				},
			},
			wantService: "has-two-ports",
			wantMatched: "0",
		},
		{
			name: "nil pointer access handled gracefully",
			rules: []Rule{
				{Query: "pod != null && pod['name'] == 'test'", Value: "'pod-service'"},
				{Query: "container != null", Value: "'container-service'"},
			},
			input: CELInput{
				Pod:       nil,
				Container: map[string]any{"name": "test"},
			},
			wantService: "container-service",
			wantMatched: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(tt.rules)
			require.NoError(t, err)

			result, err := engine.Evaluate(tt.input)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantService, result.ServiceName)
				assert.Equal(t, "cel", result.SourceName)
				assert.Equal(t, tt.wantMatched, result.MatchedRule)
			}
		})
	}
}

func TestEvaluate_RuleNameInMatchedRule(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{
			Name:  "redis-rule",
			Query: "container['name'] == 'redis'",
			Value: "'redis-service'",
		},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "redis"},
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "redis-service", result.ServiceName)
	assert.Equal(t, "redis-rule", result.MatchedRule) // Name, not index
}

func TestEvaluate_RuleIndexWhenNameEmpty(t *testing.T) {
	engine, err := NewEngine([]Rule{
		{
			Query: "container['name'] == 'redis'",
			Value: "'redis-service'",
		},
	})
	require.NoError(t, err)

	input := CELInput{
		Container: map[string]any{"name": "redis"},
	}

	result, err := engine.Evaluate(input)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "redis-service", result.ServiceName)
	assert.Equal(t, "0", result.MatchedRule) // Index when no name
}
