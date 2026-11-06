// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sed_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return runScriptCtx(context.Background(), t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption)}, opts...)
	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()
	if dir != "" {
		runner.Dir = dir
	}
	err = runner.Run(ctx, prog)
	exitCode := 0
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

func cmdRun(t *testing.T, script, dir string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	require.NoError(t, err)
}

func setupDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	return dir
}

// --- Basic Substitution ---

func TestSubstituteBasic(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/world/earth/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello earth\n", stdout)
}

func TestSubstituteGlobal(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "aaa bbb aaa\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/aaa/zzz/g' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "zzz bbb zzz\n", stdout)
}

func TestSubstituteNth(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "ab ab ab ab\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/ab/XY/2' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ab XY ab ab\n", stdout)
}

func TestSubstituteCaseInsensitive(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "Hello HELLO hello\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/hello/bye/i' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "bye HELLO hello\n", stdout)
}

func TestSubstituteAlternateDelimiter(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "/usr/local/bin\n",
	})
	stdout, _, code := cmdRun(t, `sed 's|/usr/local|/opt|' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "/opt/bin\n", stdout)
}

func TestSubstituteAmpersand(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/hello/[&]/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "[hello]\n", stdout)
}

func TestSubstituteEmptyPattern(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/^/prefix: /' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "prefix: hello\n", stdout)
}

func TestSubstituteWithPrint(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "aaa\nbbb\naaa\n",
	})
	stdout, _, code := cmdRun(t, `sed -n 's/aaa/zzz/p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "zzz\nzzz\n", stdout)
}

// --- Print and Output ---

func TestPrint(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\n",
	})
	stdout, _, code := cmdRun(t, `sed 'p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline1\nline2\nline2\n", stdout)
}

func TestSuppressAutoPrint(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\n",
	})
	stdout, _, code := cmdRun(t, `sed -n 'p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline2\nline3\n", stdout)
}

func TestLineNumber(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "aaa\nbbb\nccc\n",
	})
	stdout, _, code := cmdRun(t, `sed '=' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\naaa\n2\nbbb\n3\nccc\n", stdout)
}

func TestPrintUnambiguous(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\tworld\n",
	})
	stdout, _, code := cmdRun(t, `sed -n 'l' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\\tworld$\n", stdout)
}

// --- Delete ---

func TestDeleteBasic(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\n",
	})
	stdout, _, code := cmdRun(t, `sed '2d' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline3\n", stdout)
}

func TestDeleteRange(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\nline4\nline5\n",
	})
	stdout, _, code := cmdRun(t, `sed '2,4d' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline5\n", stdout)
}

// --- Addressing ---

func TestAddressLine(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "first\nsecond\nthird\n",
	})
	stdout, _, code := cmdRun(t, `sed '2s/second/SECOND/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nSECOND\nthird\n", stdout)
}

func TestAddressLastLine(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "first\nsecond\nthird\n",
	})
	stdout, _, code := cmdRun(t, `sed '$s/third/THIRD/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\nsecond\nTHIRD\n", stdout)
}

func TestAddressRegex(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "apple\nbanana\ncherry\n",
	})
	stdout, _, code := cmdRun(t, `sed '/banana/d' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\ncherry\n", stdout)
}

func TestAddressRange(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "1\n2\n3\n4\n5\n",
	})
	stdout, _, code := cmdRun(t, `sed -n '2,4p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\n3\n4\n", stdout)
}

func TestAddressRegexRange(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "start\nmiddle1\nmiddle2\nend\nafter\n",
	})
	stdout, _, code := cmdRun(t, `sed -n '/start/,/end/p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "start\nmiddle1\nmiddle2\nend\n", stdout)
}

func TestAddressNegation(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "keep\ndelete\nkeep\n",
	})
	stdout, _, code := cmdRun(t, `sed '/keep/!d' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "keep\nkeep\n", stdout)
}

