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
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTailScript is a helper that creates a Runner with a temp dir and runs the
// given script. The temp dir is passed as WithDir so relative paths resolve.
func runTailScript(t *testing.T, dir, script string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	opts := []Option{
		WithStdout(&out),
		WithStderr(&errOut),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	}
	if dir != "" {
		opts = append(opts, WithDir(dir))
	}
	r := New(opts...)
	err = r.Run(context.Background(), script)
	return out.String(), errOut.String(), err
}

// writeTempFile creates a temp file with the given content in dir and returns its basename.
func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return name
}

// =============================================================================
// Basic default behaviour
// =============================================================================

func TestTail_Default10Lines(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 20; i++ {
		lines = append(lines, fmt.Sprintf("line%d", i))
	}
	writeTempFile(t, dir, "f.txt", strings.Join(lines, "\n")+"\n")

	stdout, _, err := runTailScript(t, dir, `tail f.txt`)
	require.NoError(t, err)
	want := strings.Join(lines[10:], "\n") + "\n"
	assert.Equal(t, want, stdout)
}

func TestTail_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "empty.txt", "")

	stdout, _, err := runTailScript(t, dir, `tail empty.txt`)
	require.NoError(t, err)
	assert.Equal(t, "", stdout)
}

func TestTail_FewerLinesThanN(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "small.txt", "a\nb\nc\n")

	stdout, _, err := runTailScript(t, dir, `tail -n 10 small.txt`)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestTail_NoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "no_nl.txt", "only line")

	stdout, _, err := runTailScript(t, dir, `tail no_nl.txt`)
	require.NoError(t, err)
	assert.Equal(t, "only line\n", stdout)
}

func TestTail_SingleLine(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "one.txt", "hello\n")

	stdout, _, err := runTailScript(t, dir, `tail one.txt`)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
}

// =============================================================================
// -n / --lines flags
// =============================================================================

