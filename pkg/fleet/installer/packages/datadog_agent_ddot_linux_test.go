// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWriteDDOTProcmgrConfig verifies the DDOT dd-procmgr config is written to the package
// processes.d with a version-specific binary path (rewritten to installRoot) and with the stable or
// experiment config directory baked in depending on isExperiment. It must be a no-op when DDOT is
// not installed.
func TestWriteDDOTProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	configPath := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)

	// No-op when DDOT is not installed (the otel-agent binary is absent).
	require.NoError(t, writeDDOTProcmgrConfig(installRoot, false))
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "must not write a procmgr config when DDOT is not installed")

	// Simulate an installed DDOT extension so the writer runs.
	otelAgentDir := filepath.Join(installRoot, "ext", "ddot", "embedded", "bin")
	require.NoError(t, os.MkdirAll(otelAgentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otelAgentDir, "otel-agent"), []byte("#!/bin/true\n"), 0755))

	require.NoError(t, writeDDOTProcmgrConfig(installRoot, false))
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Stable config points at the stable config directory.
	assert.Contains(t, string(content), "/etc/datadog-agent/otel-config.yaml")
	assert.Contains(t, string(content), "/etc/datadog-agent/datadog.yaml")
	assert.NotContains(t, string(content), "/etc/datadog-agent-exp")
	// The binary path is version-specific: rewritten to this installRoot, never left at /opt/datadog-agent.
	assert.Contains(t, string(content), filepath.Join(installRoot, "ext", "ddot", "embedded", "bin", "otel-agent"))
	assert.NotContains(t, string(content), "/opt/datadog-agent/")

	require.NoError(t, writeDDOTProcmgrConfig(installRoot, true))
	expContent, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Experiment config points at the experiment config directory.
	assert.Contains(t, string(expContent), "/etc/datadog-agent-exp/otel-config.yaml")
	assert.Contains(t, string(expContent), "/etc/datadog-agent-exp/datadog.yaml")
	assert.Contains(t, string(expContent), "DD_INVENTORIES_FIRST_RUN_DELAY")

	// Removal clears the file.
	require.NoError(t, removeDDOTProcmgrConfig(installRoot))
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}

// TestSetDDOTProcmgrConfigForConfigExperiment verifies the config-experiment helper resolves the
// package install root from the hook context, toggles the procmgr config between the experiment and
// stable config directories, and is a no-op when DDOT is not installed.
func TestSetDDOTProcmgrConfigForConfigExperiment(t *testing.T) {
	installRoot := t.TempDir()
	ctx := HookContext{PackagePath: installRoot}
	configPath := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)

	// No-op when DDOT is not installed under ctx.PackagePath (and /opt/datadog-agent is absent in CI).
	require.NoError(t, setDDOTProcmgrConfigForConfigExperiment(ctx, true))
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "must not write a procmgr config when DDOT is not installed")

	// Simulate an installed DDOT extension so the writer runs.
	otelAgentDir := filepath.Join(installRoot, "ext", "ddot", "embedded", "bin")
	require.NoError(t, os.MkdirAll(otelAgentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otelAgentDir, "otel-agent"), []byte("#!/bin/true\n"), 0755))

	// Experiment: the collector is pointed at the experiment config directory.
	require.NoError(t, setDDOTProcmgrConfigForConfigExperiment(ctx, true))
	expContent, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(expContent), "/etc/datadog-agent-exp/otel-config.yaml")
	assert.Contains(t, string(expContent), "/etc/datadog-agent-exp/datadog.yaml")

	// Stable: the collector is pointed back at the stable config directory.
	require.NoError(t, setDDOTProcmgrConfigForConfigExperiment(ctx, false))
	stableContent, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(stableContent), "/etc/datadog-agent/otel-config.yaml")
	assert.NotContains(t, string(stableContent), "/etc/datadog-agent-exp")
}
