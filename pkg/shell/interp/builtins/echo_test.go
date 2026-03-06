// Copyright (c) Datadog, Inc.
// See LICENSE for licensing information

package builtins

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func runEcho(args []string) (stdout string, r Result) {
	var out bytes.Buffer
	callCtx := &CallContext{Stdout: &out, Stderr: &bytes.Buffer{}}
	r = builtinEcho(context.Background(), callCtx, args)
	return out.String(), r
}

func TestEcho_NoArgs(t *testing.T) {
	out, r := runEcho(nil)
	assert.Equal(t, "\n", out)
	assert.Equal(t, uint8(0), r.Code)
}

func TestEcho_Simple(t *testing.T) {
	out, _ := runEcho([]string{"hello"})
	assert.Equal(t, "hello\n", out)
}

func TestEcho_MultipleArgs(t *testing.T) {
	out, _ := runEcho([]string{"hello", "world"})
	assert.Equal(t, "hello world\n", out)
}

func TestEcho_EmptyStringArg(t *testing.T) {
	out, _ := runEcho([]string{""})
	assert.Equal(t, "\n", out)
}

// --- Flag parsing ---

func TestEcho_FlagN(t *testing.T) {
	out, _ := runEcho([]string{"-n", "hello"})
	assert.Equal(t, "hello", out)
}

func TestEcho_FlagE(t *testing.T) {
	out, _ := runEcho([]string{"-e", "hello"})
	assert.Equal(t, "hello\n", out)
}

func TestEcho_FlagE_WithEscape(t *testing.T) {
	out, _ := runEcho([]string{"-e", `hello\nworld`})
	assert.Equal(t, "hello\nworld\n", out)
}

func TestEcho_FlagBigE(t *testing.T) {
	out, _ := runEcho([]string{"-E", "hello"})
	assert.Equal(t, "hello\n", out)
}

func TestEcho_FlagNE(t *testing.T) {
	out, _ := runEcho([]string{"-ne", `hello\tworld`})
	assert.Equal(t, "hello\tworld", out)
}

func TestEcho_FlagEN(t *testing.T) {
	out, _ := runEcho([]string{"-en", `a\nb`})
	assert.Equal(t, "a\nb", out)
}

func TestEcho_FlagEE_DisablesEscapes(t *testing.T) {
	out, _ := runEcho([]string{"-eE", `hello\nworld`})
	assert.Equal(t, `hello\nworld`+"\n", out)
}

func TestEcho_FlagNEE(t *testing.T) {
	out, _ := runEcho([]string{"-nEe", `a\tb`})
	assert.Equal(t, "a\tb", out)
}

func TestEcho_MultipleFlagArgs(t *testing.T) {
	out, _ := runEcho([]string{"-n", "-e", `a\nb`})
	assert.Equal(t, "a\nb", out)
}

func TestEcho_InvalidFlagTreatedAsLiteral(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"dash_a", []string{"-a", "hello"}, "-a hello\n"},
		{"dash_dash", []string{"--", "hello"}, "-- hello\n"},
		{"single_dash", []string{"-", "hello"}, "- hello\n"},
		{"dash_nx", []string{"-nx", "hello"}, "-nx hello\n"},
		{"empty_dash", []string{"-"}, "-\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := runEcho(tt.args)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestEcho_FlagAfterNonFlag(t *testing.T) {
	out, _ := runEcho([]string{"hello", "-n", "world"})
	assert.Equal(t, "hello -n world\n", out)
}

// --- Escape sequences (with -e) ---

func TestEcho_Escapes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"alert", `\a`, "\a"},
		{"backspace", `\b`, "\b"},
		{"escape_lower", `\e`, "\x1b"},
		{"escape_upper", `\E`, "\x1b"},
		{"formfeed", `\f`, "\f"},
		{"newline", `\n`, "\n"},
		{"carriage_return", `\r`, "\r"},
		{"tab", `\t`, "\t"},
		{"vertical_tab", `\v`, "\v"},
		{"backslash", `\\`, "\\"},
		{"multiple", `a\tb\nc`, "a\tb\nc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := runEcho([]string{"-ne", tt.input})
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestEcho_EscapeC_StopsOutput(t *testing.T) {
	out, _ := runEcho([]string{"-e", `hello\cworld`})
	assert.Equal(t, "hello", out)
}

func TestEcho_EscapeC_StopsBeforeNextArg(t *testing.T) {
	out, _ := runEcho([]string{"-e", `first\c`, "second"})
	assert.Equal(t, "first", out)
}

func TestEcho_UnknownEscape(t *testing.T) {
	out, _ := runEcho([]string{"-ne", `\z`})
	assert.Equal(t, `\z`, out)
}

func TestEcho_TrailingBackslash(t *testing.T) {
	out, _ := runEcho([]string{"-ne", `hello\`})
	assert.Equal(t, `hello\`, out)
}

func TestEcho_NoEscapesWithoutFlag(t *testing.T) {
	out, _ := runEcho([]string{`hello\nworld`})
	assert.Equal(t, `hello\nworld`+"\n", out)
}

// --- Octal escapes ---

func TestEcho_Octal(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"null", `\0`, "\x00"},
		{"one_digit", `\01`, "\x01"},
		{"two_digits", `\012`, "\n"},
		{"three_digits", `\0101`, "A"},
		{"max_three_digits", `\0377`, "\xff"},
		{"octal_with_trailing", `\0101B`, "AB"},
		{"no_octal_digits", `\0hello`, "\x00hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := runEcho([]string{"-ne", tt.input})
			assert.Equal(t, tt.want, out)
		})
	}
}

// --- Hex escapes ---

func TestEcho_Hex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"one_digit", `\x41`, "A"},
		{"lowercase", `\x6f`, "o"},
		{"uppercase", `\x6F`, "o"},
		{"two_digit", `\xff`, "\xff"},
		{"single_digit", `\xA`, "\n"},
		{"with_trailing", `\x41BC`, "ABC"},
		{"no_hex_digits", `\xzz`, `\xzz`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, _ := runEcho([]string{"-ne", tt.input})
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestEcho_AlwaysReturnsZero(t *testing.T) {
	_, r := runEcho([]string{"-e", `\c`})
	assert.Equal(t, uint8(0), r.Code)
	assert.False(t, r.Exiting)
}
