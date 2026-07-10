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
// processes.d with a version-specific binary path (rewritten to installRoot) and config paths left
// as the ${DD_CONF_DIR} placeholder, which the supervising dd-procmgr substitutes at launch with
// its stable or experiment config directory. It must be a no-op when DDOT is not installed.
func TestWriteDDOTProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	configPath := filepath.Join(installRoot, "processes.d", ddotProcmgrConfigName)

	// No-op when DDOT is not installed (the otel-agent binary is absent).
	require.NoError(t, writeDDOTProcmgrConfig(installRoot))
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "must not write a procmgr config when DDOT is not installed")

	// Simulate an installed DDOT extension so the writer runs.
	otelAgentDir := filepath.Join(installRoot, "ext", "ddot", "embedded", "bin")
	require.NoError(t, os.MkdirAll(otelAgentDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(otelAgentDir, "otel-agent"), []byte("#!/bin/true\n"), 0755))

	require.NoError(t, writeDDOTProcmgrConfig(installRoot))
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	// Config paths stay as the placeholder (resolved by dd-procmgr per stable/experiment).
	assert.Contains(t, string(content), "${DD_CONF_DIR}/otel-config.yaml")
	assert.Contains(t, string(content), "${DD_CONF_DIR}/datadog.yaml")
	// The binary path is version-specific: rewritten to this installRoot, never left at /opt/datadog-agent.
	assert.Contains(t, string(content), filepath.Join(installRoot, "ext", "ddot", "embedded", "bin", "otel-agent"))
	assert.NotContains(t, string(content), "/opt/datadog-agent/")

	// Removal clears the file.
	require.NoError(t, removeDDOTProcmgrConfig(installRoot))
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}
