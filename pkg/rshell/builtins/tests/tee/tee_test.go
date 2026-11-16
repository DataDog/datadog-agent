// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package tee_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/tee"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// --- Happy path ---

func TestTeeWritesFileAndStdout(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee out.txt", dir, "hello\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
	assert.Equal(t, "hello\n", readFile(t, filepath.Join(dir, "out.txt")))
}

func TestTeeMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee a.txt b.txt", dir, "hi\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi\n", stdout)
	assert.Equal(t, "hi\n", readFile(t, filepath.Join(dir, "a.txt")))
	assert.Equal(t, "hi\n", readFile(t, filepath.Join(dir, "b.txt")))
}

func TestTeeNoFileOperandsIsPassthrough(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := teeRunStdin(t, "tee", dir, "just stdout\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "just stdout\n", stdout)
	assert.Equal(t, "", stderr)
}

func TestTeeOverwritesByDefault(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "out.txt", "old content that is longer")
	_, _, code := teeRunStdin(t, "tee out.txt", dir, "new\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "new\n", readFile(t, path))
}

func TestTeeAppendFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "out.txt", "first\n")
	_, _, code := teeRunStdin(t, "tee -a out.txt", dir, "second\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nsecond\n", readFile(t, path))
}

func TestTeeAppendLongFlag(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "out.txt", "first\n")
	_, _, code := teeRunStdin(t, "tee --append out.txt", dir, "second\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nsecond\n", readFile(t, path))
}

func TestTeeCreatesMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, _, code := teeRunStdin(t, "tee newfile.txt", dir, "content\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "content\n", readFile(t, filepath.Join(dir, "newfile.txt")))
}

func TestTeeEmptyStdin(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee out.txt", dir, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", readFile(t, filepath.Join(dir, "out.txt")))
}

func TestTeeNoStdinAtAll(t *testing.T) {
	// When stdin is not wired at all (callCtx.Stdin == nil, matching
	// interp.StdIO(nil, ...)), tee must not error or hang; it behaves as
	// if reading an empty source.
	dir := t.TempDir()
	stdout, stderr, code := teeRun(t, "tee out.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
	assert.Equal(t, "", readFile(t, filepath.Join(dir, "out.txt")))
}

// TestTeeDashOperandIsLiteralFile pins current GNU tee behavior: "-" names
// an actual file called "-", opened through the sandbox like any other
// operand. GNU tee treated "-" as a stdout alias in coreutils 5.3.0 through
// 8.23, but that special case was removed in 8.24 (POSIX-mandated), and
// real bash/GNU tee on the CI runner confirms the current behavior.
func TestTeeDashOperandIsLiteralFile(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee -", dir, "viadash\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "viadash\n", stdout)
	assert.Equal(t, "viadash\n", readFile(t, filepath.Join(dir, "-")))
}

func TestTeeDashOperandMixedWithFile(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee - out.txt", dir, "mixed\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "mixed\n", stdout)
	assert.Equal(t, "mixed\n", readFile(t, filepath.Join(dir, "-")))
	assert.Equal(t, "mixed\n", readFile(t, filepath.Join(dir, "out.txt")))
}

// --- Help ---

func TestTeeHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := teeRun(t, "tee --help", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stderr)
	assert.Contains(t, stdout, "Usage: tee")
	assert.Contains(t, stdout, "--append")
}

func TestTeeHelpDeniedOutsideRemediation(t *testing.T) {
	// --help must be refused exactly like any other invocation in
	// read-only mode, matching the RULES.md dispatch-gate requirement for
	// RemediationOnly builtins.
	dir := t.TempDir()
	stdout, stderr, code := runScript(t, "tee --help", dir, interp.AllowedPaths([]string{dir + ":rw"}))
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "tee:")
}

// --- Remediation mode gating ---

func TestTeeDeniedOutsideRemediationMode(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runScript(t, "echo hi | tee out.txt", dir, interp.AllowedPaths([]string{dir + ":rw"}))
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "tee:")
	assert.NoFileExists(t, filepath.Join(dir, "out.txt"))
}

