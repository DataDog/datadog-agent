// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanKernelModulePathsIn(t *testing.T) {
	t.Run("plain .ko", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/fs/cifs"), 0o755))
		modulePath := filepath.Join(root, "kernel/fs/cifs/cifs.ko")
		require.NoError(t, os.WriteFile(modulePath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		require.Contains(t, paths, "cifs")
		assert.Equal(t, modulePath, paths["cifs"])
	})

	t.Run("compressed .ko.xz", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/net/cifs"), 0o755))
		modulePath := filepath.Join(root, "kernel/net/cifs/cifs.ko.xz")
		require.NoError(t, os.WriteFile(modulePath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		require.Contains(t, paths, "cifs")
		assert.Equal(t, modulePath, paths["cifs"])
	})

	t.Run("compressed .ko.zst", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/fs/cifs"), 0o755))
		modulePath := filepath.Join(root, "kernel/fs/cifs/cifs.ko.zst")
		require.NoError(t, os.WriteFile(modulePath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		require.Contains(t, paths, "cifs")
		assert.Equal(t, modulePath, paths["cifs"])
	})

	t.Run("compressed .ko.gz", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/fs/cifs"), 0o755))
		modulePath := filepath.Join(root, "kernel/fs/cifs/cifs.ko.gz")
		require.NoError(t, os.WriteFile(modulePath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		require.Contains(t, paths, "cifs")
		assert.Equal(t, modulePath, paths["cifs"])
	})

	t.Run("missing root", func(t *testing.T) {
		paths := scanKernelModulePathsIn(filepath.Join(t.TempDir(), "does", "not", "exist"))
		assert.Nil(t, paths)
	})

	t.Run("empty tree", func(t *testing.T) {
		root := t.TempDir()
		paths := scanKernelModulePathsIn(root)
		assert.Empty(t, paths)
	})

	t.Run("non-module files are ignored", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "kernel/modules.alias"), []byte(""), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "kernel/README"), []byte(""), 0o644))

		paths := scanKernelModulePathsIn(root)
		assert.Empty(t, paths)
	})

	t.Run("dash filename normalised to underscore key", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/drivers"), 0o755))
		modulePath := filepath.Join(root, "kernel/drivers/some-driver.ko")
		require.NoError(t, os.WriteFile(modulePath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		// /proc/modules reports this as some_driver, so the key must be the
		// underscore form regardless of how the file is named on disk.
		require.Contains(t, paths, "some_driver")
		assert.Equal(t, modulePath, paths["some_driver"])
		assert.NotContains(t, paths, "some-driver")
	})

	t.Run("symlinked parent resolved on path", func(t *testing.T) {
		realDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(realDir, "kernel/fs/cifs"), 0o755))
		realModulePath := filepath.Join(realDir, "kernel/fs/cifs/cifs.ko")
		require.NoError(t, os.WriteFile(realModulePath, []byte("stub"), 0o644))

		linkRoot := filepath.Join(t.TempDir(), "linked")
		require.NoError(t, os.Symlink(realDir, linkRoot))

		paths := scanKernelModulePathsIn(linkRoot)
		require.Contains(t, paths, "cifs")
		resolvedReal, err := filepath.EvalSymlinks(realModulePath)
		require.NoError(t, err)
		assert.Equal(t, resolvedReal, paths["cifs"])
	})

	t.Run("multiple modules in one scan", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/fs/cifs"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "kernel/net/nf"), 0o755))

		cifsPath := filepath.Join(root, "kernel/fs/cifs/cifs.ko")
		nfNatPath := filepath.Join(root, "kernel/net/nf/nf_nat.ko.xz")
		require.NoError(t, os.WriteFile(cifsPath, []byte("stub"), 0o644))
		require.NoError(t, os.WriteFile(nfNatPath, []byte("stub"), 0o644))

		paths := scanKernelModulePathsIn(root)
		assert.Equal(t, cifsPath, paths["cifs"])
		assert.Equal(t, nfNatPath, paths["nf_nat"])
		assert.Len(t, paths, 2)
	})
}
