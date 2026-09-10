// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/paths"
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

// withTempInstallerPaths points paths.AgentConfigDir and paths.PackagesPath at
// fresh temp dirs for the duration of the test, restoring the originals on
// cleanup. Neither dir contains a "datadog-apm-inject/stable" package, so
// postInstallAPMInject always fails at the EvalSymlinks step past
// enableSystemProbeConfig -- there is no fake ddinjector-installer.exe to
// install the driver, so the success path can only be verified up to (and
// including) the system-probe config edit.
func withTempInstallerPaths(t *testing.T) (agentConfigDir string) {
	t.Helper()
	origAgentConfigDir := paths.AgentConfigDir
	origPackagesPath := paths.PackagesPath
	t.Cleanup(func() {
		paths.AgentConfigDir = origAgentConfigDir
		paths.PackagesPath = origPackagesPath
	})
	agentConfigDir = t.TempDir()
	paths.AgentConfigDir = agentConfigDir
	paths.PackagesPath = t.TempDir()
	return agentConfigDir
}

func TestPostInstallAPMInject_SystemProbeConfigSuccess_InstallContinues(t *testing.T) {
	agentConfigDir := withTempInstallerPaths(t)
	ctx := HookContext{Context: context.Background(), Package: packageAPMInject, PackageType: PackageTypeOCI}

	err := postInstallAPMInject(ctx)

	// No fake driver installer binary is set up, so the install still fails,
	// but at the later EvalSymlinks step -- proving enableSystemProbeConfig
	// succeeded and the flow proceeded past it unchanged.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "system-probe")

	data, readErr := os.ReadFile(filepath.Join(agentConfigDir, "system-probe.yaml"))
	require.NoError(t, readErr)
	var cfg config.SystemProbeConfig
	require.NoError(t, yaml.Unmarshal(data, &cfg))
	require.NotNil(t, cfg.WindowsCrashDetection.Enabled)
	assert.True(t, *cfg.WindowsCrashDetection.Enabled)
}

func TestPostInstallAPMInject_SystemProbeConfigFailureIsNonFatal(t *testing.T) {
	agentConfigDir := withTempInstallerPaths(t)
	// Make system-probe.yaml a directory so os.ReadFile fails with an error
	// other than "not exist", forcing enableSystemProbeConfig to fail.
	require.NoError(t, os.Mkdir(filepath.Join(agentConfigDir, "system-probe.yaml"), 0755))
	ctx := HookContext{Context: context.Background(), Package: packageAPMInject, PackageType: PackageTypeOCI}

	err := postInstallAPMInject(ctx)

	// The install must not abort because the system-probe config edit
	// failed: any error returned here must come from a later step (no
	// stable package present), never mention system-probe config.
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "system-probe")
}
