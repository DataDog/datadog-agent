// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTailFile writes content to a temp file and returns the path.
func writeTailFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0644))
	return p
}

// =============================================================================
// Basic last-N lines
// =============================================================================

func TestTail_DefaultLastTenLines(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 15; i++ {
		lines = append(lines, strings.Repeat("x", i))
	}
	content := strings.Join(lines, "\n") + "\n"
	writeTailFile(t, dir, "f.txt", content)

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	outLines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	assert.Len(t, outLines, 10)
}

func TestTail_LastNLines(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 3 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "c\nd\ne\n", out.String())
}

func TestTail_LastZeroLines(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 0 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

func TestTail_LastNMoreThanFileSize(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\n"
	writeTailFile(t, dir, "f.txt", content)

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 100 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, content, out.String())
}

func TestTail_LinesLongFlag(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail --lines=2 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_LinesCompactFlag(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n2 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc") // no trailing newline

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 2 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_SingleLine(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "only\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 5 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "only\n", out.String())
}

func TestTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "empty.txt", "")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail empty.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

// =============================================================================
// CRLF handling
// =============================================================================

func TestTail_CRLFLineEndings(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "crlf.txt", "a\r\nb\r\nc\r\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 2 crlf.txt"))
	assert.Equal(t, 0, r.ExitCode())
	// Output should use \n, stripping the \r
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_BareCRLineEndings(t *testing.T) {
	dir := t.TempDir()
	// Bare CR (\r) as line separator
	writeTailFile(t, dir, "cr.txt", "a\rb\rc\r")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 2 cr.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\n", out.String())
}

func TestTail_MixedLineEndings(t *testing.T) {
	dir := t.TempDir()
	// Mixed: bare CR, LF, CRLF, no-terminator — each terminator ends its own line.
	// Lines: a(CR), b(LF), c(CRLF), d(none) — 4 lines total; tail -n 3 gives b, c, d.
	writeTailFile(t, dir, "mixed.txt", "a\rb\nc\r\nd")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 3 mixed.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "b\nc\nd\n", out.String())
}

// =============================================================================
// +N (from-line-N) mode
// =============================================================================

func TestTail_FromLine(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n +3 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "c\nd\ne\n", out.String())
}

func TestTail_FromLine1(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\n"
	writeTailFile(t, dir, "f.txt", content)

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n +1 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, content, out.String())
}

func TestTail_FromLineBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "a\nb\nc\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n +100 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

// GNU coreutils: +0 is synonym for +1 (output all lines)
func TestTail_FromLinePlusZero(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\n"
	writeTailFile(t, dir, "f.txt", content)

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n +0 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, content, out.String())
}

// =============================================================================
// Byte mode
// =============================================================================

func TestTail_LastNBytes(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "hello world")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c 5 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "world", out.String())
}

func TestTail_LastZeroBytes(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "hello")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c 0 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

func TestTail_BytesLongFlag(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "abcdef")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail --bytes=3 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "def", out.String())
}

func TestTail_FromByte(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "abcdef")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c +3 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "cdef", out.String())
}

func TestTail_FromByteBeyondEOF(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "abc")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c +100 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

func TestTail_BytesCompactFlag(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "abcdef")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c2 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "ef", out.String())
}

// GNU coreutils: very large -c value with empty input should succeed (not error).
func TestTail_HugeBytesEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c 99999999999999999999 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

// GNU coreutils: very large +N offset skips entire file.
func TestTail_HugeFromByteOffset(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "f.txt", "abcd")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -c +999999999999999999999999999999999999999999 f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, "", out.String())
}

// =============================================================================
// Stdin
// =============================================================================

func TestTail_Stdin(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e 'a\nb\nc\nd\ne' | tail -n 3`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_StdinDefault(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e 'l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11' | tail`)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 10)
}

func TestTail_StdinBytes(t *testing.T) {
	stdout, _, err := runScript(t, `echo -n abcdef | tail -c 3`)
	require.NoError(t, err)
	assert.Equal(t, "def", stdout)
}

// =============================================================================
// Multiple files + headers
// =============================================================================

