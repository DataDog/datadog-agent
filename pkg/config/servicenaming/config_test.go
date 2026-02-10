// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAgentConfig_IsActive tests the IsActive method
func TestAgentConfig_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		config   AgentServiceDiscoveryConfig
		expected bool
	}{
		{
			name:     "disabled with no rules",
			config:   AgentServiceDiscoveryConfig{Enabled: false, ServiceDefinitions: nil},
			expected: false,
		},
		{
			name:     "disabled with rules",
			config:   AgentServiceDiscoveryConfig{Enabled: false, ServiceDefinitions: []ServiceDefinition{{Query: "true", Value: "container['name']"}}},
			expected: false,
		},
		{
			name:     "enabled with no rules",
			config:   AgentServiceDiscoveryConfig{Enabled: true, ServiceDefinitions: nil},
			expected: false,
		},
		{
			name:     "enabled with rules",
			config:   AgentServiceDiscoveryConfig{Enabled: true, ServiceDefinitions: []ServiceDefinition{{Query: "true", Value: "container['name']"}}},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.config.IsActive())
		})
	}
}

// TestAgentConfig_CompileEngine tests that CompileEngine creates a working engine
func TestAgentConfig_CompileEngine(t *testing.T) {
	config := &AgentServiceDiscoveryConfig{
		Enabled: true,
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: "'test-service'"},
		},
	}

	eng, err := config.CompileEngine()
	require.NoError(t, err)
	require.NotNil(t, eng)

	// Verify the engine works
	result := eng.Evaluate(ToEngineInput(CELInput{}))
	require.NotNil(t, result)
	assert.Equal(t, "test-service", result.ServiceName)
	assert.Equal(t, "0", result.MatchedRule) // Rule identified by index
}

// TestAgentConfig_CompileEngine_Inactive tests that CompileEngine returns nil when inactive
func TestAgentConfig_CompileEngine_Inactive(t *testing.T) {
	config := &AgentServiceDiscoveryConfig{
		Enabled:            false,
		ServiceDefinitions: []ServiceDefinition{{Query: "true", Value: "'test'"}},
	}

	eng, err := config.CompileEngine()
	require.NoError(t, err)
	assert.Nil(t, eng) // Inactive config returns nil engine, not an error
}

// mockConfigReader is a minimal mock that implements only the methods needed for testing.
// It satisfies the internal configReader interface (GetBool, Get).
type mockConfigReader struct {
	values map[string]interface{}
}

func (m *mockConfigReader) GetBool(key string) bool {
	if v, ok := m.values[key].(bool); ok {
		return v
	}
	return false
}

func (m *mockConfigReader) Get(key string) interface{} {
	return m.values[key]
}

// TestLoadFromReader tests loading config using the internal loadFromReader function
func TestLoadFromReader(t *testing.T) {
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": true,
			"service_discovery.service_definitions": []interface{}{
				map[interface{}]interface{}{
					"query": "container['labels']['app'] == 'redis'",
					"value": "container['labels']['service']",
				},
				map[interface{}]interface{}{
					"query": "true",
					"value": "container['name']",
				},
			},
		},
	}

	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.True(t, config.IsActive())
	assert.Len(t, config.ServiceDefinitions, 2)
	assert.Equal(t, "container['labels']['app'] == 'redis'", config.ServiceDefinitions[0].Query)
	assert.Equal(t, "container['labels']['service']", config.ServiceDefinitions[0].Value)
}

// TestLoadFromReader_Disabled tests disabled config
func TestLoadFromReader_Disabled(t *testing.T) {
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": false,
		},
	}

	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.False(t, config.Enabled)
	assert.False(t, config.IsActive())
}

// TestLoadFromReader_DisabledSkipsValidation tests that config loading succeeds when disabled.
// This ensures invalid CEL syntax in disabled config doesn't block agent startup.
func TestLoadFromReader_DisabledSkipsValidation(t *testing.T) {
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": false,
			"service_discovery.service_definitions": []interface{}{
				map[interface{}]interface{}{
					"query": "invalid syntax {{{", // This would fail engine compilation if enabled
					"value": "'test'",
				},
			},
		},
	}

	// Should succeed because engine compilation is skipped when disabled
	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.False(t, config.Enabled)
	assert.Len(t, config.ServiceDefinitions, 1)
}
