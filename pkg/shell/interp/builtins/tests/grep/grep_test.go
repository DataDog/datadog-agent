// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package grep_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/shell/interp"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{
		interp.StdIO(nil, &outBuf, &errBuf),
	}, opts...)

	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(ctx, prog)
	exitCode = 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func setupGrepDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	}
	return dir
}

func grepRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// ── Basic matching ────────────────────────────────────────────────────────────

func TestGrepBasicMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	stdout, stderr, code := grepRun(t, "grep beta f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "beta\n", stdout)
	assert.Equal(t, "", stderr)
}

func TestGrepNoMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	stdout, stderr, code := grepRun(t, "grep delta f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestGrepMultipleMatchLines(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "foo bar\nbaz\nfoo qux\n"})
	stdout, _, code := grepRun(t, "grep foo f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo bar\nfoo qux\n", stdout)
}

func TestGrepEmptyFile(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"empty.txt": ""})
	stdout, stderr, code := grepRun(t, "grep pattern empty.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestGrepNoTrailingNewline(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta"})
	stdout, _, code := grepRun(t, "grep beta f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "beta\n", stdout)
}

func TestGrepRegexMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "abc\ndef\n123\n"})
	stdout, _, code := grepRun(t, `grep '[0-9]+' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "123\n", stdout)
}

func TestGrepMultipleFiles(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match here\nno\n",
		"b.txt": "no\nmatch too\n",
	})
	stdout, _, code := grepRun(t, "grep match a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:match here\nb.txt:match too\n", stdout)
}

// ── Flag: -i / --ignore-case ──────────────────────────────────────────────────

func TestGrepIgnoreCase(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "Hello World\nhello world\nHELLO WORLD\n"})
	stdout, _, code := grepRun(t, "grep -i hello f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Hello World\nhello world\nHELLO WORLD\n", stdout)
}

func TestGrepIgnoreCaseLong(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "ALPHA\nbeta\n"})
	stdout, _, code := grepRun(t, "grep --ignore-case alpha f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ALPHA\n", stdout)
}

// ── Flag: -v / --invert-match ─────────────────────────────────────────────────

func TestGrepInvertMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	stdout, _, code := grepRun(t, "grep -v beta f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "alpha\ngamma\n", stdout)
}

func TestGrepInvertMatchNoLines(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "foo\nfoo\n"})
	_, _, code := grepRun(t, "grep -v foo f.txt", dir)
	assert.Equal(t, 1, code)
}

// ── Flag: -n / --line-number ──────────────────────────────────────────────────

func TestGrepLineNumber(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "one\ntwo\nthree\n"})
	stdout, _, code := grepRun(t, "grep -n two f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2:two\n", stdout)
}

func TestGrepLineNumberMultipleMatches(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "a\nb\na\n"})
	stdout, _, code := grepRun(t, "grep -n a f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1:a\n3:a\n", stdout)
}

// ── Flag: -c / --count ────────────────────────────────────────────────────────

func TestGrepCount(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nno\nmatch\nmatch\nno\n"})
	stdout, _, code := grepRun(t, "grep -c match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3\n", stdout)
}

func TestGrepCountNoMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\n"})
	stdout, _, code := grepRun(t, "grep -c delta f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
}

func TestGrepCountMultipleFiles(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match\nno\nmatch\n",
		"b.txt": "no\nmatch\n",
	})
	stdout, _, code := grepRun(t, "grep -c match a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt:2\nb.txt:1\n", stdout)
}

// ── Flag: -l / -L ─────────────────────────────────────────────────────────────

func TestGrepFilesWithMatches(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match\nno\n",
		"b.txt": "no\nno\n",
		"c.txt": "also match\n",
	})
	stdout, _, code := grepRun(t, "grep -l match a.txt b.txt c.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt\nc.txt\n", stdout)
}

func TestGrepFilesWithoutMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match\nno\n",
		"b.txt": "no\nno\n",
		"c.txt": "also match\n",
	})
	stdout, _, code := grepRun(t, "grep -L match a.txt b.txt c.txt", dir)
	// b.txt was printed (no match), so exit 0.
	assert.Equal(t, 0, code)
	assert.Equal(t, "b.txt\n", stdout)
}

func TestGrepFilesWithoutMatchNoneFound(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match\n",
		"b.txt": "match\n",
	})
	stdout, _, code := grepRun(t, "grep -L match a.txt b.txt", dir)
	// All files had matches, nothing printed → exit 1.
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

func TestGrepFilesWithoutMatchAllNoMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "nomatch\n",
		"b.txt": "other\n",
	})
	stdout, _, code := grepRun(t, "grep -L something a.txt b.txt", dir)
	// All files had no match, all are printed → exit 0.
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "a.txt")
	assert.Contains(t, stdout, "b.txt")
}

// ── Flag: -q / --quiet ────────────────────────────────────────────────────────

func TestGrepQuietMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	stdout, stderr, code := grepRun(t, "grep -q beta f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

func TestGrepQuietNoMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	_, _, code := grepRun(t, "grep -q delta f.txt", dir)
	assert.Equal(t, 1, code)
}

func TestGrepSilentAlias(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	stdout, _, code := grepRun(t, "grep --silent hello f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// ── Flag: -s / --no-messages ──────────────────────────────────────────────────

func TestGrepNoMessages(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := grepRun(t, "grep -s pattern nonexistent.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// ── Flag: -w / --word-regexp ──────────────────────────────────────────────────

func TestGrepWordRegexp(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "cat\ncatfish\nthe cat sat\n"})
	stdout, _, code := grepRun(t, "grep -w cat f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cat\nthe cat sat\n", stdout)
}

// ── Flag: -x / --line-regexp ──────────────────────────────────────────────────

func TestGrepLineRegexp(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "foo\nfoobar\nfoo bar\n"})
	stdout, _, code := grepRun(t, "grep -x foo f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo\n", stdout)
}

// ── Flag: -F / --fixed-strings ────────────────────────────────────────────────

func TestGrepFixedStrings(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "a.b\naXb\na[b\n"})
	stdout, _, code := grepRun(t, "grep -F 'a.b' f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.b\n", stdout)
}

func TestGrepFixedStringsSpecialChars(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "foo+bar\nfoo bar\n"})
	stdout, _, code := grepRun(t, `grep -F 'foo+bar' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "foo+bar\n", stdout)
}

