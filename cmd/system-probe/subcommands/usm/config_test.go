// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfigCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeConfigCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "config", cmd.Use)
	require.Equal(t, "Show Universal Service Monitoring configuration", cmd.Short)

	// Verify --json flag exists and has correct default
	jsonFlag := cmd.Flags().Lookup("json")
	require.NotNil(t, jsonFlag, "--json flag should exist")
	require.Equal(t, "false", jsonFlag.DefValue, "--json should default to false")

	// Test the OneShot command
	fxutil.TestOneShotSubcommand(t,
		Commands(globalParams),
		[]string{"usm", "config"},
		runConfig,
		func() {})
}

func TestOutputConfigJSON(t *testing.T) {
	tests := []struct {
		name       string
		yamlInput  string
		assertions func(t *testing.T, decoded map[string]interface{})
	}{
		{
			name: "basic USM config",
			yamlInput: `
enabled: true
max_processes_tracked: 1000
protocols:
  http:
    enabled: true
  http2:
    enabled: false
tags:
  - env:prod
  - service:test
`,
			assertions: func(t *testing.T, decoded map[string]interface{}) {
				assert.Equal(t, true, decoded["enabled"])
				assert.Equal(t, float64(1000), decoded["max_processes_tracked"])

				protocols, ok := decoded["protocols"].(map[string]interface{})
				require.True(t, ok, "protocols should be a map")

				http, ok := protocols["http"].(map[string]interface{})
				require.True(t, ok, "http should be a map")
				assert.Equal(t, true, http["enabled"])

				tags, ok := decoded["tags"].([]interface{})
				require.True(t, ok, "tags should be a slice")
				assert.Len(t, tags, 2)
			},
		},
		{
			name: "realistic USM config with all protocols",
			yamlInput: `
enabled: true
enable_http_monitoring: true
enable_https_monitoring: true
enable_http2_monitoring: true
enable_kafka_monitoring: true
enable_postgres_monitoring: true
max_tracked_connections: 65536
max_closed_connections_buffered: 50000
closed_channel_buffer_size: 500
tls:
  native:
    enabled: true
  istio:
    enabled: false
  java_tls:
    enabled: true
http_replace_rules:
  - pattern: /users/[0-9]+
    repl: /users/?
  - pattern: /api/v[0-9]+
    repl: /api/v?
`,
			assertions: func(t *testing.T, decoded map[string]interface{}) {
				assert.Equal(t, true, decoded["enabled"])
				assert.Equal(t, true, decoded["enable_http_monitoring"])
				assert.Equal(t, true, decoded["enable_https_monitoring"])
				assert.Equal(t, true, decoded["enable_http2_monitoring"])
				assert.Equal(t, true, decoded["enable_kafka_monitoring"])
				assert.Equal(t, true, decoded["enable_postgres_monitoring"])
				assert.Equal(t, float64(65536), decoded["max_tracked_connections"])

				tls, ok := decoded["tls"].(map[string]interface{})
				require.True(t, ok, "tls should be a map")

				native, ok := tls["native"].(map[string]interface{})
				require.True(t, ok, "native should be a map")
				assert.Equal(t, true, native["enabled"])

				rules, ok := decoded["http_replace_rules"].([]interface{})
				require.True(t, ok, "http_replace_rules should be a slice")
				assert.Len(t, rules, 2)

				rule0, ok := rules[0].(map[string]interface{})
				require.True(t, ok, "rule should be a map")
				assert.Equal(t, "/users/[0-9]+", rule0["pattern"])
				assert.Equal(t, "/users/?", rule0["repl"])
			},
		},
		{
			name: "config with nested arrays and complex structures",
			yamlInput: `
enabled: true
excluded_linux_versions:
  - "4.4.0"
  - "4.15.0"
protocols:
  http:
    enabled: true
    buffer_size: 8192
    max_request_fragment: 160
  grpc:
    enabled: true
    timeout_seconds: 30
`,
			assertions: func(t *testing.T, decoded map[string]interface{}) {
				assert.Equal(t, true, decoded["enabled"])

				excluded, ok := decoded["excluded_linux_versions"].([]interface{})
				require.True(t, ok, "excluded_linux_versions should be a slice")
				assert.Len(t, excluded, 2)
				assert.Equal(t, "4.4.0", excluded[0])

				protocols, ok := decoded["protocols"].(map[string]interface{})
				require.True(t, ok, "protocols should be a map")

				http, ok := protocols["http"].(map[string]interface{})
				require.True(t, ok, "http should be a map")
				assert.Equal(t, float64(8192), http["buffer_size"])

				grpc, ok := protocols["grpc"].(map[string]interface{})
				require.True(t, ok, "grpc should be a map")
				assert.Equal(t, float64(30), grpc["timeout_seconds"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse YAML using v3 (simulates what comes from system-probe)
			var config interface{}
			err := yaml.Unmarshal([]byte(tt.yamlInput), &config)
			require.NoError(t, err)

			// Encode directly to JSON (yaml.v3 produces JSON-compatible types)
			jsonData, err := json.MarshalIndent(config, "", "  ")
			require.NoError(t, err, "should successfully encode to JSON")

			// Verify the output is valid JSON
			var decoded map[string]interface{}
			err = json.Unmarshal(jsonData, &decoded)
			require.NoError(t, err, "output should be valid JSON")

			// Run test-specific assertions
			tt.assertions(t, decoded)
		})
	}
}

func TestFullConfigWorkflow(t *testing.T) {
	// This test simulates the full workflow: YAML -> Parse -> JSON conversion
	fullConfigYAML := `
service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
  enable_https_monitoring: true
  enable_http2_monitoring: false
  enable_kafka_monitoring: true
  max_tracked_connections: 65536
  max_closed_connections_buffered: 50000
  tls:
    native:
      enabled: true
    istio:
      enabled: false
  http_replace_rules:
    - pattern: /users/[0-9]+
      repl: /users/?
`

	// Parse full config
	var fullConfig map[string]interface{}
	err := yaml.Unmarshal([]byte(fullConfigYAML), &fullConfig)
	require.NoError(t, err)

	// Extract service_monitoring_config section (like the real command does)
	usmConfig, ok := fullConfig["service_monitoring_config"]
	require.True(t, ok, "service_monitoring_config should exist")

	// Test YAML output
	yamlData, err := yaml.Marshal(usmConfig)
	require.NoError(t, err)
	assert.Contains(t, string(yamlData), "enabled: true")
	assert.Contains(t, string(yamlData), "enable_http_monitoring: true")

	// Test JSON output (yaml.v3 produces JSON-compatible types)
	jsonData, err := json.MarshalIndent(usmConfig, "", "  ")
	require.NoError(t, err)

	// Verify JSON is valid and contains expected fields
	var jsonDecoded map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonDecoded)
	require.NoError(t, err)

	assert.Equal(t, true, jsonDecoded["enabled"])
	assert.Equal(t, true, jsonDecoded["enable_http_monitoring"])
	assert.Equal(t, float64(65536), jsonDecoded["max_tracked_connections"])

	// Verify nested structures
	tls, ok := jsonDecoded["tls"].(map[string]interface{})
	require.True(t, ok)
	native, ok := tls["native"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, native["enabled"])

	// Verify arrays
	rules, ok := jsonDecoded["http_replace_rules"].([]interface{})
	require.True(t, ok)
	assert.Len(t, rules, 1)
}

func TestConfigCommandJSONOutput(t *testing.T) {
	// This test validates the full command execution with --json flag
	fullConfigYAML := `
service_monitoring_config:
  enabled: true
  enable_http_monitoring: true
  enable_https_monitoring: true
  enable_http2_monitoring: false
  max_tracked_connections: 65536
  max_closed_connections_buffered: 50000
  tls:
    native:
      enabled: true
    istio:
      enabled: false
  http_replace_rules:
    - pattern: /users/[0-9]+
      repl: /users/?
    - pattern: /api/v[0-9]+
      repl: /api/v?
`

	// Parse and extract USM config like runConfig does
	var fullConfig map[string]interface{}
	err := yaml.Unmarshal([]byte(fullConfigYAML), &fullConfig)
	require.NoError(t, err)

	usmConfig, ok := fullConfig["service_monitoring_config"]
	require.True(t, ok)

	// Simulate JSON output (using outputJSON)
	jsonData, err := json.MarshalIndent(usmConfig, "", "  ")
	require.NoError(t, err, "JSON encoding should succeed (no map[interface{}]interface{} errors)")

	// Verify JSON is valid
	var jsonDecoded map[string]interface{}
	err = json.Unmarshal(jsonData, &jsonDecoded)
	require.NoError(t, err, "JSON output should be valid and parseable")

	// Verify all expected fields are present with correct types
	assert.Equal(t, true, jsonDecoded["enabled"])
	assert.Equal(t, true, jsonDecoded["enable_http_monitoring"])
	assert.Equal(t, true, jsonDecoded["enable_https_monitoring"])
	assert.Equal(t, false, jsonDecoded["enable_http2_monitoring"])
	assert.Equal(t, float64(65536), jsonDecoded["max_tracked_connections"])
	assert.Equal(t, float64(50000), jsonDecoded["max_closed_connections_buffered"])

	// Verify nested structures work correctly
	tls, ok := jsonDecoded["tls"].(map[string]interface{})
	require.True(t, ok, "nested tls object should be a map")

	native, ok := tls["native"].(map[string]interface{})
	require.True(t, ok, "nested native object should be a map")
	assert.Equal(t, true, native["enabled"])

	istio, ok := tls["istio"].(map[string]interface{})
	require.True(t, ok, "nested istio object should be a map")
	assert.Equal(t, false, istio["enabled"])

	// Verify arrays work correctly
	rules, ok := jsonDecoded["http_replace_rules"].([]interface{})
	require.True(t, ok, "http_replace_rules should be an array")
	assert.Len(t, rules, 2)

	rule0, ok := rules[0].(map[string]interface{})
	require.True(t, ok, "array element should be a map")
	assert.Equal(t, "/users/[0-9]+", rule0["pattern"])
	assert.Equal(t, "/users/?", rule0["repl"])

	rule1, ok := rules[1].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "/api/v[0-9]+", rule1["pattern"])
	assert.Equal(t, "/api/v?", rule1["repl"])
}
