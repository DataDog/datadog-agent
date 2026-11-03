// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package xargs_test

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

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, opts...)
}

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// --- Default invocation (no flags, default command = echo) ---

func TestXargsDefaultEcho(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsExplicitCommand(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs echo HEAD", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "HEAD a b c\n", stdout)
}

func TestXargsMultipleLinesJoined(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "alpha\nbeta\ngamma\n")
	stdout, _, code := cmdRun(t, "xargs < in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha beta gamma\n", stdout)
}

func TestXargsEmptyInputRunsOnce(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo -n '' | xargs echo done", dir)
	assert.Equal(t, 0, code)
	// echo prints "done" + newline.
	assert.Equal(t, "done\n", stdout)
}

func TestXargsEmptyInputWithR(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo -n '' | xargs -r echo done", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestXargsEmptyInputWithRLong(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo -n '' | xargs --no-run-if-empty echo skip", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -n / --max-args ---

func TestXargsMaxArgsOne(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs -n 1 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestXargsMaxArgsTwo(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c d e' | xargs -n 2 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc d\ne\n", stdout)
}

func TestXargsMaxArgsLongForm(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs --max-args=2 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc\n", stdout)
}

func TestXargsMaxArgsZeroRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -n 0 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsMaxArgsNegativeRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -n -1 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- -L / --max-lines ---

func TestXargsMaxLinesOne(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a b\\nc d\\n' | xargs -L 1 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc d\n", stdout)
}

func TestXargsMaxLinesTwo(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\nb\\nc\\nd\\n' | xargs -L 2 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\nc d\n", stdout)
}

func TestXargsMaxLinesIgnoresBlankLines(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\n\\n\\nb\\n' | xargs -L 1 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

// --- -0 / --null ---

func TestXargsNullSeparated(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.bin"),
		[]byte{'a', ' ', 'b', 0, 'c', 0, 'd', 0}, 0644))
	stdout, _, code := cmdRun(t, "xargs -0 < in.bin", dir)
	assert.Equal(t, 0, code)
	// Items: "a b", "c", "d" (whitespace inside null-separated runs is literal).
	assert.Equal(t, "a b c d\n", stdout)
}

func TestXargsNullPreservesQuotes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.bin"),
		[]byte{'\'', 'a', 0, '"', 'b', 0}, 0644))
	stdout, _, code := cmdRun(t, "xargs -0 < in.bin", dir)
	assert.Equal(t, 0, code)
	// In -0 mode quotes are literal — they survive into output.
	assert.Equal(t, `'a "b`+"\n", stdout)
}

// --- -d / --delimiter ---

func TestXargsCustomDelimiter(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo -n 'a,b,c' | xargs -d , echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsDelimiterEscapeNewline(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf 'a\nb\nc' | xargs -d '\n' echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsDelimiterAndNullLastWins(t *testing.T) {
	// GNU xargs treats -0 and -d as mutex with last-wins. `-0 -d ,`
	// honors -d; comma-separated tokens.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a,b' | xargs -0 -d , echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

func TestXargsMultiByteDelimiterRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -d ab echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "delimiter")
}

// --- -E / EOF marker ---

func TestXargsEofMarker(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b STOP c d' | xargs -E STOP echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

func TestXargsEofMarkerIgnoredInNullMode(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.bin"),
		[]byte("a\x00STOP\x00b\x00"), 0644))
	stdout, _, code := cmdRun(t, "xargs -0 -E STOP < in.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a STOP b\n", stdout)
}

// --- -I / --replace ---

func TestXargsReplaceDefault(t *testing.T) {
	// GNU xargs -I: tokenisation is newline-based; whole line == one item.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\nb\\nc\\n' | xargs -I {} echo 'item: {}'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "item: a\nitem: b\nitem: c\n", stdout)
}

func TestXargsReplaceWholeLineIsItem(t *testing.T) {
	// Whitespace inside the line is part of the item with -I.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "echo 'a b c' | xargs -I {} echo 'item: {}'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "item: a b c\n", stdout)
}

func TestXargsReplaceCustomString(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'foo\\nbar\\n' | xargs -I XX echo 'XX-XX'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo-foo\nbar-bar\n", stdout)
}

func TestXargsReplaceEmptyRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, `echo a | xargs -I "" echo`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- -s / --max-chars ---

func TestXargsMaxCharsForcesBatching(t *testing.T) {
	dir := t.TempDir()
	// Each "a"+space ≈ 2 chars. With -s small, force splits.
	stdout, _, code := cmdRun(t, "echo 'a b c d' | xargs -s 12 echo", dir)
	assert.Equal(t, 0, code)
	// "echo " (5) + "a b" (3) + null (1) = 9 ≤ 12 ✓; adding " c" = 11; " d" = 13 > 12.
	// Exact split is implementation-defined; assert items are preserved.
	parts := strings.Fields(strings.ReplaceAll(stdout, "\n", " "))
	assert.Equal(t, []string{"a", "b", "c", "d"}, parts)
}

