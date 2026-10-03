// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoveConfigFilesMissingFromSource(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	writeFile := func(root, path string) {
		t.Helper()
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
		require.NoError(t, os.WriteFile(fullPath, []byte(path), 0644))
	}

	for _, path := range []string{
		"datadog.yaml",
		"conf.d/existing.d/config.yaml",
	} {
		writeFile(sourceDir, path)
		writeFile(targetDir, path)
	}
	for _, path := range []string{
		"security-agent.yaml",
		"conf.d/new.d/config.yaml",
		"files/conf.d/customer.yaml",
		"conf.d/new.d/README.txt",
		"managed/datadog-agent/stable/metadata.json",
		"managed-user.yaml",
		"managed-custom/customer.yaml",
		"managed/datadog-agent/other/customer.yaml",
		"managed/datadog-agent/stable-custom/customer.yaml",
		"auth_token",
	} {
		writeFile(targetDir, path)
	}

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))

	assert.FileExists(t, filepath.Join(targetDir, "datadog.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "conf.d", "existing.d", "config.yaml"))
	assert.NoFileExists(t, filepath.Join(targetDir, "security-agent.yaml"))
	assert.NoFileExists(t, filepath.Join(targetDir, "conf.d", "new.d", "config.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "files", "conf.d", "customer.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "conf.d", "new.d", "README.txt"))
	assert.FileExists(t, filepath.Join(targetDir, "managed", "datadog-agent", "stable", "metadata.json"))
	assert.FileExists(t, filepath.Join(targetDir, "managed-user.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "managed-custom", "customer.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "managed", "datadog-agent", "other", "customer.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "managed", "datadog-agent", "stable-custom", "customer.yaml"))
	assert.FileExists(t, filepath.Join(targetDir, "auth_token"))
}

func TestRemoveConfigFilesMissingFromSourcePreservesNestedUnmanagedYAML(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	nestedUnmanaged := filepath.Join(targetDir, "conf.d", "check.d", "private", "customer.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(nestedUnmanaged), 0755))
	require.NoError(t, os.WriteFile(nestedUnmanaged, []byte("customer: true\n"), 0644))

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))
	assert.FileExists(t, nestedUnmanaged)
}

func TestRemoveConfigFilesMissingFromSourcePrunesEmptiedDirs(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// A check directory the experiment added, absent from the backup.
	newCheck := filepath.Join(targetDir, "conf.d", "new.d")
	require.NoError(t, os.MkdirAll(newCheck, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(newCheck, "config.yaml"), []byte("enabled: true\n"), 0644))

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))

	assert.NoFileExists(t, filepath.Join(newCheck, "config.yaml"))
	assert.NoDirExists(t, newCheck, "the emptied check directory should be pruned")
	assert.NoDirExists(t, filepath.Join(targetDir, "conf.d"), "conf.d should be pruned once it is empty as well")
}

func TestRemoveConfigFilesMissingFromSourceKeepsDirsWithUnmanagedFiles(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	checkDir := filepath.Join(targetDir, "conf.d", "new.d")
	require.NoError(t, os.MkdirAll(checkDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(checkDir, "config.yaml"), []byte("enabled: true\n"), 0644))
	unmanaged := filepath.Join(checkDir, "notes.txt")
	require.NoError(t, os.WriteFile(unmanaged, []byte("keep me\n"), 0644))

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))

	assert.NoFileExists(t, filepath.Join(checkDir, "config.yaml"))
	assert.FileExists(t, unmanaged, "unmanaged files must survive the cleanup")
	assert.DirExists(t, checkDir, "a directory still holding unmanaged files must not be pruned")
	assert.DirExists(t, filepath.Join(targetDir, "conf.d"))
}

func TestRemoveConfigFilesMissingFromSourceKeepsUntouchedEmptyDirs(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	// An already empty check directory that the cleanup never removes a file from.
	untouched := filepath.Join(targetDir, "conf.d", "untouched.d")
	require.NoError(t, os.MkdirAll(untouched, 0755))

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))

	assert.DirExists(t, untouched, "directories the cleanup did not empty must be left alone")
	assert.DirExists(t, filepath.Join(targetDir, "conf.d"))
}

