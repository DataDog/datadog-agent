// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdIdentifier_SingularForm tests spec uses ad_identifier (singular)
func TestAdIdentifier_SingularForm(t *testing.T) {
	yaml := `ad_identifier:
  - "process.binary.name.startsWith('java')"
  - "container.image.shortname == 'redis'"
service_discovery:
  service_name: "container.name"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err)
	// After normalization, singular form is merged into plural and cleared
	assert.Len(t, config.AdIdentifiers, 2)
	assert.Nil(t, config.AdIdentifier)
	assert.Equal(t, "process.binary.name.startsWith('java')", config.AdIdentifiers[0])
}

// TestAdIdentifier_PluralForm tests ad_identifiers (plural) also works
func TestAdIdentifier_PluralForm(t *testing.T) {
	yaml := `ad_identifiers:
  - "process.binary.name.startsWith('java')"
service_discovery:
  service_name: "container.name"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, config.AdIdentifiers, 1)
}

// TestAdIdentifier_BothForms tests both singular and plural can coexist
func TestAdIdentifier_BothForms(t *testing.T) {
	yaml := `ad_identifier:
  - "process.binary.name.startsWith('java')"
ad_identifiers:
  - "container.image.shortname == 'redis'"
service_discovery:
  service_name: "container.name"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err)
	// After normalization, both are merged into AdIdentifiers
	assert.Len(t, config.AdIdentifiers, 2)
	assert.Nil(t, config.AdIdentifier)
	assert.Contains(t, config.AdIdentifiers, "process.binary.name.startsWith('java')")
	assert.Contains(t, config.AdIdentifiers, "container.image.shortname == 'redis'")
}

// TestAdIdentifier_RequiresServiceDiscovery tests validation
func TestAdIdentifier_RequiresServiceDiscovery(t *testing.T) {
	yaml := `ad_identifier:
  - "process.binary.name.startsWith('java')"
`
	_, err := LoadIntegrationConfig([]byte(yaml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "service_discovery section is required")
}

// TestVersionDefinition_Literal tests version can be literal string
func TestVersionDefinition_Literal(t *testing.T) {
	yaml := `service_discovery:
  service_definitions:
    - query: "true"
      value: "container.name"
  version_definition: "1.0.0"
`
	tmpfile := createTempFile(t, yaml)
	defer os.Remove(tmpfile)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", config.VersionDefinition)
}

// TestVersionDefinition_CEL tests version can be CEL expression
func TestVersionDefinition_CEL(t *testing.T) {
	yaml := `service_discovery:
  service_definitions:
    - query: "true"
      value: "container.name"
  version_definition: "container.image.tag"
`
	tmpfile := createTempFile(t, yaml)
	defer os.Remove(tmpfile)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.Equal(t, "container.image.tag", config.VersionDefinition)
}

// TestQueryBooleanValue tests query: true without quotes
func TestQueryBooleanValue(t *testing.T) {
	yaml := `service_discovery:
  service_definitions:
    - query: true
      value: "container.name"
    - query: false
      value: "container.image.shortname"
`
	tmpfile := createTempFile(t, yaml)
	defer os.Remove(tmpfile)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.Len(t, config.ServiceDefinitions, 2)
	// YAML unmarshals boolean true/false to string "true"/"false"
	assert.Equal(t, "true", config.ServiceDefinitions[0].Query)
	assert.Equal(t, "false", config.ServiceDefinitions[1].Query)

	// Verify they validate as boolean CEL expressions
	err = config.Validate()
	assert.NoError(t, err)
}

// TestLegacyPrioritySorting tests legacy rules are sorted by priority desc
func TestLegacyPrioritySorting(t *testing.T) {
	yaml := `enabled: true
rules:
  - name: "low-priority"
    priority: 10
    expression: "container.name"
  - name: "high-priority"
    priority: 100
    expression: "container.image.name"
  - name: "medium-priority"
    priority: 50
    expression: "container.id"
`
	tmpfile := createTempFile(t, yaml)
	defer os.Remove(tmpfile)

	config, err := LoadAgentConfig(tmpfile)
	require.NoError(t, err)
	assert.Len(t, config.ServiceDefinitions, 3)

	// Rules should be sorted by priority descending (100, 50, 10)
	assert.Equal(t, "container.image.name", config.ServiceDefinitions[0].Value)
	assert.Equal(t, "container.id", config.ServiceDefinitions[1].Value)
	assert.Equal(t, "container.name", config.ServiceDefinitions[2].Value)
}

// TestLegacyDetection_RobustCheck tests improved legacy detection
func TestLegacyDetection_RobustCheck(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		isLegacy bool
	}{
		{
			name: "legacy format",
			yaml: `enabled: true
rules:
  - name: "rule1"
    priority: 100
    expression: "container.name"
`,
			isLegacy: true,
		},
		{
			name: "new format with enabled in different context",
			yaml: `service_discovery:
  service_definitions:
    - query: "true"
      value: "container.name"
some_other_config:
  enabled: true
`,
			isLegacy: false,
		},
		{
			name: "new format",
			yaml: `service_discovery:
  service_definitions:
    - query: "true"
      value: "container.name"
`,
			isLegacy: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpfile := createTempFile(t, tt.yaml)
			defer os.Remove(tmpfile)

			config, err := LoadAgentConfig(tmpfile)
			require.NoError(t, err)

			if tt.isLegacy {
				// Legacy format should have query: "true"
				if len(config.ServiceDefinitions) > 0 {
					assert.Equal(t, "true", config.ServiceDefinitions[0].Query)
				}
			}
		})
	}
}

// TestEvaluator_SingularAdIdentifier tests evaluator with singular form
func TestEvaluator_SingularAdIdentifier(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	// Use LoadIntegrationConfig to trigger normalization
	yaml := `ad_identifier:
  - "container.image.shortname == 'myapp'"
service_discovery:
  service_name: "container.name"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err)

	container := &ContainerCEL{
		Name: "test-container",
		Image: ImageCEL{
			ShortName: "myapp",
		},
	}

	result, err := ev.EvaluateIntegrationConfig(config, nil, container, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-container", result.ServiceName)
}

