// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import "strings"

// TokenListSignature computes the signature of a list of tokens.
// Sequences of word-like tokens separated by single '.' or ':' are collapsed into "specialWord".
func TokenListSignature(tokens []Token) string {
	n := len(tokens)
	if n == 0 {
		return ""
	}

	sigs := make([]string, n)
	for i, t := range tokens {
		sigs[i] = t.Signature()
	}

	// In-place collapse: chains of word-like tokens separated by '.' or ':' become "specialWord".
	// Reading always moves forward (lookahead at j+1 is never behind any write position),
	// so mutating sigs in place is safe.
	i := 0
	for i < n {
		if !isWordLikeForConcat(sigs[i]) {
			i++
			continue
		}
		chainStart := i
		j := i + 1
		for j+1 < n {
			if tokens[j].Type == TypeSpecialCharacter &&
				(tokens[j].Value == "." || tokens[j].Value == ":") &&
				isWordLikeForConcat(sigs[j+1]) {
				j += 2
			} else {
				break
			}
		}
		if j > chainStart+1 {
			sigs[chainStart] = "specialWord"
			for k := chainStart + 1; k < j; k++ {
				sigs[k] = ""
			}
		}
		i = j
	}

	var b strings.Builder
	for _, s := range sigs {
		if s != "" {
			b.WriteString(s)
		}
	}
	return b.String()
}

func isWordLikeForConcat(sig string) bool {
	return sig == "word" || sig == "specialWord"
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
