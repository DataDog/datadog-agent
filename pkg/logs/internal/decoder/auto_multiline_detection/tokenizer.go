// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"math"
	"strings"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
)

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 below.
const maxRun = 10

// tokenLookup is a 256-byte lookup table for single-byte token classification.
// Initialized once at package load time for O(1) lookups.
var tokenLookup [256]tokens.Token

func init() {
	// Default everything to C1 (character)
	for i := range tokenLookup {
		tokenLookup[i] = tokens.C1
	}

	// Digits
	for c := byte('0'); c <= '9'; c++ {
		tokenLookup[c] = tokens.D1
	}

	// Whitespace
	tokenLookup[' '] = tokens.Space
	tokenLookup['\t'] = tokens.Space
	tokenLookup['\n'] = tokens.Space
	tokenLookup['\r'] = tokens.Space

	// Special characters
	tokenLookup[':'] = tokens.Colon
	tokenLookup[';'] = tokens.Semicolon
	tokenLookup['-'] = tokens.Dash
	tokenLookup['_'] = tokens.Underscore
	tokenLookup['/'] = tokens.Fslash
	tokenLookup['\\'] = tokens.Bslash
	tokenLookup['.'] = tokens.Period
	tokenLookup[','] = tokens.Comma
	tokenLookup['\''] = tokens.Singlequote
	tokenLookup['"'] = tokens.Doublequote
	tokenLookup['`'] = tokens.Backtick
	tokenLookup['~'] = tokens.Tilda
	tokenLookup['*'] = tokens.Star
	tokenLookup['+'] = tokens.Plus
	tokenLookup['='] = tokens.Equal
	tokenLookup['('] = tokens.Parenopen
	tokenLookup[')'] = tokens.Parenclose
	tokenLookup['{'] = tokens.Braceopen
	tokenLookup['}'] = tokens.Braceclose
	tokenLookup['['] = tokens.Bracketopen
	tokenLookup[']'] = tokens.Bracketclose
	tokenLookup['&'] = tokens.Ampersand
	tokenLookup['!'] = tokens.Exclamation
	tokenLookup['@'] = tokens.At
	tokenLookup['#'] = tokens.Pound
	tokenLookup['$'] = tokens.Dollar
	tokenLookup['%'] = tokens.Percent
	tokenLookup['^'] = tokens.Uparrow
}

// Tokenizer is a heuristic to compute tokens from a log message.
// The tokenizer is used to convert a log message (string of bytes) into a list of tokens that
// represents the underlying structure of the log. The string of tokens is a compact slice of bytes
// that can be used to compare log messages structure. A tokenizer instance is not thread safe
// as buffers are reused to avoid allocations.
type Tokenizer struct {
	maxEvalBytes int
	strBuf       [maxRun]byte // Fixed-size buffer, no heap allocation
	strLen       int          // Current length of content in strBuf
}

// NewTokenizer returns a new Tokenizer detection heuristic.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	return &Tokenizer{
		maxEvalBytes: maxEvalBytes,
	}
}

// ProcessAndContinue enriches the message context with tokens.
// This implements the Heuristic interface - this heuristic does not stop processing.
func (t *Tokenizer) ProcessAndContinue(context *messageContext) bool {
	maxBytes := min(len(context.rawMessage), t.maxEvalBytes)
	tokens, indicies := t.tokenize(context.rawMessage[:maxBytes])
	context.tokens = tokens
	context.tokenIndicies = indicies
	return true
}