func TestTeeDeniedWithNoFileOperandsOutsideRemediation(t *testing.T) {
	// Even with no FILE operands (pure passthrough), tee must still be
	// refused outside remediation mode rather than silently succeeding.
	dir := t.TempDir()
	_, stderr, code := runScript(t, "echo hi | tee", dir, interp.AllowedPaths([]string{dir + ":rw"}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tee:")
}

// TestTeeDeniedWithNoAllowedPathsConfiguredAtAll pins the distinct
// no-writable-root diagnostic: remediation mode is on, but AllowedPaths
// was never configured at all, so callCtx.OpenFile/StatFile are never
// wired at all (nil) by the interpreter, and hasWritableRoot's own
// AllowedPathsList nil-check also short-circuits. This is a different
// code path (and message) than TestTeeDeniedOnReadOnlyAllowedPath below,
// which grants a read-only AllowedPaths root (OpenFile/StatFile are wired
// there, but the sandbox itself rejects the write).
func TestTeeDeniedWithNoAllowedPathsConfiguredAtAll(t *testing.T) {
	_, stderr, code := runScript(t, "printf hi | tee out.txt", "", interp.WithMode(interp.ModeRemediation))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no writable path is configured")
}

func TestTeeDeniedOnReadOnlyAllowedPath(t *testing.T) {
	// Remediation mode is on, but the AllowedPaths root has no :rw access.
	// tee's hasWritableRoot check (matching truncate's) catches this before
	// ever reaching the sandbox, reporting the more specific
	// no-writable-root diagnostic rather than falling through to the
	// sandbox's own "permission denied".
	dir := t.TempDir()
	_, stderr, code := runScript(t, "echo hi | tee out.txt", dir,
		interp.AllowedPaths([]string{dir}),
		interp.WithMode(interp.ModeRemediation),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no writable path is configured")
	assert.NoFileExists(t, filepath.Join(dir, "out.txt"))
}

// --- Resource limits ---

func TestTeeRejectsTooManyFileOperands(t *testing.T) {
	dir := t.TempDir()
	count := tee.MaxFileOperands + 1
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		names = append(names, fmt.Sprintf("f%d.txt", i))
	}
	script := "tee " + strings.Join(names, " ")
	_, stderr, code := teeRunStdin(t, script, dir, "hi\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "too many operands")
	// No destination must have been opened.
	assert.NoFileExists(t, filepath.Join(dir, "f0.txt"))
}

func TestTeeAllowsExactlyMaxFileOperands(t *testing.T) {
	dir := t.TempDir()
	count := tee.MaxFileOperands
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		names = append(names, fmt.Sprintf("f%d.txt", i))
	}
	script := "tee " + strings.Join(names, " ")
	stdout, stderr, code := teeRunStdin(t, script, dir, "hi\n")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Equal(t, "hi\n", stdout)
	assert.Equal(t, "hi\n", readFile(t, filepath.Join(dir, "f0.txt")))
	assert.Equal(t, "hi\n", readFile(t, filepath.Join(dir, fmt.Sprintf("f%d.txt", count-1))))
}

// --- Sandbox containment ---

func TestTeeRejectsPathOutsideAllowedRoots(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "escape.txt")
	_, stderr, code := teeRunStdin(t, "tee "+target, dir, "data\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tee:")
	assert.NoFileExists(t, target)
}

// --- Diagnostic escaping ---

// TestTeeEscapesNewlineInOperandDiagnostic verifies that a FILE operand
// containing a newline cannot forge an additional diagnostic line: the
// operand name printed in the error must have the newline escaped to the
// literal two-character sequence \n rather than a real line break.
func TestTeeEscapesNewlineInOperandDiagnostic(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := teeRunStdin(t, "tee 'evil\nFORGED LINE/missing/out.txt'", dir, "hi\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, `evil\nFORGED LINE`)
	// No raw newline from the operand itself should appear as a bare line
	// break in the operand portion of the message; every line of stderr
	// output must start with "tee:" rather than the forged continuation
	// text landing on its own unprefixed line.
	for _, line := range strings.Split(strings.TrimRight(stderr, "\n"), "\n") {
		if line == "" {
			continue
		}
		assert.True(t, strings.HasPrefix(line, "tee:"), "stderr line missing tee: prefix (possible forged line): %q", line)
	}
}

// TestTeeFileNamedStandardOutputDoesNotCollideWithStdoutSentinel verifies
// that a FILE operand literally named "standard output" is written to as an
// ordinary file destination like any other operand, not confused with the
// diagnostic label used for the real standard-output destination.
func TestTeeFileNamedStandardOutputDoesNotCollideWithStdoutSentinel(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRunStdin(t, "tee 'standard output'", dir, "hi\n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi\n", stdout)
	assert.Equal(t, "hi\n", readFile(t, filepath.Join(dir, "standard output")))
}

// --- Partial failure semantics ---

func TestTeeContinuesToOtherDestinationsAfterOneFails(t *testing.T) {
	// A file outside the sandbox fails to open, but the sandboxed
	// destination and stdout must still receive the data (GNU tee
	// semantics: one bad destination does not stop the others).
	dir := t.TempDir()
	outside := t.TempDir()
	badTarget := filepath.Join(outside, "denied.txt")
	stdout, stderr, code := teeRunStdin(t, "tee "+badTarget+" good.txt", dir, "payload\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tee:")
	assert.Equal(t, "payload\n", stdout)
	assert.Equal(t, "payload\n", readFile(t, filepath.Join(dir, "good.txt")))
}

func TestTeeMissingParentDirectoryFails(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := teeRunStdin(t, "tee missing_dir/out.txt", dir, "data\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tee:")
	// stdout still receives the data even though the file destination failed.
	assert.Equal(t, "data\n", stdout)
}

func TestTeeDirectoryAsTargetFails(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "adir"), 0755))
	_, stderr, code := teeRunStdin(t, "tee adir", dir, "data\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tee:")
}

