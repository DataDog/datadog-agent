// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProcessManagerEnabledFromEnvPersistedFallback guards against the bug where a bare CLI
// invocation with no DD_PROCESS_MANAGER_ENABLED set (e.g. `datadog-agent otel install`, or a
// plain `datadog-installer <command>` not spawned by the daemon) would always read the package
// default (true) instead of the last value persisted by the process-manager flip command or at
// install time, silently reverting a prior "disable".
func TestProcessManagerEnabledFromEnvPersistedFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "environment")
	oldPath := ProcessManagerEnvFilePath
	ProcessManagerEnvFilePath = path
	t.Cleanup(func() { ProcessManagerEnvFilePath = oldPath })

	t.Setenv(envProcessManagerEnabled, "")

	// Nothing persisted yet: falls back to the package default (true).
	assert.True(t, ProcessManagerEnabledFromEnv())

	// Once persisted, a process with no explicit env var reads the persisted value.
	require.NoError(t, os.WriteFile(path, []byte("DD_PROCESS_MANAGER_ENABLED=false\n"), 0644))
	assert.False(t, ProcessManagerEnabledFromEnv(), "persisted state must apply even with no DD_PROCESS_MANAGER_ENABLED in this process's own environment")

	// An explicit env var still wins over the persisted value.
	t.Setenv(envProcessManagerEnabled, "true")
	assert.True(t, ProcessManagerEnabledFromEnv(), "an explicit env var must override the persisted state")
}
