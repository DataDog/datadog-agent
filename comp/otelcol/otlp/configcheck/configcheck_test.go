// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package configcheck

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

func TestIsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		envVars  map[string]string
		expected bool
	}{
		{
			name:     "empty config",
			yaml:     "",
			expected: false,
		},
		{
			name: "otlp_config section missing",
			yaml: `
some_other_config: value
`,
			expected: false,
		},
		{
			name: "otlp_config exists but empty",
			yaml: `
otlp_config:
`,
			expected: false,
		},
		{
			name: "otlp_config exists but receiver key missing",
			yaml: `
otlp_config:
  other_key: value
`,
			expected: false,
		},
		{
			name: "receiver section exists but empty",
			yaml: `
otlp_config:
  receiver:
`,
			expected: true,
		},
		{
			name: "receiver section exists but null",
			yaml: `
otlp_config:
  receiver: null
`,
			expected: true,
		},
		{
			name: "receiver section with protocols but empty",
			yaml: `
otlp_config:
  receiver:
    protocols:
`,
			expected: true,
		},
		{
			name: "receiver section with protocols grpc and http empty",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
      http:
`,
			expected: true,
		},
		{
			name: "receiver section with protocols grpc and http null",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc: null
      http: null
`,
			expected: true,
		},
		{
			name: "receiver section with full grpc configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
			expected: true,
		},
		{
			name: "receiver section with full http configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      http:
        endpoint: 0.0.0.0:4318
`,
			expected: true,
		},
		{
			name: "receiver section with both grpc and http configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
`,
			expected: true,
		},
		{
			name: "empty config with env var setting grpc endpoint",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT": "0.0.0.0:9993",
			},
			expected: true,
		},
		{
			name: "empty config with env var setting http endpoint",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9994",
			},
			expected: true,
		},
		{
			name: "empty config with multiple env vars",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT":          "0.0.0.0:9993",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT":          "0.0.0.0:9994",
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_MAX_RECV_MSG_SIZE": "4194304",
			},
			expected: true,
		},
		{
			name: "config with receiver and env var override",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
			envVars: map[string]string{
				"DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT": "0.0.0.0:9994",
			},
			expected: true,
		},
		{
			name: "env var for non-receiver otlp config should not enable",
			yaml: "",
			envVars: map[string]string{
				"DD_OTLP_CONFIG_TRACES_ENABLED": "true",
				"DD_OTLP_CONFIG_LOGS_ENABLED":   "true",
			},
			expected: false,
		},
		{
			name: "otlp_config with other sections but no receiver",
			yaml: `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create mock config
			cfg := configmock.New(t)
			pkgconfigsetup.OTLP(cfg)

			// Create temporary file and read config from it
			if tt.yaml != "" {
				tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
				require.NoError(t, err, "Failed to create temp file")
				defer os.Remove(tmpFile.Name())

				_, err = tmpFile.WriteString(tt.yaml)
				require.NoError(t, err, "Failed to write YAML to temp file")
				tmpFile.Close()

				cfg.SetConfigFile(tmpFile.Name())
				err = cfg.ReadInConfig()
				require.NoError(t, err, "Failed to read YAML config")
			}

			result := IsEnabled(cfg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasSectionEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		section  string
		expected bool
	}{
		{
			name: "section exists with nested empty map",
			yaml: `
otlp_config:
  receiver:
    nested:
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section exists with deeply nested structure",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: localhost:4317
        tls:
          insecure: true
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with boolean value",
			yaml: `
otlp_config:
  receiver: true
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with string value",
			yaml: `
otlp_config:
  receiver: some_string_value
`,
			section:  "receiver",
			expected: true,
		},
		{
			name: "section with numeric value",
			yaml: `
otlp_config:
  receiver: 12345
`,
			section:  "receiver",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			pkgconfigsetup.OTLP(cfg)

			tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
			require.NoError(t, err, "Failed to create temp file")
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.yaml)
			require.NoError(t, err, "Failed to write YAML to temp file")
			tmpFile.Close()

			cfg.SetConfigFile(tmpFile.Name())
			err = cfg.ReadInConfig()
			require.NoError(t, err, "Failed to read YAML config")

			result := hasSection(cfg, tt.section)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsEnabledConsistencyWithReadConfigSection(t *testing.T) {
	// This test ensures that IsEnabled is consistent with ReadConfigSection behavior
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "receiver section exists but empty",
			yaml: `
otlp_config:
  receiver:
`,
		},
		{
			name: "receiver section exists but null",
			yaml: `
otlp_config:
  receiver: null
`,
		},
		{
			name: "receiver section with configuration",
			yaml: `
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configmock.New(t)
			pkgconfigsetup.OTLP(cfg)

			tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
			require.NoError(t, err, "Failed to create temp file")
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.yaml)
			require.NoError(t, err, "Failed to write YAML to temp file")
			tmpFile.Close()

			cfg.SetConfigFile(tmpFile.Name())
			err = cfg.ReadInConfig()
			require.NoError(t, err, "Failed to read YAML config")

			// IsEnabled should return true if ReadConfigSection can find the receiver key
			isEnabled := IsEnabled(cfg)
			configSection := ReadConfigSection(cfg, pkgconfigsetup.OTLPSection)
			_, hasReceiverKey := configSection.ToStringMap()[pkgconfigsetup.OTLPReceiverSubSectionKey]

			assert.Equal(t, hasReceiverKey, isEnabled,
				"IsEnabled should be consistent with ReadConfigSection's ability to find receiver key")
		})
	}
}

