// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTail is a test helper that runs a tail command and returns stdout, stderr,
// and the exit code. It uses a fresh Runner for each invocation.
func runTail(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	_ = r.Run(context.Background(), script)
	return out.String(), errOut.String(), r.ExitCode()
}

// runTailInDir runs a script with a specific working directory.
func runTailInDir(t *testing.T, dir, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut), WithDir(dir))
	_ = r.Run(context.Background(), script)
	return out.String(), errOut.String(), r.ExitCode()
}

// writeTempFile creates a file in dir with the given content and returns
// its absolute path.
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

// --- Default (last 10 lines) ---

func TestTail_DefaultLast10(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, fmt.Sprintf("%d", i))
	}
	f := writeTempFile(t, dir, "input.txt", strings.Join(lines, "\n")+"\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", f))
	assert.Equal(t, 0, ec)
	var expected []string
	for i := 6; i <= 15; i++ {
		expected = append(expected, fmt.Sprintf("%d", i))
	}
	assert.Equal(t, strings.Join(expected, "\n")+"\n", stdout)
}

func TestTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "empty.txt", "")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "", stdout)
}

func TestTail_SingleLine(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "single.txt", "only\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "only\n", stdout)
}

func TestTail_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "nonl.txt", "a\nb\nc")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c", stdout)
}

// --- -n flag ---

func TestTail_NLines(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\nd\ne\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 3 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_NLines0(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 0 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "", stdout)
}

func TestTail_NLinesFromStart(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\nd\ne\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n +3 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_NLinesFromStartPlus1(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	// +1 = entire file
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n +1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestTail_NLinesFromStartPlus0(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	// +0 = same as +1 (all lines)
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n +0 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestTail_NLinesLong(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail --lines 2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "b\nc\n", stdout)
}

func TestTail_NLinesLongEquals(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail --lines=2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "b\nc\n", stdout)
}

// --- -c flag ---

func TestTail_CBytes(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdefghij")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 5 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "fghij", stdout)
}

func TestTail_CBytes0(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdefghij")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 0 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "", stdout)
}

func TestTail_CBytesFromStart(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdefghij")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c +4 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "defghij", stdout)
}

func TestTail_CBytesFromStartPlus1(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdefghij")

	// +1 = entire file
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c +1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "abcdefghij", stdout)
}

func TestTail_CBytesLargerThanFile(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abc")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 100 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "abc", stdout)
}

// --- -q and -v flags ---

func TestTail_MultiFileHeaders(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTempFile(t, dir, "a.txt", "aaa\n")
	f2 := writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1 %s %s", f1, f2))
	assert.Equal(t, 0, ec)
	assert.Contains(t, stdout, "==> "+f1+" <==")
	assert.Contains(t, stdout, "==> "+f2+" <==")
	assert.Contains(t, stdout, "aaa")
	assert.Contains(t, stdout, "bbb")
}

func TestTail_QuietSuppressHeaders(t *testing.T) {
	dir := t.TempDir()
	f1 := writeTempFile(t, dir, "a.txt", "aaa\n")
	f2 := writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -q -n 1 %s %s", f1, f2))
	assert.Equal(t, 0, ec)
	assert.NotContains(t, stdout, "==>")
	assert.Equal(t, "aaa\nbbb\n", stdout)
}

func TestTail_VerboseSingleFile(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "a.txt", "aaa\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -v -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Contains(t, stdout, "==> "+f+" <==")
	assert.Contains(t, stdout, "aaa")
}

func TestTail_SingleFileNoHeader(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "a.txt", "aaa\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.NotContains(t, stdout, "==>")
	assert.Equal(t, "aaa\n", stdout)
}

// --- --help flag ---

func TestTail_Help(t *testing.T) {
	stdout, _, ec := runTail(t, "tail --help")
	assert.Equal(t, 0, ec)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "tail")
}

// --- stdin ---

