// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
)

func TestEnableSystemProbeConfig_NoExistingFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	var cfg config.SystemProbeConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.WindowsCrashDetection.Enabled)
	assert.True(t, *cfg.WindowsCrashDetection.Enabled)
}

func TestEnableSystemProbeConfig_AlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	writeFile(t, configPath, `windows_crash_detection:
  enabled: true
runtime_security_config:
  enabled: true
`)
	contentBefore, err := os.ReadFile(configPath)
	require.NoError(t, err)

	err = enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	contentAfter, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Equal(t, string(contentBefore), string(contentAfter), "file content should not change when already enabled")
}

func TestEnableSystemProbeConfig_PreservesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	writeFile(t, configPath, `runtime_security_config:
  enabled: true
gpu_monitoring:
  enabled: true
`)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	var result config.SystemProbeConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &result))

	require.NotNil(t, result.WindowsCrashDetection.Enabled)
	assert.True(t, *result.WindowsCrashDetection.Enabled)
	require.NotNil(t, result.RuntimeSecurityConfig.Enabled)
	assert.True(t, *result.RuntimeSecurityConfig.Enabled)
	require.NotNil(t, result.GPUMonitoringConfig.Enabled)
	assert.True(t, *result.GPUMonitoringConfig.Enabled)
}

func TestEnableSystemProbeConfig_FlipsDisabledToEnabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	writeFile(t, configPath, `windows_crash_detection:
  enabled: false
runtime_security_config:
  enabled: true
`)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	var result config.SystemProbeConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &result))

	assert.True(t, *result.WindowsCrashDetection.Enabled)
	assert.True(t, *result.RuntimeSecurityConfig.Enabled, "existing settings should be preserved")
}

func TestEnableSystemProbeConfig_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	writeFile(t, configPath, `windows_crash_detection:
  enabled: false
system_probe_config:
  max_tracked_connections: 65536
network_config:
  enabled: true
  conntrack: true
some_future_key:
  nested: value
`)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Parse back as generic map to verify all keys survived
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(data, &raw))

	// windows_crash_detection.enabled should be flipped to true
	wcd := raw["windows_crash_detection"].(map[string]any)
	assert.Equal(t, true, wcd["enabled"])

	// unknown key under system_probe_config preserved
	spc := raw["system_probe_config"].(map[string]any)
	assert.Equal(t, 65536, spc["max_tracked_connections"])

	// top-level unknown sections preserved
	nc := raw["network_config"].(map[string]any)
	assert.Equal(t, true, nc["enabled"])
	assert.Equal(t, true, nc["conntrack"])

	sfk := raw["some_future_key"].(map[string]any)
	assert.Equal(t, "value", sfk["nested"])
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0640))
}
