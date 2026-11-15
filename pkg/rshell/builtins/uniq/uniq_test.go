// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package uniq_test

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

// --- Default behaviour ---

func TestUniqEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	stdout, _, code := cmdRun(t, "uniq empty.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestUniqAdjacentDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

func TestUniqNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

func TestUniqDifferentLines(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestUniqMixedDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestUniqAllUnique(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\nc\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestUniqNonAdjacentDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "b\na\na\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "b\na\n", stdout)
}

// --- -c / --count ---

func TestUniqCountBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\n")
	stdout, _, code := cmdRun(t, "uniq -c in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      1 a\n      1 b\n", stdout)
}

func TestUniqCountDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -c in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      2 a\n", stdout)
}

func TestUniqCountLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --count in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "      2 a\n      1 b\n", stdout)
}

// --- -d / --repeated ---

func TestUniqRepeatedBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -d in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

func TestUniqRepeatedNone(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\n")
	stdout, _, code := cmdRun(t, "uniq -d in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestUniqRepeatedNonAdjacent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\na\n")
	stdout, _, code := cmdRun(t, "uniq -d in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -u / --unique ---

func TestUniqUniqueBasic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -u in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestUniqUniqueAllUnique(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\n")
	stdout, _, code := cmdRun(t, "uniq -u in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestUniqUniqueMixed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\na\n")
	stdout, _, code := cmdRun(t, "uniq -u in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\na\n", stdout)
}

// --- -d -u combined ---

func TestUniqRepeatedAndUniqueSuppressAll(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n\x08")
	stdout, _, code := cmdRun(t, "uniq -d -u in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// --- -i / --ignore-case ---

func TestUniqIgnoreCase(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "A\na\n")
	stdout, _, code := cmdRun(t, "uniq -i in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestUniqIgnoreCaseLongForm(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "A\na\n")
	stdout, _, code := cmdRun(t, "uniq --ignore-case in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestUniqCaseSensitiveDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "A\na\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\na\n", stdout)
}

// --- -f / --skip-fields ---

func TestUniqSkipFields1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a\nb a\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a\n", stdout)
}

func TestUniqSkipFields1DifferentAfterField(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a\nb b\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a\nb b\n", stdout)
}

func TestUniqSkipFieldsTabVsSpace(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\ta\na a\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\ta\na a\n", stdout)
}

func TestUniqSkipFields2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a c\nb a c\n")
	stdout, _, code := cmdRun(t, "uniq -f 2 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a c\n", stdout)
}

// --- -s / --skip-chars ---

func TestUniqSkipChars1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "aaa\naaa\n")
	stdout, _, code := cmdRun(t, "uniq -s 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "aaa\n", stdout)
}

func TestUniqSkipChars2(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "baa\naaa\n")
	stdout, _, code := cmdRun(t, "uniq -s 2 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "baa\n", stdout)
}

func TestUniqSkipChars4ShortLine(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "abc\nabcd\n")
	stdout, _, code := cmdRun(t, "uniq -s 4 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\n", stdout)
}

// --- -w / --check-chars ---

func TestUniqCheckChars0(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "abc\nabcd\n")
	stdout, _, code := cmdRun(t, "uniq -w 0 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "abc\n", stdout)
}

func TestUniqCheckChars1(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a\nb a\n")
	stdout, _, code := cmdRun(t, "uniq -w 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a\nb a\n", stdout)
}

func TestUniqCheckCharsWithSkipFields(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a a\nb a c\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 -w 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a a\n", stdout)
}

// --- -z / --zero-terminated ---

func TestUniqZeroTerminated(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\x00a\x00b")
	stdout, _, code := cmdRun(t, "uniq -z in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\x00b\x00", stdout)
}

func TestUniqZeroTerminatedNewlinesPreserved(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -z in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\x00", stdout)
}

// --- -D / --all-repeated ---

func TestUniqAllRepeatedDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -D in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n", stdout)
}

func TestUniqAllRepeatedSeparate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nc\nc\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nc\nc\n", stdout)
}

func TestUniqAllRepeatedPrepend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n", stdout)
}

func TestUniqAllRepeatedPrependMultiple(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nc\nc\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nc\nc\n", stdout)
}

func TestUniqAllRepeatedNoneOnUniqueInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\nb\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestUniqAllRepeatedNoneMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nb\n")
	stdout, _, code := cmdRun(t, "uniq -D in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\nb\nb\n", stdout)
}

func TestUniqAllRepeatedSeparateMultipleGroups(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\nb\nc\n")
	stdout, _, code := cmdRun(t, "uniq --all-repeated=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\nb\n", stdout)
}

func TestUniqAllRepeatedWithCheckChars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a a\na b\n")
	stdout, _, code := cmdRun(t, "uniq -D -w1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a a\na b\n", stdout)
}

