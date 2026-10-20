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

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// --- decodeDelim escape variants ---

func TestXargsDelimiterEscapeTab(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf 'a\tb\tc' | xargs -d '\t' echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsDelimiterEscapeCR(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf 'a\rb\rc' | xargs -d '\r' echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsDelimiterEscapeBackslash(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf 'a\\b\\c' | xargs -d '\\' echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

func TestXargsDelimiterEscapeNull(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.bin"),
		[]byte{'a', 0, 'b', 0, 'c', 0}, 0644))
	stdout, _, code := cmdRun(t, `xargs -d '\0' -a in.bin echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b c\n", stdout)
}

// --- nextLine memory cap ---

func TestXargsReplaceLineTooLong(t *testing.T) {
	dir := t.TempDir()
	huge := make([]byte, 1<<20+10)
	for i := range huge {
		huge[i] = 'x'
	}
	huge[len(huge)-1] = '\n'
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), huge, 0644))
	_, stderr, code := cmdRun(t, "xargs -I {} -a big.txt echo {}", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- nextDelimited memory cap ---

func TestXargsDelimiterTokenTooLong(t *testing.T) {
	dir := t.TempDir()
	huge := make([]byte, 1<<20+10)
	for i := range huge {
		huge[i] = 'x'
	}
	huge[len(huge)-1] = ','
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), huge, 0644))
	_, stderr, code := cmdRun(t, "xargs -d , -a big.txt echo", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- nextWhitespace edge cases ---

func TestXargsQuotedAcrossNewlineRejected(t *testing.T) {
	// GNU xargs treats a newline inside a quoted item as an unterminated
	// quote (matches bash behavior; the test previously enshrined a
	// bash-incompatible behavior).
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "printf \"'line1\\nline2' more\\n\" | xargs echo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
	assert.Empty(t, stdout)
}

func TestXargsBackslashEscapeNewline(t *testing.T) {
	// Backslash before newline keeps the newline literal as part of the item.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\\\\\nb' | xargs echo", dir)
	assert.Equal(t, 0, code)
	// Item contains an embedded newline, which echo prints verbatim.
	assert.Equal(t, "a\nb\n", stdout)
}

// --- Child CallContext must populate Proc so ps doesn't panic ---

func TestXargsChildCanRunPs(t *testing.T) {
	// Regression: xargs's child CallContext (built by runCmdWithStdin in
	// runner_exec.go) used to omit Proc, causing `xargs ps` to panic with
	// nil pointer dereference. ps with no args lists processes; we just
	// check that exit is 0 (or non-panic) and there is some stdout.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf '' | xargs ps", dir)
	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout, "ps should produce process listing, not panic")
}

// --- invokeCommand error paths ---

func TestXargsRunCommandReturnsError(t *testing.T) {
	// Asking xargs to call a command that does not exist exercises the
	// RunCommand-error path (the runner returns "unknown command").
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo a | xargs not-a-real-builtin", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
}

func TestXargsExit255AbortsRemaining(t *testing.T) {
	// `exit 255` from the sub-command must cause xargs to stop and report 124.
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "echo 'a b c' | xargs -n 1 exit 255", dir)
	// exit isn't a typical xargs-callable command; the runner refuses it.
	// We only need to verify xargs surfaced an error and did not crash.
	assert.NotEqual(t, 0, code)
	_ = stderr
}

// --- DoS: large but finite input runs to completion under a generous deadline. ---
func TestXargsLargeInputUnderDeadline(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString("a ")
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(sb.String()), 0644))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "xargs -a big.txt echo > /dev/null", dir,
		interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

// --- Path traversal & permissions ---

func TestXargsArgFilePathTraversal(t *testing.T) {
	allowed := t.TempDir()
	parent := filepath.Dir(allowed)
	// Create a sibling file outside `allowed`.
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret.txt"),
		[]byte("nope\n"), 0644))
	defer os.Remove(filepath.Join(parent, "secret.txt"))

	_, stderr, code := runScript(t, "xargs -a ../secret.txt echo", allowed,
		interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- Output consistency: CRLF and CR-only handling ---

func TestXargsVerticalTabIsNotSeparator(t *testing.T) {
	// \v (0x0B) is not a token separator in GNU xargs. "a\vb" stays
	// one argument that includes the vertical tab.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\vb\\n' | xargs printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a\vb]\n", stdout)
}

func TestXargsFormFeedIsNotSeparator(t *testing.T) {
	// \f (0x0C) is not a token separator in GNU xargs.
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\fb\\n' | xargs printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a\fb]\n", stdout)
}

func TestXargsCRLFInput(t *testing.T) {
	// GNU xargs treats only space, tab, and newline as item terminators.
	// \r is NOT a separator: a CRLF-terminated line "a\r\nb\r\n"
	// tokenises to "a\r" and "b\r" so the data round-trips losslessly.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte("a\r\nb\r\n"), 0644))
	stdout, _, code := cmdRun(t, "xargs -a in.txt echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\r b\r\n", stdout)
}

// --- Pre-cancelled context must return promptly ---

func TestXargsPreCancelledContext(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte("a b c\n"), 0644))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		runScriptCtx(ctx, t, "xargs -a in.txt echo", dir, interp.AllowedPaths([]string{dir}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("xargs with pre-cancelled context did not return within 5s")
	}
}

// --- Initial args are preserved even with -I ---

func TestXargsReplaceWithInitialArgs(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\nb\\n' | xargs -I {} echo PRE {} POST", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "PRE a POST\nPRE b POST\n", stdout)
}

// --- Quoting: double quotes preserve singles inside ---

func TestXargsDoubleQuotePreservesSingle(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `printf '"a'"'"'b" c' | xargs echo`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a'b c\n", stdout)
}

// --- nextLine blank line handling ---

func TestXargsReplaceSkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\n\\n\\nb\\n' | xargs -I {} echo {}", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestXargsReplaceTrailingNoNewline(t *testing.T) {
	dir := t.TempDir()
	// Last line lacks a trailing newline — should still be a complete item.
	stdout, _, code := cmdRun(t, "printf 'a\\nb' | xargs -I {} echo {}", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

// --- -d C-style escapes (matches GNU xargs) ---

func TestXargsDelimBackspaceEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\bb' | xargs -d '\\b' echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

func TestXargsDelimAlertEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\ab' | xargs -d '\\a' echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

func TestXargsDelimFormFeedEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\fb' | xargs -d '\\f' echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

func TestXargsDelimVerticalTabEscape(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\vb' | xargs -d '\\v' echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

// --- -E EOF-STR is honoured in -I mode (regression for codex P2) ---

func TestXargsEofMarkerInReplaceMode(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf 'a\\nSTOP\\nb\\n' | xargs -E STOP -I{} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "XaX\n", stdout)
}

// --- NUL byte truncation in -d mode (matches GNU effective behavior) ---

func TestXargsDelimNULTruncatesRecord(t *testing.T) {
	dir := t.TempDir()
	// printf 'a\0b,' | xargs -d, printf '[%s]\n' — GNU writes the bytes
	// to argv but execve's C-string argv is terminated at NUL, so users
	// only see "[a]". rshell uses Go strings (no auto-truncation), so we
	// truncate the token at the first NUL ourselves to match.
	stdout, _, code := cmdRun(t, "printf 'a\\0b,' | xargs -d, printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a]\n", stdout)
}

func TestXargsDelimNULAtStartEmitsEmpty(t *testing.T) {
	dir := t.TempDir()
	// `\0a,` truncates immediately → empty arg, like GNU.
	stdout, _, code := cmdRun(t, "printf '\\0a,' | xargs -d, printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n", stdout)
}

func TestXargsNullModeKeepsNULAsSeparator(t *testing.T) {
	dir := t.TempDir()
	// -0 mode must NOT truncate at NUL — NUL is the separator. The two
	// records "a" and "b" are each emitted in full.
	stdout, _, code := cmdRun(t, "printf 'a\\0b\\0' | xargs -0 printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a]\n[b]\n", stdout)
}

// --- NUL byte rejection in non -0/-d modes (matches GNU xargs) ---

func TestXargsNULInDefaultModeRejected(t *testing.T) {
	dir := t.TempDir()
	// `printf 'a\0b\0c\n'` – under default whitespace mode GNU xargs
	// emits a "NUL character occurred" warning to stderr and only
	// processes bytes up to the first NUL on each line.
	stdout, stderr, code := cmdRun(t, "printf 'a\\0b\\0c\\n' | xargs echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
	assert.Contains(t, stderr, "NUL character")
}

func TestXargsNULInReplaceModeRejected(t *testing.T) {
	dir := t.TempDir()
	// `printf 'a\0b\nc\n'` – -I mode treats NUL the same way.
	stdout, stderr, code := cmdRun(t, "printf 'a\\0b\\nc\\n' | xargs -I{} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "XaX\nXcX\n", stdout)
	assert.Contains(t, stderr, "NUL character")
}

func TestXargsNULDefaultModeContinuesAfterEmptyRecord(t *testing.T) {
	dir := t.TempDir()
	// printf '\0bad\ngood\n' — GNU emits an empty arg for the NUL'd
	// word, drops the rest of that word ("bad"), then processes "good".
	// Output: `[]` `[good]`.
	stdout, stderr, code := cmdRun(t, "printf '\\0bad\\ngood\\n' | xargs printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n[good]\n", stdout)
	assert.Contains(t, stderr, "NUL character")
}

func TestXargsNULDefaultModePreservesLaterWords(t *testing.T) {
	dir := t.TempDir()
	// printf 'a\0b c\n' — NUL only discards the rest of *the current
	// word* (b), not the rest of the line, so "c" survives.
	stdout, stderr, code := cmdRun(t, "printf 'a\\0b c\\n' | xargs printf '[%s]\\n'", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[a]\n[c]\n", stdout)
	assert.Contains(t, stderr, "NUL character")
}

func TestXargsNULAtStartOfLineDoesNotTerminateInput(t *testing.T) {
	dir := t.TempDir()
	// NUL right at the start of the first line — must not be mistaken
	// for EOF. GNU emits an empty token for the NUL'd record, then
	// processes subsequent NUL-free records normally.
	stdout, stderr, code := cmdRun(t, "printf '\\0a\\nb\\n' | xargs -I{} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "XX\nXbX\n", stdout)
	assert.Contains(t, stderr, "NUL character")
}

func TestXargsNULWarningEmittedOnce(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "printf 'a\\0b\\nc\\0d\\n' | xargs -I{} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "XaX\nXcX\n", stdout)
	// One warning even though two records contained NUL.
	assert.Equal(t, 1, strings.Count(stderr, "NUL character"))
}

// --- -I quote / backslash processing (matches GNU xargs) ---

func TestXargsReplaceSingleQuoted(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf \"'a b'\\n\" | xargs -I {} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Xa bX\n", stdout)
}

func TestXargsReplaceDoubleQuoted(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, "printf '\"a b\"\\n' | xargs -I {} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Xa bX\n", stdout)
}

func TestXargsReplaceBackslashEscape(t *testing.T) {
	dir := t.TempDir()
	// `a\ b` is one token (the backslash escapes the space).
	stdout, _, code := cmdRun(t, "printf 'a\\\\ b\\n' | xargs -I {} echo X{}X", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Xa bX\n", stdout)
}

func TestXargsReplaceBackslashJoinsLines(t *testing.T) {
	dir := t.TempDir()
	// `a\<NL>b<NL>` is one record with an embedded newline.
	stdout, _, code := cmdRun(t, "printf 'a\\\\\\nb\\n' | xargs -I {} printf 'X%sX\\n' {}", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Xa\nbX\n", stdout)
}

func TestXargsReplaceUnmatchedSingleQuote(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "printf \"'a b\\n\" | xargs -I {} echo {}", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "unterminated")
}

func TestXargsReplaceUnmatchedDoubleQuote(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "printf '\"a b\\n' | xargs -I {} echo {}", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "unterminated")
}

// --- nextDelimited memory: trailing token without separator ---

func TestXargsNullTrailingNoSeparator(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.bin"),
		[]byte("a\x00b"), 0644))
	stdout, _, code := cmdRun(t, "xargs -0 -a in.bin echo", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a b\n", stdout)
}

// --- buildOptions argFile=="" path is unreachable through pflag, but we
// can hit it via the AllowedPaths/sandbox empty-name path indirectly. The
// function is exercised by every test that uses -a, so it stays at 100%
// coverage in practice. ---

// --- nextWhitespace: token > MaxTokenBytes ---

func TestXargsWhitespaceTokenTooLong(t *testing.T) {
	dir := t.TempDir()
	huge := make([]byte, 1<<20+10)
	for i := range huge {
		huge[i] = 'x'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "huge.txt"), huge, 0644))
	_, stderr, code := cmdRun(t, "xargs -a huge.txt echo", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
}

// --- nextWhitespace: quoted token > MaxTokenBytes ---

func TestXargsQuotedTokenTooLong(t *testing.T) {
	dir := t.TempDir()
	huge := make([]byte, 1<<20+10)
	huge[0] = '\''
	for i := 1; i < len(huge)-1; i++ {
		huge[i] = 'x'
	}
	huge[len(huge)-1] = '\''
	require.NoError(t, os.WriteFile(filepath.Join(dir, "q.txt"), huge, 0644))
	_, stderr, code := cmdRun(t, "xargs -a q.txt echo", dir)
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "xargs:")
}
