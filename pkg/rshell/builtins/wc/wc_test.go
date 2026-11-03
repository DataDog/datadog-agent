// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package wc_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, opts...)
}

func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	return name
}

// --- Default mode (lines, words, bytes) ---

func TestWcDefaultEmptyStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "wc empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 0 0 empty.txt\n", stdout)
}

func TestWcDefaultBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a b\nc\n")
	stdout, _, code := cmdRun(t, "wc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 3 6 file.txt\n", stdout)
}

func TestWcDefaultNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello world")
	stdout, _, code := cmdRun(t, "wc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, " 0  2 11 file.txt\n", stdout)
}

// --- Lines ---

func TestWcLinesEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "")
	stdout, _, code := cmdRun(t, "wc -l file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

func TestWcLinesNoNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x y")
	stdout, _, code := cmdRun(t, "wc -l file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

func TestWcLinesOneNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x y\n")
	stdout, _, code := cmdRun(t, "wc -l file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 file.txt\n", stdout)
}

func TestWcLinesTwoNewlines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x\ny\n")
	stdout, _, code := cmdRun(t, "wc -l file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 file.txt\n", stdout)
}

func TestWcLinesLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\nb\nc\n")
	stdout, _, code := cmdRun(t, "wc --lines file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3 file.txt\n", stdout)
}

// --- Words ---

func TestWcWordsEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "")
	stdout, _, code := cmdRun(t, "wc -w file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

func TestWcWordsSingle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x")
	stdout, _, code := cmdRun(t, "wc -w file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 file.txt\n", stdout)
}

func TestWcWordsMultiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x y\nz")
	stdout, _, code := cmdRun(t, "wc -w file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3 file.txt\n", stdout)
}

func TestWcWordsControlCharNotWord(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "\x01\n")
	stdout, _, code := cmdRun(t, "wc -w file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

// --- Bytes ---

func TestWcBytesEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "")
	stdout, _, code := cmdRun(t, "wc -c file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

func TestWcBytesSingle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "x")
	stdout, _, code := cmdRun(t, "wc -c file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 file.txt\n", stdout)
}

func TestWcBytesMulti(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello\n")
	stdout, _, code := cmdRun(t, "wc -c file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "6 file.txt\n", stdout)
}

// --- Chars ---

func TestWcCharsASCII(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello\n")
	stdout, _, code := cmdRun(t, "wc -m file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "6 file.txt\n", stdout)
}

func TestWcCharsMultibyte(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "café\n")
	stdout, _, code := cmdRun(t, "wc -m file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5 file.txt\n", stdout)
}

func TestWcBytesMultibyte(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "café\n")
	stdout, _, code := cmdRun(t, "wc -c file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "6 file.txt\n", stdout)
}

func TestWcCharsAndBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "café\n")
	stdout, _, code := cmdRun(t, "wc -cm file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5 6 file.txt\n", stdout)
}

// --- Max line length ---

func TestWcMaxLineLenBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "1\n12\n")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 file.txt\n", stdout)
}

func TestWcMaxLineLenThreeLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "1\n123\n1\n")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3 file.txt\n", stdout)
}

func TestWcMaxLineLenNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "\n123456")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "6 file.txt\n", stdout)
}

func TestWcMaxLineLenEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0 file.txt\n", stdout)
}

// --- Multiple files ---

func TestWcMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello\n")
	writeFile(t, dir, "b.txt", "world foo\n")
	stdout, _, code := cmdRun(t, "wc a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, " 1  1  6 a.txt\n 1  2 10 b.txt\n 2  3 16 total\n", stdout)
}

func TestWcMultipleFilesPartialFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello\n")
	stdout, stderr, code := cmdRun(t, "wc a.txt missing.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "a.txt")
	assert.Contains(t, stdout, "total")
	assert.Contains(t, stderr, "wc:")
}

// --- Stdin ---

func TestWcStdinImplicit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a b\nc\n")
	stdout, _, code := cmdRun(t, "cat file.txt | wc", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      2       3       6\n", stdout)
}

func TestWcStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a b\nc\n")
	stdout, _, code := cmdRun(t, "cat file.txt | wc -", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      2       3       6 -\n", stdout)
}

func TestWcNilStdin(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := runScript(t, "wc", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      0       0       0\n", stdout)
}

// --- Help ---

func TestWcHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "wc --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}

func TestWcHelpShortRejected(t *testing.T) {
	// GNU wc does not support -h; it's an invalid option.
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "wc -h", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "wc:")
}

// --- Error cases ---

func TestWcMissingFile(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "wc nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "wc:")
}

func TestWcUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "wc --definitely-invalid", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "wc:")
}

func TestWcFiles0FromRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "wc --files0-from=foo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "wc:")
}

func TestWcDirectory(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "wc .", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "wc:")
	// GNU wc prints a zero count line with width-7 padding (non-regular file)
	assert.Equal(t, "      0       0       0 .\n", stdout)
}

// --- Hardening ---

func TestWcDoubleDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello\n")
	stdout, _, code := cmdRun(t, "wc -- file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 1 6 file.txt\n", stdout)
}

func TestWcContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", strings.Repeat("x\n", 100))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "wc file.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

func TestWcPipeInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "alpha\nbeta\ngamma\n")
	stdout, _, code := cmdRun(t, "cat file.txt | wc -l", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3\n", stdout)
}

// --- Combined flags ---

func TestWcAllFlags(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a b\nc\n")
	stdout, _, code := cmdRun(t, "wc -lwmcL file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 3 6 6 3 file.txt\n", stdout)
}

func TestWcLinesAndWords(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a b\nc\n")
	stdout, _, code := cmdRun(t, "wc -lw file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 3 file.txt\n", stdout)
}

// --- Width formatting ---

func TestWcWidthDeterminedByTotal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", strings.Repeat("word ", 20)+"\n")
	writeFile(t, dir, "b.txt", "x\n")
	stdout, _, code := cmdRun(t, "wc -w a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "total\n")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Equal(t, 3, len(lines))
}

// --- Max line length: tab and CR ---

func TestWcMaxLineLenTab(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\tb\n")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "9 file.txt\n", stdout)
}

func TestWcMaxLineLenCR(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello\rworld\n")
	stdout, _, code := cmdRun(t, "wc -L file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5 file.txt\n", stdout)
}

func TestWcCRLFLineCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\r\nb\r\n")
	stdout, _, code := cmdRun(t, "wc -l file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2 file.txt\n", stdout)
}

// --- Binary / non-UTF8 input ---

func TestWcBinaryInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.bin", string([]byte{0x00, 0xFF, 0xFE, 0x0A, 0x41}))
	stdout, _, code := cmdRun(t, "wc file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "file.bin")
	assert.Equal(t, 0, code)
}

// --- Multibyte chars ---

func TestWcCharsMultibyteEmoji(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hi 💐\n")
	stdout, _, code := cmdRun(t, "wc -m file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "5 file.txt\n", stdout)
}

// TestWcChunkBoundaryMultibyte verifies that a multibyte character straddling
// the 32 KiB read-buffer boundary is not double-counted. This requires
// programmatic file generation so it lives as a Go test rather than a scenario.
func TestWcChunkBoundaryMultibyte(t *testing.T) {
	dir := t.TempDir()
	// 💐 is 4 bytes; placing it at offset 32766 means it spans bytes 32766-32769,
	// straddling the 32768-byte chunk boundary and exercising the carry logic.
	prefix := strings.Repeat("a", 32*1024-2)
	content := prefix + "💐\n"
	writeFile(t, dir, "file.txt", content)
	stdout, _, code := cmdRun(t, "wc -mL file.txt", dir)
	assert.Equal(t, 0, code)
	// chars: 32766 'a' + 1 emoji + 1 newline = 32768
	// max line length: 32766 + 2 (emoji display width) = 32768
	assert.Equal(t, "32768 32768 file.txt\n", stdout)
}
