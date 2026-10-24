// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logrotate_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --help must not leak the RegisterNoArgBool NUL sentinel into pflag's
// PrintDefaults output. Without clearing NoOptDefVal first, pflag renders
// bare no-arg bools as "--dry-run[=<NUL>]" since their NoOptDefVal is
// non-empty and not the literal string "true".
func TestLogrotateHelpContainsNoNulByte(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, code := logrotateRun(t, "logrotate --help", dir)

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.False(t, strings.ContainsRune(stdout, '\x00'), "help output must not contain a NUL byte: %q", stdout)
}

// --dry-run must leave the target byte-for-byte identical, not merely the same
// length. The sandbox opens the file O_WRONLY even in dry-run mode, so this
// pins that the open is validation-only.
func TestLogrotateDryRunDoesNotMutateContent(t *testing.T) {
	dir := t.TempDir()
	const content = "line one\nline two\n"
	path := writeFile(t, dir, "app.log", content)
	before, err := os.Stat(path)
	require.NoError(t, err)

	stdout, stderr, code := logrotateRun(t, "logrotate --dry-run --force app.log", dir)

	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "logrotate: app.log: would truncate 18 bytes\n", stdout)
	assertContent(t, path, content)

	after, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime(), "dry run must not touch mtime")
	assert.Equal(t, before.Mode(), after.Mode(), "dry run must not touch mode")
}

// The help text promises "Symlinked write targets are rejected; pass the real
// log path instead". interp/builtin_logrotate_unix_test.go covers the dry-run
// spelling; this covers the real mutating one, and asserts the symlink itself
// survives (it is not replaced by a regular empty file).
func TestLogrotateSymlinkWriteTargetRejected(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "target.log", "payload")
	if err := os.Symlink("target.log", filepath.Join(dir, "app.log")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	stdout, stderr, code := logrotateRun(t, "logrotate --force app.log", dir)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `logrotate: "app.log": symlinks are not supported as write targets`)
	assertContent(t, target, "payload")

	info, err := os.Lstat(filepath.Join(dir, "app.log"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the symlink itself must survive")
}

// A directory operand must fail with EISDIR rather than being silently walked
// or rotated. Mirrors hardening/directory_target.yaml at the Go level so the
// error type, not just the rendered string, is pinned.
func TestLogrotateDirectoryTargetRejected(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "logs"), 0o755))
	inner := writeFile(t, filepath.Join(dir, "logs"), "app.log", "payload")

	stdout, stderr, code := logrotateRun(t, "logrotate --force logs", dir)

	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `logrotate: "logs": is a directory`)
	assertContent(t, inner, "payload")
}

// A missing target is never created. logrotate is a truncation helper, not an
// O_CREAT writer, so an absent log must stay absent.
func TestLogrotateMissingTargetIsNotCreated(t *testing.T) {
	dir := t.TempDir()

	_, stderr, code := logrotateRun(t, "logrotate --force missing.log", dir)

	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `logrotate: "missing.log": no such file or directory`)
	_, err := os.Lstat(filepath.Join(dir, "missing.log"))
	assert.True(t, os.IsNotExist(err), "logrotate must not create the missing target")
}