func TestTail_Stdin(t *testing.T) {
	stdout, _, ec := runTail(t, `echo -e "a\nb\nc\nd\ne" | tail -n 2`)
	assert.Equal(t, 0, ec)
	assert.Equal(t, "d\ne\n", stdout)
}

func TestTail_StdinBytes(t *testing.T) {
	stdout, _, ec := runTail(t, `echo -n "abcdef" | tail -c 3`)
	assert.Equal(t, 0, ec)
	assert.Equal(t, "def", stdout)
}

func TestTail_StdinFromStart(t *testing.T) {
	stdout, _, ec := runTail(t, `echo -e "a\nb\nc\nd\ne" | tail -n +3`)
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

// --- Error cases ---

func TestTail_MissingFile(t *testing.T) {
	_, stderr, ec := runTail(t, "tail /nonexistent/path/file.txt")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "tail:")
}

func TestTail_RejectsFollowFlag(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -f somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsFollowLong(t *testing.T) {
	_, stderr, ec := runTail(t, "tail --follow somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsBigF(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -F somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsZeroTerminated(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -z somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_InvalidLineNumber(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -n abc somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "invalid")
}

func TestTail_NegativeLineNumber(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -n -5 somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "invalid")
}

func TestTail_InvalidByteNumber(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -c abc somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "invalid")
}

func TestTail_NegativeByteNumber(t *testing.T) {
	_, stderr, ec := runTail(t, "tail -c -5 somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "invalid")
}

// --- Integer overflow ---

func TestTail_HugeLineCount(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "small.txt", "hello\n")

	// Should clamp to maxInt, not OOM
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 99999999999999999999 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "hello\n", stdout)
}

func TestTail_HugeByteCount(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "small.txt", "hello")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 99999999999999999999 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "hello", stdout)
}

// --- Resource limits ---

func TestTail_MaxLinesClamp(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "small.txt", "hello\n")

	// Request more than tailMaxLines, should still work without OOM
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n %d %s", math.MaxInt32, f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "hello\n", stdout)
}

// --- Pipes ---

func TestTail_Pipe(t *testing.T) {
	stdout, _, ec := runTail(t, `echo -e "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12" | tail`)
	assert.Equal(t, 0, ec)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 10)
	assert.Equal(t, "3", lines[0])
	assert.Equal(t, "12", lines[9])
}

// --- Context cancellation ---

func TestTail_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should not hang — context cancellation should stop the read
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	err := r.Run(ctx, `tail`)
	// Either context error or normal completion is fine
	_ = err
}

// --- Mixed file errors ---

func TestTail_SomeFilesMissing(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "good.txt", "hello\n")

	stdout, stderr, ec := runTail(t, fmt.Sprintf("tail %s /nonexistent/file %s", f, f))
	assert.Equal(t, 1, ec) // error because one file missing
	assert.Contains(t, stderr, "tail:")
	assert.Contains(t, stdout, "hello") // good files still processed
}

// --- Directory as argument ---

func TestTail_Directory(t *testing.T) {
	dir := t.TempDir()

	_, stderr, ec := runTail(t, fmt.Sprintf("tail %s", dir))
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "tail:")
}

// --- CRLF and CR line endings ---

func TestTail_CRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "crlf.txt", "a\r\nb\r\nc\r\nd\r\ne\r\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "d\r\ne\r\n", stdout)
}

func TestTail_CRLineEndings(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "cr.txt", "a\rb\rc\rd\re\r")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "d\re\r", stdout)
}

// --- Double dash ---

func TestTail_DoubleDash(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "file.txt", "content\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -- %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "content\n", stdout)
}

// --- Compact flag forms ---

func TestTail_CompactN(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "lines.txt", "a\nb\nc\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "b\nc\n", stdout)
}

func TestTail_CompactC(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdef")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c3 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "def", stdout)
}

// --- Stdin with dash ---

