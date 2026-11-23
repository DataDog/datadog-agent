// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package printf_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

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

// cmdRun runs a printf command (no file access needed).
func cmdRun(t *testing.T, script string) (stdout, stderr string, exitCode int) {
	t.Helper()
	return runScript(t, script, "")
}

// --- Basic functionality ---

func TestPrintfSimpleString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestPrintfNoArgs(t *testing.T) {
	_, stderr, code := cmdRun(t, `printf`)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfFormatOnly(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "hello world\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

func TestPrintfMultipleArgs(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s %s\n" hello world`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

func TestPrintfFormatReuse(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s\n" a b c`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

func TestPrintfMissingArgString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s and %s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello and \n", stdout)
}

func TestPrintfMissingArgNumber(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d and %d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42 and 0\n", stdout)
}

func TestPrintfPercentLiteral(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "100%%\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "100%\n", stdout)
}

func TestPrintfEmptyFormat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf ""`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestPrintfNoNewline(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "hello"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

// --- Escape sequences ---

func TestPrintfEscapeNewline(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "a\nb\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\n", stdout)
}

func TestPrintfEscapeTab(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "a\tb\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\tb\n", stdout)
}

func TestPrintfEscapeBackslash(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "a\\\\b\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\\b\n", stdout)
}

func TestPrintfEscapeCarriageReturn(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "hello\rworld\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\rworld\n", stdout)
}

func TestPrintfEscapeOctal(t *testing.T) {
	// \101 = octal 101 = 65 = 'A'
	stdout, _, code := cmdRun(t, `printf "\101\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestPrintfEscapeHex(t *testing.T) {
	// \x41 = hex 41 = 65 = 'A'
	stdout, _, code := cmdRun(t, `printf "\x41\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestPrintfEscapeBell(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "\a"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\a", stdout)
}

func TestPrintfEscapeFormFeed(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "\f"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\f", stdout)
}

func TestPrintfEscapeVerticalTab(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "\v"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\v", stdout)
}

func TestPrintfEscapeBackspace(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "\b"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\b", stdout)
}

// --- Format specifiers ---

func TestPrintfSpecifierString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

func TestPrintfSpecifierChar(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%c\n" A`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestPrintfSpecifierCharEmpty(t *testing.T) {
	// Empty arg for %c should produce a NUL byte (bash behavior)
	stdout, _, code := cmdRun(t, `printf "%c" ""`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x00", stdout)
}

func TestPrintfSpecifierDecimal(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42\n", stdout)
}

func TestPrintfSpecifierInteger(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%i\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42\n", stdout)
}

func TestPrintfSpecifierOctal(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%o\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "377\n", stdout)
}

func TestPrintfSpecifierUnsigned(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%u\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42\n", stdout)
}

func TestPrintfSpecifierHexLower(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%x\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ff\n", stdout)
}

func TestPrintfSpecifierHexUpper(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%X\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "FF\n", stdout)
}

func TestPrintfSpecifierFloat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000\n", stdout)
}

func TestPrintfSpecifierScientific(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%e\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000e+00\n", stdout)
}

func TestPrintfSpecifierScientificUpper(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%E\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000E+00\n", stdout)
}

func TestPrintfSpecifierShortest(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%g\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.14\n", stdout)
}

func TestPrintfSpecifierShortestUpper(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%G\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.14\n", stdout)
}

func TestPrintfSpecifierFloatF(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%F\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000\n", stdout)
}

// --- %b specifier ---

func TestPrintfSpecifierBEscapes(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b\n" 'hello\tworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\tworld\n", stdout)
}

func TestPrintfSpecifierBBackslashC(t *testing.T) {
	// \c stops all output
	stdout, _, code := cmdRun(t, `printf "%b" 'hello\cworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

func TestPrintfSpecifierBOctal(t *testing.T) {
	// %b uses \0NNN (with leading zero) for octal
	stdout, _, code := cmdRun(t, `printf "%b\n" '\0101'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

// --- Width and precision ---

func TestPrintfWidthRightAlign(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%10s\n" hi`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "        hi\n", stdout)
}

func TestPrintfWidthLeftAlign(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%-10s|\n" hi`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi        |\n", stdout)
}

func TestPrintfWidthZeroPad(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%05d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "00042\n", stdout)
}

func TestPrintfPrecisionFloat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%.2f\n" 3.14159`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.14\n", stdout)
}

func TestPrintfPrecisionString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%.3s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hel\n", stdout)
}

func TestPrintfWidthAndPrecision(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%10.3s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "       hel\n", stdout)
}

func TestPrintfFlagPlus(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%+d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "+42\n", stdout)
}

func TestPrintfFlagSpace(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "% d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, " 42\n", stdout)
}

func TestPrintfFlagHash(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%#x\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0xff\n", stdout)
}

func TestPrintfFlagHashOctal(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%#o\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0377\n", stdout)
}

// --- Numeric argument formats ---

func TestPrintfNumericNegative(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" -42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "-42\n", stdout)
}

