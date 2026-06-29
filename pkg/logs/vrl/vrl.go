// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build vrl

// Package vrl provides a CGo bridge to the VRL (Vector Remap Language) Rust library
// for evaluating boolean filter expressions against log messages.
//
// The static library libvrl_filter.a must be built from the Rust crate in this
// directory (cargo build --release) and placed here before building with the vrl tag.
package vrl

/*
#cgo LDFLAGS: -L${SRCDIR} -lvrl_filter -ldl -lpthread -lm
#include "include/vrl.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Program is a compiled VRL filter program.
type Program struct {
	ptr *C.vrl_program_t
}

// Compile compiles a VRL boolean expression.
// The returned Program must be closed with Close when no longer needed.
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
		return nil, fmt.Errorf("vrl compile failed (unknown error)")
	}
	return &Program{ptr: ptr}, nil
}

// Filter evaluates the program against a log message.
// Returns (true, nil) on match, (false, nil) on no match or abort,
// and (false, err) on a runtime error (caller should treat as no match).
func (p *Program) Filter(message []byte) (bool, error) {
	if p == nil || p.ptr == nil {
		return false, fmt.Errorf("vrl: nil program")
	}

	var errPtr *C.char
	ret := C.vrl_eval(
		p.ptr,
		(*C.char)(unsafe.Pointer(&message[0])),
		C.size_t(len(message)),
		&errPtr,
	)

	switch ret {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		var err error
		if errPtr != nil {
			err = fmt.Errorf("vrl eval error: %s", C.GoString(errPtr))
			C.vrl_free_string(errPtr)
		} else {
			err = fmt.Errorf("vrl eval failed (unknown error)")
		}
		return false, err
	}
}

// Close releases the compiled program.
func (p *Program) Close() {
	if p != nil && p.ptr != nil {
		C.vrl_free_program(p.ptr)
		p.ptr = nil
	}
}