func TestTail_StdinDash(t *testing.T) {
	stdout, _, ec := runTail(t, `echo -e "a\nb\nc" | tail -n 1 -`)
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c\n", stdout)
}

// --- Multiple stdin ---

func TestTail_MultipleStdinDash(t *testing.T) {
	// Second - reads from already-consumed stdin, so it should be empty
	stdout, _, ec := runTail(t, `echo -e "a\nb\nc" | tail -n 1 - -`)
	assert.Equal(t, 0, ec)
	assert.Contains(t, stdout, "c")
}

// --- dev/null ---

func TestTail_DevNull(t *testing.T) {
	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", os.DevNull))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "", stdout)
}

// =============================================================================
// Hardening tests — RULES.md compliance and resource safety
// =============================================================================

// --- parseTailCount edge cases ---

func TestParseTailCount(t *testing.T) {
	tests := []struct {
		input        string
		wantCount    int
		wantFromOff  bool
		wantErr      bool
	}{
		{"10", 10, false, false},
		{"+5", 5, true, false},
		{"+0", 0, true, false},
		{"+1", 1, true, false},
		{"0", 0, false, false},
		// Overflow clamps to MaxInt
		{"99999999999999999999", math.MaxInt, false, false},
		{"+99999999999999999999", math.MaxInt, true, false},
		// Negative
		{"-5", 0, false, true},
		// Empty
		{"", 0, false, true},
		// Non-numeric
		{"abc", 0, false, true},
		// Whitespace
		{"  ", 0, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			count, fromOff, err := parseTailCount(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantCount, count)
				assert.Equal(t, tt.wantFromOff, fromOff)
			}
		})
	}
}

// --- Long line handling ---

func TestTail_LongLine(t *testing.T) {
	dir := t.TempDir()
	// Create a file with a line near tailMaxLineBytes
	longLine := strings.Repeat("x", tailMaxLineBytes-1)
	f := writeTempFile(t, dir, "long.txt", "short\n"+longLine+"\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, longLine+"\n", stdout)
}

// --- Ring buffer correctness at scale ---

func TestTail_ManyLines(t *testing.T) {
	dir := t.TempDir()
	var b strings.Builder
	for i := 0; i < 1000; i++ {
		fmt.Fprintf(&b, "line%d\n", i)
	}
	f := writeTempFile(t, dir, "many.txt", b.String())

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 5 %s", f))
	assert.Equal(t, 0, ec)
	expected := "line995\nline996\nline997\nline998\nline999\n"
	assert.Equal(t, expected, stdout)
}

// --- Byte mode ring buffer correctness ---

func TestTail_ByteModeSmallFile(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "small.txt", "ab")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 100 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "ab", stdout)
}

func TestTail_ByteModeExact(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "exact.txt", "abcde")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 5 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "abcde", stdout)
}

// --- findLineEnd tests ---

func TestFindLineEnd(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello\nworld", 5},
		{"hello\r\nworld", 6}, // returns index of LF in CRLF
		{"hello\rworld", 5},   // standalone CR
		{"hello", -1},
		{"", -1},
		{"\n", 0},
		{"\r\n", 1},
		{"\r", 0},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q", tt.input), func(t *testing.T) {
			result := findLineEnd([]byte(tt.input))
			assert.Equal(t, tt.expected, result)
		})
	}
}

// --- isWindowsReservedName tests ---

func TestIsWindowsReservedName(t *testing.T) {
	assert.True(t, isWindowsReservedName("CON"))
	assert.True(t, isWindowsReservedName("con"))
	assert.True(t, isWindowsReservedName("PRN"))
	assert.True(t, isWindowsReservedName("AUX"))
	assert.True(t, isWindowsReservedName("NUL"))
	assert.True(t, isWindowsReservedName("COM1"))
	assert.True(t, isWindowsReservedName("COM9"))
	assert.True(t, isWindowsReservedName("LPT1"))
	assert.True(t, isWindowsReservedName("LPT9"))
	assert.True(t, isWindowsReservedName("CON.txt"))
	assert.False(t, isWindowsReservedName("CONN"))
	assert.False(t, isWindowsReservedName("COM0"))
	assert.False(t, isWindowsReservedName("file.txt"))
	assert.False(t, isWindowsReservedName(""))
}

