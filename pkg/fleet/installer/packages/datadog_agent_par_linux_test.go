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

func TestWritePARProcmgrConfigs(t *testing.T) {
	installRoot := t.TempDir()
	require.NoError(t, writePARProcmgrConfigs(installRoot))

	executor := readProcmgrConfig(t, installRoot, parExecutorProcmgrConfigName)
	assert.Contains(t, executor, "${DD_CONF_DIR}/datadog.yaml")
	assert.Contains(t, executor, filepath.Join(installRoot, "embedded", "bin", "privateactionrunner"))
	// Only par-control starts the executor, and procmgr must not resurrect it.
	assert.Contains(t, executor, "auto_start: false")
	assert.Contains(t, executor, "restart: never")
	assert.NotContains(t, executor, "/opt/datadog-agent/")

	control := readProcmgrConfig(t, installRoot, parControlProcmgrConfigName)
	assert.Contains(t, control, "${DD_CONF_DIR}/datadog.yaml")
	assert.Contains(t, control, filepath.Join(installRoot, "embedded", "bin", "par-control"))
	// The control plane is always on: procmgr starts it with the Agent and restarts
	// it on crash. on-failure ignores the clean exit taken when split mode is off.
	assert.Contains(t, control, "auto_start: true")
	assert.Contains(t, control, "stop_timeout: 30")
	assert.Contains(t, control, "restart: on-failure")
	assert.NotContains(t, control, "/opt/datadog-agent/")
}

func readProcmgrConfig(t *testing.T, installRoot, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(installRoot, "processes.d", name))
	require.NoError(t, err)
	return string(content)
}