func TestAddressStep(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "1\n2\n3\n4\n5\n6\n",
	})
	stdout, _, code := cmdRun(t, `sed -n '1~2p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n3\n5\n", stdout)
}

// --- Text Commands ---

func TestAppend(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\n",
	})
	stdout, _, code := cmdRun(t, `sed '1a\appended' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nappended\nline2\n", stdout)
}

func TestInsert(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\n",
	})
	stdout, _, code := cmdRun(t, `sed '2i\inserted' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\ninserted\nline2\n", stdout)
}

func TestChange(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\n",
	})
	stdout, _, code := cmdRun(t, `sed '2c\changed' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nchanged\nline3\n", stdout)
}

// --- Hold Space ---

func TestHoldCopy(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "first\nsecond\n",
	})
	// Copy first line to hold space, on second line replace pattern with hold
	stdout, _, code := cmdRun(t, `sed -n '1h;2{g;p}' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "first\n", stdout)
}

func TestHoldAppend(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "a\nb\nc\n",
	})
	// Accumulate all lines in hold space, print at end
	stdout, _, code := cmdRun(t, `sed -n 'H;${g;p}' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\nb\nc\n", stdout)
}

func TestExchange(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "pattern\n",
	})
	// Exchange swaps pattern space (content) with hold space (initially empty)
	stdout, _, code := cmdRun(t, `sed -n 'x;p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

// --- Branching ---

func TestBranch(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	// b with no label branches to end of script, skipping subsequent commands
	stdout, _, code := cmdRun(t, `sed 'b;s/hello/bye/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestBranchLabel(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	stdout, _, code := cmdRun(t, "sed 'b skip;s/hello/bye/;:skip' input.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestBranchConditional(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "aXb\n",
	})
	// t branches if substitution was made
	stdout, _, code := cmdRun(t, `sed 's/X/Y/;t done;s/a/Z/;:done' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "aYb\n", stdout)
}

func TestBranchConditionalNoSub(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\nworld\n",
	})
	// T branches if NO substitution was made.
	// On "hello": s/hello/HI/ succeeds, T does not branch, s/HI/BYE/ runs → "BYE"
	// On "world": s/hello/HI/ fails, T branches to done, s/HI/BYE/ skipped → "world"
	stdout, _, code := cmdRun(t, `sed 's/hello/HI/;T done;s/HI/BYE/;:done' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "BYE\nworld\n", stdout)
}

// --- Next Line ---

func TestNext(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\nline4\n",
	})
	// n prints current line (unless -n), reads next line into pattern space
	stdout, _, code := cmdRun(t, `sed -n 'n;p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line2\nline4\n", stdout)
}

func TestNextAppend(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\n",
	})
	// N appends next line to pattern space with embedded newline
	stdout, _, code := cmdRun(t, `sed -n 'N;p' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "line1")
	assert.Contains(t, stdout, "line2")
}

// --- Transliterate ---

func TestTransliterate(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	stdout, _, code := cmdRun(t, `sed 'y/helo/HELO/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "HELLO\n", stdout)
}

// --- Quit ---

func TestQuit(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\n",
	})
	// q prints current line then exits
	stdout, _, code := cmdRun(t, `sed '2q' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\nline2\n", stdout)
}

func TestQuitNoPrint(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "line1\nline2\nline3\n",
	})
	// Q exits without printing current line
	stdout, _, code := cmdRun(t, `sed '2Q' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "line1\n", stdout)
}

// --- Multiple Expressions ---

func TestMultipleExpressions(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, _, code := cmdRun(t, `sed -e 's/hello/hi/' -e 's/world/earth/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi earth\n", stdout)
}

func TestSemicolonSeparator(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/hello/hi/;s/world/earth/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi earth\n", stdout)
}

// --- Extended Regex ---

