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
	require.NotNil(t, cfg.SystemProbeSettings.Enabled)
	assert.True(t, *cfg.SystemProbeSettings.Enabled)
}

func TestEnableSystemProbeConfig_AlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	initial := config.SystemProbeConfig{
		SystemProbeSettings: config.SystemProbeSettings{Enabled: config.BoolToPtr(true)},
		RuntimeSecurityConfig: config.RuntimeSecurityConfig{
			Enabled: config.BoolToPtr(true),
		},
	}
	writeYAML(t, configPath, initial)
	infoBefore, _ := os.Stat(configPath)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	infoAfter, _ := os.Stat(configPath)
	assert.Equal(t, infoBefore.ModTime(), infoAfter.ModTime(), "file should not be rewritten when already enabled")
}

func TestEnableSystemProbeConfig_PreservesExistingSettings(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	initial := config.SystemProbeConfig{
		RuntimeSecurityConfig: config.RuntimeSecurityConfig{
			Enabled: config.BoolToPtr(true),
		},
		GPUMonitoringConfig: config.GPUMonitoringConfig{
			Enabled: config.BoolToPtr(true),
		},
	}
	writeYAML(t, configPath, initial)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	var result config.SystemProbeConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &result))

	require.NotNil(t, result.SystemProbeSettings.Enabled)
	assert.True(t, *result.SystemProbeSettings.Enabled)
	require.NotNil(t, result.RuntimeSecurityConfig.Enabled)
	assert.True(t, *result.RuntimeSecurityConfig.Enabled)
	require.NotNil(t, result.GPUMonitoringConfig.Enabled)
	assert.True(t, *result.GPUMonitoringConfig.Enabled)
}

func TestEnableSystemProbeConfig_FlipsDisabledToEnabled(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "system-probe.yaml")

	initial := config.SystemProbeConfig{
		SystemProbeSettings: config.SystemProbeSettings{Enabled: config.BoolToPtr(false)},
		RuntimeSecurityConfig: config.RuntimeSecurityConfig{
			Enabled: config.BoolToPtr(true),
		},
	}
	writeYAML(t, configPath, initial)

	err := enableSystemProbeConfigAt(configPath)
	require.NoError(t, err)

	var result config.SystemProbeConfig
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, yaml.Unmarshal(data, &result))

	assert.True(t, *result.SystemProbeSettings.Enabled)
	assert.True(t, *result.RuntimeSecurityConfig.Enabled, "existing settings should be preserved")
}

func writeYAML(t *testing.T, path string, v any) {
	t.Helper()
	data, err := yaml.Marshal(v)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0640))
}
