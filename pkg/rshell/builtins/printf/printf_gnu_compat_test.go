// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package printf_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// GNU compatibility tests for printf.
//
// These tests verify byte-for-byte output equivalence with GNU coreutils
// printf (captured from bash on Debian bookworm). Each test documents the
// exact GNU invocation used to produce the reference output.

// TestGNUCompatSimpleString — basic string output.
//
// GNU command: printf "%s\n" hello
// Expected: "hello\n"
func TestGNUCompatSimpleString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

// TestGNUCompatFormatReuse — format reuse for excess arguments.
//
// GNU command: printf "%s\n" a b c
// Expected: "a\nb\nc\n"
func TestGNUCompatFormatReuse(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s\n" a b c`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb\nc\n", stdout)
}

// TestGNUCompatMissingArgs — missing args default to "" and 0.
//
// GNU command: printf "%s:%d\n" hello
// Expected: "hello:0\n"
func TestGNUCompatMissingArgs(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%s:%d\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello:0\n", stdout)
}

// TestGNUCompatPercentLiteral — %% produces a single %.
//
// GNU command: printf "100%%\n"
// Expected: "100%\n"
func TestGNUCompatPercentLiteral(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "100%%\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "100%\n", stdout)
}

// TestGNUCompatZeroPad — zero-padded integer.
//
// GNU command: printf "%05d\n" 42
// Expected: "00042\n"
func TestGNUCompatZeroPad(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%05d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "00042\n", stdout)
}

// TestGNUCompatWidthString — right-aligned string with width.
//
// GNU command: printf "%10s\n" hi
// Expected: "        hi\n"
func TestGNUCompatWidthString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%10s\n" hi`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "        hi\n", stdout)
}

// TestGNUCompatLeftAlign — left-aligned string.
//
// GNU command: printf "%-10s|\n" hi
// Expected: "hi        |\n"
func TestGNUCompatLeftAlign(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%-10s|\n" hi`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hi        |\n", stdout)
}

// TestGNUCompatPrecisionFloat — float with precision.
//
// GNU command: printf "%.2f\n" 3.14159
// Expected: "3.14\n"
func TestGNUCompatPrecisionFloat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%.2f\n" 3.14159`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.14\n", stdout)
}

// TestGNUCompatPrecisionString — string truncation with precision.
//
// GNU command: printf "%.3s\n" hello
// Expected: "hel\n"
func TestGNUCompatPrecisionString(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%.3s\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hel\n", stdout)
}

// TestGNUCompatOctalOutput — %o format.
//
// GNU command: printf "%o\n" 255
// Expected: "377\n"
func TestGNUCompatOctalOutput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%o\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "377\n", stdout)
}

// TestGNUCompatHexOutput — %x and %X format.
//
// GNU command: printf "%x %X\n" 255 255
// Expected: "ff FF\n"
func TestGNUCompatHexOutput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%x %X\n" 255 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "ff FF\n", stdout)
}

// TestGNUCompatScientific — %e format.
//
// GNU command: printf "%e\n" 3.14
// Expected: "3.140000e+00\n"
func TestGNUCompatScientific(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%e\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000e+00\n", stdout)
}

// TestGNUCompatShortestFloat — %g format.
//
// GNU command: printf "%g\n" 3.14
// Expected: "3.14\n"
func TestGNUCompatShortestFloat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%g\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.14\n", stdout)
}

// TestGNUCompatCharConstant — character constant argument.
//
// GNU command: printf "%d\n" "'A"
// Expected: "65\n"
func TestGNUCompatCharConstant(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" "'A"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "65\n", stdout)
}

// TestGNUCompatHexInput — hex input parsing.
//
// GNU command: printf "%d\n" 0xff
// Expected: "255\n"
func TestGNUCompatHexInput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 0xff`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "255\n", stdout)
}

// TestGNUCompatOctalInput — octal input parsing.
//
// GNU command: printf "%d\n" 0755
// Expected: "493\n"
func TestGNUCompatOctalInput(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%d\n" 0755`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "493\n", stdout)
}

// TestGNUCompatHashFlag — %#x adds 0x prefix.
//
// GNU command: printf "%#x\n" 255
// Expected: "0xff\n"
func TestGNUCompatHashFlag(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%#x\n" 255`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "0xff\n", stdout)
}

// TestGNUCompatPlusFlag — %+d adds sign.
//
// GNU command: printf "%+d\n" 42
// Expected: "+42\n"
func TestGNUCompatPlusFlag(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%+d\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "+42\n", stdout)
}

// TestGNUCompatInvalidNumber — non-numeric arg for %d.
//
// GNU command: printf "%d\n" abc
// Expected stdout: "0\n", stderr: "printf: 'abc': invalid number", exit code: 1
func TestGNUCompatInvalidNumber(t *testing.T) {
	stdout, stderr, code := cmdRun(t, `printf "%d\n" abc`)
	assert.Equal(t, 1, code)
	assert.Equal(t, "0\n", stdout)
	assert.Contains(t, stderr, "printf:")
}

// TestGNUCompatBSpecifierBackslashC — %b with \c stops output.
//
// GNU command: printf "%b" 'hello\cworld'
// Expected: "hello"
func TestGNUCompatBSpecifierBackslashC(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%b" 'hello\cworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

// TestGNUCompatEmptyFormat — empty format string.
//
// GNU command: printf ""
// Expected: ""
func TestGNUCompatEmptyFormat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf ""`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

// TestGNUCompatCharFirstOnly — %c takes only the first character.
//
// GNU command: printf "%c\n" hello
// Expected: "h\n"
func TestGNUCompatCharFirstOnly(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%c\n" hello`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "h\n", stdout)
}

// TestGNUCompatUnsigned — %u format.
//
// GNU command: printf "%u\n" 42
// Expected: "42\n"
func TestGNUCompatUnsigned(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%u\n" 42`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "42\n", stdout)
}

// TestGNUCompatDefaultFloat — %f default precision is 6.
//
// GNU command: printf "%f\n" 3.14
// Expected: "3.140000\n"
func TestGNUCompatDefaultFloat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "%f\n" 3.14`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "3.140000\n", stdout)
}

// TestGNUCompatOctalEscapeInFormat — \NNN in format string.
//
// GNU command: printf "\101\n"
// Expected: "A\n"
func TestGNUCompatOctalEscapeInFormat(t *testing.T) {
	stdout, _, code := cmdRun(t, `printf "\101\n"`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}