func TestExtendedRegex(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "abc123def\n",
	})
	stdout, _, code := cmdRun(t, `sed -E 's/[0-9]+/NUM/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcNUMdef\n", stdout)
}

func TestExtendedRegexR(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "abc123def\n",
	})
	stdout, _, code := cmdRun(t, `sed -r 's/[0-9]+/NUM/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcNUMdef\n", stdout)
}

// --- Stdin ---

func TestStdinPipe(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, _, code := cmdRun(t, `cat input.txt | sed 's/world/earth/'`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello earth\n", stdout)
}

func TestStdinDash(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, _, code := cmdRun(t, `cat input.txt | sed 's/world/earth/' -`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello earth\n", stdout)
}

// --- Edge Cases ---

func TestEmptyFile(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "",
	})
	stdout, _, code := cmdRun(t, `sed 's/a/b/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestSingleLine(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "only line\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/only/single/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "single line\n", stdout)
}

func TestNoTrailingNewline(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "no newline",
	})
	stdout, _, code := cmdRun(t, `sed 's/no/with/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "with newline\n", stdout)
}

func TestMultipleFiles(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"a.txt": "alpha\n",
		"b.txt": "beta\n",
	})
	stdout, _, code := cmdRun(t, `sed 's/^/> /' a.txt b.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "> alpha\n> beta\n", stdout)
}

// --- Error Cases ---

func TestMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, `sed 's/a/b/' nonexistent.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
}

func TestNoScript(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, `sed`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
}

func TestInvalidRegex(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed 's/[invalid/replacement/' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
}

func TestBlockedWriteCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed 'w output.txt' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestBlockedExecuteCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed 'e' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestBlockedInPlaceFlag(t *testing.T) {
	// cmdRun (see cmdRun's AllowedPaths call) does not enable remediation
	// mode, so -i must be refused exactly as it would be for any other
	// remediation-gated capability.
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed -i 's/hello/bye/' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "remediation mode")
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content), "file must be left untouched when -i is refused")
}

// --- In-place editing (-i) ---

func inPlaceRun(t *testing.T, script, dir string) (stdout, stderr string, code int) {
	t.Helper()
	return runScript(t, script, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)
}

func TestInPlaceBasicSubstitute(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello world\n",
	})
	stdout, stderr, code := inPlaceRun(t, `sed -i 's/hello/goodbye/' input.txt`, dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout, "-i must not write to stdout")
	assert.Empty(t, stderr)
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "goodbye world\n", string(content))
}

func TestInPlaceRequiresRemediationMode(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	// AllowedPaths grants :rw but WithMode(ModeRemediation) is intentionally
	// omitted — -i must still be refused, since RemediationMode gates the
	// capability independently of the sandbox's own read/write mode.
	_, stderr, code := runScript(t, `sed -i 's/hello/bye/' input.txt`, dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "remediation mode")
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content))
}

// TestInPlaceRequiresWritableRoot verifies that -i is refused with a
// distinct "no writable path is configured" hint (not the generic
// remediation-mode message) when remediation mode is on but AllowedPaths
// grants no :rw root — matching the truncate/rm hasWritableRoot pattern.
// Without this check, -i would otherwise read and transform the whole file
// before the destructive write attempt fails, reporting a misleading
// combined write-then-restore-also-failed error instead of this direct
// guidance, and never even attempts the destructive write since the check
// runs first.
func TestInPlaceRequiresWritableRoot(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	// :ro (not :rw): remediation mode is on, but no writable root exists.
	_, stderr, code := runScript(t, `sed -i 's/hello/bye/' input.txt`, dir,
		interp.AllowedPaths([]string{dir + ":ro"}),
		interp.WithMode(interp.ModeRemediation),
	)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no writable path is configured")
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content))
}

func TestInPlaceRejectsStdin(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := inPlaceRun(t, `echo hi | sed -i 's/hi/bye/' -`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "standard input")
}