func TestPrintfNumericHexInput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 0xff`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "255\n", stdout)
}

func TestPrintfNumericOctalInput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 0755`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "493\n", stdout)
}

func TestPrintfNumericCharConstant(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" "'A"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "65\n", stdout)
}

func TestPrintfNumericZero(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 0`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0\n", stdout)
}

// --- Error handling ---

func TestPrintfInvalidNumber(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%d\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfRejectedPercentN(t *testing.T) {
	_, stderr, code := cmdRun(t, `printf "%n" foo`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfRejectedVFlag(t *testing.T) {
	_, stderr, code := cmdRun(t, `printf -v var "%s" hello`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "printf:")
}

// --- Help ---

func TestPrintfHelp(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf --help`)
	assert.Equal(t, 2, code)
	assert.Contains(t, stdout, "printf: usage:")
}

func TestPrintfHelpShort(t *testing.T) {
	// -h is not a valid flag in bash; it's rejected with exit 2
	_, stderr, code := cmdRun(t, `printf -h`)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "invalid option")
}

// --- Format reuse edge cases ---

func TestPrintfFormatReuseMultipleSpecifiers(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s=%d\n" a 1 b 2 c 3`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a=1\nb=2\nc=3\n", stdout)
}

func TestPrintfFormatReusePartialFill(t *testing.T) {
	// When format has 2 specifiers but odd number of extra args
	stdout, _, code := cmdRun(t, `printf "%s=%d\n" a 1 b`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a=1\nb=0\n", stdout)
}

func TestPrintfNoSpecifiers(t *testing.T) {
	// Format with no specifiers and extra args — format is still printed
	// but args are not consumed (no specifiers to consume them)
	stdout, _, code := cmdRun(t, `printf "hello\n" extra args`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// --- Shell integration ---

func TestPrintfInPipeline(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s\n" hello | cat`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestPrintfInForLoop(t *testing.T) {
	stdout, _, code := cmdRun(t, `for i in 1 2 3; do printf "%d " "$i"; done; printf "\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "1 2 3 \n", stdout)
}

func TestPrintfVariableExpansion(t *testing.T) {
	stdout, _, code := cmdRun(t, `NAME=world; printf "hello %s\n" "$NAME"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

func TestPrintfZeroPaddedInt(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%05d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "00042\n", stdout)
}

// --- Context cancellation ---

func TestPrintfContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Large format reuse should respect context cancellation
	// This script tries to print many items but should be bounded
	_, _, code := runScriptCtx(ctx, t, `printf "%s\n" a b c d e f g h i j`, "")
	assert.Equal(t, 0, code)
}

// --- Double-dash separator ---

func TestPrintfDoubleDash(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf -- "%s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// --- Octal escape edge cases ---

func TestPrintfEscapeOctalZeroPrefix(t *testing.T) {
	// \0101: the leading 0 counts as the first of 3 octal digits,
	// so \010 = backspace (octal 010 = 8), then literal '1'.
	// This matches bash behavior.
	stdout, _, code := cmdRun(t, `printf "\0101\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x081\n", stdout)
}

func TestPrintfEscapeOctalNulByte(t *testing.T) {
	// \0 alone = NUL byte
	stdout, _, code := cmdRun(t, `printf "a\0b"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\x00b", stdout)
}

// --- Mixed format string and args ---

func TestPrintfMixedText(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "Name: %s, Age: %d\n" Alice 30`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "Name: Alice, Age: 30\n", stdout)
}

func TestPrintfMultiplePercent(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d%%\n" 100`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "100%\n", stdout)
}

// --- Coverage: rejected specifiers ---

func TestPrintfRejectedQ(t *testing.T) {
	_, stderr, code := cmdRun(t, `printf "%q" hello`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfRejectedA(t *testing.T) {
	_, stderr, code := cmdRun(t, `printf "%a" 3.14`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "printf:")
}

// --- Coverage: unknown specifier ---

func TestPrintfUnknownSpecifier(t *testing.T) {
	// Bash stops processing format string after unknown specifier — no \n output.
	stdout, stderr, code := cmdRun(t, `printf "%z\n"`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "invalid format character")
}

// --- Coverage: escape edge cases ---

func TestPrintfEscapeDoubleQuote(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf '\"hello\"'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\"hello\"", stdout)
}

func TestPrintfEscapeUnknown(t *testing.T) {
	// Unknown escape should output backslash and character
	stdout, _, code := cmdRun(t, `printf '\q'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\q", stdout)
}

func TestPrintfTrailingBackslash(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf 'hello\'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\\", stdout)
}

// --- Coverage: %b escape sequences ---

func TestPrintfBEscapeTab(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" 'a\tb'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\tb", stdout)
}

func TestPrintfBEscapeNewline(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" 'a\nb'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb", stdout)
}

func TestPrintfBEscapeBackslash(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" 'a\\b'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\\b", stdout)
}

func TestPrintfBEscapeHex(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\x41'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A", stdout)
}

func TestPrintfBEscapeHexInvalid(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\xZZ'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\xZZ", stdout)
}

func TestPrintfBEscapeBell(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\a'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\a", stdout)
}

func TestPrintfBEscapeFormFeed(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\f'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\f", stdout)
}

func TestPrintfBEscapeCarriageReturn(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\r'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\r", stdout)
}

func TestPrintfBEscapeVerticalTab(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\v'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\v", stdout)
}

func TestPrintfBEscapeBackspace(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\b'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\b", stdout)
}

func TestPrintfBEscapeUnknown(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" '\q'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\q", stdout)
}

// --- Coverage: parseFloatArg ---

func TestPrintfFloatHexInput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" 0xff`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "255.000000\n", stdout)
}

func TestPrintfFloatInfinity(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" inf`)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "inf")
}

func TestPrintfFloatNegInfinity(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" -inf`)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "-inf")
}

func TestPrintfFloatCharConstant(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" "'A"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "65.000000\n", stdout)
}

func TestPrintfFloatInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%f\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0.000000\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

// --- Coverage: parseUintArg ---

func TestPrintfUnsignedCharConstant(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%u\n" "'A"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "65\n", stdout)
}

func TestPrintfUnsignedInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%u\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfOctalInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%o\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfHexInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%x\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfHexUpperInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%X\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

// --- Coverage: float specifiers errors ---

func TestPrintfScientificInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%e\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0.000000e+00\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfScientificUpperInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%E\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0.000000E+00\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfShortestInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%g\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfShortestUpperInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%G\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

func TestPrintfFloatFUpperInvalid(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%F\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0.000000\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

// --- Coverage: incomplete specifier ---

func TestPrintfIncompleteSpecifier(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%"`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "missing format character")
}

// --- Coverage: hex escape in format with no valid digits ---

func TestPrintfHexEscapeNoDigits(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf '\xZZ'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\xZZ", stdout)
}

// --- Coverage: width clamping ---

func TestPrintfWidthClamped(t *testing.T) {
	// Very large width should be clamped, not cause OOM
	stdout, _, code := cmdRun(t, `printf "%99999s\n" hi`)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "hi")
	// Width clamped to 10000
	assert.LessOrEqual(t, len(stdout), 10002)
}

// --- Coverage: negative width clamping ---

func TestPrintfNegativeWidthClamped(t *testing.T) {
	// Very large negative width should be clamped to -10000
	stdout, _, code := cmdRun(t, `printf "%-99999s|\n" hi`)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "hi")
	assert.LessOrEqual(t, len(stdout), 10003) // 10000 + |+ \n
}

// --- Coverage: precision clamping boundary ---

func TestPrintfPrecisionClamped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, code := runScriptCtx(ctx, t, `printf "%.99999s\n" hello`, "")
	assert.Equal(t, 0, code)
	// Precision on strings truncates; clamped to 10000 but "hello" is only 5 chars
	assert.Equal(t, "hello\n", stdout)
}

// NOTE: unsigned negative wrapping, double-quote char constants, %b escapes,
// octal/hex truncation, incomplete specifiers, conflicting flags, star
// width/precision with zero — all covered by YAML scenario tests in
// tests/scenarios/cmd/printf/

// --- Coverage: star width/precision clamping ---

func TestPrintfStarWidthClamped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, code := runScriptCtx(ctx, t, `printf "%*d\n" 99999 42`, "")
	assert.Equal(t, 0, code)
	assert.LessOrEqual(t, len(stdout), 10002)
	assert.Contains(t, stdout, "42")
}

func TestPrintfStarPrecisionClamped(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, _, code := runScriptCtx(ctx, t, `printf "%.*f\n" 99999 3.14`, "")
	assert.Equal(t, 0, code)
	assert.LessOrEqual(t, len(stdout), 10010)
}

// NOTE: %c multi-byte, NaN case, empty arg with width, octal digits 8/9,
// %F uppercase inf/nan, zero-padded scientific, %b \c stops reuse —
// all covered by YAML scenario tests in tests/scenarios/cmd/printf/

// --- Coverage: format reuse iteration limit ---

func TestPrintfFormatReuseIterationLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Supply more than 10000 args while staying below the shell-wide argument
	// limit: format reuse should still stop at 10000 iterations.
	args := strings.Repeat("x ", 12000)
	stdout, _, code := runScriptCtx(ctx, t, `printf "%s" `+args, "")
	assert.Equal(t, 0, code)
	// Should produce at most 10001 x's (first pass + 10000 iterations)
	// Actually the first x is consumed in the first pass, then 10000 more iterations
	assert.LessOrEqual(t, len(stdout), 10001)
}

// --- Coverage: context cancellation actually stops loop ---

func TestPrintfContextCancellationStopsLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// Try to print a very large number of args; timeout should kill it
	args := strings.Repeat("x ", 100000)
	_, _, _ = runScriptCtx(ctx, t, `printf "%s" `+args, "")
	// We only care that it didn't hang — the timeout handled it
}

// NOTE: unsigned large hex, star width float, star width/precision empty —
// all covered by YAML scenario tests in tests/scenarios/cmd/printf/
