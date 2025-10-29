// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package jmxclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceConfigParse(t *testing.T) {
	tests := []struct {
		name        string
		configYAML  string
		wantErr     bool
		errContains string
		validate    func(*testing.T, *InstanceConfig)
	}{
		{
			name: "valid host port config",
			configYAML: `
host: localhost
port: 9999
tags:
  - env:test
  - service:myapp
`,
			wantErr: false,
			validate: func(t *testing.T, c *InstanceConfig) {
				assert.Equal(t, "localhost", c.Host)
				assert.Equal(t, 9999, c.Port)
				assert.Equal(t, []string{"env:test", "service:myapp"}, c.Tags)
				assert.Equal(t, defaultRefreshBeans, c.RefreshBeans)
			},
		},
		{
			name: "valid jmx url config",
			configYAML: `
jmx_url: "service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi"
`,
			wantErr: false,
			validate: func(t *testing.T, c *InstanceConfig) {
				assert.Equal(t, "service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi", c.JMXURL)
			},
		},		{
			name: "custom refresh beans",
			configYAML: `
host: localhost
port: 9999
refresh_beans: 300
`,
			wantErr: false,
			validate: func(t *testing.T, c *InstanceConfig) {
				assert.Equal(t, 300, c.RefreshBeans)
			},
		},
		{
			name: "missing connection config",
			configYAML: `
tags:
  - env:test
`,
			wantErr:     true,
			errContains: "must be specified",
		},
		{
			name: "host without port",
			configYAML: `
host: localhost
`,
			wantErr:     true,
			errContains: "port' must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &InstanceConfig{}
			err := config.Parse([]byte(tt.configYAML))

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, config)
				}
			}
		})
	}
}

func TestInitConfigParse(t *testing.T) {
	configYAML := `
is_jmx: true
collect_default_metrics: false
conf:
  - include:
      domain: "kafka.producer"
      bean_regex: "kafka\\.producer:type=ProducerRequestMetrics,.*"
      attribute:
        Count:
          metric_type: rate
          alias: kafka.producer.request_rate
  - include:
      domain: "java.lang"
      type: "MemoryPool"
      attribute:
        Usage.used:
          metric_type: gauge
          alias: jvm.heap_memory.used
`

	config := &InitConfig{}
	err := config.Parse([]byte(configYAML))
	require.NoError(t, err)

	assert.True(t, config.IsJMX)
	assert.False(t, config.CollectDefaultMetrics)
	assert.Len(t, config.Conf, 2)

	// Validate first config
	assert.NotNil(t, config.Conf[0].Include)
	assert.Equal(t, "kafka.producer", config.Conf[0].Include.Domain)
	assert.NotNil(t, config.Conf[0].Include.Attribute)
	assert.Contains(t, config.Conf[0].Include.Attribute, "Count")
	assert.Equal(t, "rate", config.Conf[0].Include.Attribute["Count"].MetricType)
	assert.Equal(t, "kafka.producer.request_rate", config.Conf[0].Include.Attribute["Count"].Alias)

	// Validate second config
	assert.NotNil(t, config.Conf[1].Include)
	assert.Equal(t, "java.lang", config.Conf[1].Include.Domain)
}

func TestInitConfigParseWithDefaultMetrics(t *testing.T) {
	configYAML := `
is_jmx: true
collect_default_metrics: true
conf:
  - include:
      domain: "kafka.producer"
      bean_regex: "kafka\\.producer:type=ProducerRequestMetrics,.*"
      attribute:
        Count:
          metric_type: rate
          alias: kafka.producer.request_rate
`

	config := &InitConfig{}
	err := config.Parse([]byte(configYAML))
	require.NoError(t, err)

	assert.True(t, config.IsJMX)
	assert.True(t, config.CollectDefaultMetrics)

	// Should have default metrics (22) + user config (1) = 23 total
	assert.Greater(t, len(config.Conf), 20, "should have loaded default metrics")

	// The last config should be the user-provided kafka config (prepended default, then user)
	lastConfig := config.Conf[len(config.Conf)-1]
	assert.NotNil(t, lastConfig.Include)
	assert.Equal(t, "kafka.producer", lastConfig.Include.Domain)
	assert.Contains(t, lastConfig.Include.Attribute, "Count")

	// Verify that default metrics include java.lang:type=Memory metrics
	foundMemoryConfig := false
	for _, conf := range config.Conf {
		if conf.Include != nil && conf.Include.Domain == "java.lang" && conf.Include.Type == "Memory" {
			foundMemoryConfig = true
			// Should have HeapMemoryUsage attributes
			assert.Contains(t, conf.Include.Attribute, "HeapMemoryUsage.used")
			break
		}
	}
	assert.True(t, foundMemoryConfig, "should have found java.lang Memory config in default metrics")
}

