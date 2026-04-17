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
	"errors"
	"hash"
	"math"
	"unsafe"
)

var (
	// Force allows hashing files smaller than 4096 bytes.
	Force = false

	// ErrFileTooSmall is returned when Sum is called on < 4096 bytes of input
	// and Force is false.
	ErrFileTooSmall = errors.New("ssdeep: input too small for meaningful hash")
)

var _ hash.Hash = &State{}

// State holds the CGO ssdeep computation state.
type State struct {
	cs       C.struct_ssdeep_state
	written  uint64
	sumError error
}

// New returns a new ssdeep hash.Hash instance.
func New() *State {
	s := &State{}
	C.ssdeep_init(&s.cs)
	return s
}

// Write feeds data into the hash. The entire buffer is processed in a single
// CGO call — no per-byte crossing of the Go/C boundary.
//
// Buffers larger than math.MaxInt32 are split into safe-sized chunks to prevent
// integer overflow on the C.int boundary.
func (s *State) Write(p []byte) (int, error) {
	total := len(p)
	for len(p) > 0 {
		chunk := len(p)
		if chunk > math.MaxInt32 {
			chunk = math.MaxInt32
		}
		C.ssdeep_update(&s.cs, (*C.uchar)(unsafe.Pointer(&p[0])), C.int(chunk))
		p = p[chunk:]
	}
	s.written += uint64(total)
	return total, nil
}

// Sum appends the ssdeep digest to b. Returns b unchanged if the input was
// too small (< 4096 bytes) and Force is false.
func (s *State) Sum(b []byte) []byte {
	if !Force && s.written < 4096 {
		s.sumError = ErrFileTooSmall
		return b
	}
	var buf [C.SSDEEP_MAX_RESULT]C.char
	n := C.ssdeep_digest(&s.cs, &buf[0], C.int(len(buf)))
	if n <= 0 {
		return b
	}
	// Defensive clamp: n should never exceed buf size due to snprintf, but
	// guard against it to prevent C.GoStringN from reading past the buffer.
	if int(n) >= len(buf) {
		n = C.int(len(buf) - 1)
	}
	return append(b, C.GoStringN(&buf[0], n)...)
}

// Err returns any error from the last Sum call (e.g., ErrFileTooSmall).
func (s *State) Err() error {
	return s.sumError
}

// Reset reinitializes the hash state.
func (s *State) Reset() {
	C.ssdeep_init(&s.cs)
	s.written = 0
	s.sumError = nil
}

// BlockSize returns the minimum block size.
func (s *State) BlockSize() int {
	return C.BLOCK_MIN
}

// Size returns the hash output size.
func (s *State) Size() int {
	return C.SPAMSUM_LENGTH
}
