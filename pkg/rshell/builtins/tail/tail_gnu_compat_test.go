// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// GNU compatibility tests for the tail builtin.
//
// Expected outputs were captured from GNU coreutils tail 9.x (macOS Homebrew
// gtail) and are embedded as string literals so the tests run without any GNU
// tooling present on CI.  To reproduce a reference output, run:
//
//	gtail [flags] [file]  # then inspect with cat -A to see exact bytes

package tail_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatDefaultOutput — default output on a 12-line file (last 10).
//
// GNU command: gtail twelve.txt
// Expected:    last 10 lines (line03..line12)
func TestGNUCompatDefaultOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "twelve.txt", twelveLines)
	stdout, _, code := cmdRun(t, "tail twelve.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line03\nline04\nline05\nline06\nline07\nline08\nline09\nline10\nline11\nline12\n", stdout)
}

// TestGNUCompatLinesN — -n 3: last 3 lines of a 5-line file.
//
// GNU command: gtail -n 3 five.txt
// Expected:    "gamma\ndelta\nepsilon\n"
func TestGNUCompatLinesN(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n 3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "gamma\ndelta\nepsilon\n", stdout)
}

// TestGNUCompatLinesZero — -n 0: no output.
//
// GNU command: gtail -n 0 five.txt
// Expected:    ""
func TestGNUCompatLinesZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n 0 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatLinesLargerThanFile — -n N larger than file: print all lines.
//
// GNU command: gtail -n 100 five.txt
// Expected:    fiveLines
func TestGNUCompatLinesLargerThanFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n 100 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

// TestGNUCompatOffsetPlus1 — +1 is from line 1 = all lines.
//
// GNU command: gtail -n +1 five.txt
// Expected:    fiveLines
func TestGNUCompatOffsetPlus1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n +1 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

// TestGNUCompatOffsetPlus2 — +2 skips 1 line.
//
// GNU command: gtail -n +2 five.txt
// Expected:    "beta\ngamma\ndelta\nepsilon\n"
func TestGNUCompatOffsetPlus2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n +2 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "beta\ngamma\ndelta\nepsilon\n", stdout)
}

// TestGNUCompatLongFormLines — --lines=N long form.
//
// GNU command: gtail --lines=3 five.txt
// Expected:    "gamma\ndelta\nepsilon\n"
func TestGNUCompatLongFormLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail --lines=3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "gamma\ndelta\nepsilon\n", stdout)
}

// TestGNUCompatNoTrailingNewline — last line without newline reproduced exactly.
//
// GNU command: gtail -n 1 nonewline.txt   (nonewline.txt = "no newline at end")
// Expected:    "no newline at end"   (no trailing \n added)
func TestGNUCompatNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "tail -n 1 nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "no newline at end", stdout)
}

// TestGNUCompatEmptyFile — empty file produces no output.
//
// GNU command: gtail empty.txt
// Expected:    ""
func TestGNUCompatEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "tail empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatVerboseSingleFile — -v prints header even for a single file.
//
// GNU command: gtail -v one.txt   (one.txt = "only one line\n")
// Expected:    "==> one.txt <==\nonly one line\n"
func TestGNUCompatVerboseSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.txt", "only one line\n")
	stdout, _, code := cmdRun(t, "tail -v one.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> one.txt <==\nonly one line\n", stdout)
}

// TestGNUCompatTwoFilesDefault — two files: headers and blank-line separator.
//
// GNU command: gtail -n 2 five.txt nonewline.txt
// Expected:    "==> five.txt <==\ndelta\nepsilon\n\n==> nonewline.txt <==\nno newline at end"
func TestGNUCompatTwoFilesDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "tail -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> five.txt <==\ndelta\nepsilon\n\n==> nonewline.txt <==\nno newline at end", stdout)
}

// TestGNUCompatQuietTwoFiles — -q suppresses headers for multiple files.
//
// GNU command: gtail -q -n 2 five.txt nonewline.txt
// Expected:    "delta\nepsilon\nno newline at end"
func TestGNUCompatQuietTwoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "tail -q -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "delta\nepsilon\nno newline at end", stdout)
}

// TestGNUCompatSilentTwoFiles — --silent is an alias for --quiet.
//
// GNU command: gtail --silent -n 2 five.txt nonewline.txt
// Expected:    "delta\nepsilon\nno newline at end"
func TestGNUCompatSilentTwoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "tail --silent -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "delta\nepsilon\nno newline at end", stdout)
}

// TestGNUCompatBytesMode — -c N outputs the last N bytes.
//
// GNU command: gtail -c 8 five.txt
// Expected:    "psilon\n"  (last 8 bytes of "...epsilon\n")
// "epsilon\n" is 8 bytes: e-p-s-i-l-o-n-\n
func TestGNUCompatBytesMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -c 8 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "epsilon\n", stdout)
}

// TestGNUCompatBytesModeOffsetPlus3 — -c +3 outputs from byte 3.
//
// GNU command: gtail -c +3 hello.txt   (hello.txt = "hello")
// Expected:    "llo"  (bytes 3-5, skip first 2)
func TestGNUCompatBytesModeOffsetPlus3(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "hello.txt", "hello")
	stdout, _, code := cmdRun(t, "tail -c +3 hello.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "llo", stdout)
}

// TestGNUCompatLastFlagWinsBytes — -n then -c: last flag (-c) wins.
//
// GNU command: gtail -n 2 -c 3 five.txt
// Expected:    last 3 bytes of fiveLines = "on\n"
func TestGNUCompatLastFlagWinsBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -n 2 -c 3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "on\n", stdout)
}

// TestGNUCompatLastFlagWinsLines — -c then -n: last flag (-n) wins.
//
// GNU command: gtail -c 5 -n 2 five.txt
// Expected:    "delta\nepsilon\n"   (line mode, 2 lines)
func TestGNUCompatLastFlagWinsLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "tail -c 5 -n 2 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "delta\nepsilon\n", stdout)
}

// TestGNUCompatRejectedFlag — unknown flag produces exit 1 and non-empty stderr.
//
// GNU command: gtail --no-such-flag five.txt   → exit 1, stderr non-empty
func TestGNUCompatRejectedFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	_, stderr, code := cmdRun(t, "tail --no-such-flag five.txt", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}

// TestGNUCompatFollowRejected — -f (--follow) must be rejected.
// We intentionally do not implement file following (tail -f) as it
// would require background goroutines and indefinite blocking,
// which is unsafe in a sandboxed shell.
//
// GNU command: gtail -f file.txt   → follows file; we reject with exit 1
func TestGNUCompatFollowRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	_, stderr, code := cmdRun(t, "tail -f five.txt", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}