func TestReadConfigSection(t *testing.T) {
	cfg := configmock.New(t)
	pkgconfigsetup.OTLP(cfg)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
    infra_attributes:
      enabled: false
  logs:
    enabled: true
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	// IsEnabled should return true if ReadConfigSection can find the receiver key
	//isEnabled := IsEnabled(cfg)
	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"logs::enabled":                                      true,
		"logs::batch::flush_timeout":                         "200ms",
		"logs::batch::max_size":                              0,
		"logs::batch::min_size":                              8192,
		"metrics::enabled":                                   true,
		"metrics::batch::flush_timeout":                      "200ms",
		"metrics::batch::max_size":                           0,
		"metrics::batch::min_size":                           8192,
		"metrics::instrumentation_scope_metadata_as_tags":    true,
		"metrics::tag_cardinality":                           "low",
		"receiver::protocols::grpc::endpoint":                "0.0.0.0:4317",
		"traces::enabled":                                    true,
		"traces::infra_attributes::enabled":                  false,
		"traces::internal_port":                              5003,
		"traces::probabilistic_sampler::sampling_percentage": float64(100),
		"traces::span_name_as_resource_name":                 false,
		"traces::span_name_remappings":                       map[string]string{},
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigEmptySection(t *testing.T) {
	cfg := configmock.New(t)
	pkgconfigsetup.OTLP(cfg)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
  receiver:
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	// IsEnabled should return true if ReadConfigSection can find the receiver key
	//isEnabled := IsEnabled(cfg)
	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"logs::enabled":                                      true,
		"logs::batch::flush_timeout":                         "200ms",
		"logs::batch::max_size":                              0,
		"logs::batch::min_size":                              8192,
		"metrics::enabled":                                   true,
		"metrics::batch::flush_timeout":                      "200ms",
		"metrics::batch::max_size":                           0,
		"metrics::batch::min_size":                           8192,
		"metrics::instrumentation_scope_metadata_as_tags":    true,
		"metrics::tag_cardinality":                           "low",
		"receiver":                                           nil,
		"traces::enabled":                                    true,
		"traces::infra_attributes::enabled":                  true,
		"traces::internal_port":                              5003,
		"traces::probabilistic_sampler::sampling_percentage": float64(100),
		"traces::span_name_as_resource_name":                 false,
		"traces::span_name_remappings":                       map[string]string{},
	}

	assert.Equal(t, expectMap, configSection)
}

func TestReadConfigSectionEnvVars(t *testing.T) {
	t.Setenv("TEST_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", "0.0.0.0:9999")
	t.Setenv("TEST_OTLP_CONFIG_DEBUG_VERBOSITY", "normal")

	cfg := configmock.New(t)
	pkgconfigsetup.OTLP(cfg)

	tmpFile, err := os.CreateTemp("", "test-config-*.yaml")
	require.NoError(t, err, "Failed to create temp file")
	defer os.Remove(tmpFile.Name())

	yamlData := `
otlp_config:
  traces:
    enabled: true
  logs:
    enabled: true
`
	_, err = tmpFile.WriteString(yamlData)
	require.NoError(t, err, "Failed to write YAML to temp file")
	tmpFile.Close()

	cfg.SetConfigFile(tmpFile.Name())
	err = cfg.ReadInConfig()
	require.NoError(t, err, "Failed to read YAML config")

	// IsEnabled should return true if ReadConfigSection can find the receiver key
	//isEnabled := IsEnabled(cfg)
	configSection := readConfigSection(cfg, "otlp_config")

	expectMap := map[string]interface{}{
		"logs::enabled":                                      true,
		"logs::batch::flush_timeout":                         "200ms",
		"logs::batch::max_size":                              0,
		"logs::batch::min_size":                              8192,
		"metrics::enabled":                                   true,
		"metrics::batch::flush_timeout":                      "200ms",
		"metrics::batch::max_size":                           0,
		"metrics::batch::min_size":                           8192,
		"metrics::instrumentation_scope_metadata_as_tags":    true,
		"metrics::tag_cardinality":                           "low",
		"traces::enabled":                                    true,
		"traces::infra_attributes::enabled":                  true,
		"traces::internal_port":                              5003,
		"traces::probabilistic_sampler::sampling_percentage": float64(100),
		"traces::span_name_as_resource_name":                 false,
		"traces::span_name_remappings":                       map[string]string{},
	}

	assert.Equal(t, expectMap, configSection)
}
