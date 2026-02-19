//go:build rust_patterns

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rtokenizer

/*
#cgo LDFLAGS: -L${SRCDIR}/../../../../../dev/lib -lpatterns -Wl,-rpath,${SRCDIR}/../../../../../dev/lib
#include <stdlib.h>
#include <stdint.h>
#include <stddef.h>

// Forward declarations for Rust FFI functions
extern const uint8_t* patterns_tokenize_log(const char* log_ptr, size_t* out_len, size_t* out_cap);
extern void patterns_vec_free(uint8_t* ptr, size_t len, size_t cap);
extern void patterns_str_free(char* ptr);
extern char* patterns_get_last_err_str();
extern char* patterns_get_last_err_str_and_reset();
extern void patterns_reset_last_err_str();
*/
import "C"
import (
	"fmt"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/logs/patterns/token"
)

// Tokenizer wraps the Rust tokenization library
// Implements token.Tokenizer interface
type Tokenizer struct{}

// NewRustTokenizer creates a new Rust tokenizer instance.
// Returns a token.Tokenizer interface.
func NewRustTokenizer() token.Tokenizer {
	return &Tokenizer{}
}

// Tokenize calls the Rust tokenization library and converts the result to Agent TokenList
// Implements token.Tokenizer interface
func (rt *Tokenizer) Tokenize(log string) (*token.TokenList, error) {
	// Convert Go string to C string
	cLog := C.CString(log)
	defer C.free(unsafe.Pointer(cLog))

	// Call Rust tokenize function
	var outLen C.size_t
	var outCap C.size_t
	cResult := C.patterns_tokenize_log(cLog, &outLen, &outCap)
	if cResult == nil {
		// Get error message from Rust
		cErr := C.patterns_get_last_err_str_and_reset()
		if cErr != nil {
			defer C.patterns_str_free(cErr)
			errMsg := C.GoString(cErr)
			return nil, fmt.Errorf("tokenization failed: %s", errMsg)
		}
		return nil, fmt.Errorf("tokenization failed: unknown error")
	}
	// Free the result buffer
	defer C.patterns_vec_free((*C.uint8_t)(unsafe.Pointer(cResult)), outLen, outCap)

	buf := unsafe.Slice((*byte)(unsafe.Pointer(cResult)), int(outLen))
	return decodeTokenizeResponse(buf, log)
}

// ResetError clears any cached error from the Rust side
func (rt *Tokenizer) ResetError() {
	C.patterns_reset_last_err_str()
}

// GetLastError retrieves the last error from the Rust side without clearing it
func (rt *Tokenizer) GetLastError() string {
	cErr := C.patterns_get_last_err_str()
	if cErr == nil {
		return ""
	}
	defer C.patterns_str_free(cErr)
	return C.GoString(cErr)
}
