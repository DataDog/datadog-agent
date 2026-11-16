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
	"go.yaml.in/yaml/v4"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
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

func TestEnableSystemProbeConfig_UnmarshalError(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	// Malformed YAML: this is the kind of failure postInstallAPMInject must
	// tolerate without failing the overall APM inject install.
	writeFile(t, configPath, `windows_crash_detection: [this is not a map`)

	err := enableSystemProbeConfigAt(configPath)
	require.Error(t, err)
}

func TestAddLocalStableConfig_NothingSet(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "application_monitoring.yaml")

	err := addLocalStableConfigAt(configPath, env.InstallScriptEnv{})
	require.NoError(t, err)

	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "file should not be created when no setting is set")
}

func TestAddLocalStableConfig_NoExistingFile(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "application_monitoring.yaml")

	err := addLocalStableConfigAt(configPath, env.InstallScriptEnv{
		ProfilingEnabled:   "auto",
		DataStreamsEnabled: config.BoolToPtr(true),
	})
	require.NoError(t, err)

	var cfg config.ApplicationMonitoringConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.Default.ProfilingEnabled)
	assert.Equal(t, "auto", *cfg.Default.ProfilingEnabled)
	require.NotNil(t, cfg.Default.DataStreamsEnabled)
	assert.True(t, *cfg.Default.DataStreamsEnabled)
	assert.Nil(t, cfg.Default.AppsecEnabled)
}

func TestAddLocalStableConfig_PreservesExistingSettings(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "application_monitoring.yaml")

	writeFile(t, configPath, `apm_configuration_default:
  DD_PROFILING_ENABLED: "false"
  DD_RUNTIME_METRICS_ENABLED: true
some_future_key: value
`)

	err := addLocalStableConfigAt(configPath, env.InstallScriptEnv{ProfilingEnabled: "auto"})
	require.NoError(t, err)

	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(data, &raw))

	defaults := raw["apm_configuration_default"].(map[string]any)
	assert.Equal(t, "auto", defaults["DD_PROFILING_ENABLED"])
	assert.Equal(t, true, defaults["DD_RUNTIME_METRICS_ENABLED"])
	assert.Equal(t, "value", raw["some_future_key"])
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0640))
}
