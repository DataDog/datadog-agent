// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/)
// Copyright 2016-present Datadog, Inc.

package servicenaming

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// AgentServiceDiscoveryConfig is the global agent-level service discovery configuration
type AgentServiceDiscoveryConfig struct {
	ServiceDefinitions []ServiceDefinition `yaml:"service_definitions"`
	SourceDefinition   string              `yaml:"source_definition"`
	VersionDefinition  string              `yaml:"version_definition"`
}

// ServiceDefinition represents a query/value pair for service name evaluation
type ServiceDefinition struct {
	Query string `yaml:"query"` // CEL boolean expression
	Value string `yaml:"value"` // CEL string expression
}

// IntegrationConfig represents service discovery configuration in autoconf.yaml
type IntegrationConfig struct {
	AdIdentifiers    []string                 `yaml:"ad_identifiers,omitempty"`
	AdIdentifier     []string                 `yaml:"ad_identifier,omitempty"` // Singular form from spec
	ServiceDiscovery *ServiceDiscoverySection `yaml:"service_discovery"`
}

// ServiceDiscoverySection contains service discovery fields in integration config
type ServiceDiscoverySection struct {
	ServiceName string `yaml:"service_name"` // CEL string expression
	SourceName  string `yaml:"source_name"`  // literal or CEL expression
	Version     string `yaml:"version"`      // CEL string expression
}

// LegacyConfig represents the old format for backward compatibility
type LegacyConfig struct {
	Enabled bool         `yaml:"enabled"`
	Rules   []LegacyRule `yaml:"rules"`
}

// LegacyRule represents a single rule in the old format
type LegacyRule struct {
	Name       string `yaml:"name"`
	Priority   int    `yaml:"priority"`
	Expression string `yaml:"expression"`
}

// LoadAgentConfig loads the agent-level service discovery configuration
func LoadAgentConfig(path string) (*AgentServiceDiscoveryConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &AgentServiceDiscoveryConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Try to detect legacy format first (has 'enabled' and 'rules', no 'service_discovery')
	var legacyCheck struct {
		Enabled          *bool                `yaml:"enabled"`
		Rules            []map[string]any     `yaml:"rules"`
		ServiceDiscovery map[string]any       `yaml:"service_discovery"`
	}
	if err := yaml.Unmarshal(data, &legacyCheck); err == nil &&
	   legacyCheck.Enabled != nil &&
	   len(legacyCheck.Rules) > 0 &&
	   legacyCheck.ServiceDiscovery == nil {
		// Legacy format detected
		return loadLegacyConfig(data)
	}

	// New format
	var wrapper struct {
		ServiceDiscovery AgentServiceDiscoveryConfig `yaml:"service_discovery"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	config := &wrapper.ServiceDiscovery
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// loadLegacyConfig converts legacy format to new format
func loadLegacyConfig(data []byte) (*AgentServiceDiscoveryConfig, error) {
	var legacy LegacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("failed to parse legacy YAML: %w", err)
	}

	if !legacy.Enabled {
		return &AgentServiceDiscoveryConfig{}, nil
	}

	// Sort rules by priority (descending) to maintain legacy behavior
	sortedRules := make([]LegacyRule, len(legacy.Rules))
	copy(sortedRules, legacy.Rules)
	sort.Slice(sortedRules, func(i, j int) bool {
		return sortedRules[i].Priority > sortedRules[j].Priority
	})

	// Convert to new format
	config := &AgentServiceDiscoveryConfig{
		ServiceDefinitions: make([]ServiceDefinition, len(sortedRules)),
	}

	for i, rule := range sortedRules {
		config.ServiceDefinitions[i] = ServiceDefinition{
			Query: "true", // Legacy rules always match
			Value: rule.Expression,
		}
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid legacy configuration: %w", err)
	}

	return config, nil
}

// Validate checks the agent configuration for errors
func (c *AgentServiceDiscoveryConfig) Validate() error {
	if len(c.ServiceDefinitions) == 0 {
		return nil // Empty config is valid (disabled)
	}

	for i, def := range c.ServiceDefinitions {
		if def.Query == "" {
			return fmt.Errorf("service_definition[%d]: query cannot be empty", i)
		}
		if def.Value == "" {
			return fmt.Errorf("service_definition[%d]: value cannot be empty", i)
		}

		// Validate query compiles as boolean
		if err := validateCELBooleanExpression(def.Query); err != nil {
			return fmt.Errorf("service_definition[%d]: invalid query: %w", i, err)
		}

		// Validate value compiles as string
		if err := validateCELStringExpression(def.Value); err != nil {
			return fmt.Errorf("service_definition[%d]: invalid value: %w", i, err)
		}
	}

	// Validate source_definition if present
	if c.SourceDefinition != "" {
		if err := validateCELStringExpressionOrLiteral(c.SourceDefinition); err != nil {
			return fmt.Errorf("source_definition: %w", err)
		}
	}

	// Validate version_definition if present (can be literal or CEL)
	if c.VersionDefinition != "" {
		if err := validateCELStringExpressionOrLiteral(c.VersionDefinition); err != nil {
			return fmt.Errorf("version_definition: %w", err)
		}
	}

	return nil
}

// LoadIntegrationConfig loads an integration configuration from autoconf.yaml
func LoadIntegrationConfig(data []byte) (*IntegrationConfig, error) {
	var config IntegrationConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Normalize: merge ad_identifier (singular) into AdIdentifiers (plural)
	if len(config.AdIdentifier) > 0 {
		config.AdIdentifiers = append(config.AdIdentifiers, config.AdIdentifier...)
		config.AdIdentifier = nil // Clear singular form after merge
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// Validate checks the integration configuration for errors
func (c *IntegrationConfig) Validate() error {
	// Validate ad_identifiers (already normalized in LoadIntegrationConfig)
	for i, adID := range c.AdIdentifiers {
		if adID == "" {
			return fmt.Errorf("ad_identifier[%d]: cannot be empty", i)
		}
		if err := validateCELBooleanExpression(adID); err != nil {
			return fmt.Errorf("ad_identifier[%d]: %w", i, err)
		}
	}

	// If ad_identifiers exist, service_discovery section is required
	if len(c.AdIdentifiers) > 0 && c.ServiceDiscovery == nil {
		return fmt.Errorf("service_discovery section is required when ad_identifier is present")
	}

	// Validate service_discovery section
	if c.ServiceDiscovery != nil {
		sd := c.ServiceDiscovery
		if sd.ServiceName == "" {
			return fmt.Errorf("service_discovery.service_name: cannot be empty")
		}
		if err := validateCELStringExpression(sd.ServiceName); err != nil {
			return fmt.Errorf("service_discovery.service_name: %w", err)
		}

		if sd.SourceName != "" {
			if err := validateCELStringExpressionOrLiteral(sd.SourceName); err != nil {
				return fmt.Errorf("service_discovery.source_name: %w", err)
			}
		}

		if sd.Version != "" {
			if err := validateCELStringExpressionOrLiteral(sd.Version); err != nil {
				return fmt.Errorf("service_discovery.version: %w", err)
			}
		}
	}

	return nil
}
