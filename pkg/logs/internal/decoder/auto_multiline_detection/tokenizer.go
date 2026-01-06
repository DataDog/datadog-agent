// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"math"
	"strings"
	"unicode"

	"github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection/tokens"
	"github.com/DataDog/datadog-agent/pkg/logs/pattern"
)

// Tokenizer is a heuristic to compute tokens from a log message.
// The tokenizer is used to convert a log message (string of bytes) into a list of tokens that
// represents the underlying structure of the log. The string of tokens is a compact slice of bytes
// that can be used to compare log messages structure. A tokenizer instance is not thread safe
// as bufferes are reused to avoid allocations.
type Tokenizer struct {
	maxEvalBytes int
	impl         *pattern.Tokenizer
}

// NewTokenizer returns a new Tokenizer detection heuristic.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	return &Tokenizer{
		maxEvalBytes: maxEvalBytes,
		impl:         pattern.NewTokenizer(),
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
	return t.impl.Tokenize(input)
}

// getToken returns a single token from a single byte.
// This helper is used by tests to ensure token/string round-tripping remains valid.
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
