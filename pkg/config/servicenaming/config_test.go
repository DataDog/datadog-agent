// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadAgentConfig_NewFormat tests loading the new query/value format
func TestLoadAgentConfig_NewFormat(t *testing.T) {
	yaml := `service_discovery:
  enabled: true
  service_definitions:
    - name: "redis-from-label"
      query: "container['labels']['app'] == 'redis'"
      value: "container['labels']['service']"
    - query: "container['image']['shortname'].startsWith('nginx')"
      value: "container['image']['shortname']"
    - query: "true"
      value: "container['name']"
`
	tmpfile := createTempFile(t, yaml)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.True(t, config.IsActive())
	assert.Len(t, config.ServiceDefinitions, 3)
	// First rule has a name
	assert.Equal(t, "redis-from-label", config.ServiceDefinitions[0].Name)
	assert.Equal(t, "container['labels']['app'] == 'redis'", config.ServiceDefinitions[0].Query)
	assert.Equal(t, "container['labels']['service']", config.ServiceDefinitions[0].Value)
	// Second rule has no name (optional)
	assert.Equal(t, "", config.ServiceDefinitions[1].Name)
}

// TestLoadAgentConfig_FileNotExists tests graceful handling of missing file
func TestLoadAgentConfig_FileNotExists(t *testing.T) {
	config, err := LoadAgentConfig("/nonexistent/path.yaml")
	require.NoError(t, err)
	assert.Len(t, config.ServiceDefinitions, 0)
}

// TestLoadAgentConfig_EmptyConfig tests empty config is valid
func TestLoadAgentConfig_EmptyConfig(t *testing.T) {
	yaml := `service_discovery:
  service_definitions: []
`
	tmpfile := createTempFile(t, yaml)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.Len(t, config.ServiceDefinitions, 0)
	assert.False(t, config.IsActive(), "empty config should not be active")
}

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

// TestAgentConfig_Validate_EmptyQuery tests validation rejects empty query
func TestAgentConfig_Validate_EmptyQuery(t *testing.T) {
	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "", Value: "container['name']"},
		},
	}
	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query cannot be empty")
}

// TestAgentConfig_Validate_EmptyValue tests validation rejects empty value
func TestAgentConfig_Validate_EmptyValue(t *testing.T) {
	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: ""},
		},
	}
	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "value cannot be empty")
}

