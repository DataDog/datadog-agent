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
	"golang.org/x/sys/unix"
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

// nsInode returns the inode of the namespace file, which is the namespace ID
func nsInode(t *testing.T, path string) uint32 {
	t.Helper()
	var stat unix.Stat_t
	require.NoError(t, unix.Stat(path, &stat))
	return uint32(stat.Ino)
}

// assertNSPath checks the ID returned for path, and that the handle points to that namespace
func assertNSPath(t *testing.T, path *NSPath, expected uint32) {
	t.Helper()

	nsID, err := path.GetNSID()
	require.NoError(t, err)
	assert.Equal(t, expected, nsID)

	handle, handleNSID, err := path.Open()
	require.NoError(t, err)
	defer handle.Close()
	assert.Equal(t, expected, handleNSID)
	assert.Equal(t, path.GetPath(), handle.Name())

	var stat unix.Stat_t
	require.NoError(t, unix.Fstat(int(handle.Fd()), &stat))
	assert.EqualValues(t, expected, stat.Ino)
}

func TestNSPathGetNSID(t *testing.T) {
	t.Run("magic link", func(t *testing.T) {
		assertNSPath(t, NewNSPathFromPath("/proc/self/ns/net", NetNsType), nsInode(t, "/proc/self/ns/net"))
	})

	t.Run("pid", func(t *testing.T) {
		assertNSPath(t, NewNSPathFromPid(Getpid(), MntNsType), nsInode(t, "/proc/self/ns/mnt"))
	})

	t.Run("for children", func(t *testing.T) {
		if _, err := os.Stat("/proc/self/ns/pid_for_children"); err != nil {
			t.Skipf("requires pid_for_children, added in Linux 4.12: %v", err)
		}
		assertNSPath(t, NewNSPathFromPath("/proc/self/ns/pid_for_children", PidForChildrenNsType), nsInode(t, "/proc/self/ns/pid_for_children"))
	})

	t.Run("symlink to a magic link", func(t *testing.T) {
		link := filepath.Join(t.TempDir(), "net")
		require.NoError(t, os.Symlink("/proc/self/ns/net", link))
		assertNSPath(t, NewNSPathFromPath(link, NetNsType), nsInode(t, "/proc/self/ns/net"))
	})

	t.Run("namespace of another type", func(t *testing.T) {
		_, err := NewNSPathFromPath("/proc/self/ns/uts", NetNsType).GetNSID()
		assert.Error(t, err)
	})

	t.Run("fifo", func(t *testing.T) {
		fifo := filepath.Join(t.TempDir(), "fifo")
		require.NoError(t, unix.Mkfifo(fifo, 0o600))
		_, _, err := NewNSPathFromPath(fifo, NetNsType).Open()
		assert.Error(t, err)
	})
}

func TestNSPathGetNSIDBindMount(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("requires root to bind mount a namespace")
	}

	target := filepath.Join(t.TempDir(), "net")
	require.NoError(t, os.WriteFile(target, nil, 0o600))
	if err := unix.Mount("/proc/self/ns/net", target, "", unix.MS_BIND, ""); err != nil {
		t.Skipf("requires CAP_SYS_ADMIN to bind mount a namespace: %v", err)
	}
	t.Cleanup(func() {
		_ = unix.Unmount(target, unix.MNT_DETACH)
	})

	assertNSPath(t, NewNSPathFromPath(target, NetNsType), nsInode(t, "/proc/self/ns/net"))

	// a bind mount is read through its file system, which must also check the namespace type
	utsTarget := filepath.Join(t.TempDir(), "uts")
	require.NoError(t, os.WriteFile(utsTarget, nil, 0o600))
	require.NoError(t, unix.Mount("/proc/self/ns/uts", utsTarget, "", unix.MS_BIND, ""))
	t.Cleanup(func() {
		_ = unix.Unmount(utsTarget, unix.MNT_DETACH)
	})
	_, err := NewNSPathFromPath(utsTarget, NetNsType).GetNSID()
	assert.Error(t, err)
}
