// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package allowedpaths

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkedFixture builds a temp tree with a read-write root at <base>/rsh, a
// file at <base>/outside/secret.txt outside every configured root, and a hard
// link to it at <base>/rsh/hard.txt. It returns the root dir and the
// out-of-sandbox path.
func linkedFixture(t *testing.T) (dir, outsideFile string) {
	t.Helper()
	base := t.TempDir()
	dir = filepath.Join(base, "rsh")
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.Mkdir(dir, 0o755))
	require.NoError(t, os.Mkdir(outside, 0o755))
	outsideFile = filepath.Join(outside, "secret.txt")
	require.NoError(t, os.WriteFile(outsideFile, []byte("SENSITIVE"), 0o644))
	if err := os.Link(outsideFile, filepath.Join(dir, "hard.txt")); err != nil {
		t.Skipf("hard links unsupported on this filesystem: %v", err)
	}
	return dir, outsideFile
}

func writableSandbox(t *testing.T, dir string) *Sandbox {
	t.Helper()
	sb, _, err := New([]string{dir + ":rw"})
	require.NoError(t, err)
	t.Cleanup(func() { sb.Close() })
	sb.SetWritable()
	return sb
}

func TestSandboxTruncateRejectsHardLink(t *testing.T) {
	dir, outsideFile := linkedFixture(t)
	sb := writableSandbox(t, dir)

	err := sb.Truncate("hard.txt", dir, 0, false)

	require.Error(t, err)
	assert.Equal(t, "hard links are not supported as write targets", PortableErrMsg(err))
	got, readErr := os.ReadFile(outsideFile)
	require.NoError(t, readErr)
	assert.Equal(t, "SENSITIVE", string(got))
}

// TestSandboxTruncateToZeroIfAtLeastRejectsHardLink covers the logrotate
// remediation capability, which reaches the guard through the same
// openWriteFile choke point.
func TestSandboxTruncateToZeroIfAtLeastRejectsHardLink(t *testing.T) {
	dir, outsideFile := linkedFixture(t)
	sb := writableSandbox(t, dir)

	sizeBefore, truncated, err := sb.TruncateToZeroIfAtLeast("hard.txt", dir, 0, false)

	assert.Zero(t, sizeBefore)
	assert.False(t, truncated)
	require.Error(t, err)
	assert.Equal(t, "hard links are not supported as write targets", PortableErrMsg(err))
	got, readErr := os.ReadFile(outsideFile)
	require.NoError(t, readErr)
	assert.Equal(t, "SENSITIVE", string(got))
}

// TestSandboxOpenTruncRejectsHardLinkWithoutTruncating pins the ordering that
// makes the guard effective at all: O_TRUNC must not reach the open syscall,
// or the shared content would already be gone before the link count could be
// inspected.
func TestSandboxOpenTruncRejectsHardLinkWithoutTruncating(t *testing.T) {
	dir, outsideFile := linkedFixture(t)
	sb := writableSandbox(t, dir)

	f, err := sb.Open("hard.txt", dir, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)

	require.Error(t, err)
	assert.Nil(t, f)
	// Sandbox.Open re-wraps through PortablePathError, so the message keeps
	// the "open <path>: " prefix rather than being the bare sentinel text.
	assert.Contains(t, PortableErrMsg(err), "hard links are not supported as write targets")
	got, readErr := os.ReadFile(outsideFile)
	require.NoError(t, readErr)
	assert.Equal(t, "SENSITIVE", string(got))
}

func TestSandboxOpenAppendRejectsHardLink(t *testing.T) {
	dir, outsideFile := linkedFixture(t)
	sb := writableSandbox(t, dir)

	f, err := sb.Open("hard.txt", dir, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)

	require.Error(t, err)
	assert.Nil(t, f)
	got, readErr := os.ReadFile(outsideFile)
	require.NoError(t, readErr)
	assert.Equal(t, "SENSITIVE", string(got))
}

// TestSandboxOpenTruncStillTruncatesSingleLinkedFile proves that withholding
// O_TRUNC from the open syscall and replaying it as an ftruncate preserves
// ordinary `>` semantics.
func TestSandboxOpenTruncStillTruncatesSingleLinkedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	require.NoError(t, os.WriteFile(path, []byte("previous contents"), 0o644))
	sb := writableSandbox(t, dir)

	f, err := sb.Open("app.log", dir, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	require.NoError(t, err)
	_, err = f.Write([]byte("new"))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new", string(got))
}

// TestSandboxRemoveAllowsHardLink documents the deliberate asymmetry: unlink
// removes one name for an inode and never touches the content the other names
// still see, so it is not a sandbox escape and is not gated. See the hard link
// entry in AGENTS.md.
func TestSandboxRemoveAllowsHardLink(t *testing.T) {
	dir, outsideFile := linkedFixture(t)
	sb := writableSandbox(t, dir)

	require.NoError(t, sb.Remove("hard.txt", dir))

	assert.NoFileExists(t, filepath.Join(dir, "hard.txt"))
	got, err := os.ReadFile(outsideFile)
	require.NoError(t, err)
	assert.Equal(t, "SENSITIVE", string(got))
}