// TestAgentConfig_Validate_InvalidQueries tests validation rejects invalid queries
func TestAgentConfig_Validate_InvalidQueries(t *testing.T) {
	tests := []struct {
		name        string
		query       string
		expectedErr string
	}{
		// Note: "container['name']" compiles to DynType and is accepted (runtime validation will ensure it's bool)
		{
			name:        "invalid CEL syntax",
			query:       "invalid syntax {{{",
			expectedErr: "compilation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: tt.query, Value: "container['name']"},
				},
			}
			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

// TestAgentConfig_Validate_ValidExpressions tests query and value expressions compile
func TestAgentConfig_Validate_ValidExpressions(t *testing.T) {
	tests := []struct {
		name  string
		query string
		value string
	}{
		{
			name:  "container labels comparison",
			query: "container['labels']['app'] == 'redis'",
			value: "container['labels']['service']",
		},
		{
			name:  "container image comparison",
			query: "container['image']['shortname'] == 'redis'",
			value: "container['name']",
		},
		{
			name:  "container name with startsWith",
			query: "container['name'].startsWith('web')",
			value: "container['image']['shortname']",
		},
		{
			name:  "image tag",
			query: "true",
			value: "container['image']['tag']",
		},
		// UST label expressions (from RFC examples)
		// Note: Use "'key' in map" syntax for map key existence, not has()
		{
			name:  "UST service label from container",
			query: "'tags.datadoghq.com/my-app.service' in container['labels']",
			value: "container['labels']['tags.datadoghq.com/my-app.service']",
		},
		{
			name:  "DD_SERVICE env var",
			query: "'DD_SERVICE' in container['envs']",
			value: "container['envs']['DD_SERVICE']",
		},
		{
			name:  "container ID access",
			query: "container['id'] != ''",
			value: "container['name']",
		},
		{
			name:  "image registry check",
			query: "container['image']['registry'] == 'gcr.io'",
			value: "container['image']['shortname']",
		},
		{
			name:  "port size check",
			query: "size(container['ports']) > 0",
			value: "container['name']",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AgentServiceDiscoveryConfig{
				ServiceDefinitions: []ServiceDefinition{
					{Query: tt.query, Value: tt.value},
				},
			}
			err := config.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestAgentConfig_CompileEngine tests that CompileEngine creates a working engine
func TestAgentConfig_CompileEngine(t *testing.T) {
	config := &AgentServiceDiscoveryConfig{
		Enabled: true,
		ServiceDefinitions: []ServiceDefinition{
			{Name: "test-rule", Query: "true", Value: "'test-service'"},
		},
	}

	eng, err := config.CompileEngine()
	require.NoError(t, err)
	require.NotNil(t, eng)

	// Verify the engine works
	result := eng.Evaluate(context.Background(), ToEngineInput(CELInput{}))
	require.NotNil(t, result)
	assert.Equal(t, "test-service", result.ServiceName)
	assert.Equal(t, "test-rule", result.MatchedRule) // Name is passed through
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
				map[string]interface{}{
					"name":  "redis-rule",
					"query": "container['labels']['app'] == 'redis'",
					"value": "container['labels']['service']",
				},
				map[string]interface{}{
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
	assert.Equal(t, "redis-rule", config.ServiceDefinitions[0].Name)
	assert.Equal(t, "container['labels']['app'] == 'redis'", config.ServiceDefinitions[0].Query)
	assert.Equal(t, "", config.ServiceDefinitions[1].Name) // No name
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

// TestLoadFromReader_InvalidQuery tests validation of invalid CEL queries
func TestLoadFromReader_InvalidQuery(t *testing.T) {
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": true,
			"service_discovery.service_definitions": []interface{}{
				map[string]interface{}{
					"query": "invalid syntax {{{",
					"value": "'test'",
				},
			},
		},
	}

	_, err := loadFromReader(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "compilation error")
}

// TestLoadFromReader_DisabledSkipsValidation tests that validation is skipped when disabled.
// This ensures invalid rules in disabled config don't block agent startup.
func TestLoadFromReader_DisabledSkipsValidation(t *testing.T) {
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": false,
			"service_discovery.service_definitions": []interface{}{
				map[string]interface{}{
					"query": "invalid syntax {{{", // This would fail validation if enabled
					"value": "'test'",
				},
			},
		},
	}

	// Should succeed because validation is skipped when disabled
	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.False(t, config.Enabled)
	assert.Len(t, config.ServiceDefinitions, 1)
}

// TestLoadFromReader_SliceOfMapsStringInterface tests handling of []map[string]interface{} input
func TestLoadFromReader_SliceOfMapsStringInterface(t *testing.T) {
	// Some config sources may return []map[string]interface{} instead of []interface{}
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": true,
			"service_discovery.service_definitions": []map[string]interface{}{
				{
					"name":  "rule1",
					"query": "true",
					"value": "'service1'",
				},
			},
		},
	}

	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.Len(t, config.ServiceDefinitions, 1)
	assert.Equal(t, "rule1", config.ServiceDefinitions[0].Name)
}

// TestLoadFromReader_SliceOfMapsInterfaceInterface tests handling of []map[interface{}]interface{} input
func TestLoadFromReader_SliceOfMapsInterfaceInterface(t *testing.T) {
	// YAML parsers can produce []map[interface{}]interface{} in some cases
	cfg := &mockConfigReader{
		values: map[string]interface{}{
			"service_discovery.enabled": true,
			"service_discovery.service_definitions": []map[interface{}]interface{}{
				{
					"name":  "rule2",
					"query": "true",
					"value": "'service2'",
				},
			},
		},
	}

	config, err := loadFromReader(cfg)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.Len(t, config.ServiceDefinitions, 1)
	assert.Equal(t, "rule2", config.ServiceDefinitions[0].Name)
}

// Helper function to create temporary YAML file
func createTempFile(t *testing.T, content string) string {
	tmpdir := t.TempDir()
	tmpfile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(tmpfile, []byte(content), 0644)
	require.NoError(t, err)
	return tmpfile
}
