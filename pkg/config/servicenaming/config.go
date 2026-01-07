// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides configuration loading and validation for service discovery.
package servicenaming

import (
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
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
	AdIdentifier     []string                 `yaml:"ad_identifier,omitempty"` // From spec (singular name, plural value)
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
		Enabled          *bool            `yaml:"enabled"`
		Rules            []map[string]any `yaml:"rules"`
		ServiceDiscovery map[string]any   `yaml:"service_discovery"`
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

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// Validate checks the integration configuration for errors
func (c *IntegrationConfig) Validate() error {
	// Validate ad_identifier
	for i, adID := range c.AdIdentifier {
		if adID == "" {
			return fmt.Errorf("ad_identifier[%d]: cannot be empty", i)
		}
		if err := validateCELBooleanExpression(adID); err != nil {
			return fmt.Errorf("ad_identifier[%d]: %w", i, err)
		}
	}

	// If ad_identifier exists, service_discovery section is required
	if len(c.AdIdentifier) > 0 && c.ServiceDiscovery == nil {
		return errors.New("service_discovery section is required when ad_identifier is present")
	}

	// Validate service_discovery section
	if c.ServiceDiscovery != nil {
		sd := c.ServiceDiscovery
		if sd.ServiceName == "" {
			return errors.New("service_discovery.service_name: cannot be empty")
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

// validateCELBooleanExpression validates that an expression compiles and returns boolean
func validateCELBooleanExpression(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// Check return type
	if ast.OutputType() != cel.BoolType {
		return fmt.Errorf("expression must return boolean, got %v", ast.OutputType())
	}

	return nil
}

// validateCELStringExpression validates that an expression compiles and returns string
func validateCELStringExpression(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// For DynType, we can't check output type statically
	// Accept dyn or string
	outType := ast.OutputType()
	if outType != cel.StringType && outType != types.DynType {
		return fmt.Errorf("expression must return string, got %v", outType)
	}

	return nil
}

// validateCELStringExpressionOrLiteral validates expression or accepts literal
func validateCELStringExpressionOrLiteral(expr string) error {
	env, err := createCELEnvironment()
	if err != nil {
		return err
	}

	_, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		// Not valid CEL → treat as literal, which is OK
		return nil
	}

	// Valid CEL → no further validation needed
	return nil
}

// createCELEnvironment creates the CEL environment for validation
func createCELEnvironment() (*cel.Env, error) {
	env, err := cel.NewEnv(
		// Declare variables as DynType for flexibility with field aliasing
		cel.Variable("process", cel.DynType),
		cel.Variable("container", cel.DynType),
		cel.Variable("pod", cel.DynType),

		// Enable standard CEL string extensions (split, startsWith, etc.)
		ext.Strings(),
	)
	if err != nil {
		return nil, err
	}

	return env, nil
}