func TestTail_FlagN(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	stdout, _, err := runTailScript(t, dir, `tail -n 3 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_FlagNCompact(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	stdout, _, err := runTailScript(t, dir, `tail -n3 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_FlagLinesEquals(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	stdout, _, err := runTailScript(t, dir, `tail --lines=2 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "d\ne\n", stdout)
}

func TestTail_FlagN_Zero(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\n")

	stdout, _, err := runTailScript(t, dir, `tail -n 0 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "", stdout)
}

// +N: from line N to end
func TestTail_FlagN_PlusOffset(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")

	stdout, _, err := runTailScript(t, dir, `tail -n +3 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

func TestTail_FlagN_PlusOffset1(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\n")

	// +1 means all lines
	stdout, _, err := runTailScript(t, dir, `tail -n +1 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// Negative line counts must be rejected (not silently clamped).
func TestTail_FlagN_Negative_Rejected(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\n")

	_, stderr, err := runTailScript(t, dir, `tail -n -5 f.txt`)
	require.NoError(t, err) // interpreter-level no error; exit code 1
	assert.Contains(t, stderr, "invalid")
	// Verify exit code was set to 1.
	var out bytes.Buffer
	var errOut bytes.Buffer
	r := New(WithStdout(&out), WithStderr(&errOut), WithDir(dir))
	_ = r.Run(context.Background(), `tail -n -5 f.txt`)
	assert.Equal(t, 1, r.ExitCode())
}

// Compact form: -n-5 should also be rejected.
func TestTail_FlagN_NegativeCompact_Rejected(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\n")

	_, stderr, err := runTailScript(t, dir, `tail -n-5 f.txt`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "invalid")
}

// =============================================================================
// -c / --bytes flags
// =============================================================================

func TestTail_FlagC(t *testing.T) {
	dir := t.TempDir()
	// "hello world\n" = 12 bytes. Last 6 = "world\n".
	writeTempFile(t, dir, "f.txt", "hello world\n")

	stdout, _, err := runTailScript(t, dir, `tail -c 6 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "world\n", stdout)
}

func TestTail_FlagC_MoreThanFile(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "hi\n")

	stdout, _, err := runTailScript(t, dir, `tail -c 100 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "hi\n", stdout)
}

func TestTail_FlagC_Zero(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "hello\n")

	stdout, _, err := runTailScript(t, dir, `tail -c 0 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "", stdout)
}

func TestTail_FlagBytesEquals(t *testing.T) {
	dir := t.TempDir()
	// "abcdef\n" = 7 bytes. Last 3 = "ef\n"
	writeTempFile(t, dir, "f.txt", "abcdef\n")

	stdout, _, err := runTailScript(t, dir, `tail --bytes=3 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "ef\n", stdout)
}

func TestTail_FlagC_PlusOffset(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "abcdef\n")

	// +4 means from byte 4 onward (1-indexed)
	stdout, _, err := runTailScript(t, dir, `tail -c +4 f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "def\n", stdout)
}

// Negative byte counts must be rejected.
func TestTail_FlagC_Negative_Rejected(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "hello\n")

	_, stderr, err := runTailScript(t, dir, `tail -c -1 f.txt`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "invalid")
}

// =============================================================================
// Invalid / missing numeric arguments
// =============================================================================

func TestTail_InvalidN_NonNumeric(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\n")

	_, stderr, err := runTailScript(t, dir, `tail -n abc f.txt`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "invalid number of lines")
}

func TestTail_InvalidC_NonNumeric(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\n")

	_, stderr, err := runTailScript(t, dir, `tail -c xyz f.txt`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "invalid number of bytes")
}

func TestTail_MissingArgN(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\n")

	_, stderr, err := runTailScript(t, dir, `tail -n`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "option requires an argument")
}

func TestTail_MissingArgC(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\n")

	_, stderr, err := runTailScript(t, dir, `tail -c`)
	require.NoError(t, err)
	assert.Contains(t, stderr, "option requires an argument")
}

// INT_MAX / very large values must clamp safely (no panic, no OOM).
func TestTail_IntMax_LineCount(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "a\nb\nc\n")

	script := fmt.Sprintf("tail -n %d f.txt", math.MaxInt32)
	stdout, _, err := runTailScript(t, dir, script)
	require.NoError(t, err)
	// Clamped to maxTailLines internally; all 3 lines returned.
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestTail_IntMax_ByteCount(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "hello\n")

	script := fmt.Sprintf("tail -c %d f.txt", math.MaxInt32)
	stdout, _, err := runTailScript(t, dir, script)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
}

// =============================================================================
// -q / -v (header) flags
// =============================================================================

func TestTail_MultipleFiles_Headers(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, err := runTailScript(t, dir, `tail a.txt b.txt`)
	require.NoError(t, err)
	assert.Equal(t, "==> a.txt <==\naaa\n\n==> b.txt <==\nbbb\n", stdout)
}

func TestTail_SingleFile_NoHeader(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")

	stdout, _, err := runTailScript(t, dir, `tail a.txt`)
	require.NoError(t, err)
	assert.Equal(t, "aaa\n", stdout)
}

func TestTail_FlagV_SingleFile_ForceHeader(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")

	stdout, _, err := runTailScript(t, dir, `tail -v a.txt`)
	require.NoError(t, err)
	assert.Equal(t, "==> a.txt <==\naaa\n", stdout)
}

func TestTail_FlagQ_SuppressHeaders(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, err := runTailScript(t, dir, `tail -q a.txt b.txt`)
	require.NoError(t, err)
	assert.Equal(t, "aaa\nbbb\n", stdout)
}

func TestTail_FlagQuiet_LongForm(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, err := runTailScript(t, dir, `tail --quiet a.txt b.txt`)
	require.NoError(t, err)
	assert.Equal(t, "aaa\nbbb\n", stdout)
}

func TestTail_FlagSilent_LongForm(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")

	stdout, _, err := runTailScript(t, dir, `tail --silent a.txt b.txt`)
	require.NoError(t, err)
	assert.Equal(t, "aaa\nbbb\n", stdout)
}

func TestTail_FlagVerbose_LongForm(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")

	stdout, _, err := runTailScript(t, dir, `tail --verbose a.txt`)
	require.NoError(t, err)
	assert.Equal(t, "==> a.txt <==\naaa\n", stdout)
}

// =============================================================================
// -z (NUL-delimited) flag
// =============================================================================

func TestTail_FlagZ_NULDelim(t *testing.T) {
	dir := t.TempDir()
	// File with NUL-separated "lines"
	content := "alpha\x00beta\x00gamma\x00delta\x00"
	writeTempFile(t, dir, "nul.txt", content)

	stdout, _, err := runTailScript(t, dir, `tail -z -n 2 nul.txt`)
	require.NoError(t, err)
	assert.Equal(t, "gamma\x00delta\x00", stdout)
}

func TestTail_FlagZeroTerminated_LongForm(t *testing.T) {
	dir := t.TempDir()
	content := "a\x00b\x00c\x00"
	writeTempFile(t, dir, "nul.txt", content)

	stdout, _, err := runTailScript(t, dir, `tail --zero-terminated -n 2 nul.txt`)
	require.NoError(t, err)
	assert.Equal(t, "b\x00c\x00", stdout)
}

// =============================================================================
// Rejection of follow-mode flags
// =============================================================================

// These tests use os.DevNull ("/dev/null" on Unix, "NUL" on Windows) instead
// of a hardcoded path so that they compile and pass on all platforms.
// The flags are rejected before any file is opened, so the path is irrelevant.

func TestTail_Reject_f(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail -f %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-f")
}

func TestTail_Reject_F(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail -F %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-F")
}

func TestTail_Reject_Follow(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --follow %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--follow")
}

func TestTail_Reject_Retry(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --retry %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--retry")
}

func TestTail_Reject_SleepInterval(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --sleep-interval=1 %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--sleep-interval")
}

func TestTail_Reject_PID(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --pid=1 %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--pid")
}

func TestTail_Reject_MaxUnchangedStats(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --max-unchanged-stats %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--max-unchanged-stats")
}

// Security regression: flag via for-loop word expansion must also be rejected.
func TestTail_Reject_FlagViaForLoop(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "hello\n")
	_, _, err := runTailScript(t, dir, `for flag in -f; do tail $flag f.txt; done`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-f")
}

func TestTail_Reject_UnknownFlag(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail --no-such-flag %s", os.DevNull))
	require.Error(t, err)
}

// =============================================================================
// Memory safety and clamping
// =============================================================================

func TestTail_Clamp_NLargerThanMax(t *testing.T) {
	// Requesting more lines than maxTailLines must not panic or OOM.
	var sb strings.Builder
	for i := range 100 {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", sb.String())

	huge := maxTailLines + 5
	script := fmt.Sprintf("tail -n %d f.txt", huge)
	stdout, _, err := runTailScript(t, dir, script)
	require.NoError(t, err)
	// All 100 lines should be returned (clamped internally but result is correct).
	assert.Equal(t, sb.String(), stdout)
}

func TestTail_StdinFallback(t *testing.T) {
	// tail reading from stdin via pipe
	stdout, _, err := runScript(t, `echo -e "a\nb\nc\nd\ne" | tail -n 3`)
	require.NoError(t, err)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

// =============================================================================
// Binary / non-UTF8 data
// =============================================================================

func TestTail_BinaryData_ByteMode(t *testing.T) {
	// Byte mode must pass through arbitrary bytes unchanged.
	dir := t.TempDir()
	data := []byte{0xFF, 0xFE, 0x00, 0x01, 0x80, 0x90, 0xAB, 0xCD}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), data, 0644))

	stdout, _, err := runTailScript(t, dir, `tail -c 4 bin.dat`)
	require.NoError(t, err)
	assert.Equal(t, string(data[4:]), stdout)
}

func TestTail_BinaryData_Pipe(t *testing.T) {
	// Non-UTF8 bytes through the pipe (non-seekable) circular buffer.
	dir := t.TempDir()
	data := make([]byte, 10)
	for i := range data {
		data[i] = byte(0x80 + i)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bin.dat"), data, 0644))

	// cat bin.dat | tail -c 5  →  last 5 bytes of the cat output.
	// cat adds a trailing newline (via fmt.Fprintln), so the 10-byte file
	// becomes 11 bytes in the pipe; last 5 = data[6:] + "\n".
	stdout, _, err := runTailScript(t, dir, `cat bin.dat | tail -c 5`)
	require.NoError(t, err)
	assert.Equal(t, string(data[6:])+"\n", stdout)
}

// =============================================================================
// Path traversal
// =============================================================================

func TestTail_PathTraversal_ReadAllowed(t *testing.T) {
	// By design the interpreter allows path traversal on reads (relies on OS
	// permissions).  Verify that a valid ../ path that exists is readable.
	parent := t.TempDir()
	child := filepath.Join(parent, "sub")
	require.NoError(t, os.Mkdir(child, 0755))
	writeTempFile(t, parent, "secret.txt", "content\n")

	stdout, _, err := runTailScript(t, child, `tail ../secret.txt`)
	require.NoError(t, err)
	assert.Equal(t, "content\n", stdout)
}

// =============================================================================
// Multiple files edge cases
// =============================================================================

func TestTail_MissingFile_ExitCode1(t *testing.T) {
	var out, errOut bytes.Buffer
	r := New(
		WithStdout(&out),
		WithStderr(&errOut),
		WithEnv([]string{"PATH=/usr/bin:/bin:/usr/local/bin"}),
	)
	err := r.Run(context.Background(), `tail /nonexistent/file`)
	require.NoError(t, err)
	assert.Equal(t, 1, r.ExitCode())
	assert.Contains(t, errOut.String(), "cannot open")
}

func TestTail_ThreeFiles_Headers(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")
	writeTempFile(t, dir, "c.txt", "ccc\n")

	stdout, _, err := runTailScript(t, dir, `tail a.txt b.txt c.txt`)
	require.NoError(t, err)
	assert.Equal(t, "==> a.txt <==\naaa\n\n==> b.txt <==\nbbb\n\n==> c.txt <==\nccc\n", stdout)
}

// =============================================================================
// Pipe integration
// =============================================================================

func TestTail_PipeWithHead(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\nl" | tail -n 5 | head -n 2`)
	require.NoError(t, err)
	assert.Equal(t, "h\ni\n", stdout)
}

func TestTail_PipeChain(t *testing.T) {
	stdout, _, err := runScript(t, `echo -e "b\na\nc" | sort | tail -n 1`)
	require.NoError(t, err)
	assert.Equal(t, "c\n", stdout)
}

// =============================================================================
// Glob expansion integration
// =============================================================================

func TestTail_GlobExpansion(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.txt", "aaa\n")
	writeTempFile(t, dir, "b.txt", "bbb\n")

	// Glob *.txt should expand to both files; headers must appear.
	stdout, _, err := runTailScript(t, dir, `tail *.txt`)
	require.NoError(t, err)
	assert.Contains(t, stdout, "==> a.txt <==")
	assert.Contains(t, stdout, "aaa")
	assert.Contains(t, stdout, "==> b.txt <==")
	assert.Contains(t, stdout, "bbb")
}

// =============================================================================
// End-of-flags (--)
// =============================================================================

func TestTail_EndOfFlags(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "f.txt", "x\ny\nz\n")

	stdout, _, err := runTailScript(t, dir, `tail -- f.txt`)
	require.NoError(t, err)
	assert.Equal(t, "x\ny\nz\n", stdout)
}

// =============================================================================
// Line ending formats (LF, CRLF, CR)
// =============================================================================

// TestTail_CRLF_LineEndings verifies that CRLF input is normalised to LF on
// output.  bufio.ScanLines strips the trailing \r from each CRLF-terminated
// token, so output always uses \n as the line separator.
func TestTail_CRLF_LineEndings(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "crlf.txt", "line1\r\nline2\r\nline3\r\n")

	stdout, _, err := runTailScript(t, dir, `tail -n 2 crlf.txt`)
	require.NoError(t, err)
	assert.Equal(t, "line2\nline3\n", stdout)
}

