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
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 below.
const maxRun = 10

// tokenLookup is a 256-byte lookup table for single-byte token classification.
// Initialized via function call to ensure it happens before other package vars use it.
var tokenLookup = makeTokenLookup()

// toUpperLookup converts lowercase to uppercase via lookup, identity otherwise
var toUpperLookup = makeToUpperLookup()

func makeToUpperLookup() [256]byte {
	var lookup [256]byte
	for i := range lookup {
		lookup[i] = byte(i)
	}
	for c := byte('a'); c <= 'z'; c++ {
		lookup[c] = c - 32
	}
	return lookup
}

func makeTokenLookup() [256]tokens.Token {
	var lookup [256]tokens.Token

	// Default everything to C1 (character)
	for i := range lookup {
		lookup[i] = tokens.C1
	}

	// Digits
	for c := byte('0'); c <= '9'; c++ {
		lookup[c] = tokens.D1
	}

	// Whitespace
	lookup[' '] = tokens.Space
	lookup['\t'] = tokens.Space
	lookup['\n'] = tokens.Space
	lookup['\r'] = tokens.Space

	// Special characters
	lookup[':'] = tokens.Colon
	lookup[';'] = tokens.Semicolon
	lookup['-'] = tokens.Dash
	lookup['_'] = tokens.Underscore
	lookup['/'] = tokens.Fslash
	lookup['\\'] = tokens.Bslash
	lookup['.'] = tokens.Period
	lookup[','] = tokens.Comma
	lookup['\''] = tokens.Singlequote
	lookup['"'] = tokens.Doublequote
	lookup['`'] = tokens.Backtick
	lookup['~'] = tokens.Tilda
	lookup['*'] = tokens.Star
	lookup['+'] = tokens.Plus
	lookup['='] = tokens.Equal
	lookup['('] = tokens.Parenopen
	lookup[')'] = tokens.Parenclose
	lookup['{'] = tokens.Braceopen
	lookup['}'] = tokens.Braceclose
	lookup['['] = tokens.Bracketopen
	lookup[']'] = tokens.Bracketclose
	lookup['&'] = tokens.Ampersand
	lookup['!'] = tokens.Exclamation
	lookup['@'] = tokens.At
	lookup['#'] = tokens.Pound
	lookup['$'] = tokens.Dollar
	lookup['%'] = tokens.Percent
	lookup['^'] = tokens.Uparrow

	return lookup
}

// Tokenizer is a heuristic to compute tokens from a log message.
// The tokenizer is used to convert a log message (string of bytes) into a list of tokens that
// represents the underlying structure of the log. The string of tokens is a compact slice of bytes
// that can be used to compare log messages structure. A tokenizer instance is not thread safe
// as buffers are reused to avoid allocations.
type Tokenizer struct {
	maxEvalBytes int
	strBuf       [maxRun]byte   // Fixed-size buffer for special token matching
	strLen       int            // Current length of content in strBuf
	tsBuf        []tokens.Token // Reusable token buffer
	idxBuf       []int          // Reusable index buffer
}

// NewTokenizer returns a new Tokenizer detection heuristic.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	// Pre-allocate reasonable initial capacity
	initCap := 64
	if maxEvalBytes > 0 && maxEvalBytes < initCap {
		initCap = maxEvalBytes
	}
	return &Tokenizer{
		maxEvalBytes: maxEvalBytes,
		tsBuf:        make([]tokens.Token, 0, initCap),
		idxBuf:       make([]int, 0, initCap),
	}
}

// ProcessAndContinue enriches the message context with tokens.
// This implements the Heuristic interface - this heuristic does not stop processing.
// If tokens are already populated (e.g., from TokenizingLineHandler), they are reused.
func (t *Tokenizer) ProcessAndContinue(context *messageContext) bool {
	// Skip tokenization if tokens already exist (reused from ParsingExtra)
	if len(context.tokens) > 0 {
		return true
	}

	maxBytes := min(len(context.rawMessage), t.maxEvalBytes)
	tokens, indicies := t.tokenize(context.rawMessage[:maxBytes])
	context.tokens = tokens
	context.tokenIndicies = indicies
	return true
}