// ── Flag: -E / --extended-regexp ─────────────────────────────────────────────

func TestGrepExtendedRegexp(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "abc\ndef\n"})
	stdout, _, code := grepRun(t, `grep -E '[a-z]+' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\ndef\n", stdout)
}

// ── Flag: -H / --with-filename ────────────────────────────────────────────────

func TestGrepWithFilename(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nno\n"})
	stdout, _, code := grepRun(t, "grep -H match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "f.txt:match\n", stdout)
}

// ── Flag: --no-filename ───────────────────────────────────────────────────────

func TestGrepNoFilename(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"a.txt": "match\n",
		"b.txt": "match too\n",
	})
	stdout, _, code := grepRun(t, "grep --no-filename match a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\nmatch too\n", stdout)
}

// ── Flag: -m / --max-count ────────────────────────────────────────────────────

func TestGrepMaxCount(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match1\nmatch2\nmatch3\nmatch4\n"})
	stdout, _, code := grepRun(t, "grep -m 2 match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match1\nmatch2\n", stdout)
}

func TestGrepMaxCountOne(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "a\na\na\n"})
	stdout, _, code := grepRun(t, "grep -m 1 a f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// ── Flag: -o / --only-matching ────────────────────────────────────────────────

func TestGrepOnlyMatching(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "foo123bar\nno digits here\n456end\n"})
	stdout, _, code := grepRun(t, `grep -o '[0-9]+' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "123\n456\n", stdout)
}

