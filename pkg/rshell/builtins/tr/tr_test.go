// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tr_test contains unit tests for the tr builtin.
//
// Some tests here intentionally overlap with tr_gnu_compat_test.go (which
// verifies exact GNU coreutils output literals).  The overlap is deliberate:
// this file tests behaviour through the rshell runner, while the GNU compat
// file serves as a regression suite anchored to captured GNU output.
package tr_test

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

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

func trRun(t *testing.T, input, trArgs string) (string, string, int) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", input)
	return runScript(t, "cat in.txt | tr "+trArgs, dir, interp.AllowedPaths([]string{dir}))
}

// --- Basic translation ---

func TestTrTranslateBasic(t *testing.T) {
	stdout, _, code := trRun(t, "abcd", "abcd ABCD")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCD", stdout)
}

func TestTrTranslateRange(t *testing.T) {
	stdout, _, code := trRun(t, "!abcd!", "a-z A-Z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "!ABCD!", stdout)
}

func TestTrSmallSet2(t *testing.T) {
	stdout, _, code := trRun(t, "@0123456789", "0-9 X")
	assert.Equal(t, 0, code)
	assert.Equal(t, "@XXXXXXXXXX", stdout)
}

func TestTrSet1LongerThanSet2(t *testing.T) {
	stdout, _, code := trRun(t, "abcde", "abcd xy")
	assert.Equal(t, 0, code)
	assert.Equal(t, "xyyye", stdout)
}

func TestTrOverridesRepeat(t *testing.T) {
	stdout, _, code := trRun(t, "aaa", "aaa xyz")
	assert.Equal(t, 0, code)
	assert.Equal(t, "zzz", stdout)
}

// --- Delete mode ---

func TestTrDelete(t *testing.T) {
	stdout, _, code := trRun(t, "aBcD", "-d a-z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "BD", stdout)
}

func TestTrDeleteComplement(t *testing.T) {
	stdout, _, code := trRun(t, "Phone: 01234 567890", "-d -c 0-9")
	assert.Equal(t, 0, code)
	assert.Equal(t, "01234567890", stdout)
}

func TestTrDeleteComplementLongForm(t *testing.T) {
	stdout, _, code := trRun(t, "Phone: 01234 567890", "-d --complement 0-9")
	assert.Equal(t, 0, code)
	assert.Equal(t, "01234567890", stdout)
}

func TestTrDeleteDigits(t *testing.T) {
	stdout, _, code := trRun(t, "a0b1c2d3e4f5g6h7i8j9k", "-d '[:digit:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcdefghijk", stdout)
}

func TestTrDeleteXdigit(t *testing.T) {
	stdout, _, code := trRun(t, "w0x1y2z3456789acbdefABCDEFz", "-d '[:xdigit:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "wxyzz", stdout)
}

func TestTrDeleteLower(t *testing.T) {
	stdout, _, code := trRun(t, "abcdefghijklmnopqrstuvwxyz", "-d '[:lower:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestTrDeleteUpper(t *testing.T) {
	stdout, _, code := trRun(t, "ABCDEFGHIJKLMNOPQRSTUVWXYZ", "-d '[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestTrDeleteAlpha(t *testing.T) {
	stdout, _, code := trRun(t, "abcABC123", "-d '[:alpha:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "123", stdout)
}

// --- Squeeze mode ---

func TestTrSqueeze(t *testing.T) {
	stdout, _, code := trRun(t, "aaBBcDcc", "-s a-z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "aBBcDc", stdout)
}

func TestTrSqueezeComplement(t *testing.T) {
	stdout, _, code := trRun(t, "aaBBcDcc", "-sc a-z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "aaBcDcc", stdout)
}

func TestTrTranslateAndSqueeze(t *testing.T) {
	stdout, _, code := trRun(t, "xx", "-s x y")
	assert.Equal(t, 0, code)
	assert.Equal(t, "y", stdout)
}

func TestTrTranslateAndSqueezeMultiLine(t *testing.T) {
	stdout, _, code := trRun(t, "xxaax\nxaaxx", "-s x y")
	assert.Equal(t, 0, code)
	assert.Equal(t, "yaay\nyaay", stdout)
}

// --- Delete and squeeze ---

func TestTrDeleteAndSqueeze(t *testing.T) {
	stdout, _, code := trRun(t, "abBcB", "-ds a-z A-Z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "B", stdout)
}

func TestTrDeleteAndSqueezeAlnum(t *testing.T) {
	stdout, _, code := trRun(t, ".abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.", "-ds '[:alnum:]' .")
	assert.Equal(t, 0, code)
	assert.Equal(t, ".", stdout)
}

func TestTrDeleteAndSqueezeXdigit(t *testing.T) {
	stdout, _, code := trRun(t, "ZZ0123456789acbdefABCDEFZZ", "-ds '[:xdigit:]' Z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "Z", stdout)
}

// --- Truncate mode ---

func TestTrTruncate(t *testing.T) {
	stdout, _, code := trRun(t, "abcde", "-t abc xy")
	assert.Equal(t, 0, code)
	assert.Equal(t, "xycde", stdout)
}

func TestTrTruncateSet1Shorter(t *testing.T) {
	stdout, _, code := trRun(t, "abcde", "-t ab xyz")
	assert.Equal(t, 0, code)
	assert.Equal(t, "xycde", stdout)
}

// --- Character classes ---

func TestTrLowerToUpper(t *testing.T) {
	stdout, _, code := trRun(t, "abcxyzABCXYZ", "'[:lower:]' '[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCXYZABCXYZ", stdout)
}

func TestTrUpperToLower(t *testing.T) {
	stdout, _, code := trRun(t, "abcxyzABCXYZ", "'[:upper:]' '[:lower:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "abcxyzabcxyz", stdout)
}

func TestTrDeleteAlnum(t *testing.T) {
	stdout, _, code := trRun(t, ".abc123.", "-d '[:alnum:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "..", stdout)
}

// --- Backslash escapes ---

func TestTrBackslashNewline(t *testing.T) {
	stdout, _, code := trRun(t, "X", `X '\n'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

func TestTrOctalEscape(t *testing.T) {
	stdout, _, code := trRun(t, "X", `X '\015'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\r", stdout)
}

func TestTrAmbiguousOctalWarning(t *testing.T) {
	stdout, stderr, code := trRun(t, "X", `X '\777'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "?", stdout) // \077 = '?'
	assert.Contains(t, stderr, "tr: warning: the ambiguous octal escape \\777 is being")
	assert.Contains(t, stderr, "interpreted as the 2-byte sequence \\077, 7")
}

func TestTrAmbiguousOctal400(t *testing.T) {
	stdout, stderr, code := trRun(t, "X", `X '\400'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, " ", stdout) // \040 = space
	assert.Contains(t, stderr, "tr: warning: the ambiguous octal escape \\400 is being")
	assert.Contains(t, stderr, "interpreted as the 2-byte sequence \\040, 0")
}

// --- Error cases ---

func TestTrMissingOperand(t *testing.T) {
	_, stderr, code := runScript(t, "tr", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tr: missing operand")
}

func TestTrMissingSecondOperand(t *testing.T) {
	_, stderr, code := runScript(t, "tr foo", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing operand after")
}

func TestTrDeleteExtraOperand(t *testing.T) {
	_, stderr, code := runScript(t, "tr -d a p", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "extra operand")
}

func TestTrUnknownFlag(t *testing.T) {
	_, stderr, code := runScript(t, "tr --invalid-flag a b", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "tr:")
}

func TestTrDeleteSqueezeMissingSecondOperand(t *testing.T) {
	_, stderr, code := runScript(t, "tr -ds a", t.TempDir())
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "Two strings must be given when both deleting and squeezing repeats")
}

func TestTrTruncateEmptySet2Passthrough(t *testing.T) {
	stdout, _, code := trRun(t, "abc", "-t abc ''")
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc", stdout)
}

func TestTrComplementLongSet1NoPanic(t *testing.T) {
	// set1 with duplicates exceeding 256 bytes should not panic
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "helloaa")
	longA := strings.Repeat("a", 300)
	stdout, _, code := runScript(t, "cat in.txt | tr -d -c '"+longA+"'", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "aa", stdout) // only 'a' chars survive complement delete
}

func TestTrRepeatWithEscapedChar(t *testing.T) {
	stdout, _, code := trRun(t, "abc", "abc '[\\n*3]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n\n\n", stdout)
}

func TestTrEmptySet2Translation(t *testing.T) {
	_, stderr, code := trRun(t, "abc", "a ''")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "string2 must be non-empty")
}

func TestTrEmptyEquivalenceClass(t *testing.T) {
	_, stderr, code := trRun(t, "", "'[==]' x")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing equivalence class character")
}

func TestTrEquivalenceClassBackslashEscape(t *testing.T) {
	stdout, _, code := trRun(t, "a\nb", `'[=\n=]' X`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "aXb", stdout)
}

func TestTrEquivalenceClassMultiByteEscaped(t *testing.T) {
	_, stderr, code := trRun(t, "", `-d '[=\na=]'`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "equivalence class operand must be a single character")
}

func TestTrMisalignedUpperInSet2(t *testing.T) {
	_, stderr, code := trRun(t, "abc", "abc '[:upper:]'")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "misaligned [:upper:] and/or [:lower:] construct")
}

func TestTrMisalignedLowerInSet2(t *testing.T) {
	_, stderr, code := trRun(t, "abc", "abc '[:lower:]'")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "misaligned [:upper:] and/or [:lower:] construct")
}

func TestTrAlignedCaseClasses(t *testing.T) {
	stdout, _, code := trRun(t, "abcABC", "'[:lower:]' '[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCABC", stdout)
}

func TestTrAlignedCaseClassesWithPrefix(t *testing.T) {
	stdout, _, code := trRun(t, "abcABC", "'a[:lower:]' 'A[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCABC", stdout)
}

func TestTrComplementSkipsAlignment(t *testing.T) {
	stdout, _, code := trRun(t, "ab", "-c a '[:upper:]x'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ax", stdout)
}

func TestTrMisalignedCaseClassOffset(t *testing.T) {
	_, stderr, code := trRun(t, "abc", "'a[:lower:]' 'AB[:upper:]'")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "misaligned [:upper:] and/or [:lower:] construct")
}

func TestTrSet1LongerSet2EndsWithClass(t *testing.T) {
	_, stderr, code := trRun(t, "abcx", "'[:lower:]x' '[:upper:]'")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "the latter string must not end with a character class")
}

func TestTrSet1LongerSet2EndsWithClassTruncateOK(t *testing.T) {
	stdout, _, code := trRun(t, "abcx", "-t '[:lower:]x' '[:upper:]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "ABCX", stdout)
}

func TestTrEmptyCharClassName(t *testing.T) {
	_, stderr, code := trRun(t, "", "'[::]' x")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing character class name")
}

// --- Help flag ---

func TestTrHelp(t *testing.T) {
	stdout, _, code := runScript(t, "tr --help", t.TempDir())
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: tr")
}

// --- Classic example ---

func TestTrClassicWordSplit(t *testing.T) {
	stdout, _, code := trRun(t, "The big black fox jumped over the fence.", `-cs '[:alnum:]' '\n'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "The\nbig\nblack\nfox\njumped\nover\nthe\nfence\n", stdout)
}

// --- Context cancellation ---

func TestTrContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, _ = runScriptCtx(ctx, t, "tr a b", t.TempDir())
}

// --- Repeat construct ---

func TestTrRepeatZeroInSet2(t *testing.T) {
	stdout, _, code := trRun(t, "abcd", "abc '[b*0]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbbd", stdout)
}

func TestTrRepeatZeros(t *testing.T) {
	stdout, _, code := trRun(t, "abcd", "abc '[b*00000000000000000000]'")
	assert.Equal(t, 0, code)
	assert.Equal(t, "bbbd", stdout)
}

// --- Range edge cases ---

func TestTrRangeAToA(t *testing.T) {
	stdout, _, code := trRun(t, "abc", "a-a z")
	assert.Equal(t, 0, code)
	assert.Equal(t, "zbc", stdout)
}

// --- Empty stdin ---

func TestTrEmptyStdin(t *testing.T) {
	stdout, stderr, code := trRun(t, "", "a b")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Fowler test ---

func TestTrFowler(t *testing.T) {
	stdout, _, code := trRun(t, "aha", "ah -H")
	assert.Equal(t, 0, code)
	assert.Equal(t, "-H-", stdout)
}
