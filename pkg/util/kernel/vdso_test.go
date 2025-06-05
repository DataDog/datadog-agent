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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuxvVDSOMemoryAddress(t *testing.T) {
	for _, testcase := range []struct {
		source  string
		is32bit bool
		address uint64
	}{
		{"auxv64le.bin", false, 0x7ffd377e5000},
		{"auxv32le.bin", true, 0xb7fc3000},
	} {
		t.Run(testcase.source, func(t *testing.T) {
			av, err := newAuxFileReader("testdata/"+testcase.source, binary.LittleEndian, testcase.is32bit)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { av.Close() })

			addr, err := vdsoMemoryAddress(av)
			if err != nil {
				t.Fatal(err)
			}

			if uint64(addr) != testcase.address {
				t.Errorf("Expected vDSO memory address %x, got %x", testcase.address, addr)
			}
		})
	}
}

func TestAuxvNoVDSO(t *testing.T) {
	// Copy of auxv.bin with the vDSO pointer removed.
	av, err := newAuxFileReader("testdata/auxv64le_no_vdso.bin", binary.LittleEndian, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { av.Close() })

	_, err = vdsoMemoryAddress(av)
	if want, got := errAuxvNoVDSO, err; !errors.Is(got, want) {
		t.Fatalf("expected error '%v', got: %v", want, got)
	}
}

func TestVDSOVersion(t *testing.T) {
	_, err := vdsoVersion()
	require.NoError(t, err)
}

func TestLinuxVersionCodeEmbedded(t *testing.T) {
	tests := []struct {
		file    string
		version uint32
	}{
		{
			"testdata/vdso.bin",
			uint32(328828), // 5.4.124
		},
		{
			"testdata/vdso_multiple_notes.bin",
			uint32(328875), // Container Optimized OS v85 with a 5.4.x kernel
		},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			vdso, err := os.Open(test.file)
			if err != nil {
				t.Fatal(err)
			}
			defer vdso.Close()

			vc, err := vdsoLinuxVersionCode(vdso)
			if err != nil {
				t.Fatal(err)
			}

			if vc != test.version {
				t.Errorf("Expected version code %d, got %d", test.version, vc)
			}
		})
	}
}
