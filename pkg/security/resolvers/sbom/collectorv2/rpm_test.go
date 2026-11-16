// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	rpmdb "github.com/knqyf263/go-rpmdb/pkg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndexedRPMFilesSizes(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		name = filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, []byte(content), 0o755))
	}
	write("top", "12345")
	write("usr/bin/kept", "12345")
	write("usr/bin/copied", "copied over at build time")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "usr/bin/dir"), 0o755))
	require.NoError(t, os.Symlink("12345", filepath.Join(dir, "usr/bin/link")))
	require.NoError(t, os.Symlink("/usr", filepath.Join(dir, "opt")))
	write("usr/lib/big", "")
	require.NoError(t, os.Truncate(filepath.Join(dir, "usr/lib/big"), 3_000_000_000))

	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()

	const reg = syscall.S_IFREG | 0o755
	installed := []rpmdb.FileInfo{
		{Path: "/top", Mode: reg, Size: 5},
		{Path: "/usr/bin/copied", Mode: reg, Size: 5},
		{Path: "/usr/bin/deleted", Mode: reg, Size: 5},
		{Path: "/usr/bin/dir", Mode: reg, Size: 5},
		{Path: "/usr/bin/kept", Mode: reg, Size: 5},
		{Path: "/usr/bin/link", Mode: reg, Size: 5},
		{Path: "/usr/lib/big", Mode: reg, Size: -1294967296}, // 3e9 in rpm's int32
		{Path: "/usr/share/doc/tool/README", Mode: reg, Size: 5, Flags: rpmdb.FileFlags(rpmdb.RPMFILE_DOC)},
		{Path: "/usr/share/gone/file", Mode: reg, Size: 5},
		{Path: "/opt/tool", Mode: reg, Size: 5},
	}

	assert.Equal(t, []string{"/top", "/usr/bin/kept", "/usr/lib/big", "/usr/share/doc/tool/README", "/opt/tool"}, indexedRPMFiles(root, installed, true))
	assert.Len(t, indexedRPMFiles(root, installed, false), len(installed))
}

// TestIndexedRPMFilesRelativePaths checks that the files a malformed header lists
// without a leading slash stay in the index unchecked.
func TestIndexedRPMFilesRelativePaths(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	defer root.Close()

	installed := []rpmdb.FileInfo{
		{Path: "tool", Mode: syscall.S_IFREG | 0o755, Size: 5},
		{Path: "usr/bin/tool", Mode: syscall.S_IFREG | 0o755, Size: 5},
	}
	assert.Equal(t, []string{"tool", "usr/bin/tool"}, indexedRPMFiles(root, installed, true))
}

func TestPrelinked(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	defer root.Close()

	assert.False(t, prelinked(root))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "etc"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "etc/prelink.cache"), nil, 0o644))
	assert.True(t, prelinked(root))
}