// --- --group ---

func TestUniqGroupSeparate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\n", stdout)
}

func TestUniqGroupPrepend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nb\n", stdout)
}

func TestUniqGroupAppend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=append in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\n\n", stdout)
}

func TestUniqGroupBoth(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group=both in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n\nb\n\n", stdout)
}

func TestUniqGroupDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq --group in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\nb\n", stdout)
}

func TestUniqGroupEmptyInput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "")
	stdout, _, code := cmdRun(t, "uniq --group in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestUniqGroupSingleGroup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq --group=prepend in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\na\na\n", stdout)
}

func TestUniqGroupSingleGroupAppend(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq --group=append in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n\n", stdout)
}

func TestUniqGroupSingleGroupSeparate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq --group=separate in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\na\n", stdout)
}

// --- Mutual exclusion errors ---

func TestUniqGroupWithCount(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq --group -c in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestUniqGroupWithRepeated(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq --group -d in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestUniqGroupWithUnique(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq --group -u in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestUniqGroupWithAllRepeated(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq --group -D in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestUniqAllRepeatedWithCount(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq -D -c in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "meaningless")
}

// --- Help ---

func TestUniqHelp(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "uniq --help", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Empty(t, stderr)
}

func TestUniqHelpShort(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := cmdRun(t, "uniq -h", dir)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
	assert.Empty(t, stderr)
}

// --- Error cases ---

func TestUniqMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq nonexistent.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uniq:")
}

func TestUniqUnknownFlag(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := cmdRun(t, "uniq --no-such-flag in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "uniq:")
}

func TestUniqExtraOperand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a\n")
	writeFile(t, dir, "b.txt", "b\n")
	_, stderr, code := cmdRun(t, "uniq a.txt b.txt", dir)
	assert.Equal(t, 1, code)
	assert.Equal(t, "uniq: extra operand 'b.txt'\nTry 'uniq --help' for more information.\n", stderr)
}

// TestUniqExtraOperandEscapesControlBytes verifies that a crafted extra
// operand containing a newline and an ANSI escape sequence cannot forge
// additional stderr lines or inject terminal control sequences — the
// dangerous bytes must come through escaped, not raw.
func TestUniqExtraOperandEscapesControlBytes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "a\n")
	script := "uniq a.txt \"evil\nrshell: forged line\x1b[31m\"\n"
	_, stderr, code := cmdRun(t, script, dir)
	assert.Equal(t, 1, code)
	assert.NotContains(t, stderr, "evil\nrshell: forged line")
	assert.NotContains(t, stderr, "\x1b[31m")
	assert.Contains(t, stderr, `evil\nrshell: forged line\x1b[31m`)
}

func TestUniqInvalidAllRepeatedMethod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\n")
	_, stderr, code := cmdRun(t, "uniq --all-repeated=badoption in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid argument")
}

func TestUniqInvalidGroupMethod(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\n")
	_, stderr, code := cmdRun(t, "uniq --group=badoption in.txt", dir)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid argument")
}

// --- Stdin ---

func TestUniqStdinPipe(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestUniqStdinDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "src.txt", "a\na\nb\n")
	stdout, _, code := cmdRun(t, "uniq - < src.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestUniqNilStdin(t *testing.T) {
	dir := t.TempDir()
	stdout, stderr, code := runScript(t, "uniq -", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
}

// --- Context cancellation ---

func TestUniqContextCancellation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\nb\n")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "uniq in.txt", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
}

// --- Null bytes ---

func TestUniqNullBytesInContent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\x00a\na\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\x00a\na\n", stdout)
}

// --- Combined skip fields + skip chars ---

func TestUniqSkipFieldsAndChars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a aaa\nb ab\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 -s 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a aaa\nb ab\n", stdout)
}

func TestUniqSkipFieldsAndCharsEqual(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a aaa\nb aaa\n")
	stdout, _, code := cmdRun(t, "uniq -f 1 -s 1 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a aaa\n", stdout)
}

// --- Double dash ---

func TestUniqDoubleDash(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "-f", "flag-looking-name\n")
	stdout, _, code := cmdRun(t, "uniq -- -f", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "flag-looking-name\n", stdout)
}

// --- Eight bit characters ---

func TestUniqEightBitChars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "ö\nv\n")
	stdout, _, code := cmdRun(t, "uniq in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ö\nv\n", stdout)
}

// --- Large count clamped ---

func TestUniqLargeSkipFieldsClamped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n")
	stdout, _, code := cmdRun(t, "uniq -f 9999999999 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\n", stdout)
}

func TestUniqOverflowCheckCharsClamped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "in.txt", "a\na\n\x08")
	stdout, _, code := cmdRun(t, "uniq -d -u -w340282366920938463463374607431768211456 in.txt", dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}
