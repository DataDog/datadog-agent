// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmxclient

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
)

const (
	defaultRefreshBeans = 600 // 600 seconds = 10 minutes
)

// InstanceConfig holds the configuration for a single JMX instance connection
type InstanceConfig struct {
	Host             string   `yaml:"host"`
	Port             int      `yaml:"port"`
	JMXURL           string   `yaml:"jmx_url"`
	ProcessNameRegex string   `yaml:"process_name_regex"`
	User             string   `yaml:"user"`
	Password         string   `yaml:"password"`
	Name             string   `yaml:"name"`
	Tags             []string `yaml:"tags"`
	JavaBinPath      string   `yaml:"java_bin_path"`
	JavaOptions      string   `yaml:"java_options"`
	TrustStorePath   string   `yaml:"trust_store_path"`
	TrustStorePass   string   `yaml:"trust_store_password"`
	RefreshBeans     int      `yaml:"refresh_beans"`
}

// InitConfig holds the init_config section containing bean collection rules
type InitConfig struct {
	IsJMX                 bool                     `yaml:"is_jmx"`
	CollectDefaultMetrics bool                     `yaml:"collect_default_metrics"`
	NewGCMetrics          bool                     `yaml:"new_gc_metrics"`
	CustomJarPaths        []string                 `yaml:"custom_jar_paths"`
	JavaBinPath           string                   `yaml:"java_bin_path"`
	JavaOptions           string                   `yaml:"java_options"`
	ToolsJarPath          string                   `yaml:"tools_jar_path"`
	Conf                  []BeanCollectionConfig   `yaml:"conf"`
}

// BeanCollectionConfig defines rules for collecting beans
type BeanCollectionConfig struct {
	Include *BeanMatcher `yaml:"include,omitempty"`
	Exclude *BeanMatcher `yaml:"exclude,omitempty"`
}

// BeanMatcher defines patterns for matching MBeans
type BeanMatcher struct {
	Domain      interface{}            `yaml:"domain,omitempty"`      // string or []string
	Bean        interface{}            `yaml:"bean,omitempty"`        // string or []string
	BeanRegex   interface{}            `yaml:"bean_regex,omitempty"`  // string or []string
	Type        interface{}            `yaml:"type,omitempty"`        // string or []string
	Scope       interface{}            `yaml:"scope,omitempty"`       // string or []string
	Name        interface{}            `yaml:"name,omitempty"`        // string or []string
	Path        interface{}            `yaml:"path,omitempty"`        // string or []string
	ExcludeTags []string               `yaml:"exclude_tags,omitempty"`
	Tags        map[string]string      `yaml:"tags,omitempty"`
	Attribute   map[string]*Attribute  `yaml:"attribute,omitempty"`
	Keyspace    interface{}            `yaml:"keyspace,omitempty"`    // For Cassandra-style configs
}

// Attribute defines how to collect a specific MBean attribute
type Attribute struct {
	MetricType string `yaml:"metric_type"` // gauge, counter, rate, histogram, monotonic_gauge
	Alias      string `yaml:"alias"`       // Custom metric name
}

// Parse parses the instance configuration from raw YAML data
func (c *InstanceConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to parse instance config: %w", err)
	}

	// Set defaults
	if c.RefreshBeans <= 0 {
		c.RefreshBeans = defaultRefreshBeans
	}

	// Validate required fields
	if c.Host == "" && c.JMXURL == "" && c.ProcessNameRegex == "" {
		return fmt.Errorf("one of 'host', 'jmx_url', or 'process_name_regex' must be specified")
	}

	if c.Host != "" && c.Port <= 0 {
		return fmt.Errorf("'port' must be specified when 'host' is provided")
	}

	return nil
}

// Parse parses the init configuration from raw YAML data
func (ic *InitConfig) Parse(data []byte) error {
	if err := yaml.Unmarshal(data, ic); err != nil {
		return fmt.Errorf("failed to parse init config: %w", err)
	}

	return nil
}

// GetConnectionString returns the connection string for this instance
func (c *InstanceConfig) GetConnectionString() string {
	if c.JMXURL != "" {
		return c.JMXURL
	}
	if c.Host != "" && c.Port > 0 {
		return fmt.Sprintf("%s:%d", c.Host, c.Port)
	}
	if c.ProcessNameRegex != "" {
		return fmt.Sprintf("process:%s", c.ProcessNameRegex)
	}
	return "unknown"
}

// GetInstanceName returns a readable name for this instance
func (c *InstanceConfig) GetInstanceName() string {
	if c.Name != "" {
		return c.Name
	}
	return c.GetConnectionString()
}

// BeanRequest represents the simplified format expected by jmxclient's prepare_beans
type BeanRequest struct {
	Path       string `json:"path"`
	Type       string `json:"type"`
	Attribute  string `json:"attribute"`
	Key        string `json:"key,omitempty"` // For composite attributes (e.g., HeapMemoryUsage.max -> key="max")
	MetricType string `json:"metric_type,omitempty"`
	Alias      string `json:"alias,omitempty"`
}

