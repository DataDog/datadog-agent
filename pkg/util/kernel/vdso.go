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
	"io"
	"math"
	"os"
	"sync"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// linuxVersionCode returns the version of the currently running kernel.
var linuxVersionCode = sync.OnceValues(detectKernelVersion)

// detectKernelVersion returns the version of the running kernel.
func detectKernelVersion() (uint32, error) {
	vc, err := vdsoVersion()
	if err != nil {
		return 0, err
	}
	return vc, nil
}

var (
	errAuxvNoVDSO = errors.New("no vdso address found in auxv")
)

// vdsoVersion returns the LINUX_VERSION_CODE embedded in the vDSO library
// linked into the current process image.
func vdsoVersion() (uint32, error) {
	av, err := newAuxvRuntimeReader()
	if err != nil {
		return 0, err
	}

	defer av.Close()

	vdsoAddr, err := vdsoMemoryAddress(av)
	if err != nil {
		return 0, fmt.Errorf("finding vDSO memory address: %w", err)
	}

	// Use /proc/self/mem rather than unsafe.Pointer tricks.
	mem, err := os.Open("/proc/self/mem")
	if err != nil {
		return 0, fmt.Errorf("opening mem: %w", err)
	}
	defer mem.Close()

	// Open ELF at provided memory address, as offset into /proc/self/mem.
	c, err := vdsoLinuxVersionCode(io.NewSectionReader(mem, int64(vdsoAddr), math.MaxInt64))
	if err != nil {
		return 0, fmt.Errorf("reading linux version code: %w", err)
	}

	return c, nil
}

// vdsoMemoryAddress returns the memory address of the vDSO library
// linked into the current process image. r is an io.Reader into an auxv blob.
func vdsoMemoryAddress(r auxvPairReader) (uintptr, error) {
	// Loop through all tag/value pairs in auxv until we find `AT_SYSINFO_EHDR`,
	// the address of a page containing the virtual Dynamic Shared Object (vDSO).
	for {
		tag, value, err := r.ReadAuxvPair()
		if err != nil {
			return 0, err
		}

		switch tag {
		case _AT_SYSINFO_EHDR:
			if value != 0 {
				return uintptr(value), nil
			}
			return 0, errors.New("invalid vDSO address in auxv")
		// _AT_NULL is always the last tag/val pair in the aux vector
		// and can be treated like EOF.
		case _AT_NULL:
			return 0, errAuxvNoVDSO
		}
	}
}

// format described at https://www.man7.org/linux/man-pages/man5/elf.5.html in section 'Notes (Nhdr)'
type elfNoteHeader struct {
	NameSize int32
	DescSize int32
	Type     int32
}

// vdsoLinuxVersionCode returns the LINUX_VERSION_CODE embedded in
// the ELF notes section of the binary provided by the reader.
func vdsoLinuxVersionCode(r io.ReaderAt) (uint32, error) {
	hdr, err := safeelf.NewFile(r)
	if err != nil {
		return 0, fmt.Errorf("reading vDSO ELF: %w", err)
	}

	sections := hdr.SectionsByType(safeelf.SHT_NOTE)
	if len(sections) == 0 {
		return 0, errors.New("no note section found in vDSO ELF")
	}

	for _, sec := range sections {
		sr := sec.Open()
		var n elfNoteHeader

		// Read notes until we find one named 'Linux'.
		for {
			if err := binary.Read(sr, hdr.ByteOrder, &n); err != nil {
				if errors.Is(err, io.EOF) {
					// We looked at all the notes in this section
					break
				}
				return 0, fmt.Errorf("reading note header: %w", err)
			}

			// If a note name is defined, it follows the note header.
			var name string
			if n.NameSize > 0 {
				// Read the note name, aligned to 4 bytes.
				buf := make([]byte, align(n.NameSize, 4))
				if err := binary.Read(sr, hdr.ByteOrder, &buf); err != nil {
					return 0, fmt.Errorf("reading note name: %w", err)
				}

				// Read nul-terminated string.
				name = unix.ByteSliceToString(buf[:n.NameSize])
			}

			// If a note descriptor is defined, it follows the name.
			// It is possible for a note to have a descriptor but not a name.
			if n.DescSize > 0 {
				// LINUX_VERSION_CODE is a uint32 value.
				if name == "Linux" && n.DescSize == 4 && n.Type == 0 {
					var version uint32
					if err := binary.Read(sr, hdr.ByteOrder, &version); err != nil {
						return 0, fmt.Errorf("reading note descriptor: %w", err)
					}
					return version, nil
				}

				// Discard the note descriptor if it exists, but we're not interested in it.
				if _, err := io.CopyN(io.Discard, sr, int64(align(n.DescSize, 4))); err != nil {
					return 0, err
				}
			}
		}
	}

	return 0, errors.New("no Linux note in ELF")
}

// align returns 'n' updated to 'alignment' boundary.
func align[I Integer](n, alignment I) I {
	return (n + alignment - 1) / alignment * alignment
}

// Integer represents all possible integer types.
// Remove when x/exp/constraints is moved to the standard library.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}
