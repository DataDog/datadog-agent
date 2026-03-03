// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Build the Rust library before building this package:
//
//go:generate cargo build --manifest-path rust/Cargo.toml --target-dir rust/target

package filterlistimpl

/*
#cgo LDFLAGS: ${SRCDIR}/rust/target/debug/libdd_tagmatcher.a -ldl -lpthread -lm
#cgo CFLAGS: -I${SRCDIR}/rust/include

#include "dd_tagmatcher.h"
#include <stdlib.h>
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// It is heap-allocated by Rust; the finalizer calls tagmatcher_regex_free to
// release it when the Go GC collects the wrapper.
type rustRegex struct {
	ptr *C.TagMatcherRegex
}

// newRustRegex compiles pattern using the Rust regex crate.
// Returns nil if the pattern is empty or fails to compile.
func newRustRegex(pattern string) *rustRegex {
	cPattern := C.CString(pattern)
	defer C.free(unsafe.Pointer(cPattern))

	ptr := C.tagmatcher_regex_new(cPattern)
	if ptr == nil {
		return nil
	}

	re := &rustRegex{ptr: ptr}
	runtime.SetFinalizer(re, func(r *rustRegex) {
		C.tagmatcher_regex_free(r.ptr)
	})
	return re
}

// isMatch reports whether s matches the compiled regex.
// The Go string's backing bytes are passed directly to Rust without copying;
// unsafe.StringData is safe to use here because the GC does not move data
// during a CGo call.
func (re *rustRegex) isMatch(s string) bool {
	if len(s) == 0 {
		return false
	}
	return bool(C.tagmatcher_regex_is_match(re.ptr,
		(*C.char)(unsafe.Pointer(unsafe.StringData(s))),
		C.size_t(len(s))))
}
