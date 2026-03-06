// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

// GNU equivalence tests for the tail builtin.
//
// Reference outputs were captured from GNU coreutils 9.5 (Homebrew):
//   /opt/homebrew/opt/coreutils/libexec/gnubin/tail
//
// These tests assert byte-for-byte output equivalence between our builtin and
// GNU tail for the cases most sensitive to formatting (line counts, newlines,
// headers, quiet/verbose, byte mode, offset mode).

package tail_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// gnuCompatTailRun creates named files in a temp dir and runs a tail script.
func gnuCompatTailRun(t *testing.T, files map[string]string, script string) (string, string, int) {
	t.Helper()
	dir := setupTailDir(t, files)
	return tailRun(t, script, dir)
}

// twelveLines returns 12 lines "line1\n" … "line12\n".
func twelveLines() string {
	var b strings.Builder
	for i := 1; i <= 12; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	return b.String()
}

// TestGNUCompatTailDefaultLast10 — default tail with 12-line file returns last 10.
//
// GNU command: tail twelve.txt
// Expected: line3..line12 (one per line, trailing newline on last)
func TestGNUCompatTailDefaultLast10(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"twelve.txt": twelveLines()},
		"tail twelve.txt",
	)
	assert.Equal(t, 0, exitCode)
	// GNU tail: lines 3–12
	want := "line3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\nline11\nline12\n"
	assert.Equal(t, want, stdout)
}

// TestGNUCompatTailNLines — -n 3 on a 3-line file returns all 3 lines.
//
// GNU command: tail -n 3 three.txt   (three.txt = "alpha\nbeta\ngamma\n")
// Expected: alpha\nbeta\ngamma\n
func TestGNUCompatTailNLines(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -n 3 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// TestGNUCompatTailNZero — -n 0 produces no output.
//
// GNU command: tail -n 0 three.txt
// Expected: (empty)
func TestGNUCompatTailNZero(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -n 0 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatTailNMoreThanFile — -n larger than line count returns whole file.
//
// GNU command: tail -n 99 three.txt
// Expected: alpha\nbeta\ngamma\n
func TestGNUCompatTailNMoreThanFile(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -n 99 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// TestGNUCompatTailNPlusOffset — -n +2 outputs from line 2 to EOF.
//
// GNU command: tail -n +2 three.txt
// Expected: beta\ngamma\n
func TestGNUCompatTailNPlusOffset(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -n +2 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "beta\ngamma\n", stdout)
}

// TestGNUCompatTailLongFormLines — --lines=N is equivalent to -n N.
//
// GNU command: tail --lines=2 three.txt
// Expected: beta\ngamma\n
func TestGNUCompatTailLongFormLines(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail --lines=2 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "beta\ngamma\n", stdout)
}

// TestGNUCompatTailBytesLast — -c 5 outputs the last 5 bytes.
//
// bytes.txt = "abcdefghij\n" (11 bytes: a-j plus newline)
// GNU command: tail -c 5 bytes.txt
// Expected: "ghij\n"  (bytes 7–11)
func TestGNUCompatTailBytesLast(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"bytes.txt": "abcdefghij\n"},
		"tail -c 5 bytes.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "ghij\n", stdout)
}

// TestGNUCompatTailBytesPlusOffset — -c +3 outputs starting at byte 3.
//
// GNU command: tail -c +3 bytes.txt
// Expected: "cdefghij\n"
func TestGNUCompatTailBytesPlusOffset(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"bytes.txt": "abcdefghij\n"},
		"tail -c +3 bytes.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "cdefghij\n", stdout)
}

// TestGNUCompatTailNoTrailingNewline — tail preserves absence of trailing newline.
//
// GNU command: tail -n 1 nonewline.txt   (file content: "no newline at end", no \n)
// Expected: "no newline at end"  (no newline appended)
func TestGNUCompatTailNoTrailingNewline(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"nonewline.txt": "no newline at end"},
		"tail -n 1 nonewline.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "no newline at end", stdout)
}

// TestGNUCompatTailEmptyFile — tail on an empty file produces no output.
//
// GNU command: tail empty.txt
// Expected: (empty string)
func TestGNUCompatTailEmptyFile(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"empty.txt": ""},
		"tail empty.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatTailVerboseSingleFile — -v prints header even for a single file.
//
// GNU command: tail -v one.txt   (one.txt = "only one line\n")
// Expected: "==> one.txt <==\nonly one line\n"
// Note: GNU tail uses the exact path string passed on the command line (relative
// if a relative path was given), which matches our builtin's behaviour.
func TestGNUCompatTailVerboseSingleFile(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"one.txt": "only one line\n"},
		"tail -v one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "==> one.txt <==\nonly one line\n", stdout)
}

