// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build rust_patterns

// Package rtokenizer provides log tokenization using a Rust DFA library via CGo/FFI.
// The Rust tokenizer performs single-pass O(n) tokenization and returns results
// as FlatBuffers for zero-copy deserialization on the Go side.
//
// Build requirement: dda inv agent.build --build-include=rust_patterns
// Without the rust_patterns build tag, a stub is compiled that returns errors at runtime.
package rtokenizer

/*
#cgo linux,amd64 LDFLAGS: -L${SRCDIR}/vendor/linux_amd64 -lpatterns -Wl,-rpath,${SRCDIR}/vendor/linux_amd64
#cgo linux,arm64 LDFLAGS: -L${SRCDIR}/vendor/linux_arm64 -lpatterns -Wl,-rpath,${SRCDIR}/vendor/linux_arm64
#cgo darwin,arm64 LDFLAGS: -L${SRCDIR}/vendor/darwin_arm64 -lpatterns -Wl,-rpath,${SRCDIR}/vendor/darwin_arm64
#cgo darwin,amd64 LDFLAGS: -L${SRCDIR}/vendor/darwin_amd64 -lpatterns -Wl,-rpath,${SRCDIR}/vendor/darwin_amd64
#cgo windows,amd64 LDFLAGS: -L${SRCDIR}/vendor/windows_amd64 -lpatterns
#cgo CFLAGS: -I${SRCDIR}/vendor/include
#include <stdlib.h>
#include <stdint.h>
#include <stddef.h>

// Forward declarations for Rust FFI functions (single-log path — unchanged).
extern uint8_t* patterns_tokenize_log(const char* log_ptr, size_t* out_len, size_t* out_cap);
extern void patterns_vec_free(uint8_t* ptr, size_t len, size_t cap);
extern void patterns_str_free(char* ptr);
extern char* patterns_get_last_err_str();
extern char* patterns_get_last_err_str_and_reset();
extern void patterns_reset_last_err_str();

// Batch tokenization — additive, does not alter the single-log API above.
// Each BatchTokenEntry carries an independent result; the thread-local
// last-error string is NOT modified by patterns_tokenize_logs_batch.
typedef struct {
    uint8_t* ptr;       // FlatBuffer bytes (free with patterns_vec_free); null on error
    size_t   len;
    size_t   cap;
    char*    err_ptr;   // per-slot error string (free with patterns_str_free); null on success
} BatchTokenEntry;

extern void patterns_tokenize_logs_batch(
    const char** log_ptrs,
    size_t count,
    BatchTokenEntry* out_entries
);
*/
import "C"
import (
	"fmt"
	"runtime"
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
	// LockOSThread reduces stack switching and signal mask overhead across multiple CGO calls.
	// Overhead (~50ns) is justified only when multiple CGO calls occur; Tokenize does 3-4.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

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
	defer C.patterns_vec_free(cResult, outLen, outCap)

	buf := unsafe.Slice((*byte)(unsafe.Pointer(cResult)), int(outLen))
	return decodeTokenizeResponse(buf, log)
}

// TokenizeBatch tokenizes multiple logs in a single CGo call via
// patterns_tokenize_logs_batch, amortizing LockOSThread and Rust call overhead.
//
// C.CString/C.free and patterns_vec_free are still O(N) — only the Rust DFA
// call and OS-thread lock are batched. Expected: ~30-40% reduction in cgocall
// CPU vs single-log Tokenize called N times.
func (rt *Tokenizer) TokenizeBatch(logs []string) ([]token.TokenizeResult, error) {
	if len(logs) == 0 {
		return nil, nil
	}

	runtime.LockOSThread() // ONE lock for the entire batch
	defer runtime.UnlockOSThread()

	// Build C string array and defer per-log frees.
	cLogs := make([]*C.char, len(logs))
	for i, log := range logs {
		cLogs[i] = C.CString(log)
	}
	defer func() {
		for _, cl := range cLogs {
			C.free(unsafe.Pointer(cl))
		}
	}()

	// Pre-allocate output entries; Rust writes directly into this slice.
	entries := make([]C.BatchTokenEntry, len(logs))

	C.patterns_tokenize_logs_batch(
		(**C.char)(unsafe.Pointer(&cLogs[0])),
		C.size_t(len(logs)),
		&entries[0],
	)

	results := make([]token.TokenizeResult, len(logs))
	for i, entry := range entries {
		if entry.err_ptr != nil {
			errMsg := C.GoString(entry.err_ptr)
			C.patterns_str_free(entry.err_ptr)
			results[i].Err = fmt.Errorf("tokenization failed: %s", errMsg)
			continue
		}
		if entry.ptr == nil {
			results[i].Err = fmt.Errorf("tokenization failed: unknown error")
			continue
		}
		buf := unsafe.Slice((*byte)(unsafe.Pointer(entry.ptr)), int(entry.len))
		tl, err := decodeTokenizeResponse(buf, logs[i])
		C.patterns_vec_free(entry.ptr, entry.len, entry.cap)
		results[i] = token.TokenizeResult{TokenList: tl, Err: err}
	}
	return results, nil
}

// ResetError clears any cached error from the Rust side
func (rt *Tokenizer) ResetError() {
	C.patterns_reset_last_err_str()
}

// GetLastError retrieves the last error from the Rust side without clearing it
func (rt *Tokenizer) GetLastError() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	cErr := C.patterns_get_last_err_str()
	if cErr == nil {
		return ""
	}
	defer C.patterns_str_free(cErr)
	return C.GoString(cErr)
}