// tokenize converts a byte slice to a list of tokens.
// This function return the slice of tokens, and a slice of indices where each token starts.
func (t *Tokenizer) tokenize(input []byte) ([]tokens.Token, []int) {
	inputLen := len(input)
	if inputLen == 0 {
		return nil, nil
	}

	// len(ts) will always be <= len(input)
	ts := make([]tokens.Token, 0, inputLen)
	indicies := make([]int, 0, inputLen)

	idx := 0
	run := 0
	firstChar := input[0]
	lastToken := tokenLookup[firstChar]

	// Reset string buffer
	t.strLen = 0
	if t.strLen < maxRun {
		t.strBuf[t.strLen] = toUpperASCII(firstChar)
		t.strLen++
	}

	for i := 1; i < inputLen; i++ {
		char := input[i]
		currentToken := tokenLookup[char]

		if currentToken != lastToken {
			// Inline insertToken logic to avoid closure overhead
			if lastToken == tokens.C1 {
				if t.strLen == 1 {
					if specialToken := getSpecialShortToken(t.strBuf[0]); specialToken != tokens.End {
						ts = append(ts, specialToken)
						indicies = append(indicies, idx)
						goto nextIteration
					}
				} else if t.strLen > 1 {
					// Use unsafe to avoid string allocation
					str := unsafe.String(&t.strBuf[0], t.strLen)
					if specialToken := getSpecialLongToken(str); specialToken != tokens.End {
						ts = append(ts, specialToken)
						indicies = append(indicies, idx-run)
						goto nextIteration
					}
				}
			}

			// Check for char or digit runs
			if lastToken == tokens.C1 || lastToken == tokens.D1 {
				indicies = append(indicies, idx-run)
				r := run
				if r >= maxRun {
					r = maxRun - 1
				}
				ts = append(ts, lastToken+tokens.Token(r))
			} else {
				ts = append(ts, lastToken)
				indicies = append(indicies, idx-run)
			}

		nextIteration:
			run = 0
			t.strLen = 0
		} else {
			run++
		}

		// Buffer character for special token matching
		if currentToken == tokens.C1 {
			if t.strLen < maxRun {
				t.strBuf[t.strLen] = toUpperASCII(char)
				t.strLen++
			}
		} else if t.strLen < maxRun {
			t.strBuf[t.strLen] = char
			t.strLen++
		}

		lastToken = currentToken
		idx++
	}

	// Flush final token (inlined)
	if lastToken == tokens.C1 {
		if t.strLen == 1 {
			if specialToken := getSpecialShortToken(t.strBuf[0]); specialToken != tokens.End {
				ts = append(ts, specialToken)
				indicies = append(indicies, idx)
				return ts, indicies
			}
		} else if t.strLen > 1 {
			str := unsafe.String(&t.strBuf[0], t.strLen)
			if specialToken := getSpecialLongToken(str); specialToken != tokens.End {
				ts = append(ts, specialToken)
				indicies = append(indicies, idx-run)
				return ts, indicies
			}
		}
	}

	if lastToken == tokens.C1 || lastToken == tokens.D1 {
		indicies = append(indicies, idx-run)
		r := run
		if r >= maxRun {
			r = maxRun - 1
		}
		ts = append(ts, lastToken+tokens.Token(r))
	} else {
		ts = append(ts, lastToken)
		indicies = append(indicies, idx-run)
	}

	return ts, indicies
}

// toUpperASCII converts ASCII lowercase to uppercase, returns char unchanged otherwise
func toUpperASCII(char byte) byte {
	if char >= 'a' && char <= 'z' {
		return char - 32 // 'a' - 'A' = 32
	}
	return char
}

// getToken returns a single token from a single byte using lookup table.
func getToken(char byte) tokens.Token {
	return tokenLookup[char]
}

func getSpecialShortToken(char byte) tokens.Token {
	// Only T and Z are special single-char tokens
	if char == 'T' {
		return tokens.T
	}
	if char == 'Z' {
		return tokens.Zone
	}
	return tokens.End
}

// getSpecialLongToken returns a special token that is > 1 character.
// NOTE: This set of tokens is non-exhaustive and can be expanded.
func getSpecialLongToken(input string) tokens.Token {
	// Length-based dispatch for faster rejection
	switch len(input) {
	case 2:
		if input == "AM" || input == "PM" {
			return tokens.Apm
		}
	case 3:
		switch input {
		case "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
			"JUL", "AUG", "SEP", "OCT", "NOV", "DEC":
			return tokens.Month
		case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
			return tokens.Day
		case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
			"MST", "MDT", "PST", "PDT", "JST", "KST",
			"IST", "MSK", "CET", "BST", "HST", "HDT",
			"NST", "NDT":
			return tokens.Zone
		}
	case 4:
		switch input {
		case "CEST", "NZST", "NZDT", "ACST", "ACDT",
			"AEST", "AEDT", "AWST", "AWDT", "AKST",
			"AKDT", "CHST", "CHDT":
			return tokens.Zone
		}
	}
	return tokens.End
}

// tokenToString converts a single token to a debug string.
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

// tokensToString converts a list of tokens to a debug string.
func tokensToString(tokens []tokens.Token) string {
	var builder strings.Builder
	for _, t := range tokens {
		builder.WriteString(tokenToString(t))
	}
	return builder.String()
}

// isMatch compares two sequences of tokens and returns true if they match within the
// given threshold. if the token strings are different lengths, the shortest string is
// used for comparison. This function is optimized to exit early if the match is impossible
// without having to compare all of the tokens.
func isMatch(seqA []tokens.Token, seqB []tokens.Token, thresh float64) bool {
	count := min(len(seqB), len(seqA))

	if count == 0 {
		return len(seqA) == len(seqB)
	}

	requiredMatches := int(math.Round(thresh * float64(count)))
	match := 0

	for i := 0; i < count; i++ {
		if seqA[i] == seqB[i] {
			match++
		}
		if match+(count-i-1) < requiredMatches {
			return false
		}
	}

	return true
}
