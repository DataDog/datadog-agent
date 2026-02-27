// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package usm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestConfigCommand(t *testing.T) {
	globalParams := &command.GlobalParams{}
	cmd := makeConfigCommand(globalParams)

	require.NotNil(t, cmd)
	require.Equal(t, "config", cmd.Use)
	require.Equal(t, "Show Universal Service Monitoring configuration", cmd.Short)

	// Test the OneShot command
	fxutil.TestOneShotSubcommand(t,
		Commands(globalParams),
		[]string{"usm", "config"},
		runConfig,
		func() {})
}

func TestYAMLConfigWorkflow(t *testing.T) {
	// This test simulates the full workflow: YAML parsing and output
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
	assert.Contains(t, string(yamlData), "max_tracked_connections: 65536")

	// Verify nested structures are preserved
	assert.Contains(t, string(yamlData), "tls:")
	assert.Contains(t, string(yamlData), "native:")
	assert.Contains(t, string(yamlData), "http_replace_rules:")
}
