// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRemediationRunner creates a runner in remediation mode with AllowedPaths
// set to dir as read-write and all commands allowed.
func newRemediationRunner(t *testing.T, dir string) (*Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir + ":rw"}),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir
	return r, &stdout, &stderr
}

// TestRemediationRedirect_WriteCreatesFile verifies that > creates a new file
// inside AllowedPaths.
func TestRemediationRedirect_WriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	r, stdout, _ := newRemediationRunner(t, dir)

	err := r.Run(context.Background(), parseScript(t, "echo hello > out.txt"))
	require.NoError(t, err, "stdout=%q", stdout.String())

	data, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(data))
}

// TestRemediationRedirect_AppendPreservesContent verifies that >> appends to an
// existing file.
func TestRemediationRedirect_AppendPreservesContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "log.txt"), []byte("first\n"), 0o600))
	r, _, _ := newRemediationRunner(t, dir)

	err := r.Run(context.Background(), parseScript(t, "echo second >> log.txt"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "log.txt"))
	require.NoError(t, err)
	assert.Equal(t, "first\nsecond\n", string(data))
}

// TestRemediationRedirect_StderrRedirect verifies that 2> captures stderr to a file.
// Redirect ordering: `2> err.txt 1>&2` first points stderr at the file, then
// points stdout at the same file, so echo output lands in err.txt.
func TestRemediationRedirect_StderrRedirect(t *testing.T) {
	dir := t.TempDir()
	r, stdout, _ := newRemediationRunner(t, dir)

	err := r.Run(context.Background(), parseScript(t, "echo captured 2> err.txt 1>&2\necho ok"))
	require.NoError(t, err, "stdout=%q", stdout.String())

	assert.Equal(t, "ok\n", stdout.String())
	data, err := os.ReadFile(filepath.Join(dir, "err.txt"))
	require.NoError(t, err)
	assert.Equal(t, "captured\n", string(data))
}

// TestRemediationRedirect_SandboxBlocked verifies that a write to a path
// outside AllowedPaths fails with permission denied.
func TestRemediationRedirect_SandboxBlocked(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	r, _, stderr := newRemediationRunner(t, dir)

	target := filepath.Join(outside, "blocked.txt")
	err := r.Run(context.Background(), parseScript(t, "echo secret > "+quoteCheckPath(target)))
	require.Error(t, err)

	var es ExitStatus
	require.True(t, errors.As(err, &es))
	assert.Equal(t, ExitStatus(1), es)
	assert.Contains(t, stderr.String(), "permission denied")

	_, statErr := os.Stat(target)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "blocked write must not create the file")
}

// TestRemediationRedirect_ReadOnlyAllowedPathRejected verifies that remediation
// mode enables write redirections only for roots explicitly marked :rw.
func TestRemediationRedirect_ReadOnlyAllowedPathRejected(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir + ":ro"}),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	err = r.Run(context.Background(), parseScript(t, "echo secret > blocked.txt"))
	require.Error(t, err)
	var es ExitStatus
	require.True(t, errors.As(err, &es))
	assert.Equal(t, ExitStatus(1), es)
	assert.Empty(t, stdout.String())
	assert.Contains(t, stderr.String(), "permission denied")

	_, statErr := os.Stat(filepath.Join(dir, "blocked.txt"))
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "read-only allowed root must not create the file")
}

// TestRemediationRedirect_UnsupportedFdRejectedAtValidation verifies that 3>
// is caught at parse/validate time (exit 2) in remediation mode, before any
// word expansion runs.
func TestRemediationRedirect_UnsupportedFdRejectedAtValidation(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		StdIO(nil, &stdout, &stderr),
		AllowedPaths([]string{dir + ":rw"}),
		WithMode(ModeRemediation),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	err = r.Run(context.Background(), parseScript(t, "echo x 3> out.txt"))
	require.Error(t, err)
	var es ExitStatus
	require.True(t, errors.As(err, &es), "expected ExitStatus, got %T: %v", err, err)
	assert.Equal(t, ExitStatus(2), es, "unsupported fd redirect must be a validation error (exit 2)")
	assert.Empty(t, stdout.String())
}

// TestRemediationRedirect_NonRegularTargetRejected verifies that writing to a
// non-regular file target (directory) is rejected.
func TestRemediationRedirect_NonRegularTargetRejected(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "sub")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	r, _, stderr := newRemediationRunner(t, dir)

	err := r.Run(context.Background(), parseScript(t, "echo x > sub"))
	require.Error(t, err)
	var es ExitStatus
	require.True(t, errors.As(err, &es))
	assert.Equal(t, ExitStatus(1), es)
	assert.Contains(t, stderr.String(), "not a regular file")
}

// TestRemediationRedirect_WriteAllBothStreams verifies that &> writes both
// stdout and stderr to the same file.
func TestRemediationRedirect_WriteAllBothStreams(t *testing.T) {
	dir := t.TempDir()
	r, _, _ := newRemediationRunner(t, dir)

	err := r.Run(context.Background(), parseScript(t, "echo out; echo err >&2"))
	require.NoError(t, err)

	r2, _, _ := newRemediationRunner(t, dir)
	err = r2.Run(context.Background(), parseScript(t, "{ echo out; echo err >&2; } &> combined.txt"))
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(dir, "combined.txt"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "out\n")
	assert.Contains(t, string(data), "err\n")
}
