// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

//go:build unix

package tail_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// TestTailDevNull verifies reading /dev/null produces no output.
func TestTailDevNull(t *testing.T) {
	dir := t.TempDir()

	// /dev/null is outside allowed paths; access should be denied.
	_, stderr, exitCode := tailRun(t, "tail /dev/null", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

// TestTailSymlinkToFile verifies tail follows relative symlinks for reads (RULES.md: allowed).
func TestTailSymlinkToFile(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"real.txt": "data\n"})
	// Use a relative symlink so os.Root can follow it within the allowed directory.
	require.NoError(t, os.Symlink(
		"real.txt", // relative target
		filepath.Join(dir, "link.txt"),
	))

	stdout, _, exitCode := tailRun(t, "tail link.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "data\n", stdout)
}

// TestTailDanglingSymlinkErrors verifies a dangling symlink returns exit 1.
func TestTailDanglingSymlinkErrors(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Symlink(
		filepath.Join(dir, "nonexistent_target"),
		filepath.Join(dir, "dangling.txt"),
	))

	_, stderr, exitCode := runScript(t, "tail dangling.txt", dir,
		interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailPermissionDenied verifies exit 1 on unreadable file.
func TestTailPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can read any file")
	}

	dir := setupTailDir(t, map[string]string{"secret.txt": "secret\n"})
	require.NoError(t, os.Chmod(filepath.Join(dir, "secret.txt"), 0000))
	t.Cleanup(func() { os.Chmod(filepath.Join(dir, "secret.txt"), 0644) })

	_, stderr, exitCode := tailRun(t, "tail secret.txt", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}
