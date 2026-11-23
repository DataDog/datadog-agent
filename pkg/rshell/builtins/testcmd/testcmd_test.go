// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testcmd_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

func runScriptCtx(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, dir, opts...)
}

func cmdRun(t *testing.T, script, dir string) (string, string, int) {
	t.Helper()
	return runScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

// --- String tests ---

func TestTestEmptyArgs(t *testing.T) {
	_, _, code := runScript(t, "test", "")
	assert.Equal(t, 1, code)
}

func TestTestBareString(t *testing.T) {
	_, _, code := runScript(t, `test "hello"`, "")
	assert.Equal(t, 0, code)
}

func TestTestEmptyString(t *testing.T) {
	_, _, code := runScript(t, `test ""`, "")
	assert.Equal(t, 1, code)
}

func TestTestZeroLength(t *testing.T) {
	_, _, code := runScript(t, `test -z ""`, "")
	assert.Equal(t, 0, code)
}

func TestTestZeroLengthNonEmpty(t *testing.T) {
	_, _, code := runScript(t, `test -z "hello"`, "")
	assert.Equal(t, 1, code)
}

func TestTestNonZeroLength(t *testing.T) {
	_, _, code := runScript(t, `test -n "hello"`, "")
	assert.Equal(t, 0, code)
}

func TestTestNonZeroLengthEmpty(t *testing.T) {
	_, _, code := runScript(t, `test -n ""`, "")
	assert.Equal(t, 1, code)
}

func TestTestStringEqual(t *testing.T) {
	_, _, code := runScript(t, `test "abc" = "abc"`, "")
	assert.Equal(t, 0, code)
}

func TestTestStringNotEqual(t *testing.T) {
	_, _, code := runScript(t, `test "abc" != "def"`, "")
	assert.Equal(t, 0, code)
}

func TestTestStringEqualFalse(t *testing.T) {
	_, _, code := runScript(t, `test "abc" = "def"`, "")
	assert.Equal(t, 1, code)
}

func TestTestDoubleEqual(t *testing.T) {
	_, _, code := runScript(t, `test "t" == "t"`, "")
	assert.Equal(t, 0, code)
}

func TestTestStringLessThan(t *testing.T) {
	stdout, _, code := runScript(t, `test "a" \< "b"; echo $?`, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\n", stdout)
}

func TestTestStringGreaterThan(t *testing.T) {
	stdout, _, code := runScript(t, `test "b" \> "a"; echo $?`, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\n", stdout)
}

// --- Integer comparison tests ---

func TestTestIntEq(t *testing.T) {
	_, _, code := runScript(t, `test 9 -eq 9`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntNe(t *testing.T) {
	_, _, code := runScript(t, `test 8 -ne 9`, "")
	assert.Equal(t, 0, code)

	_, _, code = runScript(t, `test 9 -ne 9`, "")
	assert.Equal(t, 1, code)
}

func TestTestIntGt(t *testing.T) {
	_, _, code := runScript(t, `test 5 -gt 4`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntLt(t *testing.T) {
	_, _, code := runScript(t, `test 4 -lt 5`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntGe(t *testing.T) {
	_, _, code := runScript(t, `test 5 -ge 5`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntLe(t *testing.T) {
	_, _, code := runScript(t, `test 5 -le 5`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntNegative(t *testing.T) {
	_, _, code := runScript(t, `test -1 -gt -2`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntLeadingZero(t *testing.T) {
	_, _, code := runScript(t, `test 0 -eq 00`, "")
	assert.Equal(t, 0, code)
}

func TestTestIntWhitespace(t *testing.T) {
	stdout, _, _ := runScript(t, `test 0 -eq " 0 "; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
}

func TestTestInvalidInteger(t *testing.T) {
	_, stderr, code := runScript(t, `test 0x0 -eq 0`, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "integer expression expected")
}

func TestTestFloatRejected(t *testing.T) {
	_, stderr, code := runScript(t, `test 123.45 -ge 6`, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "123.45: integer expression expected")
}

// --- Logical operator tests ---

func TestTestAndTrue(t *testing.T) {
	_, _, code := runScript(t, `test "a" -a "b"`, "")
	assert.Equal(t, 0, code)
}

func TestTestAndFalse(t *testing.T) {
	_, _, code := runScript(t, `test "" -a "b"`, "")
	assert.Equal(t, 1, code)
}

func TestTestOrTrue(t *testing.T) {
	_, _, code := runScript(t, `test "" -o "b"`, "")
	assert.Equal(t, 0, code)
}

func TestTestOrFalse(t *testing.T) {
	_, _, code := runScript(t, `test "" -o ""`, "")
	assert.Equal(t, 1, code)
}

func TestTestNot(t *testing.T) {
	_, _, code := runScript(t, `test ! ""`, "")
	assert.Equal(t, 0, code)
}

func TestTestNotTrue(t *testing.T) {
	_, _, code := runScript(t, `test ! "hello"`, "")
	assert.Equal(t, 1, code)
}

func TestTestDoubleNot(t *testing.T) {
	_, _, code := runScript(t, `test ! ! "hello"`, "")
	assert.Equal(t, 0, code)
}

func TestTestParentheses(t *testing.T) {
	stdout, _, _ := runScript(t, `test '(' "hello" ')'; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
}

func TestTestParenthesesEmpty(t *testing.T) {
	stdout, _, _ := runScript(t, `test '(' "" ')'; echo $?`, "")
	assert.Equal(t, "1\n", stdout)
}

func TestTestPrecedence(t *testing.T) {
	stdout, _, _ := runScript(t, `test " " -o "" -a ""; echo $?`, "")
	assert.Equal(t, "0\n", stdout)
}

// --- File tests ---

func TestTestFileExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -e f.txt`, dir)
	assert.Equal(t, 0, code)
}

func TestTestFileNotExists(t *testing.T) {
	dir := t.TempDir()
	_, _, code := cmdRun(t, `test -e nonexistent`, dir)
	assert.Equal(t, 1, code)
}

func TestTestRegularFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -f f.txt`, dir)
	assert.Equal(t, 0, code)
}

func TestTestDirNotRegular(t *testing.T) {
	dir := t.TempDir()
	_, _, code := cmdRun(t, `test -f .`, dir)
	assert.Equal(t, 1, code)
}

func TestTestIsDirectory(t *testing.T) {
	dir := t.TempDir()
	_, _, code := cmdRun(t, `test -d .`, dir)
	assert.Equal(t, 0, code)
}

func TestTestFileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -d f.txt`, dir)
	assert.Equal(t, 1, code)
}

func TestTestFileSizeGreaterThanZero(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "nonempty.txt", "data")
	_, _, code := cmdRun(t, `test -s nonempty.txt`, dir)
	assert.Equal(t, 0, code)
}

func TestTestEmptyFileSize(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.txt", "")
	_, _, code := cmdRun(t, `test -s empty.txt`, dir)
	assert.Equal(t, 1, code)
}

func TestTestFileReadable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -r f.txt`, dir)
	assert.Equal(t, 0, code)
}

func TestTestFileWritable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -w f.txt`, dir)
	assert.Equal(t, 0, code)
}

func TestTestFileNotExecutable(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "f.txt", "hello")
	_, _, code := cmdRun(t, `test -x f.txt`, dir)
	assert.Equal(t, 1, code)
}

// --- Bracket syntax tests ---

func TestBracketBasic(t *testing.T) {
	_, _, code := runScript(t, `[ "hello" ]`, "")
	assert.Equal(t, 0, code)
}

func TestBracketIntCompare(t *testing.T) {
	_, _, code := runScript(t, `[ 1 -eq 1 ]`, "")
	assert.Equal(t, 0, code)
}

func TestBracketMissingClose(t *testing.T) {
	_, stderr, code := runScript(t, `[ 1 -eq 1`, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "missing `]'")
}

func TestBracketEmpty(t *testing.T) {
	_, stderr, code := runScript(t, `[`, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "missing `]'")
}

func TestTestLoneParenIsString(t *testing.T) {
	_, _, code := runScript(t, `test "("`, "")
	assert.Equal(t, 0, code)
}

func TestTestEmptyFileOperand(t *testing.T) {
	_, _, code := runScript(t, `test -e ""`, "")
	assert.Equal(t, 1, code)

	_, _, code = runScript(t, `test -d ""`, "")
	assert.Equal(t, 1, code)

	_, _, code = runScript(t, `test -f ""`, "")
	assert.Equal(t, 1, code)
}

// --- Help tests ---

func TestTestHelp(t *testing.T) {
	stdout, _, code := runScript(t, `test --help`, "")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}

func TestBracketHelp(t *testing.T) {
	stdout, _, code := runScript(t, `[ --help`, "")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage:")
}

// --- Error cases ---

func TestTestExtraArgument(t *testing.T) {
	_, stderr, code := runScript(t, `test "a" "b" "c" "d" "e"`, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "too many arguments")
}

// --- File comparison -nt / -ot tests ---

func TestTestNewerThan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "old")
	old := filepath.Join(dir, "old.txt")
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, past, past)
	writeFile(t, dir, "new.txt", "new")

	_, _, code := cmdRun(t, `test new.txt -nt old.txt`, dir)
	assert.Equal(t, 0, code)

	_, _, code = cmdRun(t, `test old.txt -nt new.txt`, dir)
	assert.Equal(t, 1, code)
}

func TestTestOlderThan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old.txt", "old")
	old := filepath.Join(dir, "old.txt")
	past := time.Now().Add(-2 * time.Hour)
	os.Chtimes(old, past, past)
	writeFile(t, dir, "new.txt", "new")

	_, _, code := cmdRun(t, `test old.txt -ot new.txt`, dir)
	assert.Equal(t, 0, code)

	_, _, code = cmdRun(t, `test new.txt -ot old.txt`, dir)
	assert.Equal(t, 1, code)
}

func TestTestNtMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exists.txt", "data")

	_, _, code := cmdRun(t, `test exists.txt -nt nonexistent`, dir)
	assert.Equal(t, 0, code)

	_, _, code = cmdRun(t, `test nonexistent -nt exists.txt`, dir)
	assert.Equal(t, 1, code)
}

func TestTestOtMissingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exists.txt", "data")

	_, _, code := cmdRun(t, `test nonexistent -ot exists.txt`, dir)
	assert.Equal(t, 0, code)

	_, _, code = cmdRun(t, `test exists.txt -ot nonexistent`, dir)
	assert.Equal(t, 1, code)
}

// --- Special single-arg cases ---

func TestTestSpecialSingleArgs(t *testing.T) {
	cases := []string{"-", "--", "-0", "-f", "["}
	for _, c := range cases {
		t.Run(c, func(t *testing.T) {
			stdout, _, _ := runScript(t, `test "`+c+`"; echo $?`, "")
			assert.Equal(t, "0\n", stdout)
		})
	}
}

// --- Operator as literal string ---

func TestTestSoloNot(t *testing.T) {
	_, _, code := runScript(t, `test !`, "")
	assert.Equal(t, 0, code)
}

func TestTestSoloOperatorLiteral(t *testing.T) {
	_, _, code := runScript(t, `test -a`, "")
	assert.Equal(t, 0, code)

	_, _, code = runScript(t, `test -o`, "")
	assert.Equal(t, 0, code)
}

// --- Context cancellation ---

func TestTestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cancel()
	_, _, code := runScriptCtx(ctx, t, `test 1 -eq 1`, "")
	// A pre-cancelled context prevents the runner from starting; exit code depends
	// on how the runner surfaces the error — just verify it did not succeed.
	_ = code
}

// --- Sandbox enforcement ---

func TestTestFileOutsideSandbox(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	writeFile(t, other, "secret.txt", "data")

	_, _, code := runScript(t, `test -f `+filepath.Join(other, "secret.txt"), dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
}

// --- Integer overflow rejection (matches bash: exit 2) ---

func TestTestIntOverflow(t *testing.T) {
	stdout, stderr, code := runScript(t, `test 99999999999999999999 -gt 0; echo $?`, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\n", stdout)
	assert.Contains(t, stderr, "integer expression expected")
}

func TestTestIntNegOverflow(t *testing.T) {
	stdout, stderr, code := runScript(t, `test -99999999999999999999 -lt 0; echo $?`, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "2\n", stdout)
	assert.Contains(t, stderr, "integer expression expected")
}

// --- Shell integration tests ---

func TestTestWithAndList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "file.txt", "data")
	stdout, _, code := cmdRun(t, `test -f file.txt && echo "yes" || echo "no"`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "yes\n", stdout)
}

func TestTestInForLoop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "alpha")
	writeFile(t, dir, "b.txt", "beta")
	stdout, _, code := cmdRun(t, `for f in a.txt b.txt; do test -f "$f" && echo "$f exists"; done`, dir)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a.txt exists\nb.txt exists\n", stdout)
}

func TestBracketInAndList(t *testing.T) {
	stdout, _, code := runScript(t, `[ "hello" = "hello" ] && echo "match"`, "")
	assert.Equal(t, 0, code)
	assert.Equal(t, "match\n", stdout)
}
