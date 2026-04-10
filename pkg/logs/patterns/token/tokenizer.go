// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package token

// Tokenizer is an interface for log tokenization implementations.
// Different tokenizers can be used based on build tags (e.g., Rust, native Go).
type Tokenizer interface {
	// Tokenize processes a single log string and returns a TokenList or an error.
	Tokenize(log string) (*TokenList, error)

	// TokenizeBatch processes multiple log strings and returns one result per input.
	// Results are guaranteed to be in the same order as the input slice.
	// Implementations that do not support true batch processing fall back to sequential
	// single-log tokenization. The Rust FFI implementation amortizes CGo boundary
	// cost across the batch.
	TokenizeBatch(logs []string) ([]TokenizeResult, error)
}

// TokenizeResult represents the tokenization result for a single log.
type TokenizeResult struct {
	TokenList *TokenList
	Err       error
}
