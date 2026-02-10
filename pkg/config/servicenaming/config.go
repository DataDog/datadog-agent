// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package servicenaming provides configuration loading and validation for service discovery.
package servicenaming

import (
	"fmt"

	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/servicenaming/engine"
)

// AgentServiceDiscoveryConfig holds the configuration for CEL-based service discovery in the agent.
type AgentServiceDiscoveryConfig struct {
	Enabled bool `yaml:"enabled"`

	ServiceDefinitions []ServiceDefinition `yaml:"service_definitions"`
}

// ServiceDefinition represents a query/value pair for service name evaluation.
type ServiceDefinition struct {
	Query string `yaml:"query"`
	Value string `yaml:"value"`
}

// configReader is a minimal interface for reading service discovery config.
// This allows for easier testing without mocking the full pkgconfigmodel.Reader.
type configReader interface {
	GetBool(key string) bool
	Get(key string) interface{}
}

// LoadFromAgentConfig loads the service discovery configuration from the given config reader.
func LoadFromAgentConfig(cfg pkgconfigmodel.Reader) (*AgentServiceDiscoveryConfig, error) {
	return loadFromReader(cfg)
}

// loadFromReader is the internal implementation of configuration loading.
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

	// Validation of query/value fields is already done in parseServiceDefinitions().
	// CEL compilation validation is deferred to engine.NewEngine().
	return config, nil
}

// parseServiceDefinitions parses the raw service definitions from datadog.yaml into structured ServiceDefinition objects.
// This function expects the YAML parser format: []interface{} containing map[interface{}]interface{} items.
func parseServiceDefinitions(raw interface{}) ([]ServiceDefinition, error) {
	// YAML parser produces []interface{}
	slice, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", raw)
	}

	defs := make([]ServiceDefinition, 0, len(slice))
	for i, item := range slice {
		// YAML parser produces map[interface{}]interface{} for each item
		m, ok := item.(map[interface{}]interface{})
		if !ok {
			return nil, fmt.Errorf("service_definitions[%d]: expected map, got %T", i, item)
		}

		def := ServiceDefinition{}

		// Query is required - check type and value separately for better error messages
		queryVal, queryExists := m["query"]
		if !queryExists {
			return nil, fmt.Errorf("service_definition[%d]: missing required field 'query'", i)
		}
		query, ok := queryVal.(string)
		if !ok {
			return nil, fmt.Errorf("service_definition[%d]: query must be a string, got %T", i, queryVal)
		}
		if query == "" {
			return nil, fmt.Errorf("service_definition[%d]: query cannot be empty", i)
		}
		def.Query = query

		// Value is required - check type and value separately for better error messages
		valueVal, valueExists := m["value"]
		if !valueExists {
			return nil, fmt.Errorf("service_definition[%d]: missing required field 'value'", i)
		}
		value, ok := valueVal.(string)
		if !ok {
			return nil, fmt.Errorf("service_definition[%d]: value must be a string, got %T", i, valueVal)
		}
		if value == "" {
			return nil, fmt.Errorf("service_definition[%d]: value cannot be empty", i)
		}
		def.Value = value

		defs = append(defs, def)
	}

	return defs, nil
}

// IsActive returns if service discovery is enabled and has at least one rule defined.
func (c *AgentServiceDiscoveryConfig) IsActive() bool {
	return c.Enabled && len(c.ServiceDefinitions) > 0
}

// CompileEngine compiles the service definitions into an executable engine.Engine instance.
func (c *AgentServiceDiscoveryConfig) CompileEngine() (*engine.Engine, error) {
	if !c.IsActive() {
		return nil, nil
	}

	// Convert ServiceDefinitions to engine Rules
	rules := make([]engine.Rule, len(c.ServiceDefinitions))
	for i, def := range c.ServiceDefinitions {
		rules[i] = engine.Rule{
			Query: def.Query,
			Value: def.Value,
		}
	}

	return engine.NewEngine(rules)
}
