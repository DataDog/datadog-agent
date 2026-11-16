// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cat_test

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

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	return name
}

const fiveLines = "alpha\nbeta\ngamma\ndelta\nepsilon\n"

// --- Basic (no flags) ---

func TestCatSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "cat file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

func TestCatMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "hello\n")
	writeFile(t, dir, "b.txt", "world\n")
	stdout, _, code := cmdRun(t, "cat a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld\n", stdout)
}

func TestCatEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "cat empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestCatNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "no newline")
	stdout, _, code := cmdRun(t, "cat file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "no newline", stdout)
}

func TestCatStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "hello\n")
	stdout, _, code := cmdRun(t, "cat - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestCatStdinImplicit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", fiveLines)
	stdout, _, code := cmdRun(t, "cat < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, fiveLines, stdout)
}

func TestCatNilStdin(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runScript(t, "cat", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- -n / --number ---

func TestCatNumberLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "alpha\nbeta\ngamma\n")
	stdout, _, code := cmdRun(t, "cat -n file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\talpha\n     2\tbeta\n     3\tgamma\n", stdout)
}

func TestCatNumberLinesLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "alpha\nbeta\n")
	stdout, _, code := cmdRun(t, "cat --number file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\talpha\n     2\tbeta\n", stdout)
}

func TestCatNumberBlankLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -n file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n     2\t\n     3\tb\n", stdout)
}

func TestCatNumberNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "line1\nline2")
	stdout, _, code := cmdRun(t, "cat -n file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\tline1\n     2\tline2", stdout)
}

func TestCatNumberAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "one\n")
	writeFile(t, dir, "b.txt", "two\nthree\n")
	stdout, _, code := cmdRun(t, "cat -n a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\tone\n     2\ttwo\n     3\tthree\n", stdout)
}

func TestCatNumberEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "cat -n empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -b / --number-nonblank ---

func TestCatNumberNonblank(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "alpha\n\nbeta\n\ngamma\n")
	stdout, _, code := cmdRun(t, "cat -b file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\talpha\n\n     2\tbeta\n\n     3\tgamma\n", stdout)
}

func TestCatNumberNonblankOverridesNumber(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -n -b file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

func TestCatNumberNonblankOverridesNumberReversed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -b -n file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

// --- -s / --squeeze-blank ---

func TestCatSqueezeBlank(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\n\n\nb\n\n\nc\n")
	stdout, _, code := cmdRun(t, "cat -s file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n\nb\n\nc\n", stdout)
}

func TestCatSqueezeBlankWithNumber(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -sn file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n     2\t\n     3\tb\n", stdout)
}

func TestCatSqueezeBlankWithNumberNonblank(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -sb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

func TestCatSqueezeAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "x\n\n")
	writeFile(t, dir, "b.txt", "\ny\n")
	stdout, _, code := cmdRun(t, "cat -s a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "x\n\ny\n", stdout)
}

// --- -E / --show-ends ---

func TestCatShowEnds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "alpha\nbeta\n")
	stdout, _, code := cmdRun(t, "cat -E file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha$\nbeta$\n", stdout)
}

func TestCatShowEndsNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello")
	stdout, _, code := cmdRun(t, "cat -E file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

func TestCatShowEndsCRLF(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\r\nb\n")
	stdout, _, code := cmdRun(t, "cat -E file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a^M$\nb$\n", stdout)
}

func TestCatShowEndsEmptyLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -E file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a$\n$\nb$\n", stdout)
}

// --- -T / --show-tabs ---

func TestCatShowTabs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\tb\n")
	stdout, _, code := cmdRun(t, "cat -T file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a^Ib\n", stdout)
}

func TestCatShowTabsNoNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "\thello")
	stdout, _, code := cmdRun(t, "cat -T file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^Ihello", stdout)
}

// --- -v / --show-nonprinting ---

func TestCatShowNonprinting(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x00, 0x01, 0x1f, '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -v file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^A^_\n", stdout)
}

func TestCatShowNonprintingDEL(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x7f, '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -v file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^?\n", stdout)
}

func TestCatShowNonprintingHighBytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x80, 0x9f, 0xa0, 0xfe, 0xff, '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -v file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "M-^@M-^_M- M-~M-^?\n", stdout)
}

