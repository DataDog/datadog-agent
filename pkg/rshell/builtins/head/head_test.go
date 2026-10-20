// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package head_test

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

// runScriptCtx runs a shell script with a context and returns stdout, stderr,
// and the exit code.
func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, opts...)
}

// runScript runs a shell script and returns stdout, stderr, and the exit code.
func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

// cmdRun runs a head command with AllowedPaths set to dir.
func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// writeFile creates a file in dir with the given content and returns its name.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	return name
}

// fiveLines is a 5-line file used across multiple tests.
const fiveLines = "alpha\nbeta\ngamma\ndelta\nepsilon\n"

// twelveLines is a 12-line file used to test the default 10-line limit.
const twelveLines = "line01\nline02\nline03\nline04\nline05\nline06\nline07\nline08\nline09\nline10\nline11\nline12\n"

// --- Default behavior ---

func TestHeadDefaultTenLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", twelveLines)
	stdout, _, code := cmdRun(t, "head file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line01\nline02\nline03\nline04\nline05\nline06\nline07\nline08\nline09\nline10\n", stdout)
}

func TestHeadFileShorterThanDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

func TestHeadEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "head empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -n / --lines flag ---

func TestHeadLinesN3(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 3 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

func TestHeadLinesN0(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 0 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestHeadLinesLargerThanFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 100 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

func TestHeadLinesLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head --lines=3 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

func TestHeadLinesPositivePrefix(t *testing.T) {
	// GNU head: "+N" is treated as plain N (positive sign).
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n +2 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

func TestHeadLinesGlued(t *testing.T) {
	// -n3 (value glued to flag) is supported by pflag.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n3 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

// --- No trailing newline preservation ---

func TestHeadNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "no newline at end")
	stdout, _, code := cmdRun(t, "head -n 2 file.txt", dir)
	assert.Equal(t, 0, code)
	// Single line without newline — output exactly as-is.
	assert.Equal(t, "no newline at end", stdout)
}

func TestHeadLastLineNoNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "line1\nline2")
	stdout, _, code := cmdRun(t, "head -n 2 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline2", stdout)
}

func TestHeadFirstLineNewlineSecondNot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "line1\nline2")
	stdout, _, code := cmdRun(t, "head -n 1 file.txt", dir)
	assert.Equal(t, 0, code)
	// Only the first line (with its newline) is printed.
	assert.Equal(t, "line1\n", stdout)
}

// --- -c / --bytes flag ---

func TestHeadBytesN5(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 5 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout)
}

func TestHeadBytesN0(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 0 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestHeadBytesLargerThanFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 9999 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

func TestHeadBytesLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head --bytes=5 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout)
}

func TestHeadBytesPositivePrefix(t *testing.T) {
	// GNU head: "+N" is treated as plain N for -c too.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c +3 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alp", stdout)
}

func TestHeadBytesBinaryContent(t *testing.T) {
	dir := t.TempDir()
	// Write binary content including null bytes.
	content := "a\x00b\x00c\x00d"
	writeFile(t, dir, "file.bin", content)
	stdout, _, code := cmdRun(t, "head -c 5 file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\x00b\x00c", stdout)
}

// --- Last flag wins (-n vs -c) ---

func TestHeadLastFlagWinsBytes(t *testing.T) {
	// -n 2 -c 5: last flag is -c, so byte mode with 5 bytes.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 2 -c 5 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout)
}

func TestHeadLastFlagWinsLines(t *testing.T) {
	// -c 5 -n 2: last flag is -n, so line mode with 2 lines.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -c 5 -n 2 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

// --- Headers (-v / -q / --silent) ---

func TestHeadVerboseSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "one.txt", "only one line\n")
	stdout, _, code := cmdRun(t, "head -v one.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> one.txt <==\nonly one line\n", stdout)
}

func TestHeadTwoFilesDefaultHeaders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha\nbeta\n")
	writeFile(t, dir, "b.txt", "gamma\n")
	stdout, _, code := cmdRun(t, "head -n 2 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> a.txt <==\nalpha\nbeta\n\n==> b.txt <==\ngamma\n", stdout)
}

