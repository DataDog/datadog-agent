// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides configuration loading and validation for service discovery.
package servicenaming

import (
	"fmt"
	"os"

	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"
)

// Note: We previously cached the CEL environment using sync.Once, but this had a critical flaw:
// if CreateCELEnvironment() failed on the first call, the error was cached permanently.
// Since CreateCELEnvironment() is lightweight (just creates a CEL env with extensions),
// we now create it fresh each time to avoid permanent error caching.

// AgentServiceDiscoveryConfig is the global agent-level service discovery configuration.
// This feature is opt-in: it only activates when Enabled is true and ServiceDefinitions are present.
type AgentServiceDiscoveryConfig struct {
	// Enabled controls whether CEL-based service discovery is active.
	// When false (default), the agent uses legacy service name detection.
	Enabled bool `yaml:"enabled"`

	// ServiceDefinitions are the CEL rules evaluated in order (first match wins).
	ServiceDefinitions []ServiceDefinition `yaml:"service_definitions"`
}

// ServiceDefinition represents a query/value pair for service name evaluation.
// Rules are evaluated in order; the first matching rule wins.
type ServiceDefinition struct {
	// Name is an optional identifier for debugging and logging.
	// If empty, the rule index will be used in logs/metrics.
	Name string `yaml:"name,omitempty"`

	// Query is a CEL boolean expression that determines if this rule matches.
	// Example: "container['labels']['app'] == 'redis'"
	Query string `yaml:"query"`

	// Value is a CEL string expression that computes the service name when Query matches.
	// Example: "container['labels']['service']"
	Value string `yaml:"value"`
}

// configReader is a minimal interface for reading service discovery config.
// This allows for easier testing without mocking the full pkgconfigmodel.Reader.
type configReader interface {
	GetBool(key string) bool
	Get(key string) interface{}
}

// LoadFromAgentConfig loads service discovery configuration from the agent's config model.
// This is the preferred way to load configuration as it integrates with the agent's
// config system (supports env vars like DD_SERVICE_DISCOVERY_ENABLED, remote config, etc.)
func LoadFromAgentConfig(cfg pkgconfigmodel.Reader) (*AgentServiceDiscoveryConfig, error) {
	return loadFromReader(cfg)
}

