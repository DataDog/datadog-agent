// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package builtins

import "testing"

func TestSafeOperand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "foo.txt", "foo.txt"},
		{"newline", "foo\nbar", `foo\nbar`},
		{"carriage return", "foo\rbar", `foo\rbar`},
		{"tab", "foo\tbar", `foo\tbar`},
		{"esc ansi sequence", "foo\x1b[31mRED\x1b[0m", `foo\x1b[31mRED\x1b[0m`},
		{"backslash", `foo\bar`, `foo\\bar`},
		{"bell and other control bytes", "foo\x07\x00bar", `foo\x07\x00bar`},
		{"unicode preserved", "café🎉", "café🎉"},
		{"unicode line separator U+2028", "foo bar", `foo\u2028bar`},
		{"unicode paragraph separator U+2029", "foo bar", `foo\u2029bar`},
		{"unicode bidi override U+202E", "foo‮bar", `foo\u202ebar`},
		{"unicode zero width space U+200B", "foo​bar", `foo\u200bbar`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeOperand(tt.in); got != tt.want {
				t.Fatalf("SafeOperand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSafeOperandNeverEmitsRawControlBytes(t *testing.T) {
	in := "line1\nline2\rline3\x1b[2Jline4"
	got := SafeOperand(in)
	for _, r := range got {
		if r == '\n' || r == '\r' || r == 0x1b {
			t.Fatalf("SafeOperand output still contains a raw control byte: %q", got)
		}
	}
}

func TestSafeOperandNeverEmitsRawSeparatorsOrFormatChars(t *testing.T) {
	in := "line1 line2 line3‮line4"
	got := SafeOperand(in)
	for _, r := range got {
		if r == ' ' || r == ' ' || r == '‮' {
			t.Fatalf("SafeOperand output still contains a raw Unicode separator/format char: %q", got)
		}
	}
}

// TestSafeMessage mirrors TestSafeOperand for every case except backslash,
// where the two functions deliberately diverge: SafeMessage is meant for
// already-formatted diagnostic text (e.g. an *os.PathError's Error()
// string) that may legitimately contain a backslash as a Windows path
// separator, so it must leave it untouched rather than doubling it.
func TestSafeMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "foo.txt", "foo.txt"},
		{"newline", "foo\nbar", `foo\nbar`},
		{"carriage return", "foo\rbar", `foo\rbar`},
		{"tab", "foo\tbar", `foo\tbar`},
		{"esc ansi sequence", "foo\x1b[31mRED\x1b[0m", `foo\x1b[31mRED\x1b[0m`},
		{"backslash preserved (Windows path separator)", `foo\bar`, `foo\bar`},
		{"bell and other control bytes", "foo\x07\x00bar", `foo\x07\x00bar`},
		{"unicode preserved", "café🎉", "café🎉"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeMessage(tt.in); got != tt.want {
				t.Fatalf("SafeMessage(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSafeMessageNeverEmitsRawControlBytes(t *testing.T) {
	in := "line1\nline2\rline3\x1b[2Jline4"
	got := SafeMessage(in)
	for _, r := range got {
		if r == '\n' || r == '\r' || r == 0x1b {
			t.Fatalf("SafeMessage output still contains a raw control byte: %q", got)
		}
	}
}