// --- Explicit values on no-argument flags ---

// TestTeeRejectsExplicitValueOnAppend pins the RegisterNoArgBool behavior:
// --append is a no-argument flag in GNU tee, so --append=false must be
// rejected rather than silently selecting truncation (a bare pflag.BoolP
// would accept it, which could silently overwrite a file the caller
// intended to append to).
func TestTeeRejectsExplicitValueOnAppend(t *testing.T) {
	dir := t.TempDir()
	path := writeFile(t, dir, "out.txt", "existing\n")
	_, stderr, code := teeRunStdin(t, "tee --append=false out.txt", dir, "new\n")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "doesn't allow an argument")
	// The file must be untouched — a malformed flag must not fall through
	// to truncating it.
	assert.Equal(t, "existing\n", readFile(t, path))
}

func TestTeeRejectsExplicitValueOnHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := teeRun(t, "tee --help=false", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "doesn't allow an argument")
}

// TestTeeHelpOutputHasNoSentinelByte verifies that RegisterNoArgBool's
// unforgeable NUL sentinel (used internally to distinguish a bare flag from
// an explicit-value one) never leaks into --help output, which would be a
// visible NUL byte in a script's captured output.
func TestTeeHelpOutputHasNoSentinelByte(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := teeRun(t, "tee --help", dir)
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "\x00")
}

// --- Unknown flags ---

func TestTeeRejectsUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := teeRun(t, "tee --ignore-interrupts out.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unrecognized option")
}

func TestTeeRejectsOutputErrorFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := teeRun(t, "tee -p out.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid option -- 'p'")
}

// Hard-link write-target rejection is exercised in
// hardlink_notwindows_test.go: the guard is unix-only (Windows cannot report
// a link count from an open handle, see AGENTS.md's hard-link entry), so it
// is not a platform-agnostic assertion. FIFO write-target rejection is
// exercised in tee_unix_test.go (Mkfifo is unix-specific and does not
// compile on Windows) for the same reason.
