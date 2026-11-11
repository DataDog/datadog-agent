// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package packages

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWireAndUnwireIntegrationDeps(t *testing.T) {
	packagesRoot := t.TempDir()
	depsPackagePath := filepath.Join(packagesRoot, IntegrationDepsPackageName, "stable")
	agentSitePackages := filepath.Join(packagesRoot, "datadog-agent", "stable", "embedded", "lib", "python3.12", "site-packages")

	require.NoError(t, os.MkdirAll(filepath.Join(depsPackagePath, "embedded", "lib", "python3.12", "site-packages", "pymysql"), 0o755))
	require.NoError(t, os.MkdirAll(agentSitePackages, 0o755))

	ctx := HookContext{Context: context.Background(), PackagePath: depsPackagePath}

	require.NoError(t, wireIntegrationDepsAt(ctx, packagesRoot, depsPackagePath))

	wired, err := os.Lstat(filepath.Join(agentSitePackages, "pymysql"))
	require.NoError(t, err)
	assert.True(t, wired.Mode()&os.ModeSymlink != 0, "pymysql must be wired through a symlink")
	target, err := os.Readlink(filepath.Join(agentSitePackages, "pymysql"))
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(target))

	require.NoError(t, unwireIntegrationDepsAt(ctx, packagesRoot, depsPackagePath))
	_, err = os.Lstat(filepath.Join(agentSitePackages, "pymysql"))
	assert.True(t, os.IsNotExist(err))
}

func TestWireIntegrationDepsDoesNotReplaceRealFiles(t *testing.T) {
	packagesRoot := t.TempDir()
	depsPackagePath := filepath.Join(packagesRoot, IntegrationDepsPackageName, "stable")
	agentSitePackages := filepath.Join(packagesRoot, "datadog-agent", "stable", "embedded", "lib", "python3.12", "site-packages")

	require.NoError(t, os.MkdirAll(filepath.Join(depsPackagePath, "embedded", "lib", "python3.12", "site-packages", "pymysql"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(agentSitePackages, "pymysql"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentSitePackages, "pymysql", "__init__.py"), []byte("shipped by the agent"), 0o644))

	ctx := HookContext{Context: context.Background(), PackagePath: depsPackagePath}
	require.NoError(t, wireIntegrationDepsAt(ctx, packagesRoot, depsPackagePath))

	content, err := os.ReadFile(filepath.Join(agentSitePackages, "pymysql", "__init__.py"))
	require.NoError(t, err)
	assert.Equal(t, "shipped by the agent", string(content), "real files in site-packages must never be replaced")
}

func TestRewireIntegrationDepsAfterAgentUpgrade(t *testing.T) {
	packagesRoot := t.TempDir()
	depsPackagePath := filepath.Join(packagesRoot, IntegrationDepsPackageName, "stable")
	oldAgentSitePackages := filepath.Join(packagesRoot, "datadog-agent", "stable", "embedded", "lib", "python3.12", "site-packages")
	newAgentSitePackages := filepath.Join(packagesRoot, "datadog-agent", "experiment", "embedded", "lib", "python3.12", "site-packages")

	require.NoError(t, os.MkdirAll(filepath.Join(depsPackagePath, "embedded", "lib", "python3.12", "site-packages", "pymysql"), 0o755))
	require.NoError(t, os.MkdirAll(oldAgentSitePackages, 0o755))
	require.NoError(t, os.MkdirAll(newAgentSitePackages, 0o755))

	ctx := HookContext{Context: context.Background(), PackagePath: depsPackagePath}
	require.NoError(t, wireIntegrationDepsIn(ctx, packagesRoot, packagesRoot))

	for _, dir := range []string{oldAgentSitePackages, newAgentSitePackages} {
		wired, err := os.Lstat(filepath.Join(dir, "pymysql"))
		require.NoError(t, err)
		assert.True(t, wired.Mode()&os.ModeSymlink != 0)
	}
}

func TestRewireIntegrationDepsWithoutPackage(t *testing.T) {
	packagesRoot := t.TempDir()
	assert.NoError(t, rewireIntegrationDepsIn(packagesRoot))
}

func rewireIntegrationDepsIn(packagesRoot string) error {
	return wireIntegrationDepsIn(HookContext{Context: context.Background()}, packagesRoot, packagesRoot)
}
