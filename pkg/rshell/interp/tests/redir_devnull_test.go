// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tests_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// redirRun runs a script with the given dir as working directory and allowed path.
func redirRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return redirRunWithOpts(t, script, dir, interp.AllowedPaths([]string{dir}), interpoption.AllowAllCommands().(interp.RunnerOption))
}

// redirRunNoAllowed runs a script with no allowed paths.
func redirRunNoAllowed(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return redirRunWithOpts(t, script, dir, interpoption.AllowAllCommands().(interp.RunnerOption))
}

func redirRunWithOpts(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf)}, opts...)

	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(context.Background(), prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// --- Stdout redirect to /dev/null ---

func TestRedirStdoutToDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello >/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirStdoutToDevNullWithSpace(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello > /dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirExplicitFd1ToDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello 1>/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Stderr redirect to /dev/null ---

func TestRedirStderrToDevNull(t *testing.T) {
	dir := t.TempDir()
	// cat on a nonexistent file produces stderr; 2>/dev/null suppresses it
	stdout, stderr, code := redirRun(t, "cat nonexistent 2>/dev/null", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirStderrToDevNullWithSpace(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "cat nonexistent 2> /dev/null", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Both stdout+stderr redirect (&>) ---

func TestRedirBothToDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello &>/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Append redirect (>>) ---

func TestRedirAppendToDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello >>/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirAppendBothToDevNull(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRun(t, "echo hello &>>/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Fd duplication ---

func TestRedirDupStderrToStdout(t *testing.T) {
	dir := t.TempDir()
	// >/dev/null 2>&1: first redirect stdout to /dev/null, then dup stderr to stdout
	// Both stdout and stderr go to /dev/null
	stdout, stderr, code := redirRun(t, "cat nonexistent >/dev/null 2>&1", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirDupStdoutToStderr(t *testing.T) {
	dir := t.TempDir()
	// >&2 redirects stdout to stderr
	stdout, stderr, code := redirRun(t, "echo hello >&2", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "hello\n", stderr)
}

// --- Exit code preservation ---

func TestRedirDevNullPreservesExitCode(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := redirRun(t, "true >/dev/null; echo $?", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\n", stdout)
}

func TestRedirDevNullPreservesFailureExitCode(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := redirRun(t, "false >/dev/null; echo $?", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n", stdout)
}

// --- Blocked redirects (still rejected) ---

func TestRedirToFileStillBlocked(t *testing.T) {
	dir := t.TempDir()
	// The validation should reject this
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello > /tmp/output.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

func TestRedirStderrToFileStillBlocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello 2> /tmp/errors.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

func TestRedirAppendToFileStillBlocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello >> /tmp/output.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

func TestRedirAllToFileStillBlocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello &> /tmp/output.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

// --- Path traversal via /dev/null ---

func TestRedirDevNullPathTraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello > /dev/null/../../../tmp/evil", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

func TestRedirDevNullExtraSlashBlocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello > /dev//null", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "file redirection is not supported")
}

// --- Unsupported fd numbers ---

func TestRedirFd3Blocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello 3>/dev/null", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "fd redirection is not supported")
}

func TestRedirDupFd3Blocked(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := redirRunNoAllowed(t, "echo hello 3>&1", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "fd duplication is not supported")
}

// --- Combination with pipes ---

func TestRedirDevNullWithPipe(t *testing.T) {
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("hello world\nfoo bar\n"), 0644)
	require.NoError(t, err)

	stdout, stderr, code := redirRun(t, "cat data.txt 2>/dev/null | grep hello", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
	assert.Equal(t, "", stderr)
}

// --- Multiple redirects on same command ---

func TestRedirMultipleDevNull(t *testing.T) {
	dir := t.TempDir()
	// Redirect both stdout and stderr separately to /dev/null
	stdout, stderr, code := redirRun(t, "echo hello >/dev/null 2>/dev/null", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Variable in redirect target is resolved at run time ---

func TestRedirVariableTargetDevNullAllowed(t *testing.T) {
	dir := t.TempDir()
	// $TARGET makes the word non-literal, so the validator defers to the
	// runtime check, which sees the expanded /dev/null and accepts it.
	stdout, stderr, code := redirRunNoAllowed(t, "TARGET=/dev/null; echo hello > $TARGET", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestRedirVariableTargetFileBlockedAtRuntime(t *testing.T) {
	dir := t.TempDir()
	// A non-/dev/null expanded target is still refused, but only that command
	// fails — the script keeps running.
	target := filepath.Join(dir, "evil")
	stdout, stderr, code := redirRunNoAllowed(t, "TARGET="+target+"; echo hello > $TARGET; echo after", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "after\n", stdout)
	assert.Contains(t, stderr, "file redirection is only supported for /dev/null")
	assert.NoFileExists(t, target)
}