// TestEvaluator_VersionLiteral tests evaluator with literal version
func TestEvaluator_VersionLiteral(t *testing.T) {
	ev, err := NewEvaluator()
	require.NoError(t, err)

	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: []ServiceDefinition{
			{Query: "true", Value: "container.name"},
		},
		SourceDefinition:  "java",
		VersionDefinition: "1.0.0", // Literal
	}

	container := &ContainerCEL{
		Name: "test-container",
	}

	result, err := ev.EvaluateAgentConfig(config, nil, container, nil)
	require.NoError(t, err)
	assert.Equal(t, "test-container", result.ServiceName)
	assert.Equal(t, "java", result.SourceName)
	assert.Equal(t, "1.0.0", result.Version)
}

// TestSpecExactExample tests exact YAML from spec
func TestSpecExactExample(t *testing.T) {
	// From spec: integration config example
	yaml := `ad_identifier:
  - "process.binary.name.startsWith('java') || process.user != 'root'"
service_discovery:
  service_name: "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]"
  source_name: "java"
  version: "container.image.tag"
`
	config, err := LoadIntegrationConfig([]byte(yaml))
	require.NoError(t, err, "Spec example YAML must parse successfully")
	// After normalization, ad_identifier is merged into AdIdentifiers
	assert.Len(t, config.AdIdentifiers, 1)
	assert.NotNil(t, config.ServiceDiscovery)
	assert.Equal(t, "process.cmd.split(' ')[process.cmd.split(' ').size() - 1]", config.ServiceDiscovery.ServiceName)
	assert.Equal(t, "java", config.ServiceDiscovery.SourceName)
	assert.Equal(t, "container.image.tag", config.ServiceDiscovery.Version)
}
