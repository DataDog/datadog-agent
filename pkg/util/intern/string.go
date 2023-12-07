// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Copyright (c) 2020, Brad Fitzpatrick
// All rights reserved.
// Redistribution and use in source and binary forms, with or without
// modification, are permitted provided that the following conditions are met:
//
// 1. Redistributions of source code must retain the above copyright notice, this
// list of conditions and the following disclaimer.
//
// 2. Redistributions in binary form must reproduce the above copyright notice,
// this list of conditions and the following disclaimer in the documentation
// and/or other materials provided with the distribution.
//
// 3. Neither the name of the copyright holder nor the names of its
// contributors may be used to endorse or promote products derived from
// this software without specific prior written permission.
//
// THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
// AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
// IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
// DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
// FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
// DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
// SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
// CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
// OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
// OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

// code adapted from go4.org/intern

// Package intern lets you make smaller comparable values by boxing
// a larger comparable value (such as a 16 byte string header) down
// into a globally unique 8 byte pointer.
//
// The globally unique pointers are garbage collected with weak
// references and finalizers. This package hides that.
package intern

import (
	"runtime"
	"sync"
	"unsafe"
)

// A StringValue pointer is the handle to the underlying string value.
// See Get how Value pointers may be used.
type StringValue struct {
	_           [0]func() // prevent people from accidentally using value type as comparable
	cmpVal      string
	resurrected bool
}

// Get the underlying string value
func (v *StringValue) Get() string {
	return v.cmpVal
}

// StringInterner interns strings while allowing them to be cleaned up by the GC.
// It can handle both string and []byte types without allocation.
type StringInterner struct {
	mu     sync.Mutex
	valMap map[string]uintptr
}

// NewStringInterner creates a new StringInterner
func NewStringInterner() *StringInterner {
	return &StringInterner{
		valMap: make(map[string]uintptr),
	}
}

// GetString returns a pointer representing the string k
//
// The returned pointer will be the same for GetString(v) and GetString(v2)
// if and only if v == v2. The returned pointer will also be the same
// for a byte slice with same contents as the string.
//
//go:nocheckptr
func (s *StringInterner) GetString(k string) *StringValue {
	s.mu.Lock()
	defer s.mu.Unlock()

	var v *StringValue
	if addr, ok := s.valMap[k]; ok {
		//goland:noinspection GoVetUnsafePointer
		v = (*StringValue)((unsafe.Pointer)(addr))
		v.resurrected = true
		return v
	}

	v = &StringValue{cmpVal: k}
	runtime.SetFinalizer(v, s.finalize)
	s.valMap[k] = uintptr(unsafe.Pointer(v))
	return v
}

// Get returns a pointer representing the []byte k
//
// The returned pointer will be the same for Get(v) and Get(v2)
// if and only if v == v2. The returned pointer will also be the same
// for a string with same contents as the byte slice.
//
//go:nocheckptr
func (s *StringInterner) Get(k []byte) *StringValue {
	s.mu.Lock()
	defer s.mu.Unlock()

	var v *StringValue
	// the compiler will optimize the following map lookup to not alloc a string
	if addr, ok := s.valMap[string(k)]; ok {
		//goland:noinspection GoVetUnsafePointer
		v = (*StringValue)((unsafe.Pointer)(addr))
		v.resurrected = true
		return v
	}

	v = &StringValue{cmpVal: string(k)}
	runtime.SetFinalizer(v, s.finalize)
	s.valMap[string(k)] = uintptr(unsafe.Pointer(v))
	return v
}

func (s *StringInterner) finalize(v *StringValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.resurrected {
		// We lost the race. Somebody resurrected it while we
		// were about to finalize it. Try again next round.
		v.resurrected = false
		runtime.SetFinalizer(v, s.finalize)
		return
	}
	delete(s.valMap, v.Get())
}
