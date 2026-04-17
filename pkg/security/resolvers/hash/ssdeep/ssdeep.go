// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ssdeep implements the ssdeep fuzzy hashing algorithm via CGO, using a
// precomputed lookup table for the FNV-based sum hash. The entire hot loop runs
// in C with a single CGO call per Write, avoiding Go bounds-check overhead and
// per-byte function call costs.
//
// Output is identical to glaslos/ssdeep v0.4.0 and the official ssdeep tool.
package ssdeep

/*
#include "ssdeep.h"
*/
import "C"

import (
	"hash"
	"unsafe"
)

var (
	// Force calculates the hash on invalid input (files < 4096 bytes)
	Force = false
)

var _ hash.Hash = &State{}

// State holds the CGO ssdeep computation state.
type State struct {
	cs C.struct_ssdeep_state
}

// New returns a new ssdeep hash.Hash instance.
func New() *State {
	s := &State{}
	C.ssdeep_init(&s.cs)
	return s
}

// Write feeds data into the hash. The entire buffer is processed in a single
// CGO call — no per-byte crossing of the Go/C boundary.
func (s *State) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	C.ssdeep_update(&s.cs, (*C.uchar)(unsafe.Pointer(&p[0])), C.int(len(p)))
	return len(p), nil
}

// Sum appends the ssdeep digest to b.
func (s *State) Sum(b []byte) []byte {
	var buf [C.SSDEEP_MAX_RESULT]C.char
	n := C.ssdeep_digest(&s.cs, &buf[0], C.int(len(buf)))
	if n <= 0 {
		return b
	}
	return append(b, C.GoStringN(&buf[0], n)...)
}

// Reset reinitializes the hash state.
func (s *State) Reset() {
	C.ssdeep_init(&s.cs)
}

// BlockSize returns the minimum block size.
func (s *State) BlockSize() int {
	return C.BLOCK_MIN
}

// Size returns the hash output size.
func (s *State) Size() int {
	return C.SPAMSUM_LENGTH
}
