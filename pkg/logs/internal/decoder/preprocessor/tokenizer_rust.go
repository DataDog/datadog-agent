// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build rust_preprocessor && cgo

package preprocessor

/*
#cgo CFLAGS:  -I${SRCDIR}/rust/include
#cgo LDFLAGS: ${SRCDIR}/../../../../../target/release/libdd_preprocessor_tokenizer.a
#include "dd_preprocessor_tokenizer.h"
*/
import "C"

import "unsafe"

// RustTokenizer wraps the Rust-based tokenizer via CGo FFI.
// It produces byte-identical output to the Go Tokenizer.
type RustTokenizer struct {
	handle    *C.dd_tokenizer
	tokensBuf []byte
	idxBuf    []C.int32_t
}

// NewRustTokenizer creates a Rust-backed tokenizer.
func NewRustTokenizer(maxEvalBytes int) *RustTokenizer {
	initCap := 256
	return &RustTokenizer{
		handle:    C.dd_tokenizer_new(C.size_t(maxEvalBytes)),
		tokensBuf: make([]byte, initCap),
		idxBuf:    make([]C.int32_t, initCap),
	}
}

// Tokenize tokenizes the input bytes and returns tokens and their start indices.
func (t *RustTokenizer) Tokenize(input []byte) ([]Token, []int) {
	if len(input) == 0 || t.handle == nil {
		return nil, nil
	}

	for {
		n := C.dd_tokenizer_tokenize(
			t.handle,
			(*C.uint8_t)(unsafe.Pointer(&input[0])),
			C.size_t(len(input)),
			(*C.uint8_t)(unsafe.Pointer(&t.tokensBuf[0])),
			(*C.int32_t)(unsafe.Pointer(&t.idxBuf[0])),
			C.size_t(len(t.tokensBuf)),
		)
		if n >= 0 {
			count := int(n)
			tokens := make([]Token, count)
			indices := make([]int, count)
			for i := 0; i < count; i++ {
				tokens[i] = Token(t.tokensBuf[i])
				indices[i] = int(t.idxBuf[i])
			}
			return tokens, indices
		}
		// Capacity insufficient — double and retry
		t.tokensBuf = make([]byte, len(t.tokensBuf)*2)
		t.idxBuf = make([]C.int32_t, len(t.idxBuf)*2)
	}
}

// Close frees the Rust tokenizer handle.
func (t *RustTokenizer) Close() {
	if t.handle != nil {
		C.dd_tokenizer_free(t.handle)
		t.handle = nil
	}
}
