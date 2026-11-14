// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package processmanager

import (
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/embedded"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/packages/service"
)

const sysprobeEntry = "datadog-agent-sysprobe.yaml"

func setUmask(t *testing.T, mask int) {
	t.Helper()
	orig := syscall.Umask(mask)
	t.Cleanup(func() { syscall.Umask(orig) })
}

func shippedEntry(t *testing.T) []byte {
	t.Helper()
	content, err := embedded.GetDarwinProcmgrProcess(sysprobeEntry)
	require.NoError(t, err)
	return content
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func assertShippedCatalog(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, service.ProcessesDirName)
	dirInfo, err := os.Lstat(dir)
	require.NoError(t, err)
	assert.True(t, dirInfo.IsDir())
	assert.Equal(t, os.FileMode(0o755), dirInfo.Mode().Perm())

	entry := filepath.Join(dir, sysprobeEntry)
	fi, err := os.Lstat(entry)
	require.NoError(t, err)
	assert.True(t, fi.Mode().IsRegular())
	assert.Equal(t, os.FileMode(0o644), fi.Mode().Perm())
	content, err := os.ReadFile(entry)
	require.NoError(t, err)
	assert.Equal(t, shippedEntry(t), content)

	for _, name := range dirNames(t, dir) {
		assert.NotContains(t, name, ".tmp-", "temp file left behind")
	}
}

func TestWriteDarwinProcmgrCatalogWritesTheEntry(t *testing.T) {
	root := t.TempDir()
	setUmask(t, 0o077)
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)
	assert.Equal(t, []string{sysprobeEntry}, dirNames(t, filepath.Join(root, service.ProcessesDirName)))
}

func TestWriteDarwinProcmgrCatalogReplacesStaleContentAndASymlink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)

	entry := filepath.Join(root, service.ProcessesDirName, sysprobeEntry)
	require.NoError(t, os.WriteFile(entry, []byte("auto_start: true\n"), 0o644))
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)

	outside := filepath.Join(t.TempDir(), "outside.yaml")
	require.NoError(t, os.WriteFile(outside, []byte("auto_start: true\n"), 0o644))
	require.NoError(t, os.Remove(entry))
	require.NoError(t, os.Symlink(outside, entry))
	require.NoError(t, WriteDarwinProcmgrCatalog(root))
	assertShippedCatalog(t, root)
	content, err := os.ReadFile(outside)
	require.NoError(t, err)
	assert.Equal(t, "auto_start: true\n", string(content))
}

func TestRemoveDarwinProcmgrCatalog(t *testing.T) {
	t.Run("keeps a foreign entry and the directory", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, WriteDarwinProcmgrCatalog(root))
		dir := filepath.Join(root, service.ProcessesDirName)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "custom.yaml"), []byte("command: /bin/custom\n"), 0o644))

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.Equal(t, []string{"custom.yaml"}, dirNames(t, dir))
	})
	t.Run("removes the directory it emptied", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, WriteDarwinProcmgrCatalog(root))

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.NoDirExists(t, filepath.Join(root, service.ProcessesDirName))
	})
	t.Run("missing directory", func(t *testing.T) {
		require.NoError(t, RemoveDarwinProcmgrCatalog(t.TempDir()))
	})
	t.Run("missing install root", func(t *testing.T) {
		require.NoError(t, RemoveDarwinProcmgrCatalog(filepath.Join(t.TempDir(), "missing")))
	})
	t.Run("symlinked processes.d", func(t *testing.T) {
		root := t.TempDir()
		target := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(target, sysprobeEntry), []byte("auto_start: false\n"), 0o644))
		require.NoError(t, os.Symlink(target, filepath.Join(root, service.ProcessesDirName)))

		require.NoError(t, RemoveDarwinProcmgrCatalog(root))
		assert.FileExists(t, filepath.Join(target, sysprobeEntry))
	})
}