// TestGNUCompatTailTwoFilesDefaultHeaders — two files get ==> … <== headers
// separated by a blank line.
//
// GNU command: tail three.txt one.txt
// Expected:
//
//	==> three.txt <==
//	alpha
//	beta
//	gamma
//
//	==> one.txt <==
//	only one line
func TestGNUCompatTailTwoFilesDefaultHeaders(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{
			"three.txt": "alpha\nbeta\ngamma\n",
			"one.txt":   "only one line\n",
		},
		"tail three.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	want := "==> three.txt <==\nalpha\nbeta\ngamma\n\n==> one.txt <==\nonly one line\n"
	assert.Equal(t, want, stdout)
}

// TestGNUCompatTailVerboseTwoFiles — -v with two files is identical to the
// default multi-file format (headers always printed).
//
// GNU command: tail -v three.txt one.txt
// Expected: same as default two-file output
func TestGNUCompatTailVerboseTwoFiles(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{
			"three.txt": "alpha\nbeta\ngamma\n",
			"one.txt":   "only one line\n",
		},
		"tail -v three.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	want := "==> three.txt <==\nalpha\nbeta\ngamma\n\n==> one.txt <==\nonly one line\n"
	assert.Equal(t, want, stdout)
}

// TestGNUCompatTailQuietTwoFiles — -q suppresses headers even with two files.
//
// GNU command: tail -q three.txt one.txt
// Expected: alpha\nbeta\ngamma\nonly one line\n  (no headers, content concatenated)
func TestGNUCompatTailQuietTwoFiles(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{
			"three.txt": "alpha\nbeta\ngamma\n",
			"one.txt":   "only one line\n",
		},
		"tail -q three.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\nonly one line\n", stdout)
}

// TestGNUCompatTailQuietSingleFile — -q on a single file: no header, content only.
//
// GNU command: tail -q one.txt
// Expected: "only one line\n"  (no header printed)
func TestGNUCompatTailQuietSingleFile(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"one.txt": "only one line\n"},
		"tail -q one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "only one line\n", stdout)
}

// TestGNUCompatTailSilentAlias — --silent is an alias for --quiet.
//
// GNU command: tail --silent three.txt one.txt
// Expected: content concatenated, no headers
func TestGNUCompatTailSilentAlias(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{
			"three.txt": "alpha\nbeta\ngamma\n",
			"one.txt":   "only one line\n",
		},
		"tail --silent three.txt one.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\nonly one line\n", stdout)
}

// TestGNUCompatTailNPlusOne — -n +1 starts from the very first line (whole file).
//
// GNU command: tail -n +1 three.txt
// Expected: alpha\nbeta\ngamma\n
func TestGNUCompatTailNPlusOne(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -n +1 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// TestGNUCompatTailCZero — -c 0 produces no output (no bytes).
//
// GNU command: tail -c 0 bytes.txt
// Expected: (empty)
func TestGNUCompatTailCZero(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"bytes.txt": "abcdefghij\n"},
		"tail -c 0 bytes.txt",
	)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatTailNAndC — when both -n and -c are given, -n wins (last flag wins).
//
// GNU command: tail -c 3 -n 2 three.txt   → last 2 lines (not last 3 bytes)
// Expected: beta\ngamma\n
func TestGNUCompatTailNAndC(t *testing.T) {
	stdout, _, exitCode := gnuCompatTailRun(t,
		map[string]string{"three.txt": "alpha\nbeta\ngamma\n"},
		"tail -c 3 -n 2 three.txt",
	)
	assert.Equal(t, 0, exitCode)
	// -n 2 overrides -c 3
	assert.Equal(t, "beta\ngamma\n", stdout)
}

// TestGNUCompatTailFollowRejected — -f / --follow is not implemented and must exit 1.
//
// GNU tail: supports -f; our builtin rejects it to prevent infinite blocking.
// Expected: exit code 1, stderr contains rejection message.
func TestGNUCompatTailFollowRejected(t *testing.T) {
	_, stderr, exitCode := gnuCompatTailRun(t,
		map[string]string{"one.txt": "only one line\n"},
		"tail -f one.txt",
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "unknown")
}

// TestGNUCompatTailAllowedPaths is an interop test that confirms the sandbox
// rejects a path outside the allowed root while accepting one inside.
func TestGNUCompatTailAllowedPaths(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"ok.txt": "safe\n"})
	runner := func(script string, opts ...interp.RunnerOption) (string, string, int) {
		return runScript(t, script, dir, opts...)
	}

	// Accessing a file inside the allowed root: should succeed.
	stdout, _, exitCode := runner("tail ok.txt", interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "safe\n", stdout)

	// Accessing with no AllowedPaths set: should fail (no sandbox configured).
	_, stderr, exitCode2 := runner("tail ok.txt")
	assert.Equal(t, 1, exitCode2)
	assert.NotEmpty(t, stderr)
}