func TestGrepOnlyMatchingMultiplePerLine(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "a1b2c3\n"})
	stdout, _, code := grepRun(t, `grep -o '[0-9]' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n2\n3\n", stdout)
}

// ── Flag: -e / multiple patterns ─────────────────────────────────────────────

func TestGrepMultiplePatterns(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "apple\nbanana\ncherry\n"})
	stdout, _, code := grepRun(t, "grep -e apple -e cherry f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\ncherry\n", stdout)
}

func TestGrepEFlagAllowsPatternStartingWithDash(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "-v\nhello\n"})
	stdout, _, code := grepRun(t, "grep -e '-v' f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "-v\n", stdout)
}

// ── Context flags: -A / -B / -C ───────────────────────────────────────────────

func TestGrepAfterContext(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "needle\nline2\nline3\nother\n"})
	stdout, _, code := grepRun(t, "grep -A 2 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "needle\nline2\nline3\n", stdout)
}

func TestGrepBeforeContext(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "line1\nline2\nneedle\nline4\n"})
	stdout, _, code := grepRun(t, "grep -B 2 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline2\nneedle\n", stdout)
}

func TestGrepContextZero(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{
		"f.txt": "needle\nctx1\nctx2\nctx3\nanother needle\nctx5\nctx6\n",
	})
	stdout, _, code := grepRun(t, "grep -C 0 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "needle\n--\nanother needle\n", stdout)
}

func TestGrepContextNoSeparatorWhenOverlapping(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "line1\nneedle\nline3\nneedle\nline5\n"})
	stdout, _, code := grepRun(t, "grep -C 2 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nneedle\nline3\nneedle\nline5\n", stdout)
}

func TestGrepContextLineFmt(t *testing.T) {
	// Context lines use '-' separator, match lines use ':' separator.
	dir := setupGrepDir(t, map[string]string{"f.txt": "ctx\nneedle\n"})
	stdout, _, code := grepRun(t, "grep -B 1 -H needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "f.txt-ctx\nf.txt:needle\n", stdout)
}

// ── Context lines: -m with -A ─────────────────────────────────────────────────

func TestGrepMaxCountWithAfterContext(t *testing.T) {
	// Derived from GNU grep max-count-vs-context: -m1 -A5 prints the first match + context.
	dir := setupGrepDir(t, map[string]string{
		"f.txt": "needle\nctx1\nctx2\nctx3\nanother needle\nctx5\nctx6\n",
	})
	stdout, _, code := grepRun(t, "grep -m 1 -A 5 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "needle\nctx1\nctx2\nctx3\nanother needle\nctx5\n", stdout)
}

// ── Stdin ─────────────────────────────────────────────────────────────────────

func TestGrepStdinNoFile(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "alpha\nbeta\ngamma\n"})
	stdout, _, code := grepRun(t, "grep beta < f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "beta\n", stdout)
}

func TestGrepStdinDash(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\nworld\n"})
	stdout, _, code := grepRun(t, "grep hello - < f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// ── Error handling ────────────────────────────────────────────────────────────

func TestGrepMissingFile(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := grepRun(t, "grep pattern nonexistent.txt", dir)
	assert.Equal(t, 2, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepNoPattern(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepInvalidRegex(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	_, stderr, code := grepRun(t, `grep '[invalid' f.txt`, dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepUnknownFlag(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	_, stderr, code := grepRun(t, "grep --no-such-option pattern f.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepRecursiveRejected(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	_, stderr, code := grepRun(t, "grep -r pattern f.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepSandboxAccessDenied(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep pattern /etc/hosts", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

// ── Combination flags ─────────────────────────────────────────────────────────

func TestGrepIgnoreCaseAndWord(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "Cat\ncatfish\nTHE CAT\n"})
	stdout, _, code := grepRun(t, "grep -iw cat f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Cat\nTHE CAT\n", stdout)
}

func TestGrepLineNumberWithFilename(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "line1\nline2\nline3\n"})
	stdout, _, code := grepRun(t, "grep -Hn line2 f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "f.txt:2:line2\n", stdout)
}

func TestGrepCountWithIgnoreCase(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "Match\nMATCH\nmatch\nno\n"})
	stdout, _, code := grepRun(t, "grep -ci match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3\n", stdout)
}

// ── RULES.md: resource limits ─────────────────────────────────────────────────

func TestGrepContextLargeClamped(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nline2\n"})
	// 99999999 > grepMaxContextLines (10000): must be clamped, not rejected.
	stdout, _, code := grepRun(t, "grep -A 99999999 match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\nline2\n", stdout)
}

func TestGrepMaxCountLargeValue(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "a\nb\nc\n"})
	stdout, _, code := grepRun(t, "grep -m 99999999999999999999 a f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

// ── Context cancellation ──────────────────────────────────────────────────────

func TestGrepContextCancellation(t *testing.T) {
	// grep must stop when context is cancelled.
	dir := setupGrepDir(t, map[string]string{"large.txt": strings.Repeat("nomatch\n", 100000)})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _, _ = runScriptCtx(ctx, t, "grep needle large.txt", dir, interp.AllowedPaths([]string{dir}))
	// Test passes as long as it doesn't hang.
}

// ── Help flag ─────────────────────────────────────────────────────────────────

func TestGrepHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := grepRun(t, "grep --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "grep")
}

func TestGrepHelpShort(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := grepRun(t, "grep -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}

// ── Single file: no filename prefix by default ────────────────────────────────

func TestGrepSingleFileNoPrefix(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "hello\n"})
	stdout, _, code := grepRun(t, "grep hello f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// ── Invert with count ─────────────────────────────────────────────────────────

func TestGrepInvertCount(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nno\nmatch\n"})
	stdout, _, code := grepRun(t, "grep -vc match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n", stdout)
}

// ── -o / --only-matching with line number and filename ────────────────────────

func TestGrepOnlyMatchingWithLineNumber(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "abc123\ndef456\n"})
	stdout, _, code := grepRun(t, `grep -on '[0-9]+' f.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1:123\n2:456\n", stdout)
}

