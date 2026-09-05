// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pattern

import "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/preprocessor"

// Token is one structural element produced from a log message.
type Token = preprocessor.Token

// Tokenizer converts log bytes into a structural token sequence.
type Tokenizer = preprocessor.Tokenizer

// UUID identifies a canonical 8-4-4-4-12 hexadecimal UUID.
const UUID = preprocessor.UUID

// NewTokenizer returns the tokenizer used by the Logs preprocessing pipeline.
// maxEvalBytes limits how much of each input is evaluated; zero means no limit.
func NewTokenizer(maxEvalBytes int) *Tokenizer {
	return preprocessor.NewTokenizer(maxEvalBytes)
}

// IsMatch reports whether the shared prefix of two token sequences satisfies
// the requested positional-equality threshold.
func IsMatch(seqA, seqB []Token, threshold float64) bool {
	return preprocessor.IsMatch(seqA, seqB, threshold)
}

// TokensToString renders a token sequence for diagnostics.
func TokensToString(tokens []Token) string {
	return preprocessor.TokensToString(tokens)
}

// Hash returns the FNV-1a hash of an exact token sequence.
func Hash(tokens []Token) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	hash := uint64(offset64)
	for _, token := range tokens {
		hash ^= uint64(token)
		hash *= prime64
	}
	return hash
}
