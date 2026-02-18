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

	// TODO: Consider adding a batch tokenization. Might be useful for high-throughput scenarios.
}

// TokenizeResult represents the tokenization result for a single log.
type TokenizeResult struct {
	TokenList *TokenList
	Err       error
}