// TokenizeMessage tokenizes a message's content and stores the tokens in ParsingExtra.
// This is used by the decoder to pre-tokenize messages before they reach line handlers.
func (t *Tokenizer) TokenizeMessage(msg *message.Message) {
	maxBytes := len(msg.GetContent())
	if t.maxEvalBytes > 0 && t.maxEvalBytes < maxBytes {
		maxBytes = t.maxEvalBytes
	}
	tokens, indices := t.tokenize(msg.GetContent()[:maxBytes])

	// Convert tokens to bytes for storage
	tokenBytes := make([]byte, len(tokens))
	for i, tok := range tokens {
		tokenBytes[i] = byte(tok)
	}
	msg.ParsingExtra.Tokens = tokenBytes
	msg.ParsingExtra.TokenIndices = indices
}

// emitToken appends a token to the output slices, checking for special tokens first.
// Returns the updated slices.
func (t *Tokenizer) emitToken(ts []tokens.Token, indicies []int, lastToken tokens.Token, run, idx int) ([]tokens.Token, []int) {
	// Check for special tokens (only for C1/letter runs, length 1-4)
	if lastToken == tokens.C1 && t.strLen > 0 && t.strLen <= 4 {
		if t.strLen == 1 {
			if specialToken := getSpecialShortToken(t.strBuf[0]); specialToken != tokens.End {
				return append(ts, specialToken), append(indicies, idx)
			}
		} else {
			str := unsafe.String(&t.strBuf[0], t.strLen)
			if specialToken := getSpecialLongToken(str); specialToken != tokens.End {
				return append(ts, specialToken), append(indicies, idx-run)
			}
		}
	}

	// Regular token - encode run length for C1/D1
	indicies = append(indicies, idx-run)
	if lastToken == tokens.C1 || lastToken == tokens.D1 {
		r := run
		if r >= maxRun {
			r = maxRun - 1
		}
		ts = append(ts, lastToken+tokens.Token(r))
	} else {
		ts = append(ts, lastToken)
	}
	return ts, indicies
}

// tokenize converts a byte slice to a list of tokens.
// This function return the slice of tokens, and a slice of indices where each token starts.
func (t *Tokenizer) tokenize(input []byte) ([]tokens.Token, []int) {
	inputLen := len(input)
	if inputLen == 0 {
		return nil, nil
	}

	// Use internal buffers for working storage, grow if needed.
	// Most logs produce ~inputLen/4 tokens, but we start smaller.
	estTokens := inputLen/4 + 8
	if cap(t.tsBuf) < estTokens {
		t.tsBuf = make([]tokens.Token, 0, estTokens)
		t.idxBuf = make([]int, 0, estTokens)
	}
	ts := t.tsBuf[:0]
	indicies := t.idxBuf[:0]

	run := 0
	firstChar := input[0]
	lastToken := tokenLookup[firstChar]

	// Reset string buffer - only track for C1 tokens
	t.strLen = 0
	if lastToken == tokens.C1 {
		t.strBuf[0] = toUpperLookup[firstChar]
		t.strLen = 1
	}

	for i := 1; i < inputLen; i++ {
		char := input[i]
		currentToken := tokenLookup[char]

		if currentToken != lastToken {
			ts, indicies = t.emitToken(ts, indicies, lastToken, run, i-1)
			run = 0
			t.strLen = 0
		} else {
			run++
		}

		// Only buffer C1 (letter) tokens for special token matching
		if currentToken == tokens.C1 && t.strLen < maxRun {
			t.strBuf[t.strLen] = toUpperLookup[char]
			t.strLen++
		}

		lastToken = currentToken
	}

	// Flush final token
	ts, indicies = t.emitToken(ts, indicies, lastToken, run, inputLen-1)

	// Store working buffers back for reuse
	t.tsBuf = ts
	t.idxBuf = indicies

	// Allocate exact-sized result slices - smaller than inputLen
	n := len(ts)
	result := make([]tokens.Token, n)
	copy(result, ts)
	resultIdx := make([]int, n)
	copy(resultIdx, indicies)
	return result, resultIdx
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
