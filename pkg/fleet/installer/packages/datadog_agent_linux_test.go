// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentPackageUninstallPathsRemoveGeneratedProcmgrConfigs(t *testing.T) {
	installRoot := t.TempDir()
	generatedConfigs := []string{
		filepath.Join("processes.d", "datadog-agent-action-executor.yaml"),
	}
	customConfigs := []string{
		filepath.Join("processes.d", "custom.yaml"),
		filepath.Join("processes.d", "datadog-agent-custom.yaml"),
	}

	for _, relPath := range append(generatedConfigs, customConfigs...) {
		fullPath := filepath.Join(installRoot, relPath)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte("description: test\n"), 0644))
	}

	require.NoError(t, agentPackageUninstallPaths.EnsureAbsent(context.Background(), installRoot))

	for _, relPath := range generatedConfigs {
		_, err := os.Stat(filepath.Join(installRoot, relPath))
		assert.True(t, os.IsNotExist(err), "%s should be removed", relPath)
	}
	for _, relPath := range customConfigs {
		_, err := os.Stat(filepath.Join(installRoot, relPath))
		assert.NoError(t, err, "non-generated process manager config %s should be preserved", relPath)
	}
}
