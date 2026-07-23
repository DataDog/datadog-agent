// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor provides tokenization functionality for log messages.
package preprocessor

import (
	"encoding/binary"
	"math"
	"strings"
)

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 below.
const maxRun = 10

// maxSpecialTokenLen is the maximum character run length eligible for special token
// promotion. Longest critical keyword: "EMERGENCY" / "EXCEPTION" = 9 chars.
const maxSpecialTokenLen = 9

// Clearing the ASCII case bit uppercases letters. The wider masks apply the
// same operation to several packed bytes at once.
const (
	asciiCaseBit     = byte(0x20)
	asciiUpperMask16 = uint16(0xdfdf)
	asciiUpperMask32 = uint32(0xdfdfdfdf)
	asciiUpperMask64 = uint64(0xdfdfdfdfdfdfdfdf)
)

// tokenLookup is a 256-byte lookup table for single-byte token classification.
var tokenLookup = makeTokenLookup()

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
	tsBuf        []Token // Reusable token buffer
	idxBuf       []int   // Reusable index buffer
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

// Tokenize returns caller-owned tokens and start indices: the returned slices
// are freshly allocated copies, so callers may retain them indefinitely and
// across future calls. This is the public API for callers that store tokens
// (e.g. config-time samples in user_samples.go, timestamp formats in
// timestamp_detector.go, adaptive-sampler rules in decoder.go).
//
// The per-log-line preprocessing pipeline does NOT use this method: it calls
// tokenizeBorrowed to avoid the two allocations below. Do not "unify" the two —
// the copy here is exactly what lets external callers own their tokens.
func (t *Tokenizer) Tokenize(input []byte) ([]Token, []int) {
	tokens, indices := t.tokenizeBorrowed(input)
	if len(tokens) == 0 {
		return nil, nil
	}

	result := make([]Token, len(tokens))
	copy(result, tokens)
	resultIndices := make([]int, len(indices))
	copy(resultIndices, indices)
	return result, resultIndices
}

// tokenizeBorrowed tokenizes input without allocating result slices: the
// returned slices ALIAS the Tokenizer's reusable scratch buffers (tsBuf/idxBuf)
// and are overwritten by the next call on t. This is the hot path, run once per
// decoded log line by the preprocessing pipeline, and is why the tokenizer is
// not thread-safe.
//
// Any consumer that keeps the tokens past the current call MUST clone them
// first (see cloneTokens). The pipeline stages that retain first-line tokens do
// exactly that: the aggregators, the adaptive sampler, and the auto-multiline
// pattern table. The labeler only reads them synchronously, so it borrows.
func (t *Tokenizer) tokenizeBorrowed(input []byte) ([]Token, []int) {
	maxBytes := len(input)
	if t.maxEvalBytes > 0 && t.maxEvalBytes < maxBytes {
		maxBytes = t.maxEvalBytes
	}
	return t.tokenizeIntoBuffers(input[:maxBytes])
}

// emitToken appends one token (and its start index) to the reusable buffers,
// checking for special-token promotion first.
//
// Performance-sensitive — two deliberate choices, do not "clean up":
//   - It writes through the t.tsBuf/t.idxBuf fields instead of taking and
//     returning the two slices. emitToken runs once per token; threading two
//     slice headers in and out is pure per-token marshalling, and keeping them
//     out of the signature frees registers for the hot per-byte scan loop.
//   - It stays a separate, non-inlined call. Manually inlining the body into
//     the scan loop was measured to regress homogeneous-run inputs (long word/
//     number runs) by ~2.5x, because the larger loop body spills registers.
func (t *Tokenizer) emitToken(input []byte, token Token, start, end int) {
	runLen := end - start

	// Check for special tokens (only for C1/letter runs, length 1-maxSpecialTokenLen)
	if token == C1 && runLen <= maxSpecialTokenLen {
		if specialToken := getSpecialToken(input[start:end]); specialToken != End {
			t.tsBuf = append(t.tsBuf, specialToken)
			t.idxBuf = append(t.idxBuf, start)
			return
		}
	}

	// Regular token - encode run length for C1/D1
	t.idxBuf = append(t.idxBuf, start)
	if token == C1 || token == D1 {
		r := runLen - 1
		if r >= maxRun {
			r = maxRun - 1
		}
		t.tsBuf = append(t.tsBuf, token+Token(r))
	} else {
		t.tsBuf = append(t.tsBuf, token)
	}
}

