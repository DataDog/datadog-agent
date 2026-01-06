// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pattern provides utilities for deriving stable signatures from log messages.
package pattern

import (
	"bytes"
	"sort"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/logs/pattern/tokens"
)

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 in tokens.
const maxRun = 10

// Tokenizer converts a log message into a list of tokens representing its structure.
// A Tokenizer instance is not thread safe as buffers are reused to avoid allocations.
type Tokenizer struct {
	strBuf *bytes.Buffer
}

// NewTokenizer returns a new Tokenizer.
func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		strBuf: bytes.NewBuffer(make([]byte, 0, maxRun)),
	}
}

// Tokenize converts a byte slice to a list of tokens.
// This function returns the slice of tokens, and a slice of indices where each token starts.
func (t *Tokenizer) Tokenize(input []byte) ([]tokens.Token, []int) {
	// len(ts) will always be <= len(input)
	ts := make([]tokens.Token, 0, len(input))
	indicies := make([]int, 0, len(input))
	if len(input) == 0 {
		return ts, indicies
	}

	idx := 0
	run := 0
	lastToken := getToken(input[0])
	t.strBuf.Reset()
	t.strBuf.WriteRune(unicode.ToUpper(rune(input[0])))

	insertToken := func() {
		defer func() {
			run = 0
			t.strBuf.Reset()
		}()

		// Only test for special tokens if the last token was a character (Special tokens are currently only A-Z).
		if lastToken == tokens.C1 {
			if t.strBuf.Len() == 1 {
				if specialToken := getSpecialShortToken(t.strBuf.Bytes()[0]); specialToken != tokens.End {
					ts = append(ts, specialToken)
					indicies = append(indicies, idx)
					return
				}
			} else if t.strBuf.Len() > 1 { // Only test special long tokens if buffer is > 1 token
				if specialToken := getSpecialLongToken(t.strBuf.String()); specialToken != tokens.End {
					ts = append(ts, specialToken)
					indicies = append(indicies, idx-run)
					return
				}
			}
		}

		// Check for char or digit runs
		if lastToken == tokens.C1 || lastToken == tokens.D1 {
			indicies = append(indicies, idx-run)
			// Limit max run size
			if run >= maxRun {
				run = maxRun - 1
			}
			ts = append(ts, lastToken+tokens.Token(run))
		} else {
			ts = append(ts, lastToken)
			indicies = append(indicies, idx-run)
		}
	}

	for _, char := range input[1:] {
		currentToken := getToken(char)
		if currentToken != lastToken {
			insertToken()
		} else {
			run++
		}
		if currentToken == tokens.C1 {
			// Store upper case A-Z characters for matching special tokens
			t.strBuf.WriteRune(unicode.ToUpper(rune(char)))
		} else {
			t.strBuf.WriteByte(char)
		}
		lastToken = currentToken
		idx++
	}

	// Flush any remaining buffered tokens
	insertToken()

	return ts, indicies
}

// Signature returns a deterministic signature for the input bytes, capped to maxEvalBytes if > 0.
// The signature is compatible with the auto multiline tokenizer debug token string representation.
func Signature(input []byte, maxEvalBytes int) string {
	if len(input) == 0 {
		return ""
	}
	maxBytes := len(input)
	if maxEvalBytes > 0 && maxBytes > maxEvalBytes {
		maxBytes = maxEvalBytes
	}
	input = input[:maxBytes]

	// Note: We intentionally build the signature directly (vs emitting tokens then converting)
	// to reduce intermediate allocations.
	var out strings.Builder
	out.Grow(len(input))

	var buf [maxRun]byte
	bufLen := 0
	resetBuf := func() { bufLen = 0 }
	appendBuf := func(b byte) {
		if bufLen < len(buf) {
			// Only store uppercase A-Z characters for matching special tokens
			if b >= 'a' && b <= 'z' {
				b = b - 'a' + 'A'
			}
			buf[bufLen] = b
			bufLen++
		}
	}

	idx := 0
	run := 0
	lastToken := getToken(input[0])
	resetBuf()
	appendBuf(input[0])

	insertToken := func() {
		defer func() {
			run = 0
			resetBuf()
		}()

		// Special tokens only apply for character sequences.
		if lastToken == tokens.C1 {
			if bufLen == 1 {
				if specialToken := getSpecialShortToken(buf[0]); specialToken != tokens.End {
					out.WriteString(tokenToString(specialToken))
					return
				}
			} else if bufLen > 1 {
				if specialToken := getSpecialLongToken(string(buf[:bufLen])); specialToken != tokens.End {
					out.WriteString(tokenToString(specialToken))
					return
				}
			}
		}

		// Char/digit runs
		if lastToken == tokens.C1 || lastToken == tokens.D1 {
			if run >= maxRun {
				run = maxRun - 1
			}
			out.WriteString(tokenToString(lastToken + tokens.Token(run)))
			return
		}

		out.WriteString(tokenToString(lastToken))
	}

	for _, char := range input[1:] {
		currentToken := getToken(char)
		if currentToken != lastToken {
			insertToken()
		} else {
			run++
		}
		if currentToken == tokens.C1 {
			appendBuf(char)
		} else {
			appendBuf(char)
		}
		lastToken = currentToken
		idx++
		_ = idx // keep parity with original logic; idx retained for readability
	}
	insertToken()

	return out.String()
}

