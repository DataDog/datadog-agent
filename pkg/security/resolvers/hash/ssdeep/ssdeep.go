// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

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

// ErrFileTooSmall is returned when the input is smaller than 4096 bytes and
// Force is false, matching glaslos/ssdeep behavior.
var ErrFileTooSmall = errors.New("ssdeep: file too small")

var (
	// Force calculates the hash on invalid input (files < 4096 bytes)
	Force = false
)

var _ hash.Hash = &State{}

// State holds the CGO ssdeep computation state.
type State struct {
	cs       C.struct_ssdeep_state
	written  int64
	sumError error
}

// New returns a new ssdeep hash.Hash instance.
func New() *State {
	s := &State{}
	C.ssdeep_init(&s.cs)
	return s
}

// Write feeds data into the hash. Large buffers are split into chunks to
// prevent integer overflow when casting len(p) to C.int.
func (s *State) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	total := 0
	for len(p) > 0 {
		chunk := len(p)
		if chunk > math.MaxInt32 {
			chunk = math.MaxInt32
		}
		rc := C.ssdeep_update(&s.cs, (*C.uchar)(unsafe.Pointer(&p[0])), C.int(chunk))
		if rc != 0 {
			s.sumError = errors.New("ssdeep: update failed")
			return total, s.sumError
		}
		s.written += int64(chunk)
		total += chunk
		p = p[chunk:]
	}
	return total, nil
}

// Sum appends the ssdeep digest to b. Returns b unchanged if the input was
// smaller than 4096 bytes and Force is false.
func (s *State) Sum(b []byte) []byte {
	if !Force && s.written < 4096 {
		s.sumError = ErrFileTooSmall
		return b
	}
	var buf [C.SSDEEP_MAX_RESULT]C.char
	n := int(C.ssdeep_digest(&s.cs, &buf[0], C.int(len(buf))))
	if n <= 0 {
		return b
	}
	if n >= int(C.SSDEEP_MAX_RESULT) {
		n = int(C.SSDEEP_MAX_RESULT) - 1
	}
	return append(b, C.GoStringN(&buf[0], C.int(n))...)
}

// Err returns any error from the last Sum call (e.g. ErrFileTooSmall).
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
	return int(C.BLOCK_MIN)
}

// Size returns the hash output size.
func (s *State) Size() int {
	return int(C.SPAMSUM_LENGTH)
}
