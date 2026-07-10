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

// TestWriteDDOTProcmgrConfig verifies that the DDOT dd-procmgr config is written to two separate
// directories: the stable config (processes.d) points the collector at the stable config tree, and
// the experiment config (processes.d.experiment) points it at the experiment config tree. Keeping
// them separate guarantees an experiment never mutates the stable collector config.
func TestWriteDDOTProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	stablePath := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)
	experimentPath := filepath.Join(installRoot, ddotProcmgrExperimentDirName, ddotProcmgrConfigName)

	// No-op when DDOT is not installed (the otel-agent binary is absent).
	require.NoError(t, writeDDOTProcmgrConfig(installRoot))
	_, err := os.Stat(stablePath)
	assert.True(t, os.IsNotExist(err), "must not write a procmgr config when DDOT is not installed")

	// Simulate an installed DDOT extension so the writer runs.
	otelAgentDir := filepath.Join(installRoot, "ext", "ddot", "embedded", "bin")
	require.NoError(t, os.MkdirAll(otelAgentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otelAgentDir, "otel-agent"), []byte("#!/bin/true\n"), 0755))

	require.NoError(t, writeDDOTProcmgrConfig(installRoot))

	stable, err := os.ReadFile(stablePath)
	require.NoError(t, err)
	assert.Contains(t, string(stable), "/etc/datadog-agent/otel-config.yaml")
	assert.NotContains(t, string(stable), "/etc/datadog-agent-exp")
	assert.Contains(t, string(stable), filepath.Join(installRoot, "ext", "ddot", "embedded", "bin", "otel-agent"))
	assert.NotContains(t, string(stable), "/opt/datadog-agent/")

	experiment, err := os.ReadFile(experimentPath)
	require.NoError(t, err)
	assert.Contains(t, string(experiment), "/etc/datadog-agent-exp/otel-config.yaml")
	assert.Contains(t, string(experiment), "/etc/datadog-agent-exp/datadog.yaml")
	assert.Contains(t, string(experiment), filepath.Join(installRoot, "ext", "ddot", "embedded", "bin", "otel-agent"))
	assert.NotContains(t, string(experiment), "/opt/datadog-agent/")

	// Removal clears both.
	require.NoError(t, removeDDOTProcmgrConfig(installRoot))
	_, err = os.Stat(stablePath)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(experimentPath)
	assert.True(t, os.IsNotExist(err))
}
