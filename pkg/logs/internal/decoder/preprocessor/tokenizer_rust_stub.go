// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !rust_preprocessor || !cgo

package preprocessor

// RustTokenizer is a stub when the Rust library is not available.
type RustTokenizer struct{}

// NewRustTokenizer returns nil when Rust support is not compiled in.
func NewRustTokenizer(_ int) *RustTokenizer {
	return nil
}

// Tokenize is a no-op stub.
func (t *RustTokenizer) Tokenize(_ []byte) ([]Token, []int) {
	return nil, nil
}

// Close is a no-op stub.
func (t *RustTokenizer) Close() {}