// getToken returns a single token from a single byte.
func getToken(char byte) tokens.Token {
	if unicode.IsDigit(rune(char)) {
		return tokens.D1
	} else if unicode.IsSpace(rune(char)) {
		return tokens.Space
	}

	switch char {
	case ':':
		return tokens.Colon
	case ';':
		return tokens.Semicolon
	case '-':
		return tokens.Dash
	case '_':
		return tokens.Underscore
	case '/':
		return tokens.Fslash
	case '\\':
		return tokens.Bslash
	case '.':
		return tokens.Period
	case ',':
		return tokens.Comma
	case '\'':
		return tokens.Singlequote
	case '"':
		return tokens.Doublequote
	case '`':
		return tokens.Backtick
	case '~':
		return tokens.Tilda
	case '*':
		return tokens.Star
	case '+':
		return tokens.Plus
	case '=':
		return tokens.Equal
	case '(':
		return tokens.Parenopen
	case ')':
		return tokens.Parenclose
	case '{':
		return tokens.Braceopen
	case '}':
		return tokens.Braceclose
	case '[':
		return tokens.Bracketopen
	case ']':
		return tokens.Bracketclose
	case '&':
		return tokens.Ampersand
	case '!':
		return tokens.Exclamation
	case '@':
		return tokens.At
	case '#':
		return tokens.Pound
	case '$':
		return tokens.Dollar
	case '%':
		return tokens.Percent
	case '^':
		return tokens.Uparrow
	}

	return tokens.C1
}

func getSpecialShortToken(char byte) tokens.Token {
	switch char {
	case 'T':
		return tokens.T
	case 'Z':
		return tokens.Zone
	}
	return tokens.End
}

// getSpecialLongToken returns a special token that is > 1 character.
// NOTE: This set of tokens is non-exhaustive and can be expanded.
func getSpecialLongToken(input string) tokens.Token {
	switch input {
	case "JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL",
		"AUG", "SEP", "OCT", "NOV", "DEC":
		return tokens.Month
	case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
		return tokens.Day
	case "AM", "PM":
		return tokens.Apm
	case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
		"MST", "MDT", "PST", "PDT", "JST", "KST",
		"IST", "MSK", "CEST", "CET", "BST", "NZST",
		"NZDT", "ACST", "ACDT", "AEST", "AEDT",
		"AWST", "AWDT", "AKST", "AKDT", "HST",
		"HDT", "CHST", "CHDT", "NST", "NDT":
		return tokens.Zone
	}

	return tokens.End
}

// tokenToString converts a single token to a debug string / signature fragment.
func tokenToString(token tokens.Token) string {
	if token >= tokens.D1 && token <= tokens.D10 {
		return strings.Repeat("D", int(token-tokens.D1)+1)
	} else if token >= tokens.C1 && token <= tokens.C10 {
		return strings.Repeat("C", int(token-tokens.C1)+1)
	}

	switch token {
	case tokens.Space:
		return " "
	case tokens.Colon:
		return ":"
	case tokens.Semicolon:
		return ";"
	case tokens.Dash:
		return "-"
	case tokens.Underscore:
		return "_"
	case tokens.Fslash:
		return "/"
	case tokens.Bslash:
		return "\\"
	case tokens.Period:
		return "."
	case tokens.Comma:
		return ","
	case tokens.Singlequote:
		return "'"
	case tokens.Doublequote:
		return "\""
	case tokens.Backtick:
		return "`"
	case tokens.Tilda:
		return "~"
	case tokens.Star:
		return "*"
	case tokens.Plus:
		return "+"
	case tokens.Equal:
		return "="
	case tokens.Parenopen:
		return "("
	case tokens.Parenclose:
		return ")"
	case tokens.Braceopen:
		return "{"
	case tokens.Braceclose:
		return "}"
	case tokens.Bracketopen:
		return "["
	case tokens.Bracketclose:
		return "]"
	case tokens.Ampersand:
		return "&"
	case tokens.Exclamation:
		return "!"
	case tokens.At:
		return "@"
	case tokens.Pound:
		return "#"
	case tokens.Dollar:
		return "$"
	case tokens.Percent:
		return "%"
	case tokens.Uparrow:
		return "^"
	case tokens.Month:
		return "MTH"
	case tokens.Day:
		return "DAY"
	case tokens.Apm:
		return "PM"
	case tokens.T:
		return "T"
	case tokens.Zone:
		return "ZONE"
	}
	return ""
}

// TokensToString converts a list of tokens to a signature string.
func TokensToString(ts []tokens.Token) string {
	var builder strings.Builder
	for _, t := range ts {
		builder.WriteString(tokenToString(t))
	}
	return builder.String()
}

// SortTags is a small helper for stable series keying in callers.
func SortTags(tags []string) []string {
	cpy := make([]string, len(tags))
	copy(cpy, tags)
	sort.Strings(cpy)
	return cpy
}


