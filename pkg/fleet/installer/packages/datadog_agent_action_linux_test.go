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

// TestWritePARProcmgrConfig verifies the Private Action Runner dd-procmgr config is written to
// the package processes.d with a version-specific binary path (rewritten to installRoot) and
// config paths left as the ${DD_CONF_DIR} placeholder, which the supervising dd-procmgr
// substitutes at launch with its stable or experiment config directory.
func TestWritePARProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	configPath := filepath.Join(installRoot, "processes.d", parProcmgrConfigName)

	require.NoError(t, writePARProcmgrConfig(installRoot))
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "${DD_CONF_DIR}/datadog.yaml")
	assert.Contains(t, string(content), filepath.Join(installRoot, "embedded", "bin", "privateactionrunner"))
	assert.NotContains(t, string(content), "/opt/datadog-agent/")

	require.NoError(t, removePARProcmgrConfig(installRoot))
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))

	// Removal must be a no-op when the config was never written.
	require.NoError(t, removePARProcmgrConfig(installRoot))
}

func TestIsPARProcessManagerEnabled(t *testing.T) {
	dir := t.TempDir()
	datadogYamlPath := filepath.Join(dir, "datadog.yaml")

	// Absent datadog.yaml defaults to disabled.
	enabled, err := isPARProcessManagerEnabled(datadogYamlPath)
	require.NoError(t, err)
	assert.False(t, enabled)

	// Key absent from an existing file defaults to disabled.
	require.NoError(t, os.WriteFile(datadogYamlPath, []byte("api_key: abc\n"), 0640))
	enabled, err = isPARProcessManagerEnabled(datadogYamlPath)
	require.NoError(t, err)
	assert.False(t, enabled)

	// Explicit opt-in.
	require.NoError(t, os.WriteFile(datadogYamlPath, []byte("private_action_runner:\n  use_process_manager: true\n"), 0640))
	enabled, err = isPARProcessManagerEnabled(datadogYamlPath)
	require.NoError(t, err)
	assert.True(t, enabled)

	// Explicit opt-out.
	require.NoError(t, os.WriteFile(datadogYamlPath, []byte("private_action_runner:\n  use_process_manager: false\n"), 0640))
	enabled, err = isPARProcessManagerEnabled(datadogYamlPath)
	require.NoError(t, err)
	assert.False(t, enabled)
}
