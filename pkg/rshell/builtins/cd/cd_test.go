// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cd_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"

	"os"
)

// cdRun runs script in dir with AllowedPaths set to that dir, mirroring
// the runtime sandbox configuration.
func cdRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// canonicalTempDir returns a fresh per-test temp dir with all symlinks
// in the path already resolved. On macOS t.TempDir() returns a
// /var/folders/... path that is itself a symlink to /private/var/...;
// using the resolved form keeps stdout comparisons stable across
// platforms.
func canonicalTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return dir
	}
	return real
}

// mkSubdir creates name inside dir and returns its absolute path.
func mkSubdir(t *testing.T, dir, name string) string {
	t.Helper()
	full := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(full, 0o755))
	return full
}

// trim is shorthand for the trailing-newline strip used in pwd-style asserts.
func trim(s string) string { return strings.TrimRight(s, "\n") }

// shPath returns a path that is safe to embed inside a shell-script
// string. On Windows, native paths use backslashes which the shell
// parser treats as escape characters (eating the path separators);
// converting to forward slashes avoids that since Windows file APIs
// accept both. On Unix this is a no-op.
func shPath(p string) string { return filepath.ToSlash(p) }

// --- Basic forms ---

func TestCdAbsolutePathChangesDir(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, stderr, code := cdRun(t, "cd "+shPath(filepath.Join(dir, "child"))+"; pwd", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

func TestCdRelativePathChangesDir(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

func TestCdDotIsNoOp(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, _, code := cdRun(t, "p1=$(pwd); cd .; p2=$(pwd); [ \"$p1\" = \"$p2\" ] && echo same", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "same", trim(stdout))
}

func TestCdDotDotClimbsOneLevel(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, filepath.Join("a", "b"))
	stdout, _, code := cdRun(t, "cd a/b; cd ..; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "a"), trim(stdout))
}

// --- $HOME / $OLDPWD / cd - ---

func TestCdNoArgsUsesHome(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "homedir")
	homeAbs := filepath.Join(dir, "homedir")
	stdout, stderr, code := cdRun(t, "HOME="+shPath(homeAbs)+"; cd; pwd", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, homeAbs, trim(stdout))
}

func TestCdNoArgsHomeUnsetErrors(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd: HOME not set")
}

func TestCdDashUsesOldpwdAndPrints(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "a")
	mkSubdir(t, dir, "b")
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	stdout, stderr, code := cdRun(t, "cd "+shPath(a)+"; cd "+shPath(b)+"; cd -", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	// `cd -` prints the new directory.
	assert.Equal(t, a, trim(stdout))
}

func TestCdDashOldpwdUnsetErrors(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd -", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd: OLDPWD not set")
}

// --- $PWD / $OLDPWD updates ---

func TestCdUpdatesPwdEnvVar(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	child := filepath.Join(dir, "child")
	stdout, _, code := cdRun(t, "cd "+shPath(child)+"; echo $PWD", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, child, trim(stdout))
}

func TestCdSetsOldpwd(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	child := filepath.Join(dir, "child")
	stdout, _, code := cdRun(t, "start=$(pwd); cd "+shPath(child)+"; echo $OLDPWD", dir)
	assert.Equal(t, 0, code)
	// $OLDPWD should equal the original pwd (which was dir).
	assert.Equal(t, dir, trim(stdout))
}

// --- -L / --logical ---

func TestCdLogicalShort(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd -L child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

func TestCdLogicalLong(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd --logical child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

// --- -P / --physical ---

func TestCdPhysicalShort(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, stderr, code := cdRun(t, "cd -P child; pwd", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

func TestCdPhysicalLong(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd --physical child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

// --- Last-wins semantics ---

func TestCdLastWinsLthenP(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd -L -P child; pwd", dir)
	assert.Equal(t, 0, code)
	// -P last; result should be the dir.
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

func TestCdLastWinsPthenL(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd -P -L child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}

// --- Help ---

func TestCdHelpShort(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, stderr, code := cdRun(t, "cd -h", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.Contains(t, stdout, "Usage: cd")
}

func TestCdHelpLong(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, _, code := cdRun(t, "cd --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: cd")
}

// --- Errors ---

func TestCdMissingDirectoryFails(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, stderr, code := cdRun(t, "cd no-such-dir", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "cd:")
	assert.Contains(t, stderr, "no-such-dir")
}

func TestCdToFileFailsNotADirectory(t *testing.T) {
	dir := canonicalTempDir(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte("hi"), 0o644))
	_, stderr, code := cdRun(t, "cd f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
	assert.Contains(t, stderr, "not a directory")
}

func TestCdOutsideSandboxFails(t *testing.T) {
	if filepath.Separator == '\\' {
		// On Windows the bare separator ('\\') is an escape character
		// in the shell parser, not an absolute path. The Unix test
		// uses '/' which is the absolute root; the equivalent on
		// Windows requires a drive letter and is exercised in the
		// platform-specific test file.
		t.Skip("Unix-only: '/' as absolute root does not exist on Windows")
	}
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd "+string(filepath.Separator), dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
	assert.Contains(t, stderr, "permission denied")
}

// TestCdEmptyOperandIsNoop documents the bash 5.2 contract: `cd ""`
// is a no-op success — pwd is unchanged, no error. Matches the
// home_empty_is_noop / oldpwd_empty_dash_prints_blank scenarios that
// exercise the env-var paths through the same code.
func TestCdEmptyOperandIsNoop(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, stderr, code := cdRun(t, `start=$(pwd); cd ""; [ "$(pwd)" = "$start" ] && echo same`, dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, "", stderr)
	assert.Equal(t, "same", trim(stdout))
}

func TestCdTooManyArgs(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd a b", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd: too many arguments")
}

func TestCdUnknownShortFlag(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd -x", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
	assert.Contains(t, stderr, "x")
}

func TestCdUnknownLongFlag(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd --no-such-flag", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd: unrecognized option")
}

func TestCdRejectsExplicitValue(t *testing.T) {
	dir := canonicalTempDir(t)
	_, stderr, code := cdRun(t, "cd --physical=true", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cd:")
}

// --- Failed cd does not mutate state ---

func TestCdFailedDoesNotChangeDir(t *testing.T) {
	dir := canonicalTempDir(t)
	stdout, _, code := cdRun(t, "p1=$(pwd); cd no-such-dir 2>/dev/null; p2=$(pwd); [ \"$p1\" = \"$p2\" ] && echo unchanged", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "unchanged", trim(stdout))
}

func TestCdFailedDoesNotSetOldpwd(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "first")
	first := filepath.Join(dir, "first")
	// First successful cd sets OLDPWD = dir. Then a failed cd should
	// leave OLDPWD alone.
	stdout, _, code := cdRun(t, "cd "+shPath(first)+"; cd nowhere 2>/dev/null; echo $OLDPWD", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, dir, trim(stdout))
}

// --- Subshell isolation ---

func TestCdInSubshellDoesNotEscape(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	child := filepath.Join(dir, "child")
	stdout, _, code := cdRun(t, "p1=$(pwd); ( cd "+shPath(child)+" ); p2=$(pwd); [ \"$p1\" = \"$p2\" ] && echo unchanged", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "unchanged", trim(stdout))
}

// --- Hardening ---

func TestCdManyIterationsNoLeak(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "a")
	mkSubdir(t, dir, "b")
	stdout, stderr, code := cdRun(t, "for i in {1..200}; do cd a; cd ..; cd b; cd ..; done; pwd", dir)
	assert.Equal(t, 0, code, "stderr=%q", stderr)
	assert.Equal(t, dir, trim(stdout))
}

func TestCdDoubleDashEndsFlags(t *testing.T) {
	dir := canonicalTempDir(t)
	mkSubdir(t, dir, "child")
	stdout, _, code := cdRun(t, "cd -- child; pwd", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, filepath.Join(dir, "child"), trim(stdout))
}
