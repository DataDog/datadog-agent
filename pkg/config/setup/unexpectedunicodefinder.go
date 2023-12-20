// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package setup

import (
	"unicode"
	"unicode/utf8"
)

// UnexpectedUnicodeCodepoint contains specifics about an occurrence of an unexpected unicode codepoint
type UnexpectedUnicodeCodepoint struct {
	codepoint rune
	reason    string
	position  int
}

// FindUnexpectedUnicode reports any _unexpected_ unicode codepoints
// found in the given 'input' string
// Unexpected here generally means invisible whitespace and control chars
func FindUnexpectedUnicode(input string) []UnexpectedUnicodeCodepoint {
	currentIndex := 0
	str := input
	results := make([]UnexpectedUnicodeCodepoint, 0)

	for len(str) > 0 {
		r, size := utf8.DecodeRuneInString(str)
		reason := ""
		switch {
		case r == utf8.RuneError:
			reason = "RuneError"
		case r == ' ' || r == '\r' || r == '\n' || r == '\t':
			// These are allowed whitespace
			reason = ""
		case unicode.IsSpace(r):
			reason = "unsupported whitespace"
		case unicode.Is(unicode.Bidi_Control, r):
			reason = "Bidirectional control"
		case unicode.Is(unicode.C, r):
			reason = "Control/surrogate"
		}

		if reason != "" {
			results = append(results, UnexpectedUnicodeCodepoint{
				codepoint: r,
				reason:    reason,
				position:  currentIndex,
			})
		}

		currentIndex += size
		str = str[size:]
	}
	return results
}
