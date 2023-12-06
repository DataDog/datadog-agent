// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.
//
// This package contains the `debug/buildinfo` package from Go 1.18,
// adapted to work in Go 1.17 and modified to only include ELF parsing.
//
// See https://github.com/golang/go/blob/master/src/debug/buildinfo/buildinfo.go
// for the original source.
//
// The original license is included in ./LICENSE
//
// Original file header:
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// Package buildinfo provides access to information embedded in a Go binary
// about how it was built. This includes the Go toolchain version, and the
// set of modules used (for binaries built in module mode).
//
// Build information is available for the currently running binary in
// runtime/debug.ReadBuildInfo.

// Package binversion provides access to information embedded in a Go binary about how it was built. This includes the
// Go toolchain version, and the set of modules used (for binaries built in module mode).
package binversion

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"errors"
	"io"
	"sync"
)

var (
	// errUnrecognizedFormat is returned when a given executable file doesn't
	// appear to be in a known format, or it breaks the rules of that format,
	// or when there are I/O errors reading the file.
	errUnrecognizedFormat = errors.New("unrecognized file format")

	// ErrNotGoExe is returned when a given executable file is valid but does
	// not contain Go build information.
	ErrNotGoExe = errors.New("not a Go executable")

	// The build info blob left by the linker is identified by
	// a 16-byte header, consisting of buildInfoMagic (14 bytes),
	// the binary's pointer size (1 byte),
	// and whether the binary is big endian (1 byte).
	buildInfoMagic = []byte("\xff Go buildinf:")
)

const (
	buildInfoAlign = 16
	buildInfoSize  = 32
	maxSizeToRead  = 64 * 1024 // 64KB
)

var (
	dataRawBuildPool = sync.Pool{
		New: func() any {
			b := make([]byte, maxSizeToRead)
			return &b
		},
	}
)

type exe interface {
	// ReadData reads and returns up to size bytes starting at virtual address addr.
	ReadData(addr, size uint64) ([]byte, error)

	// DataStart returns the virtual address of the segment or section that
	// should contain build information. This is either a specially named section
	// or the first writable non-zero data segment.
	DataStart() uint64
}

// ReadElfBuildInfo extracts the Go toolchain version and module information
// strings from a Go binary. On success, vers should be non-empty. mod
// is empty if the binary was not built with modules enabled.
func ReadElfBuildInfo(elfFile *elf.File) (vers string, err error) {
	x := &elfExe{f: elfFile}

	// Read the first 64kB of dataAddr to find the build info blob.
	// On some platforms, the blob will be in its own section, and DataStart
	// returns the address of that section. On others, it's somewhere in the
	// data segment; the linker puts it near the beginning.
	// See cmd/link/internal/ld.Link.buildinfo.
	dataAddr := x.DataStart()
	dataPtr := dataRawBuildPool.Get().(*[]byte)
	data := *dataPtr
	defer func() {
		data := *dataPtr

		// Zeroing the array. We cannot simply do data = data[:0], as this method changes the len to 0, which messes
		// with ReadAt.
		for i := range data {
			data[i] = 0
		}
		dataRawBuildPool.Put(dataPtr)
	}()

	if err := x.ReadDataWithPool(dataAddr, data); err != nil {
		return "", err
	}

	for {
		i := bytes.Index(data, buildInfoMagic)
		if i < 0 || len(data)-i < buildInfoSize {
			return "", ErrNotGoExe
		}
		if i%buildInfoAlign == 0 && len(data)-i >= buildInfoSize {
			data = data[i:]
			break
		}
		data = data[(i+buildInfoAlign-1)&^buildInfoAlign:]
	}

	// Decode the blob.
	// The first 14 bytes are buildInfoMagic.
	// The next two bytes indicate pointer size in bytes (4 or 8) and endianness
	// (0 for little, 1 for big).
	// Two virtual addresses to Go strings follow that: runtime.buildVersion,
	// and runtime.modinfo.
	// On 32-bit platforms, the last 8 bytes are unused.
	// If the endianness has the 2 bit set, then the pointers are zero
	// and the 32-byte header is followed by varint-prefixed string data
	// for the two string values we care about.
	ptrSize := int(data[14])
	if data[15]&2 != 0 {
		vers, _ = decodeString(data[32:])
	} else {
		bigEndian := data[15] != 0
		var bo binary.ByteOrder
		if bigEndian {
			bo = binary.BigEndian
		} else {
			bo = binary.LittleEndian
		}
		var readPtr func([]byte) uint64
		if ptrSize == 4 {
			readPtr = func(b []byte) uint64 { return uint64(bo.Uint32(b)) }
		} else {
			readPtr = bo.Uint64
		}
		vers = readString(x, ptrSize, readPtr, readPtr(data[16:]))
	}
	if vers == "" {
		return "", ErrNotGoExe
	}

	return vers, nil
}

func decodeString(data []byte) (s string, rest []byte) {
	u, n := binary.Uvarint(data)
	if n <= 0 || u >= uint64(len(data)-n) {
		return "", nil
	}
	return string(data[n : uint64(n)+u]), data[uint64(n)+u:]
}

// readString returns the string at address addr in the executable x.
func readString(x exe, ptrSize int, readPtr func([]byte) uint64, addr uint64) string {
	hdr, err := x.ReadData(addr, uint64(2*ptrSize))
	if err != nil || len(hdr) < 2*ptrSize {
		return ""
	}
	dataAddr := readPtr(hdr)
	dataLen := readPtr(hdr[ptrSize:])
	data, err := x.ReadData(dataAddr, dataLen)
	if err != nil || uint64(len(data)) < dataLen {
		return ""
	}
	return string(data)
}

// elfExe is the ELF implementation of the exe interface.
type elfExe struct {
	f *elf.File
}

func (x *elfExe) ReadData(addr, size uint64) ([]byte, error) {
	for _, prog := range x.f.Progs {
		if prog.Vaddr <= addr && addr <= prog.Vaddr+prog.Filesz-1 {
			n := prog.Vaddr + prog.Filesz - addr
			if n > size {
				n = size
			}
			data := make([]byte, n)
			_, err := prog.ReadAt(data, int64(addr-prog.Vaddr))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, errUnrecognizedFormat
}

// ReadDataWithPool is an implementation of ReadData, but without allocating arrays, we get a pooled array from the
// caller and spare allocations.
func (x *elfExe) ReadDataWithPool(addr uint64, data []byte) error {
	for _, prog := range x.f.Progs {
		if prog.Vaddr <= addr && addr <= prog.Vaddr+prog.Filesz-1 {
			expectedSizeToRead := prog.Vaddr + prog.Filesz - addr
			if expectedSizeToRead > uint64(len(data)) {
				expectedSizeToRead = uint64(len(data))
			}
			readSize, err := prog.ReadAt(data, int64(addr-prog.Vaddr))
			// If there is an error, and the error is not "EOF" caused due to the fact we tried to read too much,
			// then report an error.
			if err != nil && (err != io.EOF && uint64(readSize) != expectedSizeToRead) {
				return err
			}
			return nil
		}
	}
	return errUnrecognizedFormat
}

func (x *elfExe) DataStart() uint64 {
	for _, s := range x.f.Sections {
		if s.Name == ".go.buildinfo" {
			return s.Addr
		}
	}
	for _, p := range x.f.Progs {
		if p.Type == elf.PT_LOAD && p.Flags&(elf.PF_X|elf.PF_W) == elf.PF_W {
			return p.Vaddr
		}
	}
	return 0
}
