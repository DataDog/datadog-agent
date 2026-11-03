// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package grep_test

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

func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func cmdRunCtx(ctx context.Context, t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
	return name
}

const sampleText = "apple\nbanana\ncherry\ndate\nelderberry\n"

// --- Basic matching ---

func TestGrepBasicMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", sampleText)
	stdout, _, code := cmdRun(t, "grep banana file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "banana\n", stdout)
}

func TestGrepNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", sampleText)
	stdout, _, code := cmdRun(t, "grep fig file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

func TestGrepMultipleMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nfoo baz\nqux\n")
	stdout, _, code := cmdRun(t, "grep foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nfoo baz\n", stdout)
}

func TestGrepStdinPipe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", sampleText)
	stdout, _, code := cmdRun(t, "cat file.txt | grep cherry", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cherry\n", stdout)
}

func TestGrepStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", sampleText)
	stdout, _, code := cmdRun(t, "grep apple - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\n", stdout)
}

func TestGrepStdinImplicit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", sampleText)
	stdout, _, code := cmdRun(t, "grep banana < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "banana\n", stdout)
}

func TestGrepEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "grep anything empty.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// --- Regex matching ---

func TestGrepBREDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "abc\nabc123\n123\n")
	stdout, _, code := cmdRun(t, "grep 'abc.*123' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc123\n", stdout)
}

