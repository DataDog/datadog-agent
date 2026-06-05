// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import "strings"

// TokenListSignature computes the signature of a list of tokens.
// Sequences of word-like tokens separated by single '.' or ':' are collapsed
// into "specialWord". A single pass over the tokens emits directly into a
// builder, with a small lookahead state for chain detection — no per-token
// signature string slice is allocated.
func TokenListSignature(tokens []Token) string {
	n := len(tokens)
	if n == 0 {
		return ""
	}

	var b strings.Builder
	// Most token signatures are short; this is a generous estimate.
	b.Grow(n * 6)

	i := 0
	for i < n {
		t := tokens[i]
		if !isTokenWordLikeForConcat(t) {
			b.WriteString(t.Signature())
			i++
			continue
		}
		// Probe forward for a chain `word ('.'|':') word ...`. We require at
		// least one separator+word to flip to "specialWord"; otherwise emit the
		// single word's normal signature.
		j := i + 1
		chained := false
		for j+1 < n &&
			tokens[j].Type == TypeSpecialCharacter &&
			(tokens[j].Value == "." || tokens[j].Value == ":") &&
			isTokenWordLikeForConcat(tokens[j+1]) {
			j += 2
			chained = true
		}
		if chained {
			b.WriteString("specialWord")
			i = j
			continue
		}
		b.WriteString(t.Signature())
		i++
	}

	return b.String()
}

// isTokenWordLikeForConcat is true when a token participates in the
// "specialWord" chain collapse. Word tokens always qualify; the chained form
// becomes "specialWord" via the caller.
func isTokenWordLikeForConcat(t Token) bool {
	return t.Type == TypeWord
}

// Parse tokenizes a message and returns the token list.
func Parse(message string) []Token {
	tokenizer := NewTokenizer()
	return tokenizer.Tokenize(message)
}

// MessageSignature tokenizes a message and returns its signature.
func MessageSignature(message string) string {
	tokens := Parse(message)
	return TokenListSignature(tokens)
}
