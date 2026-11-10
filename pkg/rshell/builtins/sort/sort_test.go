// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sort_test

import (
	"context"
	"os"
	"path/filepath"
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

// cmdRun runs a sort command with AllowedPaths set to dir.
func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

// writeFile creates a file in dir with the given content.
func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0644))
}

// --- Default behavior (lexicographic sort) ---

func TestSortDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nbanana\ncherry\n", stdout)
}

func TestSortAlreadySorted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestSortEmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestSortSingleLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "only\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "only\n", stdout)
}

func TestSortSingleLineNoNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "only")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "only\n", stdout)
}

func TestSortDuplicateLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nb\na\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\nb\nb\n", stdout)
}

// --- Reverse sort ---

func TestSortReverse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "sort -r f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cherry\nbanana\napple\n", stdout)
}

func TestSortReverseLong(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "sort --reverse f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "cherry\nbanana\napple\n", stdout)
}

func TestSortReverseLastResortTieBreaker(t *testing.T) {
	// When -r is global and keys tie, last-resort comparison must also reverse.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a:1\na:2\nb:1\n")
	stdout, _, code := cmdRun(t, "sort -r -t : -k 1,1 f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b:1\na:2\na:1\n", stdout)
}

func TestSortNumericNonNumericAsZero(t *testing.T) {
	// Non-numeric lines should compare as 0 in -n mode (matching GNU).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "A\n0\nB\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\nA\nB\n", stdout)
}

// --- Numeric sort ---

func TestSortNumeric(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "10\n2\n1\n20\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n2\n10\n20\n", stdout)
}

func TestSortNumericReverse(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "10\n2\n1\n20\n")
	stdout, _, code := cmdRun(t, "sort -n -r f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "20\n10\n2\n1\n", stdout)
}

func TestSortNumericWithNonNumeric(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "10\nabc\n2\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\n2\n10\n", stdout)
}

func TestSortNumericNegative(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "5\n-3\n0\n-10\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "-10\n-3\n0\n5\n", stdout)
}

// --- Unique ---

func TestSortUnique(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nb\na\nc\n")
	stdout, _, code := cmdRun(t, "sort -u f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestSortUniqueNumeric(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "2\n1\n2\n3\n1\n")
	stdout, _, code := cmdRun(t, "sort -n -u f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1\n2\n3\n", stdout)
}

func TestSortUniqueCaseInsensitive(t *testing.T) {
	// sort -f -u should treat A and a as equal (no last-resort byte comparison).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "A\na\nB\nb\n")
	stdout, _, code := cmdRun(t, "sort -f -u f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\nB\n", stdout)
}

func TestSortCheckUniqueDuplicates(t *testing.T) {
	// sort -c -u should fail on adjacent equal lines.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\na\nb\n")
	_, stderr, code := cmdRun(t, "sort -c -u f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "disorder")
}

func TestSortCheckUniqueSorted(t *testing.T) {
	// sort -c -u on strictly unique sorted input should succeed.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	_, _, code := cmdRun(t, "sort -c -u f.txt", dir)
	assert.Equal(t, 0, code)
}

func TestSortNumericLargeIntegers(t *testing.T) {
	// sort -n should correctly order very large integers (beyond float64 precision).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "100000000000000000000\n99999999999999999999\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "99999999999999999999\n100000000000000000000\n", stdout)
}

func TestSortCheckInvalidValue(t *testing.T) {
	// --check=foo should be rejected.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\n")
	_, stderr, code := cmdRun(t, "sort --check=foo f.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "invalid argument")
}

func TestSortNumericDecimal(t *testing.T) {
	// sort -n should handle decimal numbers.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "1.5\n1.3\n1.7\n1.05\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1.05\n1.3\n1.5\n1.7\n", stdout)
}

// --- Ignore case ---

func TestSortIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "Banana\napple\nCherry\n")
	stdout, _, code := cmdRun(t, "sort -f f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nBanana\nCherry\n", stdout)
}

// --- Dictionary order ---

func TestSortDictionaryOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b-b\na.a\nc_c\n")
	stdout, _, code := cmdRun(t, "sort -d f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.a\nb-b\nc_c\n", stdout)
}

// --- Ignore leading blanks ---