func TestGrepBREGrouping(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "abcabc\nabc\nab\n")
	// BRE: \(abc\) is a group, no repetition — matches lines containing "abc"
	stdout, _, code := cmdRun(t, `grep '\(abc\)' file.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcabc\nabc\n", stdout)
}

func TestGrepERE(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "cat\nbat\nhat\ndog\n")
	stdout, _, code := cmdRun(t, "grep -E '(c|b)at' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cat\nbat\n", stdout)
}

func TestGrepEREAlternation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nbaz\n")
	stdout, _, code := cmdRun(t, "grep -E 'foo|baz' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nbaz\n", stdout)
}

func TestGrepFixedStrings(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "a.b\na*b\naxb\n")
	stdout, _, code := cmdRun(t, "grep -F 'a.b' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.b\n", stdout)
}

func TestGrepFixedStringsRegexChars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo(bar)\nfoo bar\n")
	stdout, _, code := cmdRun(t, "grep -F 'foo(bar)' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo(bar)\n", stdout)
}

// --- Case insensitive ---

func TestGrepIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "Hello\nhello\nHELLO\nworld\n")
	stdout, _, code := cmdRun(t, "grep -i hello file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Hello\nhello\nHELLO\n", stdout)
}

// --- Invert match ---

func TestGrepInvertMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nbaz\n")
	stdout, _, code := cmdRun(t, "grep -v foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bar\nbaz\n", stdout)
}

func TestGrepInvertMatchNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nfoo\n")
	stdout, _, code := cmdRun(t, "grep -v foo file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// --- Count ---

func TestGrepCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nfoo baz\nqux\n")
	stdout, _, code := cmdRun(t, "grep -c foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\n", stdout)
}

func TestGrepCountNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -c xyz file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
}

func TestGrepCountMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\nbar\n")
	writeFile(t, dir, "b.txt", "foo\nfoo\n")
	stdout, _, code := cmdRun(t, "grep -c foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:1\nb.txt:2\n", stdout)
}

// --- Files with/without matches ---

func TestGrepFilesWithMatches(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "bar\n")
	writeFile(t, dir, "c.txt", "foo bar\n")
	stdout, _, code := cmdRun(t, "grep -l foo a.txt b.txt c.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt\nc.txt\n", stdout)
}

func TestGrepFilesWithoutMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "bar\n")
	writeFile(t, dir, "c.txt", "foo bar\n")
	stdout, _, code := cmdRun(t, "grep -L foo a.txt b.txt c.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b.txt\n", stdout)
}

// --- Line number ---

func TestGrepLineNumber(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nbbb\n")
	stdout, _, code := cmdRun(t, "grep -n bbb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2:bbb\n4:bbb\n", stdout)
}

// --- Filename control ---

func TestGrepWithFilenameMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:foo\nb.txt:foo\n", stdout)
}

func TestGrepNoFilenameSingleFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep foo a.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\n", stdout)
}

func TestGrepForceFilenameH(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep -H foo a.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:foo\n", stdout)
}

func TestGrepSuppressFilenameh(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep -h foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nfoo\n", stdout)
}

// --- Only matching ---

func TestGrepOnlyMatching(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foobar\nbazfoo\n")
	stdout, _, code := cmdRun(t, "grep -o foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nfoo\n", stdout)
}

func TestGrepOnlyMatchingMultiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "abcabc\n")
	stdout, _, code := cmdRun(t, "grep -o abc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\nabc\n", stdout)
}

// --- Quiet mode ---

func TestGrepQuiet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -q foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestGrepQuietNoMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -q xyz file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// --- No messages ---

func TestGrepNoMessages(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep -s foo nonexistent.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stderr)
}

// --- Word regexp ---

func TestGrepWordRegexp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nfoobar\nbar foo baz\n")
	stdout, _, code := cmdRun(t, "grep -w foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nbar foo baz\n", stdout)
}

// --- Line regexp ---

func TestGrepLineRegexp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nfoo bar\nbar foo\n")
	stdout, _, code := cmdRun(t, "grep -x foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\n", stdout)
}

// --- Multiple patterns with -e ---

func TestGrepMultiplePatterns(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nbaz\n")
	stdout, _, code := cmdRun(t, "grep -e foo -e baz file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nbaz\n", stdout)
}

// --- Max count ---

func TestGrepMaxCount(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nfoo\nfoo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -m 2 foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nfoo\n", stdout)
}

func TestGrepMaxCountZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -m 0 foo file.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// --- Context lines ---

func TestGrepAfterContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -A 1 bbb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbb\nccc\n", stdout)
}

func TestGrepBeforeContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -B 1 ccc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbb\nccc\n", stdout)
}

func TestGrepContextBoth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -C 1 ccc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbb\nccc\nddd\n", stdout)
}

func TestGrepContextGroupSeparator(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\nfff\nggg\n")
	stdout, _, code := cmdRun(t, "grep -C 0 -e bbb -e fff file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbb\n--\nfff\n", stdout)
}

func TestGrepContextOverlapping(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -C 1 -e bbb -e ddd file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "aaa\nbbb\nccc\nddd\neee\n", stdout)
}

func TestGrepAfterContextWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -n -A 1 bbb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2:bbb\n3-ccc\n", stdout)
}

func TestGrepBeforeContextWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -n -B 1 ccc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2-bbb\n3:ccc\n", stdout)
}

func TestGrepContextWithFilenames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\n")
	stdout, _, code := cmdRun(t, "grep -H -A 1 aaa file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "file.txt:aaa\nfile.txt-bbb\n", stdout)
}

// --- Combined flags ---

func TestGrepLineNumberWithFilename(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\nbar\n")
	writeFile(t, dir, "b.txt", "baz\nfoo\n")
	stdout, _, code := cmdRun(t, "grep -n foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:1:foo\nb.txt:2:foo\n", stdout)
}

func TestGrepCountInvert(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nbaz\n")
	stdout, _, code := cmdRun(t, "grep -vc foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\n", stdout)
}

// --- Error cases ---

func TestGrepNoPattern(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepInvalidRegex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\n")
	_, stderr, code := cmdRun(t, "grep -E '[invalid' file.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep foo nonexistent.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep --recursive foo", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "grep:")
}

// --- Exit code semantics ---

func TestGrepExitCodeMatchMultiFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "bar\n")
	_, _, code := cmdRun(t, "grep foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
}

func TestGrepExitCodeNoMatchMultiFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "bar\n")
	writeFile(t, dir, "b.txt", "baz\n")
	_, _, code := cmdRun(t, "grep foo a.txt b.txt", dir)
	assert.Equal(t, 1, code)
}

// --- Context cancellation ---

func TestGrepContextCancellation(t *testing.T) {
	dir := t.TempDir()
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		sb.WriteString("line\n")
	}
	writeFile(t, dir, "big.txt", sb.String())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		cmdRunCtx(ctx, t, "grep line big.txt", dir)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("grep did not complete within timeout")
	}
}

// --- Multiple files with some errors ---

func TestGrepMultipleFilesSomeErrors(t *testing.T) {
	// GNU grep returns 2 when errors occur, even if matches were found.
	dir := t.TempDir()
	writeFile(t, dir, "good.txt", "foo\n")
	stdout, stderr, code := cmdRun(t, "grep foo good.txt nonexistent.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stdout, "foo")
	assert.Contains(t, stderr, "grep:")
}

func TestGrepQuietMultipleFilesSomeErrors(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "good.txt", "foo\n")
	stdout, stderr, code := cmdRun(t, "grep -q foo good.txt nonexistent.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Pipe chain ---

func TestGrepPipeChain(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "apple pie\nbanana split\ncherry pie\n")
	stdout, _, code := cmdRun(t, "cat file.txt | grep pie | grep -v cherry", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple pie\n", stdout)
}

// --- Double dash ---

func TestGrepDoubleDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "-v\nfoo\n")
	stdout, _, code := cmdRun(t, "grep -- -v file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "-v\n", stdout)
}

// --- Empty pattern ---

func TestGrepEmptyPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, `grep '' file.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nbar\n", stdout)
}

