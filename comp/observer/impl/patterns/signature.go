// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import "strings"

// TokenListSignature computes the signature of a list of tokens.
// Sequences of word-like tokens separated by single '.' or ':' are collapsed into "specialWord".
func TokenListSignature(tokens []Token) string {
	if len(tokens) == 0 {
		return ""
	}

	sigs := make([]string, len(tokens))
	for i, t := range tokens {
		sigs[i] = t.Signature()
	}

	return concatSignatures(tokens, sigs)
}

// concatSignatures concatenates token signatures, collapsing chains of word-like
// tokens separated by '.' or ':' into a single "specialWord" signature.
func concatSignatures(tokens []Token, sigs []string) string {
	n := len(tokens)
	resultSigs := make([]string, n)
	copy(resultSigs, sigs)

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
				j+1 < n && isWordLikeForConcat(sigs[j+1]) {
				j += 2
			} else {
				break
			}
		}

		if j > chainStart+1 {
			resultSigs[chainStart] = "specialWord"
			for k := chainStart + 1; k < j; k++ {
				resultSigs[k] = ""
			}
		}
		i = j
	}

	var b strings.Builder
	for i := 0; i < n; i++ {
		if resultSigs[i] != "" {
			b.WriteString(resultSigs[i])
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