// TestTail_CR_LineEndings verifies CR-only files (legacy Mac \r separators).
// bufio.ScanLines only splits on \n, so CR-only content has no recognised
// terminator and the entire file is treated as a single line.
func TestTail_CR_LineEndings(t *testing.T) {
	dir := t.TempDir()
	// Three "records" separated by bare \r — no \n present.
	writeTempFile(t, dir, "cr.txt", "a\rb\rc")

	stdout, _, err := runTailScript(t, dir, `tail -n 1 cr.txt`)
	require.NoError(t, err)
	// Whole file is one line; output gets a trailing \n.
	assert.Equal(t, "a\rb\rc\n", stdout)
}

// TestTail_CRLF_ByteMode verifies that -c (byte mode) passes CRLF bytes
// through unchanged — no normalisation in raw byte copy.
func TestTail_CRLF_ByteMode(t *testing.T) {
	dir := t.TempDir()
	// "ab\r\n" = 4 bytes; last 3 = "b\r\n"
	writeTempFile(t, dir, "crlf.txt", "ab\r\n")

	stdout, _, err := runTailScript(t, dir, `tail -c 3 crlf.txt`)
	require.NoError(t, err)
	assert.Equal(t, "b\r\n", stdout)
}

// =============================================================================
// Windows reserved filename handling
// =============================================================================