func TestCatShowNonprintingPreservesTab(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\tb\n")
	stdout, _, code := cmdRun(t, "cat -v file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\tb\n", stdout)
}

// --- -A / --show-all ---

func TestCatShowAll(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x00, '\t', 'a', '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -A file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^Ia$\n", stdout)
}

// --- -e (equivalent to -vE) ---

func TestCatFlagE(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x00, 'a', '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -e file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@a$\n", stdout)
}

func TestCatFlagEPreservesTab(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "\ta\n")
	stdout, _, code := cmdRun(t, "cat -e file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\ta$\n", stdout)
}

// --- -t (equivalent to -vT) ---

func TestCatFlagT(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.bin"), []byte{0x00, '\t', '\n'}, 0644))
	stdout, _, code := cmdRun(t, "cat -t file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "^@^I\n", stdout)
}

// --- -u (ignored) ---

func TestCatUIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "hello\n")
	stdout, _, code := cmdRun(t, "cat -u file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// --- Combined flags ---

func TestCatCombinedSNB(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\n\n\n\nb\n")
	stdout, _, code := cmdRun(t, "cat -snb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta\n\n     2\tb\n", stdout)
}

func TestCatNumberedShowEndsShowTabs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a\tb\n")
	stdout, _, code := cmdRun(t, "cat -nET file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\ta^Ib$\n", stdout)
}

// --- Help ---

func TestCatHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "cat --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "--number")
	assert.Empty(t, stderr)
}

func TestCatHelpShortH(t *testing.T) {
	// GNU cat does not have -h; it should be treated as an unknown option.
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cat -h", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

// --- Error cases ---

func TestCatMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cat nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

func TestCatDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))
	_, stderr, code := cmdRun(t, "cat subdir", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

func TestCatUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cat --follow file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

func TestCatUnknownShortFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "cat -f file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

func TestCatMultipleFilesSomeFailSomeSucceed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.txt", "hello\n")
	stdout, stderr, code := cmdRun(t, "cat good.txt nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "hello")
	assert.Contains(t, stderr, "cat:")
}

func TestCatOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret"), 0644))

	secretPath := strings.ReplaceAll(filepath.Join(secret, "secret.txt"), `\`, `/`)
	_, stderr, code := runScript(t, "cat "+secretPath, allowed, interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

// --- RULES.md compliance ---

func TestCatDoubleDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "-n", "flag-looking-name\n")
	stdout, _, code := cmdRun(t, "cat -- -n", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "flag-looking-name\n", stdout)
}

func TestCatNullBytesPassthrough(t *testing.T) {
	dir := t.TempDir()
	content := "a\x00b\x00c\n"
	writeFile(t, dir, "file.bin", content)
	stdout, _, code := cmdRun(t, "cat file.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, content, stdout)
}

func TestCatCRLFPreserved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "line1\r\nline2\r\n")
	stdout, _, code := cmdRun(t, "cat file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\r\nline2\r\n", stdout)
}

func TestCatBadUTF8Passthrough(t *testing.T) {
	dir := t.TempDir()
	content := []byte{0xfc, 0x80, 0x80, 0x80, 0x80, 0xaf, '\n'}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "bad.bin"), content, 0644))
	stdout, _, code := cmdRun(t, "cat bad.bin", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, string(content), stdout)
}

func TestCatLineModeLineBeyondCap(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 1<<20+1)
	for i := range content {
		content[i] = 'a'
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "huge.txt"), content, 0644))
	_, stderr, code := cmdRun(t, "cat -n huge.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cat:")
}

func TestCatLineModeLineBelowCap(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, 1<<20-1)
	for i := range content {
		content[i] = 'b'
	}
	content = append(content, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.txt"), content, 0644))
	stdout, _, code := cmdRun(t, "cat -n large.txt", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "     1\t")
}

func TestCatContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "cat file.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

func TestCatContextPreCancelled(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() {
		runScriptCtx(ctx, t, "cat file.txt", dir, interp.AllowedPaths([]string{dir}))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cat with pre-cancelled context did not return within 5s")
	}
}

func TestCatPipeInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", fiveLines)
	stdout, _, code := cmdRun(t, "cat file.txt | cat -n", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "     1\talpha\n     2\tbeta\n     3\tgamma\n     4\tdelta\n     5\tepsilon\n", stdout)
}