func TestXargsMaxCharsZeroRejected(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -s 0 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsMaxCharsTooSmall(t *testing.T) {
	// GNU xargs always exits 1 when -s is too small to fit a single arg.
	// -x is unnecessary for this case (it only governs -n/-L batch
	// overflow, which we never hit because items are added one at a time).
	dir := t.TempDir()
	long := strings.Repeat("a", 100)
	writeFile(t, dir, "in.txt", long+"\n")
	stdout, stderr, code := cmdRun(t, "xargs -a in.txt -s 10 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
	assert.Empty(t, stdout)
}

// --- -t / --verbose ---

func TestXargsVerbose(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "echo 'a b' | xargs -t echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
	assert.Equal(t, "echo a b\n", stderr)
}

// --- -a / --arg-file ---

func TestXargsArgFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "items.txt", "x y z\n")
	stdout, _, code := cmdRun(t, "xargs -a items.txt echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "x y z\n", stdout)
}

func TestXargsArgFileMissing(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "xargs -a nonexistent.txt echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsArgFileOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	other := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(other, "items.txt"), []byte("x\n"), 0644))
	otherPath := strings.ReplaceAll(filepath.Join(other, "items.txt"), `\`, `/`)
	_, stderr, code := runScript(t, "xargs -a "+otherPath+" echo", allowed,
		interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- Quoting and escapes (default mode) ---

func TestXargsSingleQuotedArg(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `echo "'hello world' bye" | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world bye\n", stdout)
}

func TestXargsDoubleQuotedArg(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `echo "\"hello world\" bye" | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world bye\n", stdout)
}

func TestXargsBackslashEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf 'a\\ b c' | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsUnterminatedQuote(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, `echo "'oops" | xargs echo`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
	// Tokenizer errors must not fall through to a default echo invocation.
	assert.Empty(t, stdout)
}

func TestXargsTrailingBackslash(t *testing.T) {
	// GNU xargs silently consumes a trailing backslash at EOF and exits 0.
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, `printf 'a\\' | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
	assert.Empty(t, stderr)
}

// --- Sandbox: command not allowed ---

func TestXargsCommandNotAllowed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "")
	// Whitelist only xargs+echo; /bin/sh is not registered so this also
	// covers the GTFOBins shell-escape pattern.
	_, stderr, code := runScript(t, "xargs -a in.txt /bin/sh", dir,
		interp.AllowedPaths([]string{dir}),
		interp.AllowedCommands([]string{"rshell:xargs", "rshell:echo"}))
	assert.NotEqual(t, 0, code)
	assert.NotContains(t, stderr, "Bourne")
	// xargs should have rejected /bin/sh either by command-not-allowed or
	// command-not-found.
	assert.True(t,
		strings.Contains(stderr, "command not allowed") ||
			strings.Contains(stderr, "unknown command"),
		"unexpected stderr: %q", stderr)
}

// --- Help ---

func TestXargsHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "xargs --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "--max-args")
	assert.Empty(t, stderr)
}

func TestXargsHelpShortFlag(t *testing.T) {
	// docs/RULES.md requires every command to register both -h and --help.
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "xargs -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Empty(t, stderr)
}

// --- Unknown flag ---

func TestXargsUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "xargs --not-a-flag", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsRejectsInteractive(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -p echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsRejectsParallel(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs -P 4 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- Sub-command exit handling ---

func TestXargsSubCommandFailure(t *testing.T) {
	dir := t.TempDir()
	// `false` returns 1 → xargs final exit 123.
	_, _, code := cmdRun(t, "echo a | xargs false", dir)
	assert.Equal(t, 123, code)
}

// --- Cancellation ---

func TestXargsContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a b c\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "xargs < in.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

// --- Memory safety ---

func TestXargsTokenTooLong(t *testing.T) {
	dir := t.TempDir()
	// Create a file with a single token > MaxTokenBytes (1 MiB).
	huge := make([]byte, 1<<20+10)
	for i := range huge {
		huge[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "huge.txt"), huge, 0644))
	stdout, stderr, code := cmdRun(t, "xargs -a huge.txt echo", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
	// Tokenizer errors must not fall through to a default echo invocation.
	assert.Empty(t, stdout)
}

func TestXargsClampMaxArgs(t *testing.T) {
	dir := t.TempDir()
	// HardMaxArgs is 1<<20; values above must be rejected.
	_, stderr, code := cmdRun(t, "echo a | xargs -n 9999999 echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsClampMaxChars(t *testing.T) {
	dir := t.TempDir()
	// HardMaxChars is 1<<20 = 1048576. Values above are clamped, not rejected.
	stdout, _, code := cmdRun(t, "echo a | xargs -s 99999999 echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// --- Stdin pipe ---

func TestXargsStdinPipe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "hello\nworld\n")
	stdout, _, code := cmdRun(t, "cat src.txt | xargs echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

// --- No-stdin (nil stdin) ---

func TestXargsNoStdin(t *testing.T) {
	dir := t.TempDir()
	// runScript supplies nil stdin. Default echo runs once with no args.
	stdout, _, code := cmdRun(t, "xargs", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

func TestXargsNoStdinWithR(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "xargs -r", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}