// loadFromReader is the internal implementation that works with the minimal configReader interface.
// It extracts service discovery configuration from the config reader and validates it if enabled.
func loadFromReader(cfg configReader) (*AgentServiceDiscoveryConfig, error) {
	config := &AgentServiceDiscoveryConfig{
		Enabled: cfg.GetBool("service_discovery.enabled"),
	}

	// Get service definitions from config
	// The config system returns []interface{} for slices
	rawDefs := cfg.Get("service_discovery.service_definitions")
	if rawDefs != nil {
		defs, err := parseServiceDefinitions(rawDefs)
		if err != nil {
			return nil, fmt.Errorf("invalid service_definitions: %w", err)
		}
		config.ServiceDefinitions = defs
	}

	// Skip validation when disabled to avoid blocking agent startup
	// with syntax errors in unused configuration
	if !config.Enabled {
		return config, nil
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// parseServiceDefinitions converts the raw config value to typed ServiceDefinition slice.
// Handles various slice types that config sources may produce:
//   - []interface{} (common from YAML unmarshaling)
//   - []map[string]interface{} (common from programmatic sources)
//   - []map[interface{}]interface{} (can occur with some YAML parsers)
//
// Returns an error if the structure is invalid or required fields are missing.
func parseServiceDefinitions(raw interface{}) ([]ServiceDefinition, error) {
	// Normalize different slice types to []interface{}
	slice, err := normalizeSlice(raw)
	if err != nil {
		return nil, err
	}

	defs := make([]ServiceDefinition, 0, len(slice))
	for i, item := range slice {
		// Convert map to uniform type (handles both map[string]interface{} and map[interface{}]interface{})
		m, err := toStringMap(item)
		if err != nil {
			return nil, fmt.Errorf("service_definitions[%d]: %w", i, err)
		}

		def := ServiceDefinition{}

		// Name is optional
		if name, ok := m["name"].(string); ok {
			def.Name = name
		}

		// Query is required - check type and value separately for better error messages
		queryVal, queryExists := m["query"]
		if !queryExists {
			return nil, fmt.Errorf("service_definitions[%d]: missing required field 'query'", i)
		}
		query, ok := queryVal.(string)
		if !ok {
			return nil, fmt.Errorf("service_definitions[%d]: query must be a string, got %T", i, queryVal)
		}
		if query == "" {
			return nil, fmt.Errorf("service_definitions[%d]: query cannot be empty", i)
		}
		def.Query = query

		// Value is required - check type and value separately for better error messages
		valueVal, valueExists := m["value"]
		if !valueExists {
			return nil, fmt.Errorf("service_definitions[%d]: missing required field 'value'", i)
		}
		value, ok := valueVal.(string)
		if !ok {
			return nil, fmt.Errorf("service_definitions[%d]: value must be a string, got %T", i, valueVal)
		}
		if value == "" {
			return nil, fmt.Errorf("service_definitions[%d]: value cannot be empty", i)
		}
		def.Value = value

		defs = append(defs, def)
	}

	return defs, nil
}

// normalizeSlice converts various slice types to []interface{}.
// Handles []interface{}, []map[string]interface{}, and []map[interface{}]interface{}.
// This is necessary because different config sources (YAML files, env vars, remote config)
// may produce different slice types.
func normalizeSlice(raw interface{}) ([]interface{}, error) {
	switch s := raw.(type) {
	case []interface{}:
		return s, nil
	case []map[string]interface{}:
		result := make([]interface{}, len(s))
		for i, v := range s {
			result[i] = v
		}
		return result, nil
	case []map[interface{}]interface{}:
		result := make([]interface{}, len(s))
		for i, v := range s {
			result[i] = v
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", raw)
	}
}

// toStringMap converts a map to map[string]interface{}, handling both
// map[string]interface{} and map[interface{}]interface{} (YAML unmarshaling returns the latter).
// This normalization is required because YAML parsers may use interface{} keys.
func toStringMap(v interface{}) (map[string]interface{}, error) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, nil
	case map[interface{}]interface{}:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			keyStr, ok := k.(string)
			if !ok {
				return nil, fmt.Errorf("expected string key, got %T", k)
			}
			result[keyStr] = val
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected map, got %T", v)
	}
}

// LoadAgentConfig loads the agent-level service discovery configuration from a YAML file.
// Deprecated: Use LoadFromAgentConfig instead, which integrates with the agent's config system.
// This function is kept for backward compatibility and testing.
func LoadAgentConfig(path string) (*AgentServiceDiscoveryConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// No config file = empty config (opt-in: will use legacy detectors)
			return &AgentServiceDiscoveryConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var wrapper struct {
		ServiceDiscovery AgentServiceDiscoveryConfig `yaml:"service_discovery"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	config := &wrapper.ServiceDiscovery

	// Skip validation when disabled to avoid blocking agent startup
	// with syntax errors in unused configuration
	if !config.Enabled {
		return config, nil
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// IsActive returns true if CEL-based service discovery should be used.
// Requires both Enabled=true and at least one service definition.
func (c *AgentServiceDiscoveryConfig) IsActive() bool {
	return c.Enabled && len(c.ServiceDefinitions) > 0
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

	return nil
}

// getCachedCELEnvironment creates a CEL environment for validation.
// Note: Despite the name, this no longer caches the environment to avoid permanent error caching.
// CreateCELEnvironment is lightweight and can be called repeatedly without performance issues.
func getCachedCELEnvironment() (*cel.Env, error) {
	return engine.CreateCELEnvironment()
}

// validateCELBooleanExpression validates that an expression compiles and returns boolean.
// It accepts both BoolType and DynType (runtime validation ensures actual boolean value).
func validateCELBooleanExpression(expr string) error {
	env, err := getCachedCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// Accept BoolType or DynType (runtime validation will ensure it's actually bool)
	outType := ast.OutputType()
	if outType != cel.BoolType && outType != cel.DynType {
		return fmt.Errorf("expression must return boolean, got %v", outType)
	}

	return nil
}

// validateCELStringExpression validates that an expression compiles and returns string.
// It accepts both StringType and DynType (runtime validation ensures actual string value).
func validateCELStringExpression(expr string) error {
	env, err := getCachedCELEnvironment()
	if err != nil {
		return err
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return fmt.Errorf("compilation error: %w", issues.Err())
	}

	// Accept StringType or DynType (runtime validation will ensure it's actually string)
	outType := ast.OutputType()
	if outType != cel.StringType && outType != cel.DynType {
		return fmt.Errorf("expression must return string, got %v", outType)
	}

	return nil
}

// CompileEngine creates a CEL engine from the agent configuration.
// Returns nil (not an error) if the configuration is not active (disabled or no rules).
// This allows callers to check: if engine != nil { use it } else { use legacy detection }
func (c *AgentServiceDiscoveryConfig) CompileEngine() (*engine.Engine, error) {
	if !c.IsActive() {
		return nil, nil
	}

	// Convert ServiceDefinitions to engine Rules
	rules := make([]engine.Rule, len(c.ServiceDefinitions))
	for i, def := range c.ServiceDefinitions {
		rules[i] = engine.Rule{
			Name:  def.Name,
			Query: def.Query,
			Value: def.Value,
		}
	}

	return engine.NewEngine(rules)
}
