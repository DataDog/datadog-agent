// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package rm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// --- Happy path ---

func TestRmRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hello world")
	_, stderr, code := rmRun(t, "rm file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assertGone(t, path)
}

func TestRmMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	a := writeFile(t, dir, "a.txt", "a")
	b := writeFile(t, dir, "b.txt", "b")
	_, _, code := rmRun(t, "rm a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assertGone(t, a)
	assertGone(t, b)
}

func TestRmVerbose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	stdout, _, code := rmRun(t, "rm -v file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "removed 'file.txt'\n", stdout)
}

func TestRmVerboseLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	stdout, _, code := rmRun(t, "rm --verbose file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "removed 'file.txt'\n", stdout)
}

func TestRmWithoutVerboseIsSilent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	stdout, stderr, code := rmRun(t, "rm file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestRmSymlinkRemovesLinkNotTarget(t *testing.T) {
	dir := t.TempDir()
	target := writeFile(t, dir, "target.txt", "keep me")
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink("target.txt", link))
	_, _, code := rmRun(t, "rm link.txt", dir)
	assert.Equal(t, 0, code)
	assertGone(t, link)
	assertExists(t, target)
}

func TestRmDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "dangling")
	require.NoError(t, os.Symlink("does-not-exist", link))
	_, _, code := rmRun(t, "rm dangling", dir)
	assert.Equal(t, 0, code)
	assertGone(t, link)
}

// --- --help ---

func TestRmHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := rmRun(t, "rm --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: rm")
	assert.Contains(t, stdout, "--verbose")
}

// --help must not leak the RegisterNoArgBool NUL sentinel into pflag's
// PrintDefaults output. Without clearing NoOptDefVal first, pflag renders
// bare no-arg bools as "--verbose[=<NUL>]" since their NoOptDefVal is
// non-empty and not the literal string "true".
func TestRmHelpContainsNoNulByte(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := rmRun(t, "rm --help", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.False(t, strings.ContainsRune(stdout, '\x00'), "help output must not contain a NUL byte: %q", stdout)
}

func TestRmHelpReadOnlyModeFails(t *testing.T) {
	dir := t.TempDir()
	// No WithMode(ModeRemediation) — Remove capability is nil, so --help
	// fails the same way an ordinary rm invocation would.
	_, stderr, code := runScript(t, "rm --help", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "remediation mode required")
}

// --- Error cases ---

func TestRmMissingOperand(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := rmRun(t, "rm", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing operand")
}

func TestRmNoSuchFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := rmRun(t, "rm missing.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing.txt")
}

func TestRmDirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0755))
	_, stderr, code := rmRun(t, "rm subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "Is a directory")
	assertExists(t, sub)
}

func TestRmNonEmptyDirectoryRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0755))
	writeFile(t, sub, "inner.txt", "data")
	_, stderr, code := rmRun(t, "rm subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "Is a directory")
}

func TestRmSymlinkToDirectoryTreatedAsSymlink(t *testing.T) {
	// A symlink whose target is a directory is still a removable symlink —
	// LstatFile (no-follow) sees the link, not the directory it points to.
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0755))
	link := filepath.Join(dir, "linktodir")
	require.NoError(t, os.Symlink("subdir", link))
	_, _, code := rmRun(t, "rm linktodir", dir)
	assert.Equal(t, 0, code)
	assertGone(t, link)
	assertExists(t, sub)
}

func TestRmSandboxBlocked(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	secret := writeFile(t, other, "secret.txt", "secret data")
	_, stderr, code := runScript(t, "rm "+secret, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "secret.txt")
	assertExists(t, secret)
}

func TestRmReadOnlyMode(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "file.txt", "hello")
	// No WithMode(ModeRemediation) — Remove will be nil.
	_, stderr, code := runScript(t, "rm file.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "remediation mode required")
	assertExists(t, path)
}

// TestRmPartialFailure verifies that rm processes all operands even after one
// fails, returning exit 1 at the end, and that the successful removal is not
// rolled back.
func TestRmPartialFailure(t *testing.T) {
	dir := t.TempDir()
	good := writeFile(t, dir, "good.txt", "hello world")
	stdout, stderr, code := rmRun(t, "rm good.txt missing.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing.txt")
	assert.Empty(t, stdout)
	assertGone(t, good)
}

func TestRmUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := rmRun(t, "rm --force file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unrecognized option")
}

func TestRmRecursiveFlagRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0755))
	_, stderr, code := rmRun(t, "rm -r subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option")
	assertExists(t, sub)
}

func TestRmDirFlagRejected(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(sub, 0755))
	_, stderr, code := rmRun(t, "rm -d subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option")
}

func TestRmForceFlagRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := rmRun(t, "rm -f missing.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option")
}

func TestRmInteractiveFlagRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	_, stderr, code := rmRun(t, "rm -i file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option")
}

// --- File-count cap ---

func TestRmCapBoundaryExactlyAllowed(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 10)
	paths := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		name := "f" + string(rune('0'+i)) + ".txt"
		names = append(names, name)
		paths = append(paths, writeFile(t, dir, name, "x"))
	}
	script := "rm " + join(names)
	_, stderr, code := rmRun(t, script, dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	for _, p := range paths {
		assertGone(t, p)
	}
}

func TestRmCapExceededRejectsWholeInvocation(t *testing.T) {
	dir := t.TempDir()
	names := make([]string, 0, 11)
	paths := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		name := "g" + string(rune('a'+i)) + ".txt"
		names = append(names, name)
		paths = append(paths, writeFile(t, dir, name, "x"))
	}
	script := "rm " + join(names)
	_, stderr, code := rmRun(t, script, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "exceeds the 10-file limit")
	// Nothing should have been deleted — the cap check runs before any
	// removal is attempted.
	for _, p := range paths {
		assertExists(t, p)
	}
}

func join(names []string) string {
	return strings.Join(names, " ")
}
