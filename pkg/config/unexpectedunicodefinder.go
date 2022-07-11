package config

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

// FreeFromUnexpectedUnicode checks whether or not a given byte slice contains any
// unicode codepoints that are 'unexpected'.
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
			reason = "RuneError, invalid unicode"
		case r == ' ' || r == '\r' || r == '\n' || r == '\t':
			// These are allowed whitespace
			reason = ""
		case unicode.IsSpace(r):
			reason = "Unsupported whitespace codepoint"
		case unicode.Is(unicode.Bidi_Control, r):
			reason = "Bidirectional control codepoint"
		case unicode.Is(unicode.C, r):
			reason = "Control/surrogate codepoint"
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