func TestGetConnectionString(t *testing.T) {
	tests := []struct {
		name     string
		config   *InstanceConfig
		expected string
	}{
		{
			name: "jmx url",
			config: &InstanceConfig{
				JMXURL: "service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi",
				Host:   "localhost",
				Port:   9999,
			},
			expected: "service:jmx:rmi:///jndi/rmi://localhost:9999/jmxrmi",
		},
		{
			name: "host port",
			config: &InstanceConfig{
				Host: "localhost",
				Port: 9999,
			},
			expected: "localhost:9999",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetConnectionString()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeStringOrList(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "single string",
			input:    "value",
			expected: []string{"value"},
		},
		{
			name:     "string slice",
			input:    []string{"value1", "value2"},
			expected: []string{"value1", "value2"},
		},
		{
			name:     "interface slice",
			input:    []interface{}{"value1", "value2"},
			expected: []string{"value1", "value2"},
		},
		{
			name:     "nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "unsupported type",
			input:    123,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeStringOrList(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToJmxClientFormat(t *testing.T) {
	tests := []struct {
		name     string
		configs  []BeanCollectionConfig
		expected []BeanRequest
	}{
		{
			name: "single path with multiple attributes",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						Path: "java.lang:type=Memory",
						Type: "Memory",
						Attribute: map[string]*Attribute{
							"HeapMemoryUsage":    {MetricType: "gauge"},
							"NonHeapMemoryUsage": {MetricType: "gauge"},
						},
					},
				},
			},
			expected: []BeanRequest{
				{Path: "java.lang:type=Memory", Type: "Memory", Attribute: "HeapMemoryUsage", MetricType: "gauge"},
				{Path: "java.lang:type=Memory", Type: "Memory", Attribute: "NonHeapMemoryUsage", MetricType: "gauge"},
			},
		},
		{
			name: "multiple paths with attributes",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						Path: []interface{}{"java.lang:type=Memory", "java.lang:type=Threading"},
						Type: "Runtime",
						Attribute: map[string]*Attribute{
							"Count": {MetricType: "gauge"},
						},
					},
				},
			},
			expected: []BeanRequest{
				{Path: "java.lang:type=Memory", Type: "Runtime", Attribute: "Count", MetricType: "gauge"},
				{Path: "java.lang:type=Threading", Type: "Runtime", Attribute: "Count", MetricType: "gauge"},
			},
		},
		{
			name: "path without attributes",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						Path: "java.lang:type=Memory",
						Type: "Memory",
					},
				},
			},
			expected: []BeanRequest{
				{Path: "java.lang:type=Memory", Type: "Memory", Attribute: ""},
			},
		},
		{
			name: "domain and type with composite attributes",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						Domain: "java.lang",
						Type:   "Memory",
						Attribute: map[string]*Attribute{
							"HeapMemoryUsage.used":      {MetricType: "gauge"},
							"HeapMemoryUsage.committed": {MetricType: "gauge"},
						},
					},
				},
			},
			expected: []BeanRequest{
				{Path: "java.lang:type=Memory", Type: "Memory", Attribute: "HeapMemoryUsage", Key: "used", MetricType: "gauge"},
				{Path: "java.lang:type=Memory", Type: "Memory", Attribute: "HeapMemoryUsage", Key: "committed", MetricType: "gauge"},
			},
		},
		{
			name: "domain and type without attributes",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						Domain: "java.lang",
						Type:   "Threading",
					},
				},
			},
			expected: []BeanRequest{
				{Path: "java.lang:type=Threading", Type: "Threading", Attribute: ""},
			},
		},
		{
			name: "bean_regex pattern",
			configs: []BeanCollectionConfig{
				{
					Include: &BeanMatcher{
						BeanRegex: "kafka\\.producer:type=producer-metrics,client-id=.*",
						Attribute: map[string]*Attribute{
							"record-send-rate": {MetricType: "gauge"},
						},
					},
				},
			},
			expected: []BeanRequest{
				{Path: "kafka\\.producer:type=producer-metrics,client-id=.*", Type: "", Attribute: "record-send-rate", MetricType: "gauge"},
			},
		},
		{
			name: "exclude config should be skipped",
			configs: []BeanCollectionConfig{
				{
					Exclude: &BeanMatcher{
						Path: "java.lang:type=Memory",
					},
				},
			},
			expected: []BeanRequest{},
		},
		{
			name:     "empty config",
			configs:  []BeanCollectionConfig{},
			expected: []BeanRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToJmxClientFormat(tt.configs)
			// Use ElementsMatch to compare slices regardless of order
			assert.ElementsMatch(t, tt.expected, result)
		})
	}
}