// TestTailIsWindowsReservedName tests the helper function directly on all
// platforms.  On non-Windows it must always return false; on Windows it must
// return true for every documented reserved name (with and without extension,
// case-insensitive).
func TestTailIsWindowsReservedName(t *testing.T) {
	reserved := []string{
		"CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM9",
		"LPT1", "LPT2", "LPT9",
		// With extensions — still reserved on Windows.
		"NUL.txt", "CON.log",
		// Case variants.
		"nul", "con", "Nul",
	}
	notReserved := []string{"normal.txt", "CONSOLE", "NULLIFY", "COM", "LPT"}

	if runtime.GOOS == "windows" {
		for _, name := range reserved {
			assert.True(t, tailIsWindowsReservedName(name), "expected %q reserved on Windows", name)
		}
		for _, name := range notReserved {
			assert.False(t, tailIsWindowsReservedName(name), "expected %q not reserved on Windows", name)
		}
	} else {
		// On non-Windows all names are allowed.
		for _, name := range append(reserved, notReserved...) {
			assert.False(t, tailIsWindowsReservedName(name), "expected %q non-reserved on non-Windows", name)
		}
	}
}

// =============================================================================
// Additional flag rejection: -s short form
// =============================================================================

func TestTail_Reject_SleepIntervalShort(t *testing.T) {
	_, _, err := runScript(t, fmt.Sprintf("tail -s 5 %s", os.DevNull))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-s")
}

// =============================================================================
// Explicit stdin via "-"
// =============================================================================

func TestTail_StdinDash(t *testing.T) {
	// Explicit "-" must read from stdin, same as omitting the filename.
	stdout, _, err := runScript(t, `echo -e "a\nb\nc" | tail -n 2 -`)
	require.NoError(t, err)
	assert.Equal(t, "b\nc\n", stdout)
}

// =============================================================================
// Paths with special characters
// =============================================================================

func TestTail_PathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my file.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0644))

	stdout, _, err := runTailScript(t, dir, `tail "my file.txt"`)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", stdout)
}