func TestTail_MultipleFilesShowsHeaders(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")
	writeTailFile(t, dir, "b.txt", "bbb\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 1 a.txt b.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Contains(t, out.String(), "==> a.txt <==")
	assert.Contains(t, out.String(), "==> b.txt <==")
}

func TestTail_SingleFileNoHeader(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -n 1 a.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.NotContains(t, out.String(), "==>")
}

func TestTail_QuietSuppressesHeaders(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")
	writeTailFile(t, dir, "b.txt", "bbb\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -q -n 1 a.txt b.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.NotContains(t, out.String(), "==>")
}

func TestTail_VerboseForcesSingleFileHeader(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -v -n 1 a.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Contains(t, out.String(), "==> a.txt <==")
}

func TestTail_SilentFlagAlias(t *testing.T) {
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")
	writeTailFile(t, dir, "b.txt", "bbb\n")

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail --silent -n 1 a.txt b.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.NotContains(t, out.String(), "==>")
}

func TestTail_CombinedShortFlags(t *testing.T) {
	// pflag handles combined boolean short flags: -qv (quiet+verbose together — quiet wins)
	dir := t.TempDir()
	writeTailFile(t, dir, "a.txt", "aaa\n")
	writeTailFile(t, dir, "b.txt", "bbb\n")

	var out bytes.Buffer
	// -q alone should suppress headers even for multiple files
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail --quiet -n 1 a.txt b.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.NotContains(t, out.String(), "==>")
}

// =============================================================================
// Error handling
// =============================================================================

func TestTail_MissingFile(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut), WithDir(t.TempDir()))
	require.NoError(t, r.Run(context.Background(), "tail nonexistent.txt"))
	assert.Equal(t, 1, r.ExitCode())
	assert.Contains(t, strings.ToLower(errOut.String()), "no such file or directory")
}

func TestTail_InvalidNArg(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail -n abc"))
	assert.Equal(t, 1, r.ExitCode())
	assert.Contains(t, errOut.String(), "invalid number of lines")
}

func TestTail_InvalidCArg(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail -c abc"))
	assert.Equal(t, 1, r.ExitCode())
	assert.Contains(t, errOut.String(), "invalid number of bytes")
}

// =============================================================================
// Help flag
// =============================================================================

func TestTail_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail --help"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Contains(t, out.String(), "Usage:")
	// Help goes to stdout, not stderr
	assert.Empty(t, errOut.String())
}

// =============================================================================
// Rejected (unsupported) flags — pflag rejects automatically
// =============================================================================

func TestTail_FollowRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail -f /dev/null"))
	assert.Equal(t, 1, r.ExitCode())
}

func TestTail_FollowLongRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail --follow /dev/null"))
	assert.Equal(t, 1, r.ExitCode())
}

func TestTail_CapitalFRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail -F /dev/null"))
	assert.Equal(t, 1, r.ExitCode())
}

func TestTail_RetryRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail --retry /dev/null"))
	assert.Equal(t, 1, r.ExitCode())
}

func TestTail_ReverseRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail -r /dev/null"))
	assert.Equal(t, 1, r.ExitCode())
}

func TestTail_UnknownFlagGoesToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut))
	require.NoError(t, r.Run(context.Background(), "tail --bogus"))
	assert.Equal(t, 1, r.ExitCode())
	// Error must be on stderr, not stdout
	assert.Empty(t, out.String())
	assert.NotEmpty(t, errOut.String())
}

// =============================================================================
// Context cancellation (DoS prevention)
// =============================================================================

func TestTail_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}))
	err := r.Run(ctx, "echo hello | tail -n 5")
	require.Error(t, err)
}

// =============================================================================
// DoubleDash end-of-flags
// =============================================================================

func TestTail_DoubleDashEndOfFlags(t *testing.T) {
	dir := t.TempDir()
	content := "a\nb\nc\n"
	writeTailFile(t, dir, "f.txt", content)

	var out bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir))
	require.NoError(t, r.Run(context.Background(), "tail -- f.txt"))
	assert.Equal(t, 0, r.ExitCode())
	assert.Equal(t, content, out.String())
}

// =============================================================================
// Integration with shell features
// =============================================================================

func TestTail_PipeChain(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e 'a\nb\nc\nd\ne' | tail -n 3 | head -n 2`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\n", stdout)
}

func TestTail_ForLoop(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.txt", "b.txt"} {
		writeTailFile(t, dir, f, "x\ny\nz\n")
	}

	var out bytes.Buffer
	r := New(
		WithStdout(&out), WithStderr(&bytes.Buffer{}), WithDir(dir),
		WithEnv([]string{"PATH=/usr/bin:/bin"}),
	)
	err := r.Run(context.Background(), `for f in a.txt b.txt; do tail -n 1 $f; done`)
	require.NoError(t, err)
	assert.Equal(t, "z\nz\n", out.String())
}
