package config

import (
	"unicode"
	"unicode/utf8"
)

type UnexpectedUnicodeCodepoint struct {
	codepoint rune
	reason    string
	position  int
}

func FreeFromUnexpectedUnicode(input []byte) []UnexpectedUnicodeCodepoint {
	totalSize := len(input)
	currentIndex := 0
	results := make([]UnexpectedUnicodeCodepoint, 0)
	for currentIndex < totalSize {
		r, size := utf8.DecodeRune(input[currentIndex:])
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
	}
	return results
}
