// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patterns

import "strings"

// TokenListSignature computes the signature of a list of tokens at the given version.
// For v5+, it applies the "specialWord" concatenation rule: sequences of
// word-like tokens separated by single '.' or ':' are collapsed into "specialWord".
func TokenListSignature(tokens []Token, version int) string {
	if len(tokens) == 0 {
		return ""
	}

	sigs := make([]string, len(tokens))
	for i, t := range tokens {
		sigs[i] = t.Signature(version)
	}

	if version < 5 {
		return concatSignatures(tokens, sigs)
	}
	return concatSignaturesV5(tokens, sigs)
}

func concatSignatures(tokens []Token, sigs []string) string {
	var b strings.Builder
	for _, sig := range sigs {
		b.WriteString(sig)
	}
	return b.String()
}

func concatSignaturesV5(tokens []Token, sigs []string) string {
	n := len(tokens)
	used := make([]bool, n)
	resultSigs := make([]string, n)
	copy(resultSigs, sigs)

	i := 0
	for i < n {
		if !isWordLikeForConcat(tokens[i], sigs[i]) {
			i++
			continue
		}

		chainStart := i
		j := i + 1
		for j+1 < n {
			if tokens[j].Type == TypeSpecialCharacter &&
				(tokens[j].Value == "." || tokens[j].Value == ":") &&
				j+1 < n && isWordLikeForConcat(tokens[j+1], sigs[j+1]) {
				j += 2
			} else {
				break
			}
		}

		if j > chainStart+1 {
			for k := chainStart; k < j; k++ {
				used[k] = true
			}
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
	_ = used
	return b.String()
}

func isWordLikeForConcat(t Token, sig string) bool {
	switch sig {
	case "word", "textWithDigits", "specialWord":
		return true
	}
	return false
}

// Parse tokenizes a message and returns the token list.
func Parse(message string) []Token {
	tokenizer := NewTokenizer()
	return tokenizer.Tokenize(message)
}

// Signature tokenizes a message and returns its signature.
func MessageSignature(message string, version int) string {
	tokens := Parse(message)
	return TokenListSignature(tokens, version)
}
