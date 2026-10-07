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

func TestIndexedRPMFiles(t *testing.T) {
	installed := []rpmdb.FileInfo{
		{Path: "/etc/passwd", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_CONFIG | rpmdb.RPMFILE_NOREPLACE)},
		{Path: "/etc/ld.so.cache", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_GHOST)},
		{Path: "/usr/share/doc/glibc/README", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_DOC)},
		{Path: "/usr/share/licenses/glibc/COPYING", Flags: rpmdb.FileFlags(rpmdb.RPMFILE_LICENSE)},
		{Path: "/usr/lib64/libc.so.6"},
		{Path: "/usr/lib64", Mode: 040755},
	}

	assert.Equal(t, []string{"/usr/lib64/libc.so.6", "/usr/lib64"}, indexedRPMFiles(nil, installed, false))
}

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
		{Path: "/usr/share/gone/file", Mode: reg, Size: 5},
		{Path: "/opt/tool", Mode: reg, Size: 5},
	}

	assert.Equal(t, []string{"/top", "/usr/bin/kept", "/usr/lib/big", "/opt/tool"}, indexedRPMFiles(root, installed, true))
	assert.Len(t, indexedRPMFiles(root, installed, false), len(installed))
}