// ToJmxClientFormat converts BeanCollectionConfig to the format expected by jmxclient
// Returns a slice of BeanRequest objects that can be marshaled to JSON
func ToJmxClientFormat(configs []BeanCollectionConfig) []BeanRequest {
	requests := []BeanRequest{}

	for i, config := range configs {
		// Only process include configurations
		if config.Include == nil {
			fmt.Printf("DEBUG: Config[%d] has nil Include, skipping\n", i)
			continue
		}

		include := config.Include

		// Build MBean ObjectName paths from domain, type, bean, or bean_regex
		paths := buildMBeanPaths(include)
		if len(paths) == 0 {
			fmt.Printf("DEBUG: Config[%d] resulted in 0 paths (domain=%v, type=%v, bean=%v, bean_regex=%v)\n",
				i, include.Domain, include.Type, include.Bean, include.BeanRegex)
			continue
		}

		// Extract type - could be a single string or list
		types := NormalizeStringOrList(include.Type)
		typeStr := ""
		if len(types) > 0 {
			typeStr = types[0] // Use first type if multiple
		}

		// If no explicit attributes specified, create a request without attribute
		if include.Attribute == nil || len(include.Attribute) == 0 {
			for _, path := range paths {
				requests = append(requests, BeanRequest{
					Path:      path,
					Type:      typeStr,
					Attribute: "",
				})
			}
			continue
		}

		// Generate one BeanRequest per path/attribute combination
		for _, path := range paths {
			for attrName, attrConfig := range include.Attribute {
				request := BeanRequest{
					Path: path,
					Type: typeStr,
				}

				// Check if attribute contains composite notation (e.g., "HeapMemoryUsage.max")
				if idx := strings.LastIndex(attrName, "."); idx > 0 {
					// Split composite attribute: HeapMemoryUsage.max -> attribute="HeapMemoryUsage", key="max"
					request.Attribute = attrName[:idx]
					request.Key = attrName[idx+1:]
				} else {
					// Simple attribute
					request.Attribute = attrName
				}

				// Add metric type and alias if provided
				if attrConfig != nil {
					request.MetricType = attrConfig.MetricType
					request.Alias = attrConfig.Alias
				}

				requests = append(requests, request)
			}
		}
	}

	fmt.Printf("DEBUG: ToJmxClientFormat generated %d bean requests\n", len(requests))
	return requests
}

// buildMBeanPaths constructs MBean ObjectName paths from the BeanMatcher configuration
// It handles explicit paths, domain+type combinations, domain+bean, and bean_regex patterns
func buildMBeanPaths(matcher *BeanMatcher) []string {
	// If explicit paths are provided, use them directly
	explicitPaths := NormalizeStringOrList(matcher.Path)
	if len(explicitPaths) > 0 {
		return explicitPaths
	}

	// If bean_regex is provided, use it directly (it's already a pattern)
	beanRegexes := NormalizeStringOrList(matcher.BeanRegex)
	if len(beanRegexes) > 0 {
		return beanRegexes
	}

	// Otherwise, construct paths from domain + type/bean/name/scope combinations
	var paths []string

	domains := NormalizeStringOrList(matcher.Domain)
	if len(domains) == 0 {
		// No domain specified, can't construct a path
		return nil
	}

	types := NormalizeStringOrList(matcher.Type)
	beans := NormalizeStringOrList(matcher.Bean)
	names := NormalizeStringOrList(matcher.Name)
	scopes := NormalizeStringOrList(matcher.Scope)
	keyspaces := NormalizeStringOrList(matcher.Keyspace)

	// Build paths for each domain
	for _, domain := range domains {
		// Build property list from available fields
		var properties []string

		// Add type property
		if len(types) > 0 {
			for _, t := range types {
				properties = append(properties, fmt.Sprintf("type=%s", t))
			}
		}

		// Add bean property (bean is sometimes used as an alias for specific bean names)
		if len(beans) > 0 {
			for _, bean := range beans {
				properties = append(properties, fmt.Sprintf("name=%s", bean))
			}
		}

		// Add name property
		if len(names) > 0 {
			for _, name := range names {
				properties = append(properties, fmt.Sprintf("name=%s", name))
			}
		}

		// Add scope property
		if len(scopes) > 0 {
			for _, scope := range scopes {
				properties = append(properties, fmt.Sprintf("scope=%s", scope))
			}
		}

		// Add keyspace property (for Cassandra-style JMX)
		if len(keyspaces) > 0 {
			for _, keyspace := range keyspaces {
				properties = append(properties, fmt.Sprintf("keyspace=%s", keyspace))
			}
		}

		// If no properties are specified, use wildcard
		if len(properties) == 0 {
			paths = append(paths, fmt.Sprintf("%s:*", domain))
		} else {
			// Create a path with all properties
			// For simplicity, we'll combine all properties with commas
			// In a more sophisticated implementation, you might want to create
			// separate paths for each combination
			path := fmt.Sprintf("%s:%s", domain, properties[0])
			if len(properties) > 1 {
				for _, prop := range properties[1:] {
					path = fmt.Sprintf("%s,%s", path, prop)
				}
			}
			paths = append(paths, path)
		}
	}

	return paths
}

// NormalizeStringOrList converts interface{} that might be a string or []string to []string
func NormalizeStringOrList(v interface{}) []string {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case string:
		return []string{val}
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
		return result
	case []string:
		return val
	default:
		return nil
	}
}
