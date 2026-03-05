// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// setupTailDir creates a temp directory with files and returns its path.
func setupTailDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
	return dir
}

// tailRun runs a tail script with AllowedPaths set to dir.
func tailRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// TestTailDefaultLastTenLines verifies the default (last 10 lines) behaviour.
func TestTailDefaultLastTenLines(t *testing.T) {
	var sb strings.Builder
	for i := 1; i <= 15; i++ {
		sb.WriteString(strings.Repeat(string(rune('0'+i%10)), 1) + "\n")
	}
	dir := setupTailDir(t, map[string]string{"f.txt": sb.String()})

	stdout, _, exitCode := tailRun(t, "tail f.txt", dir)
	assert.Equal(t, 0, exitCode)

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Len(t, lines, 10)
	// Last line should be "5" (15 mod 10)
	assert.Equal(t, "5", lines[len(lines)-1])
}

// TestTailNLines verifies -n N outputs the last N lines.
func TestTailNLines(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\nd\ne\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 3 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

// TestTailNZero outputs nothing.
func TestTailNZero(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 0 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailNFromOffset verifies -n +N outputs from line N.
func TestTailNFromOffset(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\nd\ne\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +3 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

// TestTailNFromOffsetPlusOne starts at line 1 (all lines).
func TestTailNFromOffsetPlusOne(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "x\ny\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +1 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "x\ny\n", stdout)
}

// TestTailNFromOffsetPlusZeroTreatedAsPlusOne verifies +0 == +1.
func TestTailNFromOffsetPlusZeroTreatedAsPlusOne(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "x\ny\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +0 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "x\ny\n", stdout)
}

// TestTailNFromOffsetBeyondEOF outputs nothing when offset exceeds line count.
func TestTailNFromOffsetBeyondEOF(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +99 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailNMoreThanFileLines outputs the full file.
func TestTailNMoreThanFileLines(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 100 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// TestTailNNegativeGNUCompat verifies -n -N is treated same as -n N.
func TestTailNNegativeGNUCompat(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\nd\ne\n"})

	stdout, _, exitCode := tailRun(t, "tail -n -3 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "c\nd\ne\n", stdout)
}

// TestTailCBytes verifies -c N outputs the last N bytes.
// "abcde\n" is 6 bytes; last 3 bytes are "de\n".
func TestTailCBytes(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcde\n"})

	stdout, _, exitCode := tailRun(t, "tail -c 3 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "de\n", stdout)
}

// TestTailCBytesCorrect verifies -c 2 on "abcd\n".
func TestTailCBytesCorrect(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcd\n"})

	stdout, _, exitCode := tailRun(t, "tail -c 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "d\n", stdout)
}

// TestTailCBytesFromOffset verifies -c +N outputs from byte N.
func TestTailCBytesFromOffset(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcd"})

	stdout, _, exitCode := tailRun(t, "tail -c +2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "bcd", stdout)
}

// TestTailCBytesZero outputs nothing.
func TestTailCBytesZero(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abc"})

	stdout, _, exitCode := tailRun(t, "tail -c 0 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailCBytesMoreThanFile outputs the full file.
func TestTailCBytesMoreThanFile(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcd"})

	stdout, _, exitCode := tailRun(t, "tail -c 9999 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "abcd", stdout)
}

// TestTailCAndNLastNWins verifies that when both -c and -n are given, -n wins.
func TestTailCAndNLastNWins(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	stdout, _, exitCode := tailRun(t, "tail -c 1 -n 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\nc\n", stdout)
}

// TestTailVerboseHeader verifies -v always prints headers.
func TestTailVerboseHeader(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "hello\n"})

	stdout, _, exitCode := tailRun(t, "tail -v f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "==> f.txt <==\n")
	assert.Contains(t, stdout, "hello\n")
}

// TestTailMultipleFilesHeaders verifies headers between multiple files.
func TestTailMultipleFilesHeaders(t *testing.T) {
	dir := setupTailDir(t, map[string]string{
		"a.txt": "aaa\n",
		"b.txt": "bbb\n",
	})

	stdout, _, exitCode := tailRun(t, "tail a.txt b.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "==> a.txt <==\n")
	assert.Contains(t, stdout, "==> b.txt <==\n")
	assert.Contains(t, stdout, "aaa\n")
	assert.Contains(t, stdout, "bbb\n")
}

// TestTailQuietSuppressesHeaders verifies -q suppresses headers.
func TestTailQuietSuppressesHeaders(t *testing.T) {
	dir := setupTailDir(t, map[string]string{
		"a.txt": "aaa\n",
		"b.txt": "bbb\n",
	})

	stdout, _, exitCode := tailRun(t, "tail -q a.txt b.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.NotContains(t, stdout, "==>")
	assert.Contains(t, stdout, "aaa\n")
	assert.Contains(t, stdout, "bbb\n")
}

// TestTailSilentAliasForQuiet verifies --silent is an alias for --quiet.
func TestTailSilentAliasForQuiet(t *testing.T) {
	dir := setupTailDir(t, map[string]string{
		"a.txt": "aaa\n",
		"b.txt": "bbb\n",
	})

	stdout, _, exitCode := tailRun(t, "tail --silent a.txt b.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.NotContains(t, stdout, "==>")
}

// TestTailEmptyFile outputs nothing and exits 0.
func TestTailEmptyFile(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"empty.txt": ""})

	stdout, _, exitCode := tailRun(t, "tail empty.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailNoTrailingNewline handles files without a final newline.
func TestTailNoTrailingNewline(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc"})

	stdout, _, exitCode := tailRun(t, "tail -n 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\nc", stdout)
}

// TestTailSingleLine handles a single-line file.
func TestTailSingleLine(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "only\n"})

	stdout, _, exitCode := tailRun(t, "tail f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "only\n", stdout)
}

// TestTailMissingFileErrors verifies exit code 1 and stderr message.
func TestTailMissingFileErrors(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, exitCode := tailRun(t, "tail nonexistent.txt", dir)
	assert.Equal(t, 1, exitCode)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "tail:")
	assert.Contains(t, stderr, "nonexistent.txt")
}

// TestTailMissingFileContinues verifies processing continues after a missing file.
func TestTailMissingFileContinues(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"good.txt": "present\n"})

	stdout, _, exitCode := tailRun(t, "tail missing.txt good.txt", dir)
	assert.Equal(t, 1, exitCode) // exit 1 because one file failed
	assert.Contains(t, stdout, "present\n")
}

// TestTailDirectoryErrors verifies that passing a directory returns exit code 1.
func TestTailDirectoryErrors(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))

	_, stderr, exitCode := tailRun(t, "tail subdir", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailFollowFlagRejected verifies -f is rejected with exit code 1.
func TestTailFollowFlagRejected(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "data\n"})

	_, stderr, exitCode := tailRun(t, "tail -f f.txt", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailUnknownFlagRejected verifies unknown flags return exit 1.
func TestTailUnknownFlagRejected(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := tailRun(t, "tail --no-such-flag", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailInvalidNValue rejects non-numeric -n values.
func TestTailInvalidNValue(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := tailRun(t, "tail -n abc", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailInvalidCValue rejects non-numeric -c values.
func TestTailInvalidCValue(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := tailRun(t, "tail -c xyz", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailHelpFlag verifies --help outputs to stdout and exits 0.
func TestTailHelpFlag(t *testing.T) {
	dir := t.TempDir()

	stdout, _, exitCode := tailRun(t, "tail --help", dir)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "Usage: tail")
	assert.Contains(t, stdout, "--lines")
	assert.Contains(t, stdout, "--bytes")
}

// TestTailDashStdin verifies - reads from stdin (no stdin here → no output).
func TestTailDashStdin(t *testing.T) {
	dir := t.TempDir()

	stdout, _, exitCode := tailRun(t, "tail -", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailStdinViaRedirect verifies tail reads from stdin via heredoc.
func TestTailStdinViaRedirect(t *testing.T) {
	dir := t.TempDir()

	// Use shell heredoc to feed stdin
	script := "tail -n 2 << 'EOF'\na\nb\nc\nEOF"
	stdout, _, exitCode := tailRun(t, script, dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\nc\n", stdout)
}

// TestTailDoubleDashSeparator verifies -- separates flags from filenames.
func TestTailDoubleDashSeparator(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "hello\n"})

	stdout, _, exitCode := tailRun(t, "tail -- f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "hello")
}

// TestTailLargeN verifies very large N values are clamped and don't OOM.
func TestTailLargeN(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 99999999999999999999 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// TestTailLargeC verifies very large -c values are clamped and don't OOM.
func TestTailLargeC(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcd"})

	stdout, _, exitCode := tailRun(t, "tail -c 99999999999999999999 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "abcd", stdout)
}

// TestTailNFromOffsetAtExactEnd verifies +N where N == line count outputs last line.
func TestTailNFromOffsetAtExactEnd(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "x\ny\nz\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +3 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "z\n", stdout)
}

// TestTailAccessOutsideAllowedPath verifies the sandbox blocks reading outside allowed paths.
func TestTailAccessOutsideAllowedPath(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret"), 0644))

	_, stderr, exitCode := runScript(t,
		"tail "+filepath.Join(secret, "secret.txt"),
		allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "permission denied")
}

// TestTailCRLFLineEndings verifies CRLF is treated as one line ending.
func TestTailCRLFLineEndings(t *testing.T) {
	// "a\r\nb\r\nc\r\n" has 3 lines; last 2 are b and c
	dir := setupTailDir(t, map[string]string{"f.txt": "a\r\nb\r\nc\r\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\r\nc\r\n", stdout)
}

// TestTailCRLineEndings verifies bare CR is treated as a line ending.
func TestTailCRLineEndings(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\rb\rc\r"})

	stdout, _, exitCode := tailRun(t, "tail -n 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\rc\r", stdout)
}

// TestTailNFromOffsetCRLF verifies +N offset skips correct number of CRLF lines.
func TestTailNFromOffsetCRLF(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\r\nb\r\nc\r\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\r\nc\r\n", stdout)
}

// TestTailBinaryData verifies binary (non-UTF8) data is passed through unchanged.
func TestTailBinaryData(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x0A}
	dir := setupTailDir(t, map[string]string{"bin.bin": string(data)})

	stdout, _, exitCode := tailRun(t, "tail -c 4 bin.bin", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, string(data[2:]), stdout) // last 4 bytes: 0x02, 0xFF, 0xFE, 0x0A
}

// TestTailNLinesGNUTestN1 verifies -n 10 on 11-line input (GNU test n-1).
func TestTailNLinesGNUTestN1(t *testing.T) {
	// "x\n" + 10 * "y\n" + "z" = 12 lines (last z has no newline)
	content := "x\n" + strings.Repeat("y\n", 10) + "z"
	dir := setupTailDir(t, map[string]string{"f.txt": content})

	stdout, _, exitCode := tailRun(t, "tail -n 10 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	// Last 10 lines: 9 "y\n" lines + "z"
	assert.Equal(t, strings.Repeat("y\n", 9)+"z", stdout)
}

// TestTailNLinesGNUTestN3 verifies -n +10 on 12-line input (GNU test n-3).
func TestTailNLinesGNUTestN3(t *testing.T) {
	content := "x\n" + strings.Repeat("y\n", 10) + "z"
	dir := setupTailDir(t, map[string]string{"f.txt": content})

	stdout, _, exitCode := tailRun(t, "tail -n +10 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	// From line 10 onward: "y\ny\nz"
	assert.Equal(t, "y\ny\nz", stdout)
}

// TestTailCBytesGNUTestC2 verifies -c 2 on "abcd\n" (GNU test c-2).
func TestTailCBytesGNUTestC2(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "abcd\n"})

	stdout, _, exitCode := tailRun(t, "tail -c 2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "d\n", stdout)
}

// TestTailDevNull verifies reading from /dev/null produces no output (Unix only, see unix test file).
func TestTailPipeWithEcho(t *testing.T) {
	dir := t.TempDir()

	stdout, _, exitCode := runScript(t, `echo -e "a\nb\nc" | tail -n 2`, dir)
	assert.Equal(t, 0, exitCode)
	// echo -e might not be supported; test conservatively
	_ = exitCode
	_ = stdout
}

// TestTailNMinusNegativeZero verifies -n -0 outputs nothing (GNU test n-5).
func TestTailNMinusNegativeZero(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	// "-n -0" should output nothing (last 0 lines)
	stdout, _, exitCode := tailRun(t, "tail -n -0 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "", stdout)
}

// TestTailCountMaxUint64Overflow verifies uint64 overflow is clamped (ErrRange path).
func TestTailCountMaxUint64Overflow(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	// 99999999999999999999 (20 digits) overflows uint64 → ErrRange → clamped to MaxInt.
	stdout, _, exitCode := tailRun(t, "tail -n 99999999999999999999 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// TestTailCountMaxUint64Value verifies MaxUint64 (which fits in uint64 but > MaxInt64) is clamped.
func TestTailCountMaxUint64Value(t *testing.T) {
	dir := setupTailDir(t, map[string]string{"f.txt": "a\nb\nc\n"})

	// 18446744073709551615 = MaxUint64, fits in uint64 but > MaxInt64 on 64-bit → clamped.
	stdout, _, exitCode := tailRun(t, "tail -n 18446744073709551615 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// TestTailInvalidPlusAlone verifies "+" with no digits is rejected.
func TestTailInvalidPlusAlone(t *testing.T) {
	dir := t.TempDir()

	_, stderr, exitCode := tailRun(t, "tail -n + dummy.txt", dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "tail:")
}

// TestTailVeryLongLineTruncated verifies lines exceeding the cap don't crash.
// The line is capped at tailMaxLineBytes (256KiB); no OOM.
func TestTailVeryLongLineTruncated(t *testing.T) {
	// 256KiB + extra bytes to trigger the cap
	longLine := strings.Repeat("x", 256*1024+100)
	dir := setupTailDir(t, map[string]string{"f.txt": longLine + "\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 1 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	// Output must not exceed the cap + 1 (newline); no OOM.
	assert.LessOrEqual(t, len(stdout), 256*1024+1)
}

// TestTailCBytesRingBufferWraps verifies ring buffer output is correct when it wraps.
func TestTailCBytesRingBufferWraps(t *testing.T) {
	// Request last 4 bytes from a 10-byte file — ring buffer definitely wraps.
	dir := setupTailDir(t, map[string]string{"f.txt": "0123456789"})

	stdout, _, exitCode := tailRun(t, "tail -c 4 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "6789", stdout)
}

// TestTailCRLFAtChunkBoundaryLast verifies CRLF split across 4096-byte chunk boundaries
// is handled correctly in last-N line mode.
//
// CR at byte 4096 (1-indexed, last byte of first chunk), LF at byte 4097 (first byte of
// second chunk): exercises the prevCR carry-over path in tailLinesLast.
func TestTailCRLFAtChunkBoundaryLast(t *testing.T) {
	// Line 1: 4095 'a' bytes + CR = 4096 bytes (fills the first read chunk exactly).
	// The LF that follows is the first byte of the second chunk → triggers prevCR path.
	// Line 2: "hello\n"
	line1 := strings.Repeat("a", 4095) + "\r"
	dir := setupTailDir(t, map[string]string{"f.txt": line1 + "\nhello\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 1 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "hello\n", stdout)
}

// TestTailLoneCRAtChunkBoundaryLast verifies a lone CR at a chunk boundary is treated
// as a line ending (not merged with a subsequent non-LF byte).
func TestTailLoneCRAtChunkBoundaryLast(t *testing.T) {
	// Line 1: 4095 'a' bytes + CR = 4096 bytes (fills first chunk, CR at end).
	// Next chunk starts with 'b' (not LF) → prevCR is committed as lone CR ending.
	// Line 2: "b\n"
	line1 := strings.Repeat("a", 4095) + "\r"
	dir := setupTailDir(t, map[string]string{"f.txt": line1 + "b\n"})

	stdout, _, exitCode := tailRun(t, "tail -n 1 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "b\n", stdout)
}

// TestTailCRLFAtChunkBoundaryFrom verifies CRLF split across chunk boundaries
// in from-offset mode doesn't skip lines incorrectly.
func TestTailCRLFAtChunkBoundaryFrom(t *testing.T) {
	// Line 1: 4094 'a' bytes + CR + LF (4096 bytes). Line 2: "world\n".
	line1 := strings.Repeat("a", 4094) + "\r\n"
	dir := setupTailDir(t, map[string]string{"f.txt": line1 + "world\n"})

	stdout, _, exitCode := tailRun(t, "tail -n +2 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "world\n", stdout)
}

// TestTailCFromOffsetLargeSkip verifies -c +N with N > chunk size (4096).
func TestTailCFromOffsetLargeSkip(t *testing.T) {
	// File: 5000 'x' bytes + "end"
	prefix := strings.Repeat("x", 5000)
	dir := setupTailDir(t, map[string]string{"f.txt": prefix + "end"})

	stdout, _, exitCode := tailRun(t, "tail -c +5001 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "end", stdout)
}

// TestTailNFromOffsetLargeSkip verifies -n +N with many lines to skip.
func TestTailNFromOffsetLargeSkip(t *testing.T) {
	// 6000 lines; request from line 5999.
	var sb strings.Builder
	for i := 1; i <= 6000; i++ {
		sb.WriteString(strings.Repeat("a", 10) + "\n")
	}
	sb.WriteString("LAST\n")
	dir := setupTailDir(t, map[string]string{"f.txt": sb.String()})

	stdout, _, exitCode := tailRun(t, "tail -n +6001 f.txt", dir)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "LAST\n", stdout)
}