// tokenizeIntoBuffers scans input a single time and emits tokens into the
// reusable buffers. The returned slices alias those buffers (see
// tokenizeBorrowed for the lifetime contract).
func (t *Tokenizer) tokenizeIntoBuffers(input []byte) ([]Token, []int) {
	inputLen := len(input)
	if inputLen == 0 {
		return nil, nil
	}

	// Reuse the scratch buffers across calls; only reallocate when the estimate
	// outgrows capacity. Most logs produce ~inputLen/4 tokens. Reusing the
	// buffers is what makes tokenizeBorrowed allocation-free (and what makes its
	// output borrowed — see there).
	estTokens := inputLen/4 + 8
	if cap(t.tsBuf) < estTokens {
		t.tsBuf = make([]Token, 0, estTokens)
		t.idxBuf = make([]int, 0, estTokens)
	} else {
		t.tsBuf = t.tsBuf[:0]
		t.idxBuf = t.idxBuf[:0]
	}

	start := 0
	lastToken := tokenLookup[input[0]]

	// Hot loop: keep it minimal — one table lookup, one compare, one branch per
	// byte — so its few variables stay register-resident. A byte only ends a run
	// when its class differs from the previous one; all real work (special-token
	// promotion, run-length encoding, appends) happens at run boundaries in
	// emitToken, which runs far less often than once per byte. Adding per-byte
	// work here (extra branches, tracking, etc.) regresses long homogeneous runs.
	for i := 1; i < inputLen; i++ {
		currentToken := tokenLookup[input[i]]

		if currentToken != lastToken {
			t.emitToken(input, lastToken, start, i)
			start = i
			lastToken = currentToken
		}
	}

	// Flush the final run.
	t.emitToken(input, lastToken, start, inputLen)

	return t.tsBuf, t.idxBuf
}

