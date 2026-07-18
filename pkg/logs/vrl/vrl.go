// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build vrl

// Package vrl provides a cgo bridge to a Rust static library (built from
// pkg/logs/vrl/rust) for compiling and evaluating VRL (Vector Remap
// Language) expressions against log messages.
//
// The static library must be built and installed next to this file before
// building with the `vrl` tag, e.g.:
//
//	bazel run //pkg/logs/vrl/rust:install_libs --destdir=pkg/logs/vrl/rust
package vrl

/*
#cgo CFLAGS: -I${SRCDIR}/rust/include
#cgo LDFLAGS: -L${SRCDIR}/rust -lvrl_filter -ldl -lpthread -lm
#include "vrl.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

// Program is a compiled VRL program, usable both as a boolean filter
// predicate and as a transform.
type Program struct {
	ptr *C.vrl_program_t
}

// Compile compiles a VRL expression against this package's curated function
// list. The returned Program must be closed with Close when no longer needed.
func Compile(source string) (*Program, error) {
	cs := C.CString(source)
	defer C.free(unsafe.Pointer(cs))

	var errPtr *C.char
	ptr := C.vrl_compile(cs, &errPtr)
	if ptr == nil {
		if errPtr != nil {
			msg := C.GoString(errPtr)
			C.vrl_free_string(errPtr)
			return nil, fmt.Errorf("vrl compile error: %s", msg)
		}
		return nil, errors.New("vrl compile failed (unknown error)")
	}
	return &Program{ptr: ptr}, nil
}

// Filter evaluates the program as a boolean predicate against message.
// Returns (true, nil) on match, (false, nil) on no match or abort, and
// (false, err) on a runtime error (caller should treat as no match).
func (p *Program) Filter(message []byte) (bool, error) {
	if p == nil || p.ptr == nil {
		return false, errors.New("vrl: nil program")
	}

	var errPtr *C.char
	ret := C.vrl_eval_filter(
		p.ptr,
		messagePtr(message),
		C.size_t(len(message)),
		&errPtr,
	)

	switch ret {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, evalError(errPtr)
	}
}

// Transform runs the program against message and returns the resulting
// message content. On error (a runtime failure, an abort, or a program that
// doesn't leave the message field intact), returns the original input
// unchanged alongside a non-nil error — callers should not assume the
// returned bytes reflect any partial mutation.
func (p *Program) Transform(message []byte) ([]byte, error) {
	if p == nil || p.ptr == nil {
		return message, errors.New("vrl: nil program")
	}

	var errPtr *C.char
	out := C.vrl_eval_transform(
		p.ptr,
		messagePtr(message),
		C.size_t(len(message)),
		&errPtr,
	)

	if out.data == nil {
		return message, evalError(errPtr)
	}
	defer C.vrl_free_bytes(out)
	return C.GoBytes(unsafe.Pointer(out.data), C.int(out.len)), nil
}

// Close releases the compiled program.
func (p *Program) Close() {
	if p != nil && p.ptr != nil {
		C.vrl_free_program(p.ptr)
		p.ptr = nil
	}
}

func messagePtr(message []byte) *C.char {
	if len(message) == 0 {
		return nil
	}
	return (*C.char)(unsafe.Pointer(&message[0]))
}

func evalError(errPtr *C.char) error {
	if errPtr == nil {
		return errors.New("vrl eval failed (unknown error)")
	}
	msg := C.GoString(errPtr)
	C.vrl_free_string(errPtr)
	return fmt.Errorf("vrl eval error: %s", msg)
}