// TestInPlaceStdinRejectionIsSequentialNotPreScanned is a regression test
// for a P2 finding: "-" (stdin) must be rejected only once that specific
// operand is actually reached in the processing sequence, not by a
// pre-scan of every operand before any file is touched. Verified against
// real GNU sed 4.9: `sed -i 's/a/b/' first.txt -` edits and commits
// first.txt before failing on the "-" operand.
func TestInPlaceStdinRejectionIsSequentialNotPreScanned(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"first.txt": "a\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i 's/a/b/' first.txt -`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "standard input")
	content, err := os.ReadFile(filepath.Join(dir, "first.txt"))
	require.NoError(t, err)
	assert.Equal(t, "b\n", string(content), "the earlier real file must still be edited before the \"-\" operand is reached and rejected")
}

// TestInPlaceQuitBeforeStdinNeverReachesStdinCheck is the complementary
// case: when an earlier file's script quits (q/Q) before "-" would be
// reached, the invocation must succeed without ever reporting the stdin
// rejection at all — GNU sed 4.9 stops the whole invocation at q and never
// reaches later operands, "-" included.
func TestInPlaceQuitBeforeStdinNeverReachesStdinCheck(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"first.txt": "x\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i q first.txt -`, dir)
	assert.Equal(t, 0, code, stderr)
	assert.Empty(t, stderr, "q on the first file must stop before \"-\" is ever reached, so no stdin-rejection error should appear")
	content, err := os.ReadFile(filepath.Join(dir, "first.txt"))
	require.NoError(t, err)
	assert.Equal(t, "x\n", string(content))
}

func TestInPlaceNoFiles(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := inPlaceRun(t, `sed -i 's/a/b/'`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no input files")
}

func TestInPlaceMissingFileNotCreated(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := inPlaceRun(t, `sed -i 's/a/b/' missing.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing.txt")
	_, err := os.Stat(filepath.Join(dir, "missing.txt"))
	assert.True(t, os.IsNotExist(err), "-i must not create a missing file")
}

// TestInPlaceRejectsSymlinkTarget is a regression/confirmation test
// pinning docs/RULES.md's "Commands MUST NOT follow symlinks during
// write operations" rule for -i: a symlink whose referent lies inside the
// same writable root must be rejected outright, not silently redirected
// to overwrite its referent (matching GNU sed's own default of replacing
// the link itself, not the referent, unless --follow-symlinks is given —
// this shell has no rename primitive to safely perform that replacement
// either, so a symlinked target is rejected entirely rather than
// attempting it). The rejection happens at
// allowedpaths.rejectSymlinkWriteTarget (openWriteFile's own check on the
// literal, unresolved path), independent of resolveWriteTarget's separate
// mode-check walk that resolves symlinks purely to validate access mode,
// never to redirect the actual write.
func TestInPlaceRejectsSymlinkTarget(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"target.txt": "hello\n",
	})
	target := filepath.Join(dir, "target.txt")
	link := filepath.Join(dir, "link.txt")
	require.NoError(t, os.Symlink(target, link))

	_, stderr, code := inPlaceRun(t, `sed -i 's/hello/bye/' link.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "symlink")

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content), "the symlink's referent must never be written through")
}

func TestInPlaceRejectsBackupSuffix(t *testing.T) {
	// Backup-suffix forms (-i.bak, --in-place=.bak) are unsupported: this
	// shell has no rename primitive to create the backup atomically.
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i.bak 's/hello/bye/' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sed:")
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(content))
}

