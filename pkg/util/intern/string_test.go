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

package intern

import (
	"fmt"
	"runtime"
	"testing"
)

func TestBasics(t *testing.T) {
	si := NewStringInterner()
	foo := si.GetString("foo")
	empty := si.GetString("")
	fooBytes := si.Get([]byte{'f', 'o', 'o'})
	foo2 := si.GetString("foo")
	empty2 := si.GetString("")
	foo2Bytes := si.Get([]byte{'f', 'o', 'o'})

	if foo.Get() != foo2.Get() {
		t.Error("foo/foo2 values differ")
	}
	if fooBytes.Get() != foo2Bytes.Get() {
		t.Error("foo/foo2 values differ")
	}
	if empty.Get() != empty2.Get() {
		t.Error("empty/empty2 values differ")
	}
	if foo.Get() != fooBytes.Get() {
		t.Error("foo/foobytes values differ")
	}

	if n := si.mapLen(); n != 2 {
		t.Errorf("map len = %d; want 2", n)
	}

	wantEmpty(t, si)
}

var (
	globalString = "not a constant"
	globalBytes  = []byte{'n', 'o', 't', 'c', 'o', 'n', 's', 't', 'a', 'n', 't'}
)

func TestGetAllocs(t *testing.T) {
	si := NewStringInterner()
	allocs := int(testing.AllocsPerRun(100, func() {
		si.Get(globalBytes)
	}))
	if allocs != 0 {
		t.Errorf("Get allocated %d objects, want 0", allocs)
	}
}

func TestGetStringAllocs(t *testing.T) {
	si := NewStringInterner()
	allocs := int(testing.AllocsPerRun(100, func() {
		si.GetString(globalString)
	}))
	if allocs != 0 {
		t.Errorf("GetString allocated %d objects, want 0", allocs)
	}
}

func (s *StringInterner) mapLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.valMap)
}

func (s *StringInterner) mapKeys() (keys []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.valMap {
		keys = append(keys, fmt.Sprint(k))
	}
	return keys
}

func wantEmpty(t testing.TB, s *StringInterner) {
	t.Helper()
	const gcTries = 5000
	for try := 0; try < gcTries; try++ {
		runtime.GC()
		n := s.mapLen()
		if n == 0 {
			break
		}
		if try == gcTries-1 {
			t.Errorf("map len = %d after (%d GC tries); want 0, contents: %v", n, gcTries, s.mapKeys())
		}
	}
}
