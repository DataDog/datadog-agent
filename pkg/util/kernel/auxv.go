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
	"fmt"
	"io"

	"golang.org/x/sys/unix"
)

type auxvPairReader interface {
	Close() error
	ReadAuxvPair() (uint64, uint64, error)
}

// See https://elixir.bootlin.com/linux/v6.5.5/source/include/uapi/linux/auxvec.h
//
//revive:disable:var-naming keep kernel naming, but do not export them
const (
	_AT_NULL         = 0  // End of vector
	_AT_SYSINFO_EHDR = 33 // Offset to vDSO blob in process image
)

//revive:enable:var-naming

type auxvRuntimeReader struct {
	data  [][2]uintptr
	index int
}

func (r *auxvRuntimeReader) Close() error {
	return nil
}

func (r *auxvRuntimeReader) ReadAuxvPair() (uint64, uint64, error) {
	if r.index >= len(r.data)+2 {
		return 0, 0, io.EOF
	}

	// we manually add the (_AT_NULL, _AT_NULL) pair at the end
	// that is not provided by the go runtime
	var tag, value uintptr
	if r.index < len(r.data) {
		tag, value = r.data[r.index][0], r.data[r.index][1]
	} else {
		tag, value = _AT_NULL, _AT_NULL
	}
	r.index++
	return uint64(tag), uint64(value), nil
}

func newAuxvRuntimeReader() (auxvPairReader, error) {
	data, err := unix.Auxv()
	if err != nil {
		return nil, fmt.Errorf("read auxv from runtime: %w", err)
	}

	return &auxvRuntimeReader{
		data:  data,
		index: 0,
	}, nil
}
