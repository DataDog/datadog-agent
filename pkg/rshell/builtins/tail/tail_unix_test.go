// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package tail_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func TestTailSymlinkToFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "real.txt", "hello from real\n")
	require.NoError(t, os.Symlink("real.txt", filepath.Join(dir, "link.txt")))
	stdout, _, code := cmdRun(t, "tail -n 1 link.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello from real\n", stdout)
}

func TestTailDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(dir, "does_not_exist.txt"), filepath.Join(dir, "dangling.txt")))
	_, stderr, code := cmdRun(t, "tail dangling.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tail:")
}

func TestTailDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := runScript(t, "tail /dev/null", dir,
		interp.AllowedPaths([]string{dir, "/dev"}),
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestTailPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read files with mode 0000; permission check not meaningful")
	}
	dir := t.TempDir()
	writeFile(t, dir, "noperms.txt", "secret\n")
	require.NoError(t, os.Chmod(filepath.Join(dir, "noperms.txt"), 0000))
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Join(dir, "noperms.txt"), 0644)
	})
	_, stderr, code := cmdRun(t, "tail noperms.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tail:")
}
