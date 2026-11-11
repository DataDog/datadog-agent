// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package echo implements the echo builtin command.
//
// echo — write arguments to standard output
//
// Usage: echo [-neE] [ARG]...
//
// Write each ARG to standard output, separated by a single space,
// followed by a newline.
//
// Flags (bash-compatible, not POSIX getopt):
//
//	-n  Do not output a trailing newline.
//	-e  Enable interpretation of backslash escape sequences.
//	-E  Disable interpretation of backslash escapes (default).
//
// Flags can be combined (e.g. -ne). Only leading arguments matching
// -[neE]+ are treated as flags; once a non-flag argument is seen,
// all remaining arguments (including it) are printed as text.
// "--" is NOT treated as an end-of-options separator.
//
// Supported escape sequences (with -e):
//
//	\\    backslash
//	\a    alert (BEL)
//	\b    backspace
//	\c    suppress further output (including trailing newline)
//	\e    escape character (0x1B)
//	\E    escape character (0x1B)
//	\f    form feed
//	\n    newline
//	\r    carriage return
//	\t    horizontal tab
//	\v    vertical tab
//	\0nnn octal value (1 to 3 digits)
//	\xHH  hexadecimal value (1 to 2 digits)
//	\uHHHH    Unicode character (1 to 4 hex digits)
//	\UHHHHHHHH Unicode character (1 to 8 hex digits)
//
// Exit codes:
//
//	0  Always succeeds.
package echo

import (
	"context"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the echo builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "echo",
	Description: "write arguments to stdout",
	Help: `echo: echo [-neE] [arg ...]
    Write arguments to the standard output.

    Display the ARGs, separated by a single space character and followed by a
    newline, on the standard output.

    Options:
      -n  do not output the trailing newline
      -e  enable interpretation of backslash escapes
      -E  disable interpretation of backslash escapes (default)

    Exit Status:
    Returns success unless a write error occurs.`,
	MakeFlags: builtins.NoFlags(run),
}

func run(_ context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
	// Parse flags: bash treats leading args matching -[neE]+ as flags.
	// Once a non-matching arg is seen, everything from that point is text.
	var noNewline, escapes bool
	textStart := 0
	for i, arg := range args {
		if !isEchoFlag(arg) {
			break
		}
		textStart = i + 1
		for _, ch := range arg[1:] { // skip leading '-'
			switch ch {
			case 'n':
				noNewline = true
			case 'e':
				escapes = true
			case 'E':
				escapes = false
			}
		}
	}

	textArgs := args[textStart:]

	for i, arg := range textArgs {
		if i > 0 {
			callCtx.Out(" ")
		}
		if escapes {
			s, stop := processEscapes(arg)
			callCtx.Out(s)
			if stop {
				return builtins.Result{}
			}
		} else {
			callCtx.Out(arg)
		}
	}

	if !noNewline {
		callCtx.Out("\n")
	}
	return builtins.Result{}
}

// isEchoFlag returns true if arg looks like a valid echo flag: starts with '-'
// and every subsequent character is one of 'n', 'e', 'E'. Must have at least
// one flag character (i.e. bare "-" is not a flag).
func isEchoFlag(arg string) bool {
	if len(arg) < 2 || arg[0] != '-' {
		return false
	}
	for i := 1; i < len(arg); i++ {
		switch arg[i] {
		case 'n', 'e', 'E':
		default:
			return false
		}
	}
	return true
}

// processEscapes interprets backslash escape sequences in s.
// Returns the processed string and whether \c was encountered (meaning
// all further output including trailing newline should be suppressed).
func processEscapes(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		// We have a backslash followed by at least one character.
		i++ // skip '\'
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case 'a':
			b.WriteByte('\a')
		case 'b':
			b.WriteByte('\b')
		case 'c':
			return b.String(), true
		case 'e', 'E':
			b.WriteByte(0x1B)
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'v':
			b.WriteByte('\v')
		case '0':
			// Octal: \0nnn (up to 3 octal digits after the '0')
			i++
			val, consumed := parseOctal(s[i:], 3)
			i += consumed
			b.WriteByte(byte(val)) // Intentional: wraps values > 255 (matches bash)
			continue
		case 'x':
			// Hex: \xHH (up to 2 hex digits)
			i++
			val, consumed := parseHex(s[i:], 2)
			if consumed == 0 {
				// No valid hex digits: output \x literally
				b.WriteByte('\\')
				b.WriteByte('x')
				continue
			}
			i += consumed
			b.WriteByte(byte(val))
			continue
		case 'u':
			// Unicode: \uHHHH (up to 4 hex digits)
			i++
			val, consumed := parseHex(s[i:], 4)
			if consumed == 0 {
				b.WriteByte('\\')
				b.WriteByte('u')
				continue
			}
			i += consumed
			writeUnicodeCodepoint(&b, int64(val))
			continue
		case 'U':
			// Unicode: \UHHHHHHHH (up to 8 hex digits)
			i++
			val, consumed := parseHex(s[i:], 8)
			if consumed == 0 {
				b.WriteByte('\\')
				b.WriteByte('U')
				continue
			}
			i += consumed
			writeUnicodeCodepoint(&b, val)
			continue
		default:
			// Unrecognized escape: output backslash and the character literally.
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
		i++
	}
	return b.String(), false
}

// writeUnicodeCodepoint writes a Unicode codepoint to the builder.
// Bash emits raw bytes for surrogates (U+D800-U+DFFF), producing invalid UTF-8,
// and drops values above a certain threshold. We replace surrogates with U+FFFD
// and drop anything beyond U+10FFFF to avoid emitting invalid UTF-8.
func writeUnicodeCodepoint(b *strings.Builder, val int64) {
	if val > 0x10FFFF {
		// Intentional divergence: bash emits raw bytes for values up to 0x7FFFFFFF,
		// but we drop them to avoid producing invalid UTF-8.
		return
	}
	// Go's WriteRune replaces surrogates (U+D800-U+DFFF) with U+FFFD automatically.
	b.WriteRune(rune(val))
}

// parseOctal reads up to maxDigits octal digits from s and returns the
// accumulated value and the number of bytes consumed.
func parseOctal(s string, maxDigits int) (int, int) {
	val := 0
	n := 0
	for n < maxDigits && n < len(s) && s[n] >= '0' && s[n] <= '7' {
		val = val*8 + int(s[n]-'0')
		n++
	}
	return val, n
}

// parseHex reads up to maxDigits hexadecimal digits from s and returns
// the accumulated value and the number of bytes consumed.
// Returns int64 to avoid overflow on 32-bit architectures when parsing
// up to 8 hex digits for \U escape sequences.
func parseHex(s string, maxDigits int) (int64, int) {
	var val int64
	n := 0
	for n < maxDigits && n < len(s) {
		ch := s[n]
		switch {
		case ch >= '0' && ch <= '9':
			val = val*16 + int64(ch-'0')
		case ch >= 'a' && ch <= 'f':
			val = val*16 + int64(ch-'a') + 10
		case ch >= 'A' && ch <= 'F':
			val = val*16 + int64(ch-'A') + 10
		default:
			return val, n
		}
		n++
	}
	return val, n
}