func TestHeadTwoFilesSecondNoNewline(t *testing.T) {
	// Verifies that the separator \n before the second header is always printed,
	// regardless of whether the first file ended with a newline.
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha\nbeta\n")
	writeFile(t, dir, "b.txt", "no newline")
	stdout, _, code := cmdRun(t, "head -n 2 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> a.txt <==\nalpha\nbeta\n\n==> b.txt <==\nno newline", stdout)
}

func TestHeadFirstFileNoNewline(t *testing.T) {
	// When first file ends without \n, the header separator still adds \n.
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "no newline")
	writeFile(t, dir, "b.txt", "next\n")
	stdout, _, code := cmdRun(t, "head -n 1 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> a.txt <==\nno newline\n==> b.txt <==\nnext\n", stdout)
}

func TestHeadQuietTwoFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha\nbeta\n")
	writeFile(t, dir, "b.txt", "gamma\n")
	stdout, _, code := cmdRun(t, "head -q -n 2 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

func TestHeadSilentAlias(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha\nbeta\n")
	writeFile(t, dir, "b.txt", "gamma\n")
	stdout, _, code := cmdRun(t, "head --silent -n 2 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\ngamma\n", stdout)
}

func TestHeadVerboseTwoFiles(t *testing.T) {
	// -v on multiple files still works (headers always printed).
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha\n")
	writeFile(t, dir, "b.txt", "beta\n")
	stdout, _, code := cmdRun(t, "head -v -n 1 a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> a.txt <==\nalpha\n\n==> b.txt <==\nbeta\n", stdout)
}

// --- Stdin ---

func TestHeadStdinImplicit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 2 < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

func TestHeadStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", fiveLines)
	stdout, _, code := cmdRun(t, "head -n 2 - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\nbeta\n", stdout)
}

func TestHeadStdinVerbose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "hello\n")
	stdout, _, code := cmdRun(t, "head -v - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> standard input <==\nhello\n", stdout)
}

// --- Help ---

func TestHeadHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "head --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "--lines")
	assert.Contains(t, stdout, "--bytes")
	assert.Empty(t, stderr)
}

func TestHeadHelpShort(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "head -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Empty(t, stderr)
}

// --- Error cases ---

func TestHeadMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "head nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))
	_, stderr, code := cmdRun(t, "head subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "head --follow file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadUnknownShortFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "head -f file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadInvalidCountString(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	_, stderr, code := cmdRun(t, "head -n abc file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadNegativeCount(t *testing.T) {
	// GNU head -n -N means "all but last N lines" — we do NOT support that.
	// We reject negative counts.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	_, stderr, code := cmdRun(t, "head -n -1 file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadNegativeBytesCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	_, stderr, code := cmdRun(t, "head -c -1 file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret"), 0644))

	secretPath := strings.ReplaceAll(filepath.Join(secret, "secret.txt"), `\`, `/`)
	_, stderr, code := runScript(t, "head "+secretPath, allowed, interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadMultipleFilesSomeFailSomeSuc(t *testing.T) {
	// When some files fail and some succeed, exit code is 1 and successful
	// files still produce output.
	dir := t.TempDir()
	writeFile(t, dir, "good.txt", "hello\n")
	stdout, stderr, code := cmdRun(t, "head -n 1 good.txt nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "hello")
	assert.Contains(t, stderr, "head:")
}

// --- RULES.md compliance ---

func TestHeadLargeCountClamped(t *testing.T) {
	// A count larger than maxHeadCount (1<<31-1) must be clamped, not cause OOM.
	// We pass a very large count on a tiny file; it should output the file content
	// without crashing or hanging.
	dir := t.TempDir()
	writeFile(t, dir, "small.txt", "tiny\n")
	stdout, _, code := cmdRun(t, "head -n 9999999999 small.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "tiny\n", stdout)
}

func TestHeadLargeByteCountClamped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "small.txt", "tiny")
	stdout, _, code := cmdRun(t, "head -c 9999999999 small.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "tiny", stdout)
}

func TestHeadContextCancellation(t *testing.T) {
	// The command must stop when the context is cancelled.
	dir := t.TempDir()
	// Use a pipe: create a heredoc that provides input.
	writeFile(t, dir, "data.txt", fiveLines)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Should complete well within 5 seconds.
	_, _, code := runScriptCtx(ctx, t, "head -n 3 data.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

func TestHeadDoubleDash(t *testing.T) {
	// After --, all args are treated as file names, even if they look like flags.
	dir := t.TempDir()
	writeFile(t, dir, "-n", "flag-looking-name\n")
	stdout, _, code := cmdRun(t, "head -- -n", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "flag-looking-name\n", stdout)
}

func TestHeadNullBytesInContent(t *testing.T) {
	// Binary content with null bytes must not crash or hang.
	dir := t.TempDir()
	content := "a\x00b\x00c\x00\n"
	writeFile(t, dir, "binary.bin", content)
	stdout, _, code := cmdRun(t, "head -n 1 binary.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, content, stdout)
}

func TestHeadCRLFPreserved(t *testing.T) {
	// CRLF line endings must be preserved exactly in the output.
	dir := t.TempDir()
	writeFile(t, dir, "crlf.txt", "line1\r\nline2\r\nline3\r\n")
	stdout, _, code := cmdRun(t, "head -n 2 crlf.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\r\nline2\r\n", stdout)
}

func TestHeadPipeInput(t *testing.T) {
	// Verify head works correctly in a pipeline.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", twelveLines)
	// cat file.txt | head -n 3
	stdout, _, code := cmdRun(t, "cat file.txt | head -n 3", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line01\nline02\nline03\n", stdout)
}

func TestHeadLineModeOnLineExactlyAtCap(t *testing.T) {
	// A line of exactly maxHeadLineBytes (1 MiB) with no newline.
	// bufio.Scanner.Buffer(buf, max) cannot hold a token of exactly max
	// bytes (the limit is exclusive), so this must error like an over-cap line.
	dir := t.TempDir()
	content := make([]byte, 1<<20)
	for i := range content {
		content[i] = 'a'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "exact.txt"), content, 0644))
	_, _, code := cmdRun(t, "head -n 1 exact.txt", dir)
	assert.Equal(t, 1, code)
}

func TestHeadLineModeOnSingleLineBeyondCap(t *testing.T) {
	// A line of maxHeadLineBytes+1 (1 MiB + 1 byte) with no newline.
	// Exceeds the scanner buffer cap and must error, not crash.
	dir := t.TempDir()
	oneMiBPlusOne := make([]byte, 1<<20+1)
	for i := range oneMiBPlusOne {
		oneMiBPlusOne[i] = 'a'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "huge.txt"), oneMiBPlusOne, 0644))
	_, stderr, code := cmdRun(t, "head -n 1 huge.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadLineModeOnLineBelowCap(t *testing.T) {
	// A line just below the 1MiB cap should succeed.
	dir := t.TempDir()
	// Write (1MiB - 1) bytes of 'b' followed by a newline.
	content := make([]byte, 1<<20-1)
	for i := range content {
		content[i] = 'b'
	}
	content = append(content, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), content, 0644))
	stdout, _, code := cmdRun(t, "head -n 1 large.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, string(content), stdout)
}

func TestHeadEmptyCountString(t *testing.T) {
	dir := t.TempDir()
	// pflag with StringP default "10" means "-n" alone with no value is an error.
	// If somehow an empty string is passed, it should be rejected.
	writeFile(t, dir, "file.txt", fiveLines)
	_, stderr, code := cmdRun(t, `head -n "" file.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "head:")
}

func TestHeadNilStdin(t *testing.T) {
	// When head is asked to read stdin ("-") but the shell has no stdin,
	// it should produce no output and exit 0 (callCtx.Stdin == nil path).
	dir := t.TempDir()
	// runScript with no stdin redirect — shell stdin stays nil.
	stdout, stderr, code := runScript(t, "head -", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestHeadNilStdinVerbose(t *testing.T) {
	// -v must print the header for stdin even when callCtx.Stdin == nil.
	// Previously the nil guard fired before the header block, silently
	// skipping the "==> standard input <==" line.
	dir := t.TempDir()
	stdout, stderr, code := runScript(t, "head -v -", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> standard input <==\n", stdout)
	assert.Equal(t, "", stderr)
}

func TestHeadBytesAppearsLastWithDoubleDash(t *testing.T) {
	// pflag stops parsing at "--", so file names after "--" are never
	// mistaken for flags. With -n and -c both set before "--", the
	// last-flag-wins logic applies (bytes mode because -c appears last).
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	// -n 3 -c 5 -- file.txt: both set, -c appears last before --, so byte mode.
	stdout, _, code := cmdRun(t, "head -n 3 -c 5 -- file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha", stdout) // first 5 bytes
}

func TestHeadContextPreCancelled(t *testing.T) {
	// A pre-cancelled context should cause the command to abort immediately.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before running

	// We don't assert a specific exit code (context cancellation may or may
	// not surface as exit code 1 depending on timing), but we must not hang.
	done := make(chan struct{})
	go func() {
		runScriptCtx(ctx, t, "head -n 5 file.txt", dir, interp.AllowedPaths([]string{dir}))
		close(done)
	}()
	select {
	case <-done:
		// completed without hanging
	case <-time.After(5 * time.Second):
		t.Fatal("head with pre-cancelled context did not return within 5s")
	}
}

func TestHeadNoOctalInterpretation08(t *testing.T) {
	// "08" must be interpreted as decimal 8, not rejected as an invalid octal.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", twelveLines)
	stdout, _, code := cmdRun(t, "head -n 08 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, 8, strings.Count(stdout, "\n"))
}

func TestHeadNoOctalInterpretation010(t *testing.T) {
	// "010" must be interpreted as decimal 10, not octal 8.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", twelveLines)
	stdout, _, code := cmdRun(t, "head -n 010 file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, 10, strings.Count(stdout, "\n"))
}

// --- Bad UTF-8 / binary passthrough ---

// TestHeadBadUTF8ByteMode verifies that invalid UTF-8 bytes are passed through
// unchanged in byte mode.
//
// Derived from uutils test_head.rs::test_bad_utf8
func TestHeadBadUTF8ByteMode(t *testing.T) {
	dir := t.TempDir()
	content := []byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.bin"), content, 0644))
	stdout, _, code := cmdRun(t, "head -c 6 bad.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, string(content), stdout)
}

// TestHeadBadUTF8LineMode verifies that invalid UTF-8 bytes within lines are
// passed through unchanged in line mode.
//
// Derived from uutils test_head.rs::test_bad_utf8_lines
func TestHeadBadUTF8LineMode(t *testing.T) {
	dir := t.TempDir()
	// Three lines, each containing invalid UTF-8; request first 2 lines.
	// input:    \xfc\x80\x80\x80\x80\xaf\n  b\xfc...\xaf\n  b\xfc...\xaf  (no final newline)
	// expected: first 2 lines only, bytes preserved verbatim.
	badSeq := []byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf}
	line1 := append(append([]byte(nil), badSeq...), '\n')
	line2 := append(append([]byte("b"), badSeq...), '\n')
	line3 := append([]byte("b"), badSeq...)
	input := append(append(append([]byte(nil), line1...), line2...), line3...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.bin"), input, 0644))

	expected := append(append([]byte(nil), line1...), line2...)
	stdout, _, code := cmdRun(t, "head -n 2 bad.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, string(expected), stdout)
}

// --- Multi-file edge cases ---

// TestHeadTwoEmptyFilesHeaders verifies that headers and the blank-line
// separator are still emitted when both files are empty.
//
// Derived from uutils test_head.rs::test_multiple_files
func TestHeadTwoEmptyFilesHeaders(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "")
	writeFile(t, dir, "b.txt", "")
	stdout, _, code := cmdRun(t, "head a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> a.txt <==\n\n==> b.txt <==\n", stdout)
}

// TestHeadMultipleFilesWithStdin verifies that '-' interleaved among file
// arguments reads stdin and prints a "standard input" header alongside the
// file headers.
//
// Derived from uutils test_head.rs::test_multiple_files_with_stdin
func TestHeadMultipleFilesWithStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	writeFile(t, dir, "stdin_src.txt", "hello\n")
	stdout, _, code := cmdRun(t, "head empty.txt - empty.txt < stdin_src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "==> empty.txt <==\n\n==> standard input <==\nhello\n\n==> empty.txt <==\n", stdout)
}

// TestHeadAllNonexistentFiles verifies that each nonexistent file gets its own
// error message and no headers are printed for failed opens.
//
// Derived from uutils test_head.rs::test_multiple_nonexistent_files
func TestHeadAllNonexistentFiles(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "head missing1.txt missing2.txt", dir)
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "missing1.txt")
	assert.Contains(t, stderr, "missing2.txt")
	assert.NotContains(t, stdout, "==> missing1.txt <==")
	assert.NotContains(t, stdout, "==> missing2.txt <==")
}
