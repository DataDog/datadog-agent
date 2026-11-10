// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package pwd_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// pwdRun runs a script in `dir` with AllowedPaths set to that dir, mirroring
// the runtime sandbox configuration.
func pwdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// canonicalTempDir returns a fresh per-test temp dir with all symlinks
// in the path already resolved. On macOS t.TempDir() returns a
// /var/folders/... path that is itself a symlink to /private/var/...;
// pwd -P (correctly) translates the AllowedPaths root prefix to its
// canonical form via the sandbox, so any test that compares against
// an absolute path produced by pwd -P must use the canonical form.
// On Linux/Windows where t.TempDir() is already canonical, this is a
// no-op.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return real
}

// --- Basic invocation ---

func TestPwdNoArgsPrintsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
	assert.True(t, strings.HasSuffix(stdout, "\n"))
}

func TestPwdNoArgsTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	stdout, _, _ := pwdRun(t, "pwd", dir)
	assert.True(t, strings.HasSuffix(stdout, "\n"))
	// There must be exactly one trailing newline.
	assert.False(t, strings.HasSuffix(stdout, "\n\n"))
}

func TestPwdExtraArgsIgnored(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd extra args here", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
}

func TestPwdNoArgsExitCodeZero(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, "pwd; echo $?", dir)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Equal(t, "0", lines[len(lines)-1])
}

// --- -L / --logical ---

func TestPwdLogicalShort(t *testing.T) {
	dir := t.TempDir()
	stdoutPlain, _, _ := pwdRun(t, "pwd", dir)
	stdoutL, _, code := pwdRun(t, "pwd -L", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdoutPlain, stdoutL, "pwd and pwd -L must agree")
}

func TestPwdLogicalLong(t *testing.T) {
	dir := t.TempDir()
	stdoutPlain, _, _ := pwdRun(t, "pwd", dir)
	stdoutL, _, code := pwdRun(t, "pwd --logical", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdoutPlain, stdoutL)
}

// --- -P / --physical ---

func TestPwdPhysicalShort(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd -P", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
}

func TestPwdPhysicalLong(t *testing.T) {
	dir := t.TempDir()
	stdoutShort, _, _ := pwdRun(t, "pwd -P", dir)
	stdoutLong, _, code := pwdRun(t, "pwd --physical", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdoutShort, stdoutLong)
}

// --- Last-wins semantics ---

func TestPwdLastWinsLthenP(t *testing.T) {
	dir := t.TempDir()
	stdoutBoth, _, code := pwdRun(t, "pwd -L -P", dir)
	stdoutP, _, _ := pwdRun(t, "pwd -P", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdoutP, stdoutBoth)
}

func TestPwdLastWinsPthenL(t *testing.T) {
	dir := t.TempDir()
	stdoutBoth, _, code := pwdRun(t, "pwd -P -L", dir)
	stdoutL, _, _ := pwdRun(t, "pwd -L", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, stdoutL, stdoutBoth)
}

func TestPwdLastWinsMixedLongShort(t *testing.T) {
	dir := t.TempDir()
	mixed, _, code := pwdRun(t, "pwd --physical -L", dir)
	logical, _, _ := pwdRun(t, "pwd -L", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, logical, mixed)
}

// --- --help ---

func TestPwdHelpLongPrintsToStdout(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd --help", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.Contains(t, stdout, "Usage: pwd")
	assert.Contains(t, stdout, "logical")
	assert.Contains(t, stdout, "physical")
}

func TestPwdHelpShortPrintsToStdout(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, "pwd -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: pwd")
}

// --- Rejected flags / arguments ---

func TestPwdUnknownLongFlagRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd --no-such-flag", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "pwd:")
	assert.Contains(t, stderr, "unrecognized option")
}

func TestPwdUnknownShortFlagRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd -x", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "pwd:")
}

func TestPwdVersionRejected(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd --version", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "pwd:")
}

// --- Stdin ---

func TestPwdStdinIgnored(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "echo unread | pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
	// pwd's output should be only one line — it should not have echoed
	// stdin back.
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 1)
}

// --- End-of-flags --- ---

func TestPwdDoubleDashEndsFlags(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := pwdRun(t, "pwd -- --not-a-flag", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
}

// --- Idempotence / determinism ---

func TestPwdMultipleInvocationsConsistent(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, "pwd; pwd; pwd", dir)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 3)
	assert.Equal(t, lines[0], lines[1])
	assert.Equal(t, lines[1], lines[2])
}

// --- Sandbox path-not-allowed for -P ---

// TestPwdPhysicalFallsBackOutsideSandbox verifies that requesting -P when
// the working directory cannot be walked through the sandbox does not
// produce a hard error. Resolution is best-effort and falls back to the
// logical path silently.
func TestPwdPhysicalFallsBackOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	otherDir := t.TempDir()
	stdout, stderr, code := testutil.RunScript(t, "pwd -P", dir, interp.AllowedPaths([]string{otherDir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.True(t, filepath.IsAbs(strings.TrimRight(stdout, "\n")))
}

// --- pwd inside command substitution / shell features ---

func TestPwdInCommandSubstitution(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, `here=$(pwd); echo "[$here]"`, dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "[")
	assert.Contains(t, stdout, "]")
}

func TestPwdInIfCondition(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, `if pwd > /dev/null; then echo ok; else echo fail; fi`, dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "ok")
}

func TestPwdInForLoop(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := pwdRun(t, `for i in 1 2 3; do pwd; done`, dir)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 3)
	for i := 1; i < len(lines); i++ {
		assert.Equal(t, lines[0], lines[i], "all iterations must agree")
	}
}
