// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package preprocessor provides tokenization functionality for log messages.
package preprocessor

import (
	"math"
	"strings"
)

// maxRun is the maximum run of a char or digit before it is capped.
// Note: This must not exceed d10 or c10 below.
const (
	maxRun         = 10
	ipv4TokenWidth = 7 // D Period D Period D Period D
	// maxIPv4OctetDigits is the longest digit run that can be an octet, so a
	// run longer than this can never close a dotted quad.
	maxIPv4OctetDigits = 3
)

// maxSpecialTokenLen and the special-token/debug-string tables are generated
// from the master list in gentokentables/main.go into token_tables_gen.go.
// `bazel run //pkg/logs/internal/decoder/preprocessor:token_tables` is
// equivalent, and its companion :token_tables_test fails if the committed file
// is stale.
//go:generate go run ./gentokentables -output token_tables_gen.go

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

	// Special characters (specialChars is generated from the master token list).
	for _, c := range specialChars {
		lookup[c.ch] = c.tok
	}

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

// Tokenize returns freshly-allocated, caller-owned tokens and start indices,
// safe to retain. It is the public API for callers that store tokens (config
// samples, timestamp formats, sampler rules). The per-line preprocessing
// pipeline uses tokenizeBorrowed instead to avoid these allocations.
func (t *Tokenizer) Tokenize(input []byte) ([]Token, []int) {
	tokens, indices := t.tokenizeCapped(input)
	if len(tokens) == 0 {
		return nil, nil
	}
	// Copy out of the scratch buffers so the caller owns the result. make+copy,
	// not slices.Clone, so the result is sized exactly.
	result := make([]Token, len(tokens))
	copy(result, tokens)
	resultIndices := make([]int, len(indices))
	copy(resultIndices, indices)
	return result, resultIndices
}

// tokenizeCapped applies the maxEvalBytes limit and tokenizes into the scratch
// buffers, returning the borrowed slices. Shared by Tokenize (which copies them)
// and tokenizeBorrowed (which wraps them).
func (t *Tokenizer) tokenizeCapped(input []byte) ([]Token, []int) {
	maxBytes := len(input)
	if t.maxEvalBytes > 0 && t.maxEvalBytes < maxBytes {
		maxBytes = t.maxEvalBytes
	}
	return t.tokenizeIntoBuffers(input[:maxBytes])
}

// tokenizeBorrowed returns a BorrowedTokens view aliasing the reusable scratch
// buffers, valid only until the next call on t (hence not thread-safe). This is
// the per-line hot path; consumers that retain the tokens must Clone them.
func (t *Tokenizer) tokenizeBorrowed(input []byte) BorrowedTokens {
	return newBorrowedTokens(t.tokenizeCapped(input))
}

// emitToken appends one token (and its start index) to the reusable buffers,
// promoting C1 letter runs to special tokens first. It writes through the
// t.tsBuf/t.idxBuf fields and is deliberately a separate, non-inlined call:
// both keep the per-byte scan loop small and register-resident. Do not inline
// it into the loop or make it take/return the slices.
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
		// A dotted quad can only ever be closed by its final octet, so this is
		// the one emission that can complete the pattern. Checking here keeps
		// the cost off every other token and avoids a second pass entirely.
		if token == D1 && runLen <= maxIPv4OctetDigits {
			t.collapseIPv4Tail()
		}
		return
	}
	t.tsBuf = append(t.tsBuf, token)
}

// tokenizeIntoBuffers scans input a single time and emits tokens into the
// reusable buffers. The returned slices alias those buffers (see
// tokenizeBorrowed for the lifetime contract).
func (t *Tokenizer) tokenizeIntoBuffers(input []byte) ([]Token, []int) {
	if len(input) == 0 {
		return nil, nil
	}
	t.emitRuns(input)
	return t.tsBuf, t.idxBuf
}

// emitRuns run-length-encodes input into tsBuf/idxBuf, collapsing hybrid tokens
// via emitToken as each one completes.
func (t *Tokenizer) emitRuns(input []byte) {
	inputLen := len(input)
	if inputLen == 0 {
		t.tsBuf = t.tsBuf[:0]
		t.idxBuf = t.idxBuf[:0]
		return
	}

	// Reuse the scratch buffers across calls; only grow when the estimate
	// (~inputLen/4 tokens) exceeds capacity. This makes the borrowed path
	// allocation-free.
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

	// Hot loop: one table lookup, one compare, one branch per byte, so its state
	// stays register-resident. All token emission happens at run boundaries via
	// emitToken. Avoid adding per-byte work here.
	for i := 1; i < inputLen; i++ {
		currentToken := tokenLookup[input[i]]

		if currentToken != lastToken {
			t.emitToken(input, lastToken, start, i)
			start = i
			lastToken = currentToken
		}
	}

	t.emitToken(input, lastToken, start, inputLen)
}

func isIPv4OctetToken(tok Token) bool {
	return tok >= D1 && tok <= D3
}

// collapseIPv4Tail rewrites a dotted quad ending at the last emitted token into
// a single IPv4 token. As separate digit/period tokens a quad looks like a
// timestamp fragment to the detector, and addresses with different octet widths
// would otherwise be different sampler patterns.
//
// Called from emitToken immediately after a 1-3 digit run is appended, which is
// the only emission that can close the pattern, so the tokenizer never makes a
// second pass over the token list. Each octet must be a 1-3 digit run and each
// separator a single '.'; a collapsed ".." / "..." Period token is rejected via
// the start indices.
func (t *Tokenizer) collapseIPv4Tail() {
	n := len(t.tsBuf)
	if n < ipv4TokenWidth {
		return
	}
	ts := t.tsBuf[n-ipv4TokenWidth:]

	// Separators first: they reject nearly every call in a single compare, and
	// the trailing octet is already known from the caller.
	if ts[5] != Period || ts[3] != Period || ts[1] != Period {
		return
	}
	if !isIPv4OctetToken(ts[0]) || !isIPv4OctetToken(ts[2]) || !isIPv4OctetToken(ts[4]) {
		return
	}
	idx := t.idxBuf[n-ipv4TokenWidth:]
	if idx[2] != idx[1]+1 || idx[4] != idx[3]+1 || idx[6] != idx[5]+1 {
		return
	}

	// Keep idx[0] (the address start) and drop the six tokens it absorbed.
	ts[0] = IPv4
	t.tsBuf = t.tsBuf[:n-ipv4TokenWidth+1]
	t.idxBuf = t.idxBuf[:n-ipv4TokenWidth+1]
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
