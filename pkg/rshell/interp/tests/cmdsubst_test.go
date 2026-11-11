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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cmdSubstRun runs a script with the given dir as working directory and allowed path.
func cmdSubstRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return cmdSubstRunWithOpts(t, script, dir, interp.AllowedPaths([]string{dir}), interpoption.AllowAllCommands().(interp.RunnerOption))
}

func cmdSubstRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return cmdSubstRunCtxWithOpts(ctx, t, script, dir, interp.AllowedPaths([]string{dir}), interpoption.AllowAllCommands().(interp.RunnerOption))
}

func cmdSubstRunWithOpts(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return cmdSubstRunCtxWithOpts(context.Background(), t, script, dir, opts...)
}

func cmdSubstRunCtxWithOpts(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
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

	err = runner.Run(ctx, prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// --- Basic command substitution ---

func TestCmdSubstBasicEcho(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdSubstRun(t, `echo $(echo hello)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Equal(t, "", stderr)
}

func TestCmdSubstBacktick(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, "echo `echo hello`", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestCmdSubstAssignment(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(echo world); echo "hello $x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

// --- Trailing newline stripping ---

func TestCmdSubstTrailingNewlinesStripped(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(printf "hello\n\n\n"); echo "[$x]"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[hello]\n", stdout)
}

// --- Empty output ---

func TestCmdSubstEmptyOutput(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(true); echo "[$x]"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n", stdout)
}

// --- Exit status propagation ---

func TestCmdSubstExitStatus(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(exit 3); echo "$?"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3\n", stdout)
}

func TestCmdSubstExitStatusFalse(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(false); echo "$?"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n", stdout)
}

// --- Nested substitution ---

func TestCmdSubstNested(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `echo $(echo $(echo nested))`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "nested\n", stdout)
}

// --- Pipes inside command substitution ---

func TestCmdSubstWithPipe(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(echo "hello world" | grep hello); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

// --- Double quotes preserve spaces ---

func TestCmdSubstInDoubleQuotes(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `echo "$(echo "hello   world")"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello   world\n", stdout)
}

// --- Word splitting without quotes ---

func TestCmdSubstWordSplitting(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `for w in $(echo "a  b  c"); do echo "[$w]"; done`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a]\n[b]\n[c]\n", stdout)
}

// --- $(<file) shortcut ---

func TestCmdSubstCatShortcut(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("file content"), 0644))
	stdout, _, code := cmdSubstRun(t, `x=$(<data.txt); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "file content\n", stdout)
}

func TestCmdSubstCatShortcutMissingFile(t *testing.T) {
	dir := t.TempDir()
	// Missing file in $(<file) sets $?=1 but does not abort the script.
	stdout, stderr, code := cmdSubstRun(t, `x=$(<nonexistent.txt); echo "$?"`, dir)
	assert.Equal(t, 0, code, "overall script should succeed")
	assert.Contains(t, stderr, "no such file")
	assert.Equal(t, "1\n", stdout, "$? should be 1 from the failed substitution")
}

// TestCmdSubstCatShortcutCommandAllowlistBypass is the regression test
// for the original vulnerability: $(<file) must be gated on `cat` being
// in AllowedCommands. With the file in AllowedPaths but cat absent from
// the allowlist, the shortcut must refuse to read.
func TestCmdSubstCatShortcutCommandAllowlistBypass(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("top secret"), 0644))
	stdout, stderr, code := cmdSubstRunWithOpts(t,
		`x=$(<secret.txt); echo "[$x]"`,
		dir,
		interp.AllowedPaths([]string{dir}),
		interp.AllowedCommands([]string{"rshell:echo"}),
	)
	assert.Equal(t, 0, code, "script completes; only the substitution fails")
	assert.Equal(t, "[]\n", stdout, "must not leak the file contents")
	assert.Contains(t, stderr, "file read not permitted")
	assert.Contains(t, stderr, "cat not in allowed commands")
}

// TestCmdSubstCatShortcutAllowedWhenCatAllowed verifies the shortcut
// works when cat is explicitly in AllowedCommands (not just
// AllowAllCommands).
func TestCmdSubstCatShortcutAllowedWhenCatAllowed(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("ok"), 0644))
	stdout, _, code := cmdSubstRunWithOpts(t,
		`x=$(<data.txt); echo "$x"`,
		dir,
		interp.AllowedPaths([]string{dir}),
		interp.AllowedCommands([]string{"rshell:cat", "rshell:echo"}),
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ok\n", stdout)
}

// --- For loop integration ---

func TestCmdSubstInForLoop(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `for x in $(echo "a b c"); do echo "$x"; done`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// --- While loop integration ---

// While loop body output is captured by command substitution. Exercises the
// $(...) -> subshell -> while loop -> limitWriter path now that while/until
// are no longer rejected at parse time.
func TestCmdSubstWhileInSubst(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(i=; while [ "$i" != aaa ]; do i="${i}a"; echo "$i"; done); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\naa\naaa\n", stdout)
}

// Until loop inside $(...) — symmetric coverage with TestCmdSubstWhileInSubst.
func TestCmdSubstUntilInSubst(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(i=; until [ "$i" = aaa ]; do i="${i}a"; echo "$i"; done); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\naa\naaa\n", stdout)
}

// --- If condition ---

func TestCmdSubstInIfCondition(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `if [ "$(echo yes)" = "yes" ]; then echo matched; fi`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "matched\n", stdout)
}

// --- Context cancellation ---

func TestCmdSubstContextCancellation(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// This should complete quickly, not hang
	stdout, _, code := cmdSubstRunCtx(ctx, t, `echo $(echo fast)`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "fast\n", stdout)
}

// --- Multiline output ---

func TestCmdSubstMultilineOutput(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, `x=$(printf "line1\nline2\nline3"); echo "$x"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline2\nline3\n", stdout)
}

// --- Heredoc with command substitution ---

func TestCmdSubstInHeredoc(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdSubstRun(t, "cat <<EOF\nhello $(echo world)\nEOF", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

// --- Process substitution remains blocked ---

func TestProcessSubstitutionBlocked(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdSubstRun(t, `cat <(echo hello)`, dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "process substitution is not supported")
}