// ── -m 0: should match nothing ────────────────────────────────────────────────

func TestGrepMaxCountZero(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nmatch\n"})
	stdout, _, code := grepRun(t, "grep -m 0 match f.txt", dir)
	// -m 0 means stop after 0 matches: no output, exit 1
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// ── Error: both missing and existing files ────────────────────────────────────

func TestGrepMixedMissingAndExistingFiles(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"good.txt": "match\n"})
	stdout, stderr, code := grepRun(t, "grep match good.txt nonexistent.txt", dir)
	// One file matched but there's also an error → exit 2
	assert.Equal(t, 2, code)
	assert.Contains(t, stdout, "match")
	assert.Contains(t, stderr, "grep:")
}

// ── Invalid context / count arguments ────────────────────────────────────────

func TestGrepInvalidAfterArg(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep -A notanumber pattern", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepInvalidBeforeArg(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep -B notanumber pattern", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepInvalidContextArg(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep -C notanumber pattern", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepInvalidMaxCountArg(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep -m notanumber pattern", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

// ── CRLF line endings ─────────────────────────────────────────────────────────

func TestGrepCRLF(t *testing.T) {
	// Files with CRLF line endings should be handled correctly.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crlf.txt"), []byte("match\r\nno\r\n"), 0644))
	stdout, _, code := grepRun(t, "grep match crlf.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\n", stdout)
}

func TestGrepLoneCR(t *testing.T) {
	// Files with lone CR (old Mac OS 9) line endings are handled like LF.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "lone_cr.txt"), []byte("match\rno\r"), 0644))
	stdout, _, code := grepRun(t, "grep match lone_cr.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\n", stdout)
}

// ── Line length cap ───────────────────────────────────────────────────────────

func TestGrepLongLineCapped(t *testing.T) {
	// A line longer than grepMaxLineBytes (256 KiB) is silently truncated.
	// The match must still work on the first grepMaxLineBytes bytes.
	dir := t.TempDir()
	// 257 KiB line, pattern at start
	longLine := "MATCH" + strings.Repeat("x", 257*1024) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "long.txt"), []byte(longLine), 0644))
	stdout, _, code := grepRun(t, "grep MATCH long.txt", dir)
	assert.Equal(t, 0, code)
	// Output line is capped; it starts with MATCH
	assert.True(t, strings.HasPrefix(stdout, "MATCH"))
}

// ── -q combined with -c ───────────────────────────────────────────────────────

func TestGrepQuietCount(t *testing.T) {
	// -q and -c together: count is computed but not printed; exit 0 if any match.
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\nmatch\n"})
	stdout, _, code := grepRun(t, "grep -qc match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestGrepQuietCountNoMatch(t *testing.T) {
	dir := setupGrepDir(t, map[string]string{"f.txt": "no\nno\n"})
	stdout, _, code := grepRun(t, "grep -qc match f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
}

// ── Context clamping ──────────────────────────────────────────────────────────

func TestGrepContextLargeValueClamped(t *testing.T) {
	// -B with a very large value is clamped to grepMaxContextLines without error.
	dir := setupGrepDir(t, map[string]string{"f.txt": "line1\nneedle\n"})
	stdout, _, code := grepRun(t, "grep -B 99999999 needle f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nneedle\n", stdout)
}

// ── parseGrepCount edge cases ─────────────────────────────────────────────────

func TestGrepCountNegative(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := grepRun(t, "grep -m -1 pattern", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "grep:")
}

func TestGrepCountOverflow(t *testing.T) {
	// Very large values are clamped to math.MaxInt — should not error.
	dir := setupGrepDir(t, map[string]string{"f.txt": "match\n"})
	stdout, _, code := grepRun(t, "grep -m 99999999999999999999 match f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\n", stdout)
}

// ── CRLF at chunk boundary ────────────────────────────────────────────────────

func TestGrepCRLFAtChunkBoundary(t *testing.T) {
	// Build a file where a CRLF straddles a 4096-byte read boundary.
	// Chunk 1 ends with CR; chunk 2 starts with LF.
	dir := t.TempDir()
	// Fill exactly 4095 bytes then CRLF then another line.
	data := []byte(strings.Repeat("x", 4095) + "\r\nmatch\r\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "crlf_boundary.txt"), data, 0644))
	stdout, _, code := grepRun(t, "grep match crlf_boundary.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\n", stdout)
}
