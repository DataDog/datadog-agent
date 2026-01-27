// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
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
    - query: "pod.metadata.labels['team'] == 'foo'"
      value: "pod.ownerref.name"
    - query: "process.binary.name.startsWith('java')"
      value: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]"
    - query: "true"
      value: "container.image.shortname"
  source_definition: "java"
  version_definition: "container.image.tag"
`
	tmpfile := createTempFile(t, yaml)
	defer os.Remove(tmpfile)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.True(t, config.Enabled)
	assert.True(t, config.IsActive())
	assert.Len(t, config.ServiceDefinitions, 3)
	assert.Equal(t, "pod.metadata.labels['team'] == 'foo'", config.ServiceDefinitions[0].Query)
	assert.Equal(t, "pod.ownerref.name", config.ServiceDefinitions[0].Value)
	assert.Equal(t, "java", config.SourceDefinition)
	assert.Equal(t, "container.image.tag", config.VersionDefinition)
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
	defer os.Remove(tmpfile)

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
			config:   AgentServiceDiscoveryConfig{Enabled: false, ServiceDefinitions: []ServiceDefinition{{Query: "true", Value: "container.name"}}},
			expected: false,
		},
		{
			name:     "enabled with no rules",
			config:   AgentServiceDiscoveryConfig{Enabled: true, ServiceDefinitions: nil},
			expected: false,
		},
		{
			name:     "enabled with rules",
			config:   AgentServiceDiscoveryConfig{Enabled: true, ServiceDefinitions: []ServiceDefinition{{Query: "true", Value: "container.name"}}},
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
			{Query: "", Value: "container.name"},
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
		// Note: "container.name" compiles to DynType and is accepted (runtime validation will ensure it's bool)
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
					{Query: tt.query, Value: "container.name"},
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
			name:  "pod labels comparison",
			query: "pod.metadata.labels['team'] == 'foo'",
			value: "pod.ownerref.name",
		},
		{
			name:  "process binary startsWith",
			query: "process.binary.name.startsWith('java')",
			value: "process.cmd",
		},
		{
			name:  "process OR user condition",
			query: "process.binary.name.startsWith('java') || process.user != 'root'",
			value: "container.name",
		},
		{
			name:  "container image comparison",
			query: "container.image.shortname == 'redis'",
			value: "container.name",
		},
		{
			name:  "cmd split with array index",
			query: "true",
			value: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]",
		},
		{
			name:  "image tag",
			query: "true",
			value: "container.image.tag",
		},
		// UST label expressions (from RFC examples)
		// Note: Use "'key' in map" syntax for map key existence, not has()
		{
			name:  "UST service label from container",
			query: "'tags.datadoghq.com/my-app.service' in container.labels",
			value: "container.labels['tags.datadoghq.com/my-app.service']",
		},
		{
			name:  "UST service label from pod",
			query: "'tags.datadoghq.com/service' in pod.metadata.labels",
			value: "pod.metadata.labels['tags.datadoghq.com/service']",
		},
		{
			name:  "DD_SERVICE env var",
			query: "'DD_SERVICE' in container.envs",
			value: "container.envs['DD_SERVICE']",
		},
		{
			name:  "container ID access",
			query: "container.id != ''",
			value: "container.name",
		},
		{
			name:  "image registry check",
			query: "container.image.registry == 'gcr.io'",
			value: "container.image.shortname",
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

// TestLoadIntegrationConfig_Valid tests loading integration config
func TestLoadIntegrationConfig_Valid(t *testing.T) {
	yaml := `ad_identifier:
  - "process.binary.name.startsWith('java')"
  - "container.image.shortname == 'redis'"
service_discovery:
  service_name: "process.cmd.split(' ')[0]"
  source_name: "java"
  version: "container.image.tag"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, config.AdIdentifier, 2)
	assert.NotNil(t, config.ServiceDiscovery)
	assert.Equal(t, "process.cmd.split(' ')[0]", config.ServiceDiscovery.ServiceName)
	assert.Equal(t, "java", config.ServiceDiscovery.SourceName)
	assert.Equal(t, "container.image.tag", config.ServiceDiscovery.Version)
}

// TestIntegrationConfig_Validate_InvalidAdIdentifier tests validation
func TestIntegrationConfig_Validate_InvalidAdIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		adID        string
		expectedErr string
	}{
		{
			name:        "empty string",
			adID:        "",
			expectedErr: "cannot be empty",
		},
		// Note: "container.name" compiles to DynType and is accepted (runtime validation will ensure it's bool)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &IntegrationConfig{
				AdIdentifier: []string{tt.adID},
			}
			err := config.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

// TestIntegrationConfig_Validate_EmptyServiceName tests validation
func TestIntegrationConfig_Validate_EmptyServiceName(t *testing.T) {
	config := &IntegrationConfig{
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "",
		},
	}
	err := config.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_name")
}

// TestIntegrationConfig_Validate_ValidExamples tests all spec examples
func TestIntegrationConfig_Validate_ValidExamples(t *testing.T) {
	tests := []struct {
		name   string
		config *IntegrationConfig
	}{
		{
			name: "process cmd split",
			config: &IntegrationConfig{
				AdIdentifier: []string{"process.binary.name.startsWith('java')"},
				ServiceDiscovery: &ServiceDiscoverySection{
					ServiceName: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]",
					SourceName:  "java",
					Version:     "container.image.tag",
				},
			},
		},
		{
			name: "container image shortname",
			config: &IntegrationConfig{
				AdIdentifier: []string{"container.image.shortname == 'java'"},
				ServiceDiscovery: &ServiceDiscoverySection{
					ServiceName: "container.name",
				},
			},
		},
		{
			name: "OR condition in ad_identifier",
			config: &IntegrationConfig{
				AdIdentifier: []string{
					"process.binary.name.startsWith('java') || process.user != 'root'",
				},
				ServiceDiscovery: &ServiceDiscoverySection{
					ServiceName: "pod.ownerref.name",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			assert.NoError(t, err)
		})
	}
}

// TestIntegrationConfig_SourceNameLiteral tests source_name as literal
func TestIntegrationConfig_SourceNameLiteral(t *testing.T) {
	config := &IntegrationConfig{
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "container.name",
			SourceName:  "java", // literal string
		},
	}
	err := config.Validate()
	assert.NoError(t, err)
}

// TestIntegrationConfig_SourceNameCEL tests source_name as CEL expression
func TestIntegrationConfig_SourceNameCEL(t *testing.T) {
	config := &IntegrationConfig{
		ServiceDiscovery: &ServiceDiscoverySection{
			ServiceName: "container.name",
			SourceName:  "container.image.shortname", // CEL expression
		},
	}
	err := config.Validate()
	assert.NoError(t, err)
}

// Helper function to create temporary YAML file
func createTempFile(t *testing.T, content string) string {
	tmpdir := t.TempDir()
	tmpfile := filepath.Join(tmpdir, "config.yaml")
	err := os.WriteFile(tmpfile, []byte(content), 0644)
	require.NoError(t, err)
	return tmpfile
}