// TestInPlaceAttachedShorthandClusterCombinesRatherThanRejects documents a
// deliberate, RULES.md-compliant limitation: unlike an explicit "=value"
// form (-i=.bak, rejected — see TestInPlaceRejectsBackupSuffix's sibling
// long-form case), an attached suffix with no "=" inside a short-option
// cluster (-iE) cannot be distinguished from an ordinary combined-flag
// cluster without a hand-rolled pre-scan loop, which docs/RULES.md's flag-
// parsing rules prohibit ("All flag parsing MUST use pflag... Do NOT write
// manual flag-parsing loops" / "Do NOT add pre-scan loops... to reject
// specific flags"). So -iE parses via pflag's standard short-cluster
// semantics as -i followed by -E, silently discarding any backup-suffix
// intent rather than rejecting it — GNU sed would instead create a backup
// file literally named "inputE", which this shell never does either way,
// so no destructive-without-a-backup surprise actually results: the edit
// still happens, in place, exactly as -i alone would do it.
func TestInPlaceAttachedShorthandClusterCombinesRatherThanRejects(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -iE 's/hel+o/bye/' input.txt`, dir)
	require.Equal(t, 0, code, stderr)
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bye\n", string(content))
	_, err = os.Stat(filepath.Join(dir, "input.txtE"))
	assert.True(t, os.IsNotExist(err), "no backup file is ever created, matching bare -i")
}

// TestInPlaceAcceptsIAsLastClusterCharacter verifies bare -i as the last
// character of a short-option cluster (e.g. -Ei, -ni) performs a normal
// in-place edit, same as -iE above but with the flags in the other order.
func TestInPlaceAcceptsIAsLastClusterCharacter(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -Ei 's/hel+o/bye/' input.txt`, dir)
	require.Equal(t, 0, code, stderr)
	content, err := os.ReadFile(filepath.Join(dir, "input.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bye\n", string(content))
}

func TestInPlaceMultipleFilesSeparateStreams(t *testing.T) {
	// Each file must be its own stream: $ matches the last line of *each*
	// file, not just the last file overall (unlike the default multi-file
	// streaming mode).
	dir := setupDir(t, map[string]string{
		"a.txt": "1a\n2a\n",
		"b.txt": "1b\n2b\n",
	})
	_, _, code := inPlaceRun(t, `sed -i '$s/$/-LAST/' a.txt b.txt`, dir)
	require.Equal(t, 0, code)
	aContent, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "1a\n2a-LAST\n", string(aContent))
	bContent, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "1b\n2b-LAST\n", string(bContent))
}

// TestInPlaceHoldSpaceResetsPerFile pins GNU sed's actual -s/-i behaviour,
// verified against real GNU sed 4.9: `sed -s '/keepme/h; $G' a.txt b.txt`
// does NOT carry a.txt's hold-space value into b.txt. b.txt's line is both
// the /keepme/ non-match and $ (its only line), so $G appends the (reset,
// empty) hold space, producing a trailing blank line rather than "keepme".
func TestInPlaceHoldSpaceResetsPerFile(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"a.txt": "keepme\nother\n",
		"b.txt": "anything\n",
	})
	_, _, code := inPlaceRun(t, `sed -i '/keepme/h; $G' a.txt b.txt`, dir)
	require.Equal(t, 0, code)
	aContent, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "keepme\nother\nkeepme\n", string(aContent))
	bContent, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "anything\n\n", string(bContent),
		"hold space must reset to empty for b.txt, not carry over a.txt's value")
}

// TestInPlaceLastRegexPersistsAcrossFiles pins the complementary GNU sed
// behaviour: unlike the hold space, the last-used regex for an empty //
// pattern is NOT reset per file. Verified against real GNU sed 4.9:
// `sed -s '/foo/ s//bar/' a.txt b.txt` (each file containing just "foo")
// still reuses a.txt's last regex (/foo/) when b.txt's s//bar/ runs.
func TestInPlaceLastRegexPersistsAcrossFiles(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"a.txt": "foo\n",
		"b.txt": "foo\n",
	})
	_, _, code := inPlaceRun(t, `sed -i '/foo/ s//bar/' a.txt b.txt`, dir)
	require.Equal(t, 0, code)
	aContent, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bar\n", string(aContent))
	bContent, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bar\n", string(bContent),
		"b.txt's empty s//bar/ must still reuse a.txt's last regex (/foo/)")
}

