// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package common

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenPathWithoutSymlinksNoSymlink(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")
	require.NoError(t, os.WriteFile(logFile, []byte("hello"), 0644))

	f, err := OpenPathWithoutSymlinks(logFile)
	require.NoError(t, err)
	defer f.Close()

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))
}

func TestOpenPathWithoutSymlinksSymlinkInFile(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.log")
	require.NoError(t, os.WriteFile(realFile, []byte("secret"), 0644))

	link := filepath.Join(dir, "link.log")
	require.NoError(t, os.Symlink(realFile, link))

	f, err := OpenPathWithoutSymlinks(link)
	assert.Error(t, err)
	assert.Nil(t, f)
	// O_NOFOLLOW on the final component should give ELOOP
	assert.ErrorIs(t, err, syscall.ELOOP)
}

func TestOpenPathWithoutSymlinksSymlinkInParentDirectory(t *testing.T) {
	dir := t.TempDir()
	realDir := filepath.Join(dir, "real")
	require.NoError(t, os.Mkdir(realDir, 0755))

	logFile := filepath.Join(realDir, "test.log")
	require.NoError(t, os.WriteFile(logFile, []byte("hello"), 0644))

	linkDir := filepath.Join(dir, "link")
	require.NoError(t, os.Symlink(realDir, linkDir))

	f, err := OpenPathWithoutSymlinks(filepath.Join(linkDir, "test.log"))
	assert.Error(t, err)
	assert.Nil(t, f)
	// O_DIRECTORY|O_NOFOLLOW on a symlinked directory component is rejected.
	assert.ErrorIs(t, err, syscall.ENOTDIR)
}

func TestOpenPathWithoutSymlinksSymlinkSwap(t *testing.T) {
	// Simulate the attacker scenario: open a real file, swap it to a symlink,
	// confirm the second open is rejected.
	dir := t.TempDir()
	realFile := filepath.Join(dir, "real.log")
	require.NoError(t, os.WriteFile(realFile, []byte("benign"), 0644))

	// First open should succeed (path is a real file at this point)
	f, err := OpenPathWithoutSymlinks(realFile)
	require.NoError(t, err)
	f.Close()

	// Attacker swaps the file for a symlink to a sensitive target
	sensitiveFile := filepath.Join(dir, "sensitive.dat")
	require.NoError(t, os.WriteFile(sensitiveFile, []byte("SECRET"), 0644))
	require.NoError(t, os.Remove(realFile))
	require.NoError(t, os.Symlink(sensitiveFile, realFile))

	// Second open must fail with ELOOP, not return the sensitive file
	f2, err := OpenPathWithoutSymlinks(realFile)
	assert.Error(t, err)
	assert.Nil(t, f2)
	assert.ErrorIs(t, err, syscall.ELOOP)
}
