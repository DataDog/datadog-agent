// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package echo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

func runScript(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, t.TempDir())
}

// --- Basic (no flags) ---

func TestEchoNoArgs(t *testing.T) {
	stdout, _, code := runScript(t, "echo")
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

func TestEchoSingleArg(t *testing.T) {
	stdout, _, code := runScript(t, "echo hello")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}

func TestEchoMultipleArgs(t *testing.T) {
	stdout, _, code := runScript(t, "echo hello world")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world\n", stdout)
}

func TestEchoEmptyString(t *testing.T) {
	stdout, _, code := runScript(t, `echo ""`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\n", stdout)
}

// --- -n flag ---

func TestEchoN(t *testing.T) {
	stdout, _, code := runScript(t, "echo -n hello")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

func TestEchoNNoArgs(t *testing.T) {
	stdout, _, code := runScript(t, "echo -n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "", stdout)
}

func TestEchoNMultipleArgs(t *testing.T) {
	stdout, _, code := runScript(t, "echo -n hello world")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello world", stdout)
}

// --- -e flag ---

func TestEchoENewline(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld\n", stdout)
}

func TestEchoETab(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'a\tb'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\tb\n", stdout)
}

func TestEchoEBackslash(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'a\\b'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\\b\n", stdout)
}

func TestEchoEAlert(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\a'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\a\n", stdout)
}

func TestEchoEBackspace(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\b'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\b\n", stdout)
}

func TestEchoEEscape(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\e'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x1b\n", stdout)
}

func TestEchoEEscapeUpperE(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\E'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x1b\n", stdout)
}

func TestEchoEFormFeed(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\f'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\f\n", stdout)
}

func TestEchoECarriageReturn(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\r'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\r\n", stdout)
}

func TestEchoEVerticalTab(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\v'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\v\n", stdout)
}

func TestEchoEC(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'hello\cworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

func TestEchoEOctal(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\0101'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestEchoEOctalZero(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\0'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x00\n", stdout)
}

func TestEchoEHex(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\x41'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestEchoEHexSingleDigit(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\x9'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\x09\n", stdout)
}

func TestEchoEHexNoDigits(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\xZZ'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\xZZ\n", stdout)
}

func TestEchoEUnicode4(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\u0041'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestEchoEUnicode8(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\U00000041'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "A\n", stdout)
}

func TestEchoEUnknownEscape(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e '\z'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "\\z\n", stdout)
}

// --- -E flag ---

func TestEchoEUpperDisablesEscapes(t *testing.T) {
	stdout, _, code := runScript(t, `echo -E 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\\nworld\n", stdout)
}

// --- Combined flags ---

func TestEchoNE(t *testing.T) {
	stdout, _, code := runScript(t, `echo -ne 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld", stdout)
}

func TestEchoEELastWins(t *testing.T) {
	stdout, _, code := runScript(t, `echo -eE 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\\nworld\n", stdout)
}

func TestEchoEeLastWins(t *testing.T) {
	stdout, _, code := runScript(t, `echo -Ee 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld\n", stdout)
}

func TestEchoMultipleFlagArgs(t *testing.T) {
	stdout, _, code := runScript(t, `echo -n -e 'hello\nworld'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\nworld", stdout)
}

// --- Flag parsing edge cases ---

func TestEchoInvalidFlagIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo -nxyz hello")
	assert.Equal(t, 0, code)
	assert.Equal(t, "-nxyz hello\n", stdout)
}

func TestEchoDoubleDashIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo -- hello")
	assert.Equal(t, 0, code)
	assert.Equal(t, "-- hello\n", stdout)
}

func TestEchoBareDashIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo - hello")
	assert.Equal(t, 0, code)
	assert.Equal(t, "- hello\n", stdout)
}

func TestEchoFlagAfterTextIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo hello -n")
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello -n\n", stdout)
}

func TestEchoHelpIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo --help")
	assert.Equal(t, 0, code)
	assert.Equal(t, "--help\n", stdout)
}

func TestEchoVersionIsLiteral(t *testing.T) {
	stdout, _, code := runScript(t, "echo --version")
	assert.Equal(t, 0, code)
	assert.Equal(t, "--version\n", stdout)
}

// --- Multiple -e args ---

func TestEchoEMultipleArgs(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'a\nb' 'c\nd'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nb c\nd\n", stdout)
}

// --- \c across args ---

func TestEchoECStopsAcrossArgs(t *testing.T) {
	stdout, _, code := runScript(t, `echo -e 'hello\c' 'world'`)
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello", stdout)
}

// --- Exit code ---

func TestEchoExitCodeAlwaysZero(t *testing.T) {
	_, _, code := runScript(t, "echo hello")
	assert.Equal(t, 0, code)
}
