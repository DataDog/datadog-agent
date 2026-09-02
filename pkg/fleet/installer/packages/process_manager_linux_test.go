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

func TestUpsertEnvFileValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "environment")

	// Creates the file (and parent directories) when absent.
	require.NoError(t, upsertEnvFileValue(path, "DD_PROCESS_MANAGER_ENABLED", "true"))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "DD_PROCESS_MANAGER_ENABLED=true\n", string(content))

	// Overwrites an existing value for the same key rather than appending a duplicate line.
	require.NoError(t, upsertEnvFileValue(path, "DD_PROCESS_MANAGER_ENABLED", "false"))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "DD_PROCESS_MANAGER_ENABLED=false\n", string(content))

	// Preserves unrelated existing lines.
	require.NoError(t, os.WriteFile(path, []byte("DD_OTHER_VAR=1\nDD_PROCESS_MANAGER_ENABLED=true\n"), 0644))
	require.NoError(t, upsertEnvFileValue(path, "DD_PROCESS_MANAGER_ENABLED", "false"))
	content, err = os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "DD_OTHER_VAR=1\nDD_PROCESS_MANAGER_ENABLED=false\n", string(content))
}