// --- -o -v combination (GNU compat) ---

func TestGrepOnlyMatchingInvert(t *testing.T) {
	// GNU grep: -o -v produces no output but exits 0.
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -o -v foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -o suppresses empty matches ---

func TestGrepOnlyMatchingSuppressEmpty(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\n")
	stdout, _, code := cmdRun(t, "grep -o -E 'o*' file.txt", dir)
	assert.Equal(t, 0, code)
	// Only non-empty matches should be printed
	assert.Equal(t, "oo\n", stdout)
}

// --- Conflicting matchers ---

func TestGrepConflictingMatchersEG(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep -E -G foo", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep: conflicting matchers specified")
}

func TestGrepConflictingMatchersFE(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "grep -F -E foo", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep: conflicting matchers specified")
}

func TestGrepSingleMatcherGNotConflict(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep -G foo file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\n", stdout)
}

// --- -l/-L precedence and -c interaction ---

func TestGrepFilesWithAndWithoutMatchLastFlagWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "bar\n")

	stdout, _, code := cmdRun(t, "grep -l -L foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b.txt\n", stdout)

	stdout, _, code = cmdRun(t, "grep -L -l foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt\n", stdout)
}

func TestGrepCountSuppressedByFileListModes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\nbar\n")
	writeFile(t, dir, "b.txt", "bar\n")

	stdout, _, code := cmdRun(t, "grep -c -l foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt\n", stdout)

	stdout, _, code = cmdRun(t, "grep -c -L foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b.txt\n", stdout)
}

// --- -h/-H last-option precedence ---

func TestGrepFilenameHhLastFlagWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "foo\n")
	writeFile(t, dir, "b.txt", "foo\n")

	// -h -H: last flag is -H, so show filenames
	stdout, _, code := cmdRun(t, "grep -h -H foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:foo\nb.txt:foo\n", stdout)

	// -H -h: last flag is -h, so suppress filenames
	stdout, _, code = cmdRun(t, "grep -H -h foo a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nfoo\n", stdout)
}

// --- -o suppresses context ---

func TestGrepOnlyMatchingSuppressesContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	// -o -A1: GNU grep outputs only matched parts, no context
	stdout, _, code := cmdRun(t, "grep -o -A 1 bbb file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbb\n", stdout)
}

func TestGrepOnlyMatchingSuppressesBeforeContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "aaa\nbbb\nccc\nddd\neee\n")
	stdout, _, code := cmdRun(t, "grep -o -B 1 ccc file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ccc\n", stdout)
}

// --- Newline-delimited patterns ---

func TestGrepNewlineDelimitedPattern(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\nbar\nbaz\n")
	// Pattern with embedded newline should match both
	stdout, _, code := cmdRun(t, "grep -e $'foo\\nbar' file.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\nbar\n", stdout)
}

// --- Stdin display name ---

func TestGrepStdinDisplayName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "foo\n")
	writeFile(t, dir, "src.txt", "foo\n")
	stdout, _, code := cmdRun(t, "grep foo file.txt - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "file.txt:foo\n(standard input):foo\n", stdout)
}