// --- discardReader with context ---

func TestTail_DiscardWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := discardReader(ctx, strings.NewReader("data"))
	assert.ErrorIs(t, err, context.Canceled)
}

// --- Empty stdin ---

func TestTail_EmptyStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&errOut),
		WithStdin(strings.NewReader("")),
	)
	_ = r.Run(context.Background(), `tail`)
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

// --- Both -n and -c specified (-n always wins because it's checked last in code) ---

func TestTail_BothNandC(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "data.txt", "abcdef\n")

	// -n always takes precedence because our code checks linesStr after bytesStr
	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 3 -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "abcdef\n", stdout) // -n 1 = last 1 line = entire line
}

// --- Path traversal ---

func TestTail_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(sub, 0755))
	writeTempFile(t, dir, "target.txt", "found\n")

	stdout, _, ec := runTailInDir(t, sub, "tail ../target.txt")
	assert.Equal(t, 0, ec)
	assert.Equal(t, "found\n", stdout)
}

// --- Multiple slashes in path ---

func TestTail_DoubleSlashPath(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "file.txt", "data\n")
	doubleSlash := strings.Replace(f, "/", "//", 1)

	stdout, _, ec := runTail(t, fmt.Sprintf("tail %s", doubleSlash))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "data\n", stdout)
}

// --- Large count on small file ---

func TestTail_LargeNOnSmallFile(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "small.txt", "one\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1000000 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "one\n", stdout)
}

// --- Additional rejected flags ---

func TestTail_RejectsRetry(t *testing.T) {
	_, stderr, ec := runTail(t, "tail --retry somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsPid(t *testing.T) {
	_, stderr, ec := runTail(t, "tail --pid=123 somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsSleepInterval(t *testing.T) {
	_, stderr, ec := runTail(t, "tail --sleep-interval=1 somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

func TestTail_RejectsMaxUnchangedStats(t *testing.T) {
	_, stderr, ec := runTail(t, "tail --max-unchanged-stats=5 somefile")
	assert.Equal(t, 1, ec)
	assert.Contains(t, stderr, "unknown")
}

// --- Binary/non-UTF8 data ---

func TestTail_BinaryData(t *testing.T) {
	dir := t.TempDir()
	// NUL bytes and invalid UTF-8
	data := "line1\n\x00\x80\xff\nline3\n"
	f := writeTempFile(t, dir, "binary.txt", data)

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 1 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "line3\n", stdout)
}

func TestTail_BinaryByteMode(t *testing.T) {
	dir := t.TempDir()
	data := "\x00\x01\x02\x03\x04"
	f := writeTempFile(t, dir, "binary.bin", data)

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -c 3 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "\x02\x03\x04", stdout)
}

// --- Mixed line endings ---

func TestTail_MixedLineEndings(t *testing.T) {
	dir := t.TempDir()
	// Mix of LF, CRLF, and CR
	f := writeTempFile(t, dir, "mixed.txt", "a\nb\r\nc\rd\n")

	stdout, _, ec := runTail(t, fmt.Sprintf("tail -n 2 %s", f))
	assert.Equal(t, 0, ec)
	assert.Equal(t, "c\rd\n", stdout)
}

// --- Integration with shell for-loop ---

func TestTail_ForLoopIntegration(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "alpha\n")
	writeTempFile(t, dir, "b.txt", "beta\n")

	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut), WithDir(dir),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}))
	err := r.Run(context.Background(), `for f in a.txt b.txt; do tail -n 1 $f; done`)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nbeta\n", out.String())
}
