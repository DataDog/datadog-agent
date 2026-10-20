// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// GNU compatibility tests for the head builtin.
//
// Expected outputs were captured from GNU coreutils head 9.10 (macOS Homebrew
// ghead) and are embedded as string literals so the tests run without any GNU
// tooling present on CI.  To reproduce a reference output, run:
//
//	ghead [flags] [file]  # then inspect with cat -A to see exact bytes

package head_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatDefaultOutput — default output on a 12-line file.
//
// GNU command: ghead twelve.txt
// Expected:    first 10 lines
func TestGNUCompatDefaultOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "twelve.txt", twelveLines)
	stdout, _, code := cmdRun(t, "head twelve.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line01\nline02\nline03\nline04\nline05\nline06\nline07\nline08\nline09\nline10\n", stdout)
}

// TestGNUCompatLinesN — -n N smaller than file length.
//
// GNU command: ghead -n 3 five.txt   (five.txt = fiveLines)
// Expected:    "alpha\nbeta\ngamma\n"
func TestGNUCompatLinesN(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// TestGNUCompatLinesZero — -n 0: no output.
//
// GNU command: ghead -n 0 five.txt
// Expected:    ""
func TestGNUCompatLinesZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 0 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatLinesLargerThanFile — -n N larger than file: print all lines.
//
// GNU command: ghead -n 100 five.txt
// Expected:    fiveLines
func TestGNUCompatLinesLargerThanFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 100 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

// TestGNUCompatPositivePrefix — +N prefix is treated as positive N (not an offset).
//
// GNU command: ghead -n +2 five.txt
// Expected:    "alpha\nbeta\n"   (same as -n 2)
func TestGNUCompatPositivePrefix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n +2 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

// TestGNUCompatLongFormLines — --lines=N long form.
//
// GNU command: ghead --lines=3 five.txt
// Expected:    "alpha\nbeta\ngamma\n"
func TestGNUCompatLongFormLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head --lines=3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// TestGNUCompatNoTrailingNewline — last line without newline is reproduced exactly.
//
// GNU command: ghead -n 2 nonewline.txt   (nonewline.txt = "no newline at end")
// Expected:    "no newline at end"   (no trailing \n added)
func TestGNUCompatNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "head -n 2 nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "no newline at end", stdout)
}

// TestGNUCompatEmptyFile — empty file produces no output.
//
// GNU command: ghead empty.txt   (empty.txt = "")
// Expected:    ""
func TestGNUCompatEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "head empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatVerboseSingleFile — -v prints header even for a single file.
//
// GNU command: ghead -v one.txt   (one.txt = "only one line\n")
// Expected:    "==> one.txt <==\nonly one line\n"
func TestGNUCompatVerboseSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.txt", "only one line\n")
	stdout, _, code := cmdRun(t, "head -v one.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> one.txt <==\nonly one line\n", stdout)
}

// TestGNUCompatTwoFilesDefault — two files: headers and blank-line separator.
//
// GNU command: ghead -n 2 five.txt nonewline.txt
// Expected:    "==> five.txt <==\nalpha\nbeta\n\n==> nonewline.txt <==\nno newline at end"
func TestGNUCompatTwoFilesDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "head -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> five.txt <==\nalpha\nbeta\n\n==> nonewline.txt <==\nno newline at end", stdout)
}

// TestGNUCompatQuietTwoFiles — -q suppresses headers for multiple files.
//
// GNU command: ghead -q -n 2 five.txt nonewline.txt
// Expected:    "alpha\nbeta\nno newline at end"
func TestGNUCompatQuietTwoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "head -q -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\nno newline at end", stdout)
}

// TestGNUCompatSilentTwoFiles — --silent is an alias for --quiet.
//
// GNU command: ghead --silent -n 2 five.txt nonewline.txt
// Expected:    "alpha\nbeta\nno newline at end"
func TestGNUCompatSilentTwoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	writeFile(t, dir, "nonewline.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "head --silent -n 2 five.txt nonewline.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\nno newline at end", stdout)
}

// TestGNUCompatBytesMode — -c N outputs exactly N bytes.
//
// GNU command: ghead -c 5 five.txt
// Expected:    "alpha"   (first 5 bytes of "alpha\nbeta\n...")
func TestGNUCompatBytesMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 5 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout)
}

// TestGNUCompatBytesModePositivePrefix — -c +N is treated as -c N.
//
// GNU command: ghead -c +3 five.txt
// Expected:    "alp"
func TestGNUCompatBytesModePositivePrefix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c +3 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alp", stdout)
}

// TestGNUCompatLastFlagWinsBytes — -n then -c: last flag (-c) wins.
//
// GNU command: ghead -n 2 -c 5 five.txt
// Expected:    "alpha"   (byte mode, 5 bytes)
func TestGNUCompatLastFlagWinsBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 2 -c 5 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout)
}

// TestGNUCompatLastFlagWinsLines — -c then -n: last flag (-n) wins.
//
// GNU command: ghead -c 5 -n 2 five.txt
// Expected:    "alpha\nbeta\n"   (line mode, 2 lines)
func TestGNUCompatLastFlagWinsLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 5 -n 2 five.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

// TestGNUCompatRejectedFlag — unknown flag produces exit 1 and non-empty stderr.
//
// GNU command: ghead --no-such-flag five.txt   → exit 1, stderr non-empty
func TestGNUCompatRejectedFlag(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "five.txt", fiveLines)
	_, stderr, code := cmdRun(t, "head --no-such-flag five.txt", dir)
	assert.Equal(t, 1, code)
	assert.NotEmpty(t, stderr)
}