// getSpecialToken returns a case-insensitive special token, or End if the run
// is not a recognized keyword.
//
// This is a SWAR (SIMD-within-a-register) match: it loads 2-8 bytes at once and
// case-folds them with a mask, comparing whole machine words against constants
// instead of uppercasing byte-by-byte into a scratch buffer or allocating a
// string. Keep it allocation-free and operating directly on the input slice —
// converting to string() or building a buffer here would undo the win, since
// this runs for every letter run up to maxSpecialTokenLen. encoding/binary
// keeps the unaligned loads portable; clearing asciiCaseBit can only fold
// letters (it never turns a non-letter into one). The case constants are
// little-endian byte packings, e.g. 'J'|'A'<<8|'N'<<16 == "JAN".
// NOTE: This set of tokens is non-exhaustive and can be expanded; keep it in
// sync with tokenToString.
func getSpecialToken(input []byte) Token {
	// Length-based dispatch for faster rejection
	switch len(input) {
	case 1:
		folded := input[0] &^ asciiCaseBit
		switch folded {
		case 'T':
			return T
		case 'Z':
			return Zone
		}
	case 2:
		folded := uint64(binary.LittleEndian.Uint16(input) & asciiUpperMask16)
		if folded == 'A'|'M'<<8 || folded == 'P'|'M'<<8 {
			return Apm
		}
	case 3:
		folded := uint64(binary.LittleEndian.Uint16(input)&asciiUpperMask16) |
			uint64(input[2]&^asciiCaseBit)<<16
		switch folded {
		case 'J' | 'A'<<8 | 'N'<<16,
			'F' | 'E'<<8 | 'B'<<16,
			'M' | 'A'<<8 | 'R'<<16,
			'A' | 'P'<<8 | 'R'<<16,
			'M' | 'A'<<8 | 'Y'<<16,
			'J' | 'U'<<8 | 'N'<<16,
			'J' | 'U'<<8 | 'L'<<16,
			'A' | 'U'<<8 | 'G'<<16,
			'S' | 'E'<<8 | 'P'<<16,
			'O' | 'C'<<8 | 'T'<<16,
			'N' | 'O'<<8 | 'V'<<16,
			'D' | 'E'<<8 | 'C'<<16:
			return Month
		case 'M' | 'O'<<8 | 'N'<<16,
			'T' | 'U'<<8 | 'E'<<16,
			'W' | 'E'<<8 | 'D'<<16,
			'T' | 'H'<<8 | 'U'<<16,
			'F' | 'R'<<8 | 'I'<<16,
			'S' | 'A'<<8 | 'T'<<16,
			'S' | 'U'<<8 | 'N'<<16:
			return Day
		case 'U' | 'T'<<8 | 'C'<<16,
			'G' | 'M'<<8 | 'T'<<16,
			'E' | 'S'<<8 | 'T'<<16,
			'E' | 'D'<<8 | 'T'<<16,
			'C' | 'S'<<8 | 'T'<<16,
			'C' | 'D'<<8 | 'T'<<16,
			'M' | 'S'<<8 | 'T'<<16,
			'M' | 'D'<<8 | 'T'<<16,
			'P' | 'S'<<8 | 'T'<<16,
			'P' | 'D'<<8 | 'T'<<16,
			'J' | 'S'<<8 | 'T'<<16,
			'K' | 'S'<<8 | 'T'<<16,
			'I' | 'S'<<8 | 'T'<<16,
			'M' | 'S'<<8 | 'K'<<16,
			'C' | 'E'<<8 | 'T'<<16,
			'B' | 'S'<<8 | 'T'<<16,
			'H' | 'S'<<8 | 'T'<<16,
			'H' | 'D'<<8 | 'T'<<16,
			'N' | 'S'<<8 | 'T'<<16,
			'N' | 'D'<<8 | 'T'<<16:
			return Zone
		}
	case 4:
		folded := uint64(binary.LittleEndian.Uint32(input) & asciiUpperMask32)
		switch folded {
		case 'W' | 'A'<<8 | 'R'<<16 | 'N'<<24:
			return Warn
		case 'C' | 'R'<<8 | 'I'<<16 | 'T'<<24:
			return Critical
		case 'C' | 'E'<<8 | 'S'<<16 | 'T'<<24,
			'N' | 'Z'<<8 | 'S'<<16 | 'T'<<24,
			'N' | 'Z'<<8 | 'D'<<16 | 'T'<<24,
			'A' | 'C'<<8 | 'S'<<16 | 'T'<<24,
			'A' | 'C'<<8 | 'D'<<16 | 'T'<<24,
			'A' | 'E'<<8 | 'S'<<16 | 'T'<<24,
			'A' | 'E'<<8 | 'D'<<16 | 'T'<<24,
			'A' | 'W'<<8 | 'S'<<16 | 'T'<<24,
			'A' | 'W'<<8 | 'D'<<16 | 'T'<<24,
			'A' | 'K'<<8 | 'S'<<16 | 'T'<<24,
			'A' | 'K'<<8 | 'D'<<16 | 'T'<<24,
			'C' | 'H'<<8 | 'S'<<16 | 'T'<<24,
			'C' | 'H'<<8 | 'D'<<16 | 'T'<<24:
			return Zone
		}
	case 5:
		folded := uint64(binary.LittleEndian.Uint32(input)&asciiUpperMask32) |
			uint64(input[4]&^asciiCaseBit)<<32
		switch folded {
		case 'F' | 'A'<<8 | 'T'<<16 | 'A'<<24 | 'L'<<32:
			return Fatal
		case 'E' | 'R'<<8 | 'R'<<16 | 'O'<<24 | 'R'<<32:
			return Error
		case 'P' | 'A'<<8 | 'N'<<16 | 'I'<<24 | 'C'<<32:
			return Panic
		case 'A' | 'L'<<8 | 'E'<<16 | 'R'<<24 | 'T'<<32:
			return Alert
		case 'E' | 'M'<<8 | 'E'<<16 | 'R'<<24 | 'G'<<32:
			return Emergency
		case 'C' | 'R'<<8 | 'A'<<16 | 'S'<<24 | 'H'<<32:
			return Crash
		}
	case 6:
		folded := uint64(binary.LittleEndian.Uint32(input)&asciiUpperMask32) |
			uint64(binary.LittleEndian.Uint16(input[4:])&asciiUpperMask16)<<32
		switch folded {
		case 'S' | 'E'<<8 | 'V'<<16 | 'E'<<24 | 'R'<<32 | 'E'<<40:
			return Severe
		case 'F' | 'A'<<8 | 'I'<<16 | 'L'<<24 | 'E'<<32 | 'D'<<40:
			return Failure
		}
	case 7:
		folded := uint64(binary.LittleEndian.Uint32(input)&asciiUpperMask32) |
			uint64(binary.LittleEndian.Uint16(input[4:])&asciiUpperMask16)<<32 |
			uint64(input[6]&^asciiCaseBit)<<48
		switch folded {
		case 'W' | 'A'<<8 | 'R'<<16 | 'N'<<24 | 'I'<<32 | 'N'<<40 | 'G'<<48:
			return Warn
		case 'C' | 'R'<<8 | 'A'<<16 | 'S'<<24 | 'H'<<32 | 'E'<<40 | 'D'<<48:
			return Crash
		case 'F' | 'A'<<8 | 'I'<<16 | 'L'<<24 | 'U'<<32 | 'R'<<40 | 'E'<<48:
			return Failure
		case 'T' | 'I'<<8 | 'M'<<16 | 'E'<<24 | 'O'<<32 | 'U'<<40 | 'T'<<48:
			return Timeout
		}
	case 8:
		folded := binary.LittleEndian.Uint64(input) & asciiUpperMask64
		switch folded {
		case 'C' | 'R'<<8 | 'I'<<16 | 'T'<<24 | 'I'<<32 | 'C'<<40 | 'A'<<48 | 'L'<<56:
			return Critical
		case 'D' | 'E'<<8 | 'A'<<16 | 'D'<<24 | 'L'<<32 | 'O'<<40 | 'C'<<48 | 'K'<<56:
			return Deadlock
		}
	case 9:
		folded := binary.LittleEndian.Uint64(input) & asciiUpperMask64
		switch folded {
		case 'E' | 'M'<<8 | 'E'<<16 | 'R'<<24 | 'G'<<32 | 'E'<<40 | 'N'<<48 | 'C'<<56:
			if input[8]&^asciiCaseBit == 'Y' {
				return Emergency
			}
		case 'E' | 'X'<<8 | 'C'<<16 | 'E'<<24 | 'P'<<32 | 'T'<<40 | 'I'<<48 | 'O'<<56:
			if input[8]&^asciiCaseBit == 'N' {
				return Exception
			}
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
	case Warn:
		return "WARN"
	case Fatal:
		return "FATAL"
	case Error:
		return "ERROR"
	case Panic:
		return "PANIC"
	case Alert:
		return "ALERT"
	case Severe:
		return "SEVERE"
	case Critical:
		return "CRIT"
	case Emergency:
		return "EMERG"
	case Exception:
		return "EXCEPTION"
	case Crash:
		return "CRASH"
	case Failure:
		return "FAILURE"
	case Deadlock:
		return "DEADLOCK"
	case Timeout:
		return "TIMEOUT"
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
