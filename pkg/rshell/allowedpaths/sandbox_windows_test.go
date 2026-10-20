// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package allowedpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAccessReadAllowedWindows verifies that Access succeeds for a
// readable file on Windows, using the OpenFile-based check.
func TestAccessReadAllowedWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readable.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("readable.txt", dir, 0x04))
}

// TestAccessNonexistentWindows verifies that Access fails for a
// missing file on Windows.
func TestAccessNonexistentWindows(t *testing.T) {
	dir := t.TempDir()

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("missing.txt", dir, 0x04)
	assert.Error(t, err)
}

// TestAccessOutsideSandboxWindows verifies that Access rejects paths
// outside the sandbox.
func TestAccessOutsideSandboxWindows(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access(filepath.Join(outside, "secret.txt"), dir, 0x04)
	assert.ErrorIs(t, err, os.ErrPermission)
}

// TestAccessDirectoryReadWindows verifies that Access works on
// directories (directory read check skips OpenFile).
func TestAccessDirectoryReadWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("subdir", dir, 0x04))
}

// TestAccessSymlinkEscapeBlockedWindows verifies that Access blocks
// symlinks that resolve outside the sandbox on Windows.
func TestAccessSymlinkEscapeBlockedWindows(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("secret"), 0644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	err = sb.Access("escape.txt", dir, 0x04)
	assert.Error(t, err)
}

// TestAccessSymlinkWithinSandboxWindows verifies that Access succeeds
// for a symlink that resolves within the sandbox on Windows.
func TestAccessSymlinkWithinSandboxWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("data"), 0644))
	require.NoError(t, os.Symlink("target.txt", filepath.Join(dir, "link.txt")))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.NoError(t, sb.Access("link.txt", dir, 0x04))
}

// TestAccessWriteDeniedWindows verifies that write checks correctly
// deny access to read-only files on Windows via mode-bit inspection.
func TestAccessWriteDeniedWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readonly.txt"), []byte("data"), 0444))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	assert.ErrorIs(t, sb.Access("readonly.txt", dir, 0x02), os.ErrPermission)
}

// TestAccessExecAlwaysDeniedWindows verifies that execute checks
// always return ErrPermission on Windows (no POSIX execute bits).
func TestAccessExecAlwaysDeniedWindows(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("data"), 0644))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	// Windows has no POSIX execute bits — always denied.
	assert.ErrorIs(t, sb.Access("data.txt", dir, 0x01), os.ErrPermission)
}
