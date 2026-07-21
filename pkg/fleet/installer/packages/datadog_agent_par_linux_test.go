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

func TestWritePARProcmgrConfig(t *testing.T) {
	installRoot := t.TempDir()
	configPath := filepath.Join(installRoot, "processes.d", parProcmgrConfigName)

	require.NoError(t, writePARProcmgrConfig(installRoot))
	_, err := os.Stat(configPath)
	assert.True(t, os.IsNotExist(err), "must not write a procmgr config when PAR is not installed")

	parBinaryDir := filepath.Join(installRoot, "embedded", "bin")
	require.NoError(t, os.MkdirAll(parBinaryDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(parBinaryDir, "privateactionrunner"), []byte("#!/bin/true\n"), 0755))

	require.NoError(t, writePARProcmgrConfig(installRoot))
	content, err := os.ReadFile(configPath)
	require.NoError(t, err)

	assert.Contains(t, string(content), "${DD_CONF_DIR}/datadog.yaml")
	assert.Contains(t, string(content), filepath.Join(installRoot, "embedded", "bin", "privateactionrunner"))
	assert.NotContains(t, string(content), "/opt/datadog-agent/")

	require.NoError(t, removePARProcmgrConfig(installRoot))
	_, err = os.Stat(configPath)
	assert.True(t, os.IsNotExist(err))
}