func TestInPlaceQuitCommitsPartialOutput(t *testing.T) {
	// q must still commit whatever output was produced before the quit
	// point (matching GNU sed's temp-file-then-rename behaviour), and must
	// stop processing any remaining files.
	dir := setupDir(t, map[string]string{
		"a.txt": "1\n2\n3\n4\n",
		"b.txt": "untouched\n",
	})
	_, _, code := inPlaceRun(t, `sed -i '2q' a.txt b.txt`, dir)
	assert.Equal(t, 0, code)
	aContent, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "1\n2\n", string(aContent))
	bContent, err := os.ReadFile(filepath.Join(dir, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "untouched\n", string(bContent), "q must stop before processing later files")
}

func TestInPlacePartialFailureContinuesRemainingFiles(t *testing.T) {
	// A hard failure on one file (e.g. missing) must not abort processing
	// of the remaining file operands; exit 1 is still returned overall.
	dir := setupDir(t, map[string]string{
		"a.txt": "hello\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i 's/hello/bye/' missing.txt a.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing.txt")
	aContent, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "bye\n", string(aContent), "later operand must still be processed")
}

// TestInPlaceQuitAfterEarlierFailureStaysFailed verifies that a later
// file's q command does not convert an earlier file's failure into overall
// success. Verified against real GNU sed 4.9: `sed -i 'q' missing.txt
// good.txt` exits 2 (its own missing-file status), not 0.
func TestInPlaceQuitAfterEarlierFailureStaysFailed(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"good.txt": "a\nb\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i 'q' missing.txt good.txt`, dir)
	assert.Equal(t, 1, code, "an earlier file's failure must not be discarded by a later q")
	assert.Contains(t, stderr, "missing.txt")
	// good.txt's own script (a bare q) still ran and committed its output:
	// q quits after the first line's auto-print, so only "a" is kept.
	content, err := os.ReadFile(filepath.Join(dir, "good.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(content))
}

// TestInPlaceQuitWithExplicitCodeAfterEarlierFailureStaysFailed verifies
// that this precedence holds even when q requests a specific, non-zero exit
// code: GNU sed's own earlier-failure status still wins. Verified against
// real GNU sed 4.9: `sed -i 'q5' missing.txt good.txt` exits 2, not 5, even
// though `sed -i 'q5' good.txt` alone (no earlier failure) does exit 5.
func TestInPlaceQuitWithExplicitCodeAfterEarlierFailureStaysFailed(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"good.txt": "a\n",
	})
	_, stderr, code := inPlaceRun(t, `sed -i 'q5' missing.txt good.txt`, dir)
	assert.Equal(t, 1, code, "an earlier file's failure must take priority over q's own requested exit code")
	assert.Contains(t, stderr, "missing.txt")
}

func TestInPlaceFindExec(t *testing.T) {
	// find -exec builds a separate child CallContext (see runner_exec.go's
	// RunCommand closure) rather than reusing the top-level dispatch path;
	// sed's own callCtx.RemediationMode gate must still work through it.
	dir := setupDir(t, map[string]string{
		"big.log": "error: bad\n",
	})
	_, _, code := inPlaceRun(t, `find . -name '*.log' -exec sed -i 's/bad/good/' {} \;`, dir)
	assert.Equal(t, 0, code)
	content, err := os.ReadFile(filepath.Join(dir, "big.log"))
	require.NoError(t, err)
	assert.Equal(t, "error: good\n", string(content))
}

// --- In-place editing (-i): missing final newline ---
//
// The default streaming mode intentionally always terminates output with
// \n regardless of the input's own termination (see
// tests/scenarios/cmd/sed/edge/no_trailing_newline.yaml and
// TestNoTrailingNewline above), trading GNU sed compatibility for
// consistent AI-agent-facing stdout. -i cannot make that same trade-off: it
// writes back to a real file that other tools read afterward, so it must
// reproduce GNU sed's actual on-disk behaviour. Every expectation below was
// verified against real GNU sed 4.9 (debian:bookworm-slim, the same oracle
// TestShellScenariosAgainstBash uses).

