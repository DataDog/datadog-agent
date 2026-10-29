// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testcmd_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGNUCompatTestEmptyIsFalse — no arguments = false.
//
// GNU: test; echo $?  → 1
func TestGNUCompatTestEmptyIsFalse(t *testing.T) {
	stdout, stderr, code := runScript(t, "test; echo $?", "")
	assert.Equal(t, "1\n", stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestZeroLengthEmpty — -z "" = true.
//
// GNU: test -z ""; echo $?  → 0
func TestGNUCompatTestZeroLengthEmpty(t *testing.T) {
	stdout, stderr, code := runScript(t, `test -z ""; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestNonZeroLength — -n "hello" = true.
//
// GNU: test -n "hello"; echo $?  → 0
func TestGNUCompatTestNonZeroLength(t *testing.T) {
	stdout, stderr, code := runScript(t, `test -n "hello"; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Empty(t, stderr)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestStringEquality — "t" = "t".
//
// GNU: test "t" = "t"; echo $?  → 0
func TestGNUCompatTestStringEquality(t *testing.T) {
	stdout, _, code := runScript(t, `test "t" = "t"; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestStringInequality — "t" = "f" → false.
//
// GNU: test "t" = "f"; echo $?  → 1
func TestGNUCompatTestStringInequality(t *testing.T) {
	stdout, _, code := runScript(t, `test "t" = "f"; echo $?`, "")
	assert.Equal(t, "1\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestIntegerEquality — 9 -eq 9.
//
// GNU: test 9 -eq 9; echo $?  → 0
func TestGNUCompatTestIntegerEquality(t *testing.T) {
	stdout, _, code := runScript(t, `test 9 -eq 9; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestIntegerWithLeadingZeros — 0 -eq 00.
//
// GNU: test 0 -eq 00; echo $?  → 0
func TestGNUCompatTestIntegerWithLeadingZeros(t *testing.T) {
	stdout, _, code := runScript(t, `test 0 -eq 00; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestInvalidIntegerHex — 0x0 -eq 00 → error.
//
// GNU: test 0x0 -eq 00; echo $?  → 2 (stderr: "test: 0x0: integer expression expected")
func TestGNUCompatTestInvalidIntegerHex(t *testing.T) {
	_, stderr, code := runScript(t, `test 0x0 -eq 00`, "")
	assert.Equal(t, 2, code)
	assert.Equal(t, "test: 0x0: integer expression expected\n", stderr)
}

// TestGNUCompatTestNegation — ! "" = true.
//
// GNU: test ! ""; echo $?  → 0
func TestGNUCompatTestNegation(t *testing.T) {
	stdout, _, code := runScript(t, `test ! ""; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestAndOperator — "t" -a "t" = true.
//
// GNU: test "t" -a "t"; echo $?  → 0
func TestGNUCompatTestAndOperator(t *testing.T) {
	stdout, _, code := runScript(t, `test "t" -a "t"; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestOrOperator — "" -o "t" = true.
//
// GNU: test "" -o "t"; echo $?  → 0
func TestGNUCompatTestOrOperator(t *testing.T) {
	stdout, _, code := runScript(t, `test "" -o "t"; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestParentheses — ( "hello" ).
//
// GNU: test '(' "hello" ')'; echo $?  → 0
func TestGNUCompatTestParentheses(t *testing.T) {
	stdout, _, code := runScript(t, `test '(' "hello" ')'; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatBracketMissingClose — [ 1 -eq → exit 2 + stderr.
//
// GNU: [ 1 -eq; echo $?  → 2 (stderr: "[: missing `]'")
func TestGNUCompatBracketMissingClose(t *testing.T) {
	_, stderr, code := runScript(t, `[ 1 -eq`, "")
	assert.Equal(t, 2, code)
	assert.Equal(t, "[: missing `]'\n", stderr)
}

// TestGNUCompatTestFileExists — -f on regular file.
//
// GNU: test -f file.txt; echo $?  → 0
func TestGNUCompatTestFileExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "content\n")
	stdout, _, code := cmdRun(t, `test -f file.txt; echo $?`, dir)
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}

// TestGNUCompatTestNtOt — file modification time comparison.
//
// GNU: test newer -nt older; echo $?  → 0
// GNU: test older -ot newer; echo $?  → 0
func TestGNUCompatTestNtOt(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "older.txt", "old")
	os.Chtimes(filepath.Join(dir, "older.txt"), time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour))
	writeFile(t, dir, "newer.txt", "new")

	stdout, _, code := cmdRun(t, `test newer.txt -nt older.txt; echo $?`, dir)
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)

	stdout, _, code = cmdRun(t, `test older.txt -ot newer.txt; echo $?`, dir)
	assert.Equal(t, "0\n", stdout)
	assert.Equal(t, 0, code)
}
