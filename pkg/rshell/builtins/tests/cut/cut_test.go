// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cut_test

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

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption)}, opts...)
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

func cmdRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// --- Basic field selection ---

func TestCutFieldBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -d: -f1,3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:c\n", stdout)
}

func TestCutFieldRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -d: -f2- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b:c\n", stdout)
}

func TestCutFieldBeyondEnd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -d: -f4 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

func TestCutFieldEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "")
	stdout, _, code := cmdRun(t, "cut -d: -f4 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- Byte selection ---

func TestCutByteSingle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcd\n")
	stdout, _, code := cmdRun(t, "cut -b2 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b\n", stdout)
}

func TestCutByteRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut -b1-3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\n", stdout)
}

func TestCutByteOpenEnd(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut -b3- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cdef\n", stdout)
}

func TestCutByteOpenStart(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut -b-3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\n", stdout)
}

func TestCutByteBeyondLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "123\n")
	stdout, _, code := cmdRun(t, "cut -c4 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

func TestCutByteEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "")
	stdout, _, code := cmdRun(t, "cut -b1 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- Character selection ---

func TestCutCharBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcd\n")
	stdout, _, code := cmdRun(t, "cut -c2 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b\n", stdout)
}

func TestCutCharMultibyte(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "\xce\xb1\xce\xb2\xce\xb3\n") // αβγ
	// GNU cut treats -c as byte-wise (same as -b), so -c1 selects only the first byte.
	stdout, _, code := cmdRun(t, "cut -c1 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\xce\n", stdout)
}

// --- Delimiter ---

func TestCutOutputDelimiter(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -d: --output-delimiter=_ -f2,3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b_c\n", stdout)
}

func TestCutMulticharOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -d: --output-delimiter=_._ -f2,3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b_._c\n", stdout)
}

func TestCutOutputDelimBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdefg\n")
	stdout, _, code := cmdRun(t, "cut -c1-3,5- --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc:efg\n", stdout)
}

// --- Suppress (-s) ---

func TestCutSuppressNoDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abc\n")
	stdout, _, code := cmdRun(t, "cut -s -d: -f2,3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestCutSuppressWithDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cut -s -d: -f3- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "c\n", stdout)
}

// --- Complement ---

func TestCutComplement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "9_1\n8_2\n")
	stdout, _, code := cmdRun(t, "cat input.txt | cut --complement -d_ -f2", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "9\n8\n", stdout)
}

// --- Newline handling ---

func TestCutNewlinePreserved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a\nb")
	stdout, _, code := cmdRun(t, "cut -f1- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestCutFieldNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:1\nb:2")
	stdout, _, code := cmdRun(t, "cut -d: -f1 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

// --- Errors ---

func TestCutNoMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a\n")
	_, stderr, code := cmdRun(t, "cut input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutZeroPosition(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -b0 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutDecreasingRange(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -f 2-0 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutSuppressWithoutFields(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -s -b4 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutDelimWithoutFields(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -d: -b1 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -b1 nonexistent", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

func TestCutMulticharDelim(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -d ab -f1 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

// --- Help ---

func TestCutHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "cut --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}

// --- Stdin ---

func TestCutStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\n")
	stdout, _, code := cmdRun(t, "cat input.txt | cut -d: -f2", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b\n", stdout)
}

// --- Edge cases from GNU tests ---

func TestCutEmptyFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", ":::\n")
	stdout, _, code := cmdRun(t, "cut -d: -f1-3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "::\n", stdout)
}

func TestCutOverlappingUnbounded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "1234\n")
	stdout, _, code := cmdRun(t, "cut -b3-,2- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "234\n", stdout)
}

func TestCutBigUnboundedRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "")
	stdout, _, code := cmdRun(t, "cut -b1234567890- input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- Coverage: processBytesWithOutDelim ---

func TestCutBytesWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdefg\n")
	// Non-contiguous byte ranges with output delimiter
	stdout, _, code := cmdRun(t, "cut -b1-2,5- --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab:efg\n", stdout)
}

func TestCutBytesWithOutputDelimBeyondLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abc\n")
	// Range extends beyond line length
	stdout, _, code := cmdRun(t, "cut -b1,5- --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// --- Coverage: processBytesComplementWithOutDelim ---

func TestCutBytesComplementWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut --complement -b3-4 --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab:ef\n", stdout)
}

// --- Coverage: complement bytes without output delim ---

func TestCutBytesComplementNoOutDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut --complement -b3-4 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abef\n", stdout)
}