func TestInPlaceNoTrailingNewlinePreserved(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "x"})
	_, _, code := inPlaceRun(t, `sed -i 's/x/y/' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "y", string(got))
}

func TestInPlaceNoTrailingNewlineMultiLine(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i 's/b/B/' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\nB", string(got))
}

// TestInPlaceNoTrailingNewlineDuplicatePrints pins GNU sed's
// output_missing_newline lazy-flush behaviour: repeated prints of the
// unterminated final line (via -n 'p;p') each individually omit the
// newline, but a deferred one is inserted before the next print so the two
// copies don't run together — only the very last byte written is missing
// its newline.
func TestInPlaceNoTrailingNewlineDuplicatePrints(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i -n 'p;p' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\na\nb\nb", string(got))
}

// TestInPlaceNoTrailingNewlineAppendedTextStillTerminated verifies that
// text queued by the `a` command still gets its own trailing newline even
// when it follows the auto-print of an unterminated final line: GNU sed
// flushes the deferred newline before writing the appended text, and the
// appended text (being script-literal, not reflecting input) always ends
// with its own newline regardless of the input's termination.
func TestInPlaceNoTrailingNewlineAppendedTextStillTerminated(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '$a appended' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\nb\nappended\n", string(got))
}

// TestInPlaceNoTrailingNewlineLineNumStillTerminated verifies that `=`
// (line number) output, being generated rather than a reflection of the
// pattern space, always ends with its own newline even immediately
// preceding the unterminated final line's own auto-print.
func TestInPlaceNoTrailingNewlineLineNumStillTerminated(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '=' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "1\na\n2\nb", string(got))
}

// TestInPlaceQuitOnUnterminatedFinalLineAlwaysTerminates is a regression
// test for a P2 finding: q's implicit print of the current pattern space
// must always be newline-terminated, even when it fires on the
// unterminated final input line — unlike ordinary end-of-file auto-print,
// which deliberately preserves a missing trailing newline. Confirmed
// against real GNU sed 4.9 (debian:bookworm-slim): `printf a > f; sed -i q
// f` leaves `a\n` on disk, not a bare `a`.
func TestInPlaceQuitOnUnterminatedFinalLineAlwaysTerminates(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a"})
	_, _, code := inPlaceRun(t, `sed -i q file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(got))
}

// TestInPlaceQuitOnTerminatedLineStillTerminated is the control case for
// TestInPlaceQuitOnUnterminatedFinalLineAlwaysTerminates: quitting on an
// earlier, ordinarily-terminated line must be entirely unaffected — also
// confirmed against real GNU sed 4.9.
func TestInPlaceQuitOnTerminatedLineStillTerminated(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '1q' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(got))
}

// TestInPlaceQuitFlushesDeferredNewlineEvenWhenSuppressed is a regression
// test for a P2 finding: -n suppresses q's own implicit print, so q's own
// call to writeLine (which would otherwise flush a deferred newline as a
// side effect) never runs — but an earlier explicit print (here, p) may
// have already deferred the unterminated final line's own trailing
// newline via pendingMissingNewline, and q must still flush that deferred
// newline before quitting even though it never itself calls writeLine.
// Confirmed against real GNU sed 4.9: `printf a > f; sed -n -i 'p;q' f`
// leaves `a\n` on disk, not a bare `a`.
func TestInPlaceQuitFlushesDeferredNewlineEvenWhenSuppressed(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a"})
	_, _, code := inPlaceRun(t, `sed -n -i 'p;q' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\n", string(got))
}

// TestInPlaceQuitNoprintDoesNotFlushDeferredNewline is the control case
// for TestInPlaceQuitFlushesDeferredNewlineEvenWhenSuppressed: Q
// (cmdQuitNoprint) must NOT flush a deferred newline the way q does —
// also confirmed against real GNU sed 4.9: `printf a > f; sed -n -i 'p;Q'
// f` leaves a bare `a`, not `a\n`.
func TestInPlaceQuitNoprintDoesNotFlushDeferredNewline(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a"})
	_, _, code := inPlaceRun(t, `sed -n -i 'p;Q' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a", string(got))
}

// TestInPlaceNoTrailingNewlineRoundTrips verifies the missing-newline
// property survives being written back and re-read across multiple -i
// invocations, rather than being silently "fixed" on the first edit.
func TestInPlaceNoTrailingNewlineRoundTrips(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i 's/b/B/' file.txt`, dir)
	require.Equal(t, 0, code)
	_, _, code = inPlaceRun(t, `sed -i 's/B/BB/' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\nBB", string(got))
}

// TestInPlaceWithTrailingNewlineUnaffected is the control case: a file that
// does end in \n must be completely unaffected by the missing-newline
// tracking logic.
func TestInPlaceWithTrailingNewlineUnaffected(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb\n"})
	_, _, code := inPlaceRun(t, `sed -i 's/b/B/' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\nB\n", string(got))
}

// TestInPlaceHoldSpaceTransferPropagatesChompState is a regression test
// for a P2 finding: h/H/g/G/x move content between the pattern space and
// the hold space, and the destination's newline-termination state must
// move with that content rather than being left as whatever the
// destination's own state happened to be beforehand. Verified against real
// GNU sed 4.9: on a file whose last line has no trailing newline,
// `sed -i '1h;2g' file` still produces a properly newline-terminated final
// line, because line 2's pattern space is entirely replaced by line 1's
// (terminated) content via g.
func TestInPlaceHoldSpaceTransferPropagatesChompState(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '1h;2g' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\na\n", string(got),
		"g must carry the hold space's own (terminated) chomp state into the pattern space, not leave line 2's own unterminated state")
}

// TestInPlaceHoldAppendPropagatesChompState covers H/G (the append forms),
// verified against real GNU sed 4.9's exact byte output for the same
// unterminated-final-line file.
func TestInPlaceHoldAppendPropagatesChompState(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '1H;2G' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n\na\n", string(got))
}

// TestInPlaceExchangePropagatesChompState covers x (exchange), verified
// against real GNU sed 4.9's exact byte output for the same
// unterminated-final-line file: the initial (empty) hold space is itself
// treated as terminated, so exchanging it into the pattern space on line 1
// produces a properly terminated empty line, and line 1's own (terminated)
// content exchanged into the hold space then surfaces as a terminated line
// when it is swapped back into the pattern space on line 2.
func TestInPlaceExchangePropagatesChompState(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a\nb"})
	_, _, code := inPlaceRun(t, `sed -i '1x;2x' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "\na\n", string(got))
}

// TestInPlaceGetCopyFromInitialEmptyHoldSpace pins the base case underlying
// the tests above: the initial (never-yet-written-to) hold space's own
// chomp state defaults to terminated, matching GNU sed 4.9's
// line.chomped-defaults-true-before-any-read behaviour, verified with a
// single-line, wholly unterminated source file.
func TestInPlaceGetCopyFromInitialEmptyHoldSpace(t *testing.T) {
	dir := setupDir(t, map[string]string{"file.txt": "a"})
	_, _, code := inPlaceRun(t, `sed -i 'g' file.txt`, dir)
	require.Equal(t, 0, code)
	got, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	require.NoError(t, err)
	assert.Equal(t, "\n", string(got))
}

func TestBlockedReadCommand(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed 'r other.txt' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

func TestBlockedWriteFlag(t *testing.T) {
	dir := setupDir(t, map[string]string{
		"input.txt": "hello\n",
	})
	_, stderr, code := cmdRun(t, `sed 's/hello/bye/w output.txt' input.txt`, dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

// --- Help ---

func TestHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := cmdRun(t, `sed --help`, dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}
