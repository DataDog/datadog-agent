// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package allowedpaths

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/allowedpaths/internal/fsstat"
)

func rawStatFSPath(parts ...string) string {
	return strings.Join(parts, string(filepath.Separator))
}

func TestSandboxStatFSAllowedPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	for _, path := range []string{".", "file"} {
		info, err := sb.StatFS(path, dir)
		require.NoError(t, err)
		assert.True(t, info.IDAvailable)
		assert.NotEmpty(t, info.TypeName)
		assert.NotZero(t, info.IOBlockSize)
		assert.NotZero(t, info.FundamentalBlockSize)
		assert.NotZero(t, info.Blocks)
	}
}

func TestSandboxStatFSRejectsOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()

	sb, _, err := New([]string{allowed})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(outside, allowed)
	assert.ErrorIs(t, err, fs.ErrPermission)
}

func TestSandboxStatFSMissingAndEmptyPaths(t *testing.T) {
	dir := t.TempDir()
	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	for _, path := range []string{"missing", ""} {
		_, err := sb.StatFS(path, dir)
		assert.ErrorIs(t, err, fs.ErrNotExist)
	}
}

func TestSandboxStatFSRejectsNonDirectoryIntermediateComponent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(filepath.Join("file", "child"), dir)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS(filepath.Join("missing", "child"), dir)
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func TestSandboxStatFSTrailingSlashRequiresDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o700))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS("file/", dir)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS("subdir/", dir)
	assert.NoError(t, err)
}

func TestSandboxStatFSPreservesDotDotComponents(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o700))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(rawStatFSPath("file", ".."), dir)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS(rawStatFSPath("missing", ".."), dir)
	assert.ErrorIs(t, err, fs.ErrNotExist)

	_, err = sb.StatFS(rawStatFSPath("subdir", "..", "file"), dir)
	assert.NoError(t, err)

	_, err = sb.StatFS(rawStatFSPath("..", "file"), filepath.Join(dir, "subdir"))
	assert.NoError(t, err)

	_, err = sb.StatFS(rawStatFSPath(dir, "subdir", "..", "file"), dir)
	assert.NoError(t, err)
}

func TestSandboxStatFSFollowsAllowedSymlink(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(target, "file"), []byte("data"), 0o600))

	link := filepath.Join(source, "link")
	if err := os.Symlink(filepath.Join(target, "file"), link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	sb, _, err := New([]string{source, target})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(link, source)
	require.NoError(t, err)

	_, err = sb.StatFS(link+string(filepath.Separator), source)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)
}

func TestSandboxStatFSPreservesDotDotAfterAllowedSymlink(t *testing.T) {
	source := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(target, "child"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(target, "file"), []byte("data"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(target, "sibling"), []byte("data"), 0o600))

	dirLink := filepath.Join(source, "dirlink")
	fileLink := filepath.Join(source, "filelink")
	rootLink := filepath.Join(source, "rootlink")
	for link, destination := range map[string]string{
		dirLink:  filepath.Join(target, "child"),
		fileLink: filepath.Join(target, "file"),
		rootLink: target,
	} {
		if err := os.Symlink(destination, link); err != nil {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
	}

	sb, _, err := New([]string{source, target})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(rawStatFSPath("dirlink", "..", "sibling"), source)
	assert.NoError(t, err)

	_, err = sb.StatFS(rawStatFSPath("filelink", "..", "sibling"), source)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS(rawStatFSPath("rootlink", ".."), source)
	assert.ErrorIs(t, err, fs.ErrPermission)
}

func TestSandboxStatFSPreservesSymlinkTargetSyntax(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file"), []byte("data"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), []byte("data"), 0o600))

	for link, target := range map[string]string{
		"file-dotdot":    rawStatFSPath("file", ".."),
		"missing-dotdot": rawStatFSPath("missing", ".."),
		"valid-dotdot":   rawStatFSPath("subdir", "..", "target"),
		"trailing-file":  "file" + string(filepath.Separator),
		"loop":           rawStatFSPath("loop", ".."),
	} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Skipf("symlink creation is unavailable: %v", err)
		}
	}

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS("file-dotdot", dir)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS("missing-dotdot", dir)
	assert.ErrorIs(t, err, fs.ErrNotExist)

	_, err = sb.StatFS("valid-dotdot", dir)
	assert.NoError(t, err)

	_, err = sb.StatFS("trailing-file", dir)
	assert.ErrorIs(t, err, fsstat.ErrNotDirectory)

	_, err = sb.StatFS("loop", dir)
	assert.Error(t, err)
}

func TestSandboxStatFSPreservesDotDotUnderSymlinkRoot(t *testing.T) {
	realParent := t.TempDir()
	target := filepath.Join(realParent, "root")
	sibling := filepath.Join(realParent, "sibling")
	require.NoError(t, os.Mkdir(target, 0o700))
	require.NoError(t, os.Mkdir(sibling, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(target, "child"), 0o700))
	if err := os.Symlink(
		rawStatFSPath("..", "sibling"),
		filepath.Join(target, "sibling-link"),
	); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	sb, _, err := New([]string{alias, sibling})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(rawStatFSPath("child", ".."), alias)
	assert.NoError(t, err)

	_, err = sb.StatFS(rawStatFSPath(alias, "child", ".."), alias)
	assert.NoError(t, err)

	_, err = sb.StatFS("sibling-link", alias)
	assert.NoError(t, err)
}

func TestSandboxStatFSRejectsSymlinkOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "file")
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	link := filepath.Join(allowed, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	sb, _, err := New([]string{allowed})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(link, allowed)
	assert.ErrorIs(t, err, fs.ErrPermission)
}
