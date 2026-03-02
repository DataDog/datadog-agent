// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor provides tokenization functionality for log messages.
package preprocessor

import (
	"math"
	"strings"
	"unsafe"
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

func makeTokenLookup() [256]Token {
	var lookup [256]Token

	// Default everything to C1 (character)
	for i := range lookup {
		lookup[i] = C1
	}

	// Digits
	for c := byte('0'); c <= '9'; c++ {
		lookup[c] = D1
	}

	// Whitespace
	lookup[' '] = Space
	lookup['\t'] = Space
	lookup['\n'] = Space
	lookup['\r'] = Space

	// Special characters
	lookup[':'] = Colon
	lookup[';'] = Semicolon
	lookup['-'] = Dash
	lookup['_'] = Underscore
	lookup['/'] = Fslash
	lookup['\\'] = Bslash
	lookup['.'] = Period
	lookup[','] = Comma
	lookup['\''] = Singlequote
	lookup['"'] = Doublequote
	lookup['`'] = Backtick
	lookup['~'] = Tilda
	lookup['*'] = Star
	lookup['+'] = Plus
	lookup['='] = Equal
	lookup['('] = Parenopen
	lookup[')'] = Parenclose
	lookup['{'] = Braceopen
	lookup['}'] = Braceclose
	lookup['['] = Bracketopen
	lookup[']'] = Bracketclose
	lookup['&'] = Ampersand
	lookup['!'] = Exclamation
	lookup['@'] = At
	lookup['#'] = Pound
	lookup['$'] = Dollar
	lookup['%'] = Percent
	lookup['^'] = Uparrow

	return lookup
}

// Tokenizer is a heuristic to compute tokens from a log message.
// The tokenizer is used to convert a log message (string of bytes) into a list of tokens that
// represents the underlying structure of the log. The string of tokens is a compact slice of bytes
// that can be used to compare log messages structure. A tokenizer instance is not thread safe
// as buffers are reused to avoid allocations.
type Tokenizer struct {
	maxEvalBytes int
	strBuf       [maxRun]byte // Fixed-size buffer for special token matching
	strLen       int          // Current length of content in strBuf
	tsBuf        []Token      // Reusable token buffer
	idxBuf       []int        // Reusable index buffer
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
		tsBuf:        make([]Token, 0, initCap),
		idxBuf:       make([]int, 0, initCap),
	}
}

// Tokenize tokenizes the input bytes and returns tokens and their start indices.
// The caller is responsible for slicing the input to the desired length.
func (t *Tokenizer) Tokenize(input []byte) ([]Token, []int) {
	maxBytes := len(input)
	if t.maxEvalBytes > 0 && t.maxEvalBytes < maxBytes {
		maxBytes = t.maxEvalBytes
	}
	return t.tokenize(input[:maxBytes])
}

// emitToken appends a token to the output slices, checking for special tokens first.
// Returns the updated slices.
func (t *Tokenizer) emitToken(ts []Token, indicies []int, lastToken Token, run, idx int) ([]Token, []int) {
	// Check for special tokens (only for C1/letter runs, length 1-4)
	if lastToken == C1 && t.strLen > 0 && t.strLen <= 4 {
		if t.strLen == 1 {
			if specialToken := getSpecialShortToken(t.strBuf[0]); specialToken != End {
				return append(ts, specialToken), append(indicies, idx)
			}
		} else {
			str := unsafe.String(&t.strBuf[0], t.strLen)
			if specialToken := getSpecialLongToken(str); specialToken != End {
				return append(ts, specialToken), append(indicies, idx-run)
			}
		}
	}

	// Regular token - encode run length for C1/D1
	indicies = append(indicies, idx-run)
	if lastToken == C1 || lastToken == D1 {
		r := run
		if r >= maxRun {
			r = maxRun - 1
		}
		ts = append(ts, lastToken+Token(r))
	} else {
		ts = append(ts, lastToken)
	}
	return ts, indicies
}

