// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package cd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cdRunFromAllowedRoot runs script with cwd=dir and AllowedPaths set to
// allowedRoot — useful when the test needs the sandbox to span more than
// the cwd so the symlink walker can see across components.
func cdRunFromAllowedRoot(t *testing.T, script, dir, allowedRoot string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{allowedRoot}))
}

// --- -P resolves a symlink target ---

func TestCdPhysicalResolvesSymlinkTarget(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.Symlink(target, link))

	stdout, stderr, code := cdRunFromAllowedRoot(t, "cd -P "+link+"; pwd", root, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	// -P resolves the symlink before printing.
	assert.Equal(t, target, trim(stdout))
}

// --- -L preserves the symlink path ---

func TestCdLogicalKeepsSymlinkPath(t *testing.T) {
	root := canonicalTempDir(t)
	target := filepath.Join(root, "real")
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Mkdir(target, 0o755))
	require.NoError(t, os.Symlink(target, link))

	stdout, _, code := cdRunFromAllowedRoot(t, "cd -L "+link+"; pwd", root, root)
	require.Equal(t, 0, code)
	assert.Equal(t, link, trim(stdout))
}

// --- -L vs -P with .. semantics differ when the cwd was reached via a symlink ---

func TestCdLogicalDotDotIsLexical(t *testing.T) {
	root := canonicalTempDir(t)
	deep := filepath.Join(root, "real", "deep")
	require.NoError(t, os.MkdirAll(deep, 0o755))
	link := filepath.Join(root, "lnk")
	require.NoError(t, os.Symlink(deep, link))

	// In -L mode, after `cd $link`, the `..` of $link is just the parent
	// of $link in the user's input — i.e. the sandbox root.
	stdout, stderr, code := cdRunFromAllowedRoot(t,
		"cd "+link+"; cd -L ..; pwd", root, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, root, trim(stdout))
}

// --- Symlink chain A -> B -> C resolves to C ---

func TestCdPhysicalSymlinkChain(t *testing.T) {
	root := canonicalTempDir(t)
	c := filepath.Join(root, "c")
	require.NoError(t, os.Mkdir(c, 0o755))
	b := filepath.Join(root, "b")
	require.NoError(t, os.Symlink(c, b))
	a := filepath.Join(root, "a")
	require.NoError(t, os.Symlink(b, a))

	stdout, stderr, code := cdRunFromAllowedRoot(t, "cd -P "+a+"; pwd", root, root)
	require.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, c, trim(stdout))
}

// --- Symlink loop is rejected with a clear error ---

func TestCdPhysicalSymlinkLoop(t *testing.T) {
	root := canonicalTempDir(t)
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	require.NoError(t, os.Symlink(b, a))
	require.NoError(t, os.Symlink(a, b))

	_, stderr, code := cdRunFromAllowedRoot(t, "cd -P "+a, root, root)
	assert.Equal(t, 1, code)
	assert.True(t,
		strings.Contains(stderr, "too many levels of symbolic links") ||
			strings.Contains(stderr, "permission denied"),
		"expected loop or permission error, got %q", stderr)
}

// --- Self-referential symlink is also rejected ---

func TestCdPhysicalSelfReferentialSymlink(t *testing.T) {
	root := canonicalTempDir(t)
	self := filepath.Join(root, "self")
	require.NoError(t, os.Symlink("self", self))

	_, stderr, code := cdRunFromAllowedRoot(t, "cd -P "+self, root, root)
	assert.Equal(t, 1, code)
	assert.True(t,
		strings.Contains(stderr, "too many levels of symbolic links") ||
			strings.Contains(stderr, "permission denied") ||
			strings.Contains(stderr, "no such file"),
		"expected loop or stat error, got %q", stderr)
}

// --- A directory without search/execute permission is rejected ---

// TestCdNoSearchPermissionRejected verifies cd refuses a directory the
// user lacks the execute (search) bit on, matching bash. Without this
// check, sandbox.Stat alone would let cd commit even though the
// resulting working directory is unusable for any subsequent operation.
func TestCdNoSearchPermissionRejected(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses permission bits — test exercises the non-root path")
	}
	root := canonicalTempDir(t)
	nox := filepath.Join(root, "nox")
	require.NoError(t, os.Mkdir(nox, 0o644)) // rw-r--r-- — no search bit
	t.Cleanup(func() { _ = os.Chmod(nox, 0o755) })

	_, stderr, code := cdRunFromAllowedRoot(t, "cd "+nox, root, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
	assert.Contains(t, stderr, "permission denied")
}

// --- Dangling symlink (target does not exist) is rejected ---

func TestCdToDanglingSymlinkRejected(t *testing.T) {
	root := canonicalTempDir(t)
	link := filepath.Join(root, "dangling")
	require.NoError(t, os.Symlink(filepath.Join(root, "missing"), link))

	_, stderr, code := cdRunFromAllowedRoot(t, "cd "+link, root, root)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
}
