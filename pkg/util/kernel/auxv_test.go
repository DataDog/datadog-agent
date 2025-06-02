// This file is licensed under the MIT License.
//
// Copyright (c) 2017 Nathan Sweet
// Copyright (c) 2018, 2019 Cloudflare
// Copyright (c) 2019 Authors of Cilium
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

//go:build linux

package kernel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

type auxvFileReader struct {
	file            *os.File
	order           binary.ByteOrder
	uintptrIs32bits bool
}

func (r *auxvFileReader) Close() error {
	return r.file.Close()
}

type auxvPair32 struct {
	Tag, Value uint32
}

type auxvPair64 struct {
	Tag, Value uint64
}

func (r *auxvFileReader) ReadAuxvPair() (tag, value uint64, _ error) {
	if r.uintptrIs32bits {
		var aux auxvPair32
		if err := binary.Read(r.file, r.order, &aux); err != nil {
			return 0, 0, fmt.Errorf("reading auxv entry: %w", err)
		}
		return uint64(aux.Tag), uint64(aux.Value), nil
	}

	var aux auxvPair64
	if err := binary.Read(r.file, r.order, &aux); err != nil {
		return 0, 0, fmt.Errorf("reading auxv entry: %w", err)
	}
	return aux.Tag, aux.Value, nil
}

func newAuxFileReader(path string, order binary.ByteOrder, uintptrIs32bits bool) (auxvPairReader, error) {
	// Read data from the auxiliary vector, which is normally passed directly
	// to the process. Go does not expose that data before go 1.21, so we must read it from procfs.
	// https://man7.org/linux/man-pages/man3/getauxval.3.html
	av, err := os.Open(path)
	if errors.Is(err, unix.EACCES) {
		return nil, fmt.Errorf("opening auxv: %w (process may not be dumpable due to file capabilities)", err)
	}
	if err != nil {
		return nil, fmt.Errorf("opening auxv: %w", err)
	}

	return &auxvFileReader{
		file:            av,
		order:           order,
		uintptrIs32bits: uintptrIs32bits,
	}, nil
}

func newDefaultAuxvFileReader() (auxvPairReader, error) {
	const uintptrIs32bits = unsafe.Sizeof((uintptr)(0)) == 4
	return newAuxFileReader("/proc/self/auxv", binary.NativeEndian, uintptrIs32bits)
}

func TestAuxvBothSourcesEqual(t *testing.T) {
	runtimeBased, err := newAuxvRuntimeReader()
	if err != nil {
		t.Fatal(err)
	}
	fileBased, err := newDefaultAuxvFileReader()
	if err != nil {
		t.Fatal(err)
	}

	for {
		runtimeTag, runtimeValue, err := runtimeBased.ReadAuxvPair()
		if err != nil {
			t.Fatal(err)
		}

		fileTag, fileValue, err := fileBased.ReadAuxvPair()
		if err != nil {
			t.Fatal(err)
		}

		if runtimeTag != fileTag {
			t.Errorf("mismatching tags: runtime=%v, file=%v", runtimeTag, fileTag)
		}

		if runtimeValue != fileValue {
			t.Errorf("mismatching values: runtime=%v, file=%v", runtimeValue, fileValue)
		}

		if runtimeTag == _AT_NULL {
			break
		}
	}
}