// tokenize converts a byte slice to a list of tokens.
// This function return the slice of tokens, and a slice of indices where each token starts.
func (t *Tokenizer) tokenize(input []byte) ([]Token, []int) {
	inputLen := len(input)
	if inputLen == 0 {
		return nil, nil
	}

	// Use internal buffers for working storage, grow if needed.
	// Most logs produce ~inputLen/4 tokens, but we start smaller.
	estTokens := inputLen/4 + 8
	if cap(t.tsBuf) < estTokens {
		t.tsBuf = make([]Token, 0, estTokens)
		t.idxBuf = make([]int, 0, estTokens)
	}
	ts := t.tsBuf[:0]
	indicies := t.idxBuf[:0]

	run := 0
	firstChar := input[0]
	lastToken := tokenLookup[firstChar]

	// Reset string buffer - only track for C1 tokens
	t.strLen = 0
	if lastToken == C1 {
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
		if currentToken == C1 && t.strLen < maxRun {
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
	result := make([]Token, n)
	copy(result, ts)
	resultIdx := make([]int, n)
	copy(resultIdx, indicies)
	return result, resultIdx
}

func getSpecialShortToken(char byte) Token {
	// Only T and Z are special single-char tokens
	if char == 'T' {
		return T
	}
	if char == 'Z' {
		return Zone
	}
	return End
}

// getSpecialLongToken returns a special token that is > 1 character.
// NOTE: This set of tokens is non-exhaustive and can be expanded.
func getSpecialLongToken(input string) Token {
	// Length-based dispatch for faster rejection
	switch len(input) {
	case 2:
		if input == "AM" || input == "PM" {
			return Apm
		}
	case 3:
		switch input {
		case "JAN", "FEB", "MAR", "APR", "MAY", "JUN",
			"JUL", "AUG", "SEP", "OCT", "NOV", "DEC":
			return Month
		case "MON", "TUE", "WED", "THU", "FRI", "SAT", "SUN":
			return Day
		case "UTC", "GMT", "EST", "EDT", "CST", "CDT",
			"MST", "MDT", "PST", "PDT", "JST", "KST",
			"IST", "MSK", "CET", "BST", "HST", "HDT",
			"NST", "NDT":
			return Zone
		}
	case 4:
		switch input {
		case "CEST", "NZST", "NZDT", "ACST", "ACDT",
			"AEST", "AEDT", "AWST", "AWDT", "AKST",
			"AKDT", "CHST", "CHDT":
			return Zone
		}
	}
	return End
}

// tokenToString converts a single token to a debug string.
func tokenToString(token Token) string {
	if token >= D1 && token <= D10 {
		return strings.Repeat("D", int(token-D1)+1)
	} else if token >= C1 && token <= C10 {
		return strings.Repeat("C", int(token-C1)+1)
	}

	switch token {
	case Space:
		return " "
	case Colon:
		return ":"
	case Semicolon:
		return ";"
	case Dash:
		return "-"
	case Underscore:
		return "_"
	case Fslash:
		return "/"
	case Bslash:
		return "\\"
	case Period:
		return "."
	case Comma:
		return ","
	case Singlequote:
		return "'"
	case Doublequote:
		return "\""
	case Backtick:
		return "`"
	case Tilda:
		return "~"
	case Star:
		return "*"
	case Plus:
		return "+"
	case Equal:
		return "="
	case Parenopen:
		return "("
	case Parenclose:
		return ")"
	case Braceopen:
		return "{"
	case Braceclose:
		return "}"
	case Bracketopen:
		return "["
	case Bracketclose:
		return "]"
	case Ampersand:
		return "&"
	case Exclamation:
		return "!"
	case At:
		return "@"
	case Pound:
		return "#"
	case Dollar:
		return "$"
	case Percent:
		return "%"
	case Uparrow:
		return "^"
	case Month:
		return "MTH"
	case Day:
		return "DAY"
	case Apm:
		return "PM"
	case T:
		return "T"
	case Zone:
		return "ZONE"
	}
	return ""
}

// tokensToString converts a list of tokens to a debug string.
func TokensToString(tokens []Token) string {
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
func IsMatch(seqA []Token, seqB []Token, thresh float64) bool {
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