func TestSortIgnoreLeadingBlanks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "  banana\napple\n   cherry\n")
	stdout, _, code := cmdRun(t, "sort -b f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\n  banana\n   cherry\n", stdout)
}

// --- Field separator and key ---

func TestSortFieldSeparatorKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "c:3\na:1\nb:2\n")
	stdout, _, code := cmdRun(t, "sort -t : -k 2 f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:1\nb:2\nc:3\n", stdout)
}

func TestSortFieldSeparatorPreservedInKey(t *testing.T) {
	// When -t is used, the separator must be preserved in multi-field keys.
	// If we incorrectly join with space, "a b" and "a:b" would compare equal.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "x:a b\ny:a:b\n")
	stdout, _, code := cmdRun(t, "sort -t : -k 2 f.txt", dir)
	assert.Equal(t, 0, code)
	// "a b" (single field containing space) < "a:b" (two fields joined with :)
	// because ' ' (0x20) < ':' (0x3a) in byte comparison.
	assert.Equal(t, "x:a b\ny:a:b\n", stdout)
}

func TestSortNumericKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "c:30\na:1\nb:20\n")
	stdout, _, code := cmdRun(t, "sort -t : -k 2n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:1\nb:20\nc:30\n", stdout)
}

func TestSortKeyWithFieldRange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "c:3:z\na:1:x\nb:2:y\n")
	stdout, _, code := cmdRun(t, "sort -t : -k 2,2n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:1:x\nb:2:y\nc:3:z\n", stdout)
}

func TestSortMultipleKeys(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a:2\nb:1\na:1\n")
	stdout, _, code := cmdRun(t, "sort -t : -k 1,1 -k 2,2n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a:1\na:2\nb:1\n", stdout)
}

// --- Check sorted ---

func TestSortCheckSorted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	_, _, code := cmdRun(t, "sort -c f.txt", dir)
	assert.Equal(t, 0, code)
}

func TestSortCheckUnsorted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nc\n")
	_, stderr, code := cmdRun(t, "sort -c f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "disorder")
}

func TestSortCheckSilentUnsorted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nc\n")
	_, stderr, code := cmdRun(t, "sort -C f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stderr)
}

func TestSortCheckSilentLongForm(t *testing.T) {
	// --check=silent should work like -C.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nc\n")
	_, stderr, code := cmdRun(t, "sort --check=silent f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stderr)
}

func TestSortCheckQuietLongForm(t *testing.T) {
	// --check=quiet should work like -C.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nc\n")
	_, stderr, code := cmdRun(t, "sort --check=quiet f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stderr)
}

func TestSortCheckMultipleFilesRejected(t *testing.T) {
	// GNU sort -c rejects multiple file operands.
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a\nb\n")
	writeFile(t, dir, "b.txt", "a\nb\n")
	_, stderr, code := cmdRun(t, "sort -c a.txt b.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "extra operand")
}

// --- Stable sort ---

func TestSortStable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b:2\na:1\nb:1\na:2\n")
	stdout, _, code := cmdRun(t, "sort -s -t : -k 1,1 f.txt", dir)
	assert.Equal(t, 0, code)
	// With stable sort, equal keys preserve input order.
	assert.Equal(t, "a:1\na:2\nb:2\nb:1\n", stdout)
}

// --- Stdin ---

func TestSortStdin(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "sort < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nbanana\ncherry\n", stdout)
}

func TestSortStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "sort - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nbanana\ncherry\n", stdout)
}

func TestSortPipe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "banana\napple\ncherry\n")
	stdout, _, code := cmdRun(t, "cat f.txt | sort", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nbanana\ncherry\n", stdout)
}

// --- Multiple files ---

func TestSortMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "cherry\napple\n")
	writeFile(t, dir, "b.txt", "banana\ndate\n")
	stdout, _, code := cmdRun(t, "sort a.txt b.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "apple\nbanana\ncherry\ndate\n", stdout)
}

// --- Help ---

func TestSortHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "sort --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Empty(t, stderr)
}

func TestSortShortHAcceptedAsHumanNumeric(t *testing.T) {
	// -h is the short form of --human-numeric-sort (GNU compat).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "1G\n500M\n2K\n")
	stdout, stderr, code := cmdRun(t, "sort -h f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "2K\n500M\n1G\n", stdout)
}

// --- Error cases ---

func TestSortMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "sort nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sort:")
}

func TestSortUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "sort --no-such-flag f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sort:")
}

func TestSortOutputFlagRejected(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello\n")
	_, stderr, code := cmdRun(t, "sort -o out.txt f.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sort:")
}

func TestSortOutsideAllowedPaths(t *testing.T) {
	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret"), 0644))
	secretPath := filepath.ToSlash(filepath.Join(secret, "secret.txt"))
	_, stderr, code := runScript(t, "sort "+secretPath, allowed, interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "sort:")
}

// --- Regression tests for codex review findings ---

func TestSortNumericPlusPrefixNonNumeric(t *testing.T) {
	// GNU sort -n treats +N as non-numeric (value 0), not as positive N.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "+2\n1\n3\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	// +2 is non-numeric (0), sorts first via last-resort byte cmp ('+' < '1')
	assert.Equal(t, "+2\n1\n3\n", stdout)
}

func TestSortNumericDotPrefix(t *testing.T) {
	// .5 should compare as 0.5, not sort before 0.4.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", ".5\n0.4\n0.6\n")
	stdout, _, code := cmdRun(t, "sort -n f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0.4\n.5\n0.6\n", stdout)
}

func TestSortEmptyTabRejected(t *testing.T) {
	// sort -t '' should be rejected with "empty tab".
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\n")
	_, stderr, code := cmdRun(t, `sort -t "" f.txt`, dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "empty tab")
}

func TestSortKeyEndFieldZeroRejected(t *testing.T) {
	// -k 1,0 should be rejected (zero field number).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\n")
	_, stderr, code := cmdRun(t, "sort -k 1,0 f.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "sort:")
}

func TestSortCRLFPreserved(t *testing.T) {
	// CRLF line endings must be preserved through sort.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\r\na\r\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\r\nb\r\n", stdout)
}

func TestSortCRLFOnlyInSomeLines(t *testing.T) {
	// Mixed line endings: \r\n and \n. CR should be preserved per line.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\r\na\nc\r\n")
	stdout, _, code := cmdRun(t, "sort f.txt", dir)
	assert.Equal(t, 0, code)
	// "a" < "b\r" < "c\r" because \r (0x0D) comes after \n but a < b < c
	assert.Equal(t, "a\nb\r\nc\r\n", stdout)
}

func TestSortWhitespaceOnlyLinePreservedAsField(t *testing.T) {
	// A whitespace-only line should get a non-empty key for -k sorting.
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "\ta\n \n")
	stdout, _, code := cmdRun(t, "sort -k 1 f.txt", dir)
	assert.Equal(t, 0, code)
	// "\ta" (tab+a) before " " (space) — tab (0x09) < space (0x20)
	assert.Equal(t, "\ta\n \n", stdout)
}

func TestSortKeyCharOffsetZeroRejected(t *testing.T) {
	// -k1.0 should be rejected (character offset must be >= 1).
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "a\nb\n")
	_, stderr, code := cmdRun(t, "sort -k 1.0 f.txt", dir)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "sort:")
}

// --- Trailing blank field preservation ---

func TestSortTrailingBlankFieldPreserved(t *testing.T) {
	dir := t.TempDir()
	// "a\n" and "a \n" differ in field 2 (empty vs blank), so -u -k2 keeps both.
	writeFile(t, dir, "f.txt", "a\na \n")
	stdout, _, code := cmdRun(t, "sort -u -k2 f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na \n", stdout)
}

// --- Unique keeps first of equal ---

func TestSortUniqueKeepsFirstOfEqual(t *testing.T) {
	dir := t.TempDir()
	// All lines are numerically zero; -u must keep the first input line.
	writeFile(t, dir, "f.txt", "B\nA\nC\n")
	stdout, _, code := cmdRun(t, "sort -n -u f.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "B\n", stdout)
}

// --- Context cancellation ---
// Note: builtin-level cancellation (checkSorted, sortCmp, dedup) is tested
// via unit tests in cancellation_test.go. Integration tests through the
// runner cannot test pre-cancelled contexts because the runner's shouldStop
// method catches context cancellation before dispatching to the builtin.

func TestSortContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "b\na\nc\n")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _, code := runScriptCtx(ctx, t, "sort f.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}
