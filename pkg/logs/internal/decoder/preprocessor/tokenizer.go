// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import logpattern "github.com/DataDog/datadog-agent/pkg/logs/pattern"

// Tokenizer wraps the shared log pattern tokenizer for compatibility with the
// decoder preprocessor package.
type Tokenizer struct {
	inner *logpattern.Tokenizer
}

// NewTokenizer returns a tokenizer that evaluates at most maxEvalBytes bytes.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	return &Tokenizer{inner: logpattern.NewTokenizer(maxEvalBytes)}
}

// Tokenize tokenizes input into its structural tokens and start indices.
func (t *Tokenizer) Tokenize(input []byte) ([]Token, []int) {
	return t.inner.Tokenize(input)
}

func (t *Tokenizer) tokenize(input []byte) ([]Token, []int) {
	return t.Tokenize(input)
}

func tokenToString(token Token) string {
	return logpattern.TokenString(token)
}

// TokensToString converts tokens into a human-readable structural pattern.
func TokensToString(tokens []Token) string {
	return logpattern.TokensToString(tokens)
}

// IsMatch reports whether two token sequences meet the positional match threshold.
func IsMatch(seqA []Token, seqB []Token, threshold float64) bool {
	return logpattern.IsMatch(seqA, seqB, threshold)
}
