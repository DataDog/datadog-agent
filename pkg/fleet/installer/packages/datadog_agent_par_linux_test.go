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

func TestWritePARExecutorProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	configPath := filepath.Join(installRoot, "processes.d", parExecutorProcmgrConfigName)

	require.NoError(t, writePARExecutorProcmgrConfig(installRoot))
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "${DD_CONF_DIR}/datadog.yaml")
	assert.Contains(t, string(content), filepath.Join(installRoot, "embedded", "bin", "privateactionrunner"))
	assert.Contains(t, string(content), "auto_start: false")
	assert.Contains(t, string(content), "restart: never")
	assert.NotContains(t, string(content), "/opt/datadog-agent/")
}