// --- Coverage: -n flag (ignored, matching GNU coreutils) ---

func TestCutBytesNFlagIsNoOp(t *testing.T) {
	dir := t.TempDir()
	// α is 2 bytes (0xCE 0xB1), β is 2 bytes (0xCE 0xB2), γ is 2 bytes (0xCE 0xB3)
	writeFile(t, dir, "input.txt", "\xce\xb1\xce\xb2\xce\xb3\n") // αβγ
	// GNU cut ignores -n: -b1 -n selects only the first byte (0xCE), not the full character
	stdout, _, code := cmdRun(t, "cut -b1 -n input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\xce\n", stdout)
}

func TestCutBytesNFlagRangeIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "\xce\xb1\xce\xb2\xce\xb3\n") // αβγ
	// GNU cut ignores -n: -b1-3 -n selects bytes 1,2,3 (0xCE 0xB1 0xCE)
	stdout, _, code := cmdRun(t, "cut -b1-3 -n input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\xce\xb1\xce\n", stdout)
}

func TestCutBytesNFlagWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "\xce\xb1\xce\xb2\xce\xb3\n") // αβγ
	// GNU cut ignores -n: -b1,5 -n selects bytes 1 and 5 (0xCE and 0xCE)
	stdout, _, code := cmdRun(t, "cut -b1,5 -n --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\xce:\xce\n", stdout)
}

func TestCutBytesNFlagComplement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "\xce\xb1\xce\xb2\xce\xb3\n") // αβγ
	// GNU cut ignores -n: --complement -b1 -n removes byte 1, keeps bytes 2-6
	stdout, _, code := cmdRun(t, "cut -b1 -n --complement input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\xb1\xce\xb2\xce\xb3\n", stdout)
}

// --- Coverage: CRLF line endings ---

func TestCutCRLFLineEnding(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\r\n")
	stdout, _, code := cmdRun(t, "cut -d: -f2 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b\n", stdout)
}

func TestCutCRLFLineEndingLastField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c\r\n")
	// GNU cut preserves \r as part of the last field content
	stdout, _, code := cmdRun(t, "cut -d: -f3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "c\r\n", stdout)
}

func TestCutCRLFByteMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "ab\r\n")
	// GNU cut treats \r as byte 3 (regular content byte)
	stdout, _, code := cmdRun(t, "cut -b3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\r\n", stdout)
}

// --- Coverage: decreasing range error ---

func TestCutDecreasingRangeBytes(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -b 5-3 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "decreasing")
}

// --- Coverage: multiple modes error ---

func TestCutMultipleModes(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cut -b1 -f1 input.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cut:")
}

// --- Coverage: chars complement ---

func TestCutCharsComplement(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut --complement -c2,4 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "acef\n", stdout)
}

// --- Coverage: chars with output delimiter ---

func TestCutCharsWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut -c1-2,5- --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab:ef\n", stdout)
}

// --- Coverage: chars complement with output delimiter ---

func TestCutCharsComplementWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "abcdef\n")
	stdout, _, code := cmdRun(t, "cut --complement -c3-4 --output-delimiter=: input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab:ef\n", stdout)
}

// --- Coverage: context cancellation ---

func TestCutContextCancelled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b\n")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _ = runScriptCtx(ctx, t, "cut -d: -f1 input.txt", dir, interp.AllowedPaths([]string{dir}))
}

// --- Coverage: stdin with no files (dash) ---

func TestCutStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "x:y:z\n")
	stdout, _, code := cmdRun(t, "cat input.txt | cut -d: -f1 -", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "x\n", stdout)
}

// --- Coverage: field complement with output delimiter ---

func TestCutFieldComplementWithOutputDelim(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "a:b:c:d\n")
	stdout, _, code := cmdRun(t, "cut -d: --complement --output-delimiter=_ -f2,3 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a_d\n", stdout)
}

// --- Coverage: empty line in byte mode ---

func TestCutBytesEmptyLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "input.txt", "\n")
	stdout, _, code := cmdRun(t, "cut -b1 input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}