func TestRemoveConfigFilesMissingFromSourceKeepsDirsPresentInSource(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "conf.d", "check.d"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "conf.d", "check.d"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "conf.d", "check.d", "config.yaml"), []byte("a: 1\n"), 0644))

	require.NoError(t, removeConfigFilesMissingFromSource(sourceDir, targetDir))

	assert.NoFileExists(t, filepath.Join(targetDir, "conf.d", "check.d", "config.yaml"))
	assert.DirExists(t, filepath.Join(targetDir, "conf.d", "check.d"), "a directory the backup still has must be kept")
}

func TestVerifyConfigFilesCopied(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()

	sourceConfigPath := filepath.Join(sourceDir, "datadog.yaml")
	require.NoError(t, os.WriteFile(sourceConfigPath, []byte("log_level: info\n"), 0644))
	unmanagedPath := filepath.Join(sourceDir, "files", "conf.d", "customer.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(unmanagedPath), 0755))
	require.NoError(t, os.WriteFile(unmanagedPath, []byte("customer: true\n"), 0644))
	legacyMetadataPath := filepath.Join(sourceDir, "managed", "datadog-agent", "stable", "metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(legacyMetadataPath), 0755))
	require.NoError(t, os.WriteFile(legacyMetadataPath, []byte("{}"), 0644))

	err := verifyConfigFilesCopied(sourceDir, targetDir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "datadog.yaml")

	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "datadog.yaml"), []byte("log_level: info\n"), 0644))
	require.NoError(t, os.MkdirAll(filepath.Join(targetDir, "files", "conf.d"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "files", "conf.d", "customer.yaml"), []byte("customer: true\n"), 0644))
	require.NoError(t, verifyConfigFilesCopied(sourceDir, targetDir))
}

func TestReconcileLegacyManagedLinksBeforeCopy(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	managedDir := filepath.Join(sourceDir, "managed", "datadog-agent")
	require.NoError(t, os.MkdirAll(filepath.Join(managedDir, "v2"), 0755))
	require.NoError(t, os.Symlink(filepath.Join(managedDir, "v2"), filepath.Join(managedDir, "stable")))
	require.NoError(t, os.Symlink(filepath.Join(managedDir, "v2"), filepath.Join(managedDir, "experiment")))

	targetManagedDir := filepath.Join(targetDir, "managed", "datadog-agent")
	require.NoError(t, os.MkdirAll(filepath.Join(targetManagedDir, "stable"), 0755))
	require.NoError(t, os.MkdirAll(filepath.Join(targetManagedDir, "experiment"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(targetManagedDir, "stable", "application_monitoring.yaml"), []byte("enabled: true\n"), 0644))

	require.NoError(t, reconcileLegacyManagedLinksBeforeCopy(sourceDir, targetDir))
	_, err := os.Lstat(filepath.Join(targetManagedDir, "stable"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
	_, err = os.Lstat(filepath.Join(targetManagedDir, "experiment"))
	require.Error(t, err)
	assert.True(t, os.IsNotExist(err))
}

func TestVerifyLegacyManagedLinksCopied(t *testing.T) {
	sourceDir := t.TempDir()
	targetDir := t.TempDir()
	managedDir := filepath.Join(sourceDir, "managed", "datadog-agent")
	require.NoError(t, os.MkdirAll(filepath.Join(managedDir, "v2"), 0755))
	linkTarget := filepath.Join(managedDir, "v2")
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(managedDir, "stable")))
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(managedDir, "experiment")))

	targetManagedDir := filepath.Join(targetDir, "managed", "datadog-agent")
	require.NoError(t, os.MkdirAll(targetManagedDir, 0755))
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(targetManagedDir, "stable")))
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(targetManagedDir, "experiment")))

	sourceRoot, err := os.OpenRoot(sourceDir)
	require.NoError(t, err)
	defer sourceRoot.Close()
	targetRoot, err := os.OpenRoot(targetDir)
	require.NoError(t, err)
	defer targetRoot.Close()

	require.NoError(t, verifyLegacyManagedLinksCopied(sourceRoot, targetRoot))

	require.NoError(t, os.Remove(filepath.Join(targetManagedDir, "stable")))
	require.NoError(t, os.MkdirAll(filepath.Join(targetManagedDir, "stable"), 0755))
	err = verifyLegacyManagedLinksCopied(sourceRoot, targetRoot)
	require.Error(t, err)
	assert.ErrorContains(t, err, "not restored as a symlink")
}
