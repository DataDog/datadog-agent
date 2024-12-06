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

// Package safeelf provides safe (from panics) wrappers around ELF parsing
package safeelf

import (
	"debug/dwarf"
	"debug/elf" //nolint:depguard
	"fmt"
	"io"
)

// File is a safe wrapper around *elf.File that handles any panics in parsing
type File struct {
	*elf.File
}

// NewFile reads an ELF safely.
//
// Any panic during parsing is turned into an error. This is necessary since
// there are a bunch of unfixed bugs in debug/elf.
//
// https://github.com/golang/go/issues?q=is%3Aissue+is%3Aopen+debug%2Felf+in%3Atitle
func NewFile(r io.ReaderAt) (safe *File, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		safe = nil
		err = fmt.Errorf("reading ELF file panicked: %s", r)
	}()

	file, err := elf.NewFile(r)
	if err != nil {
		return nil, err
	}

	return &File{file}, nil
}

// Open reads an ELF from a file.
//
// It works like NewFile, with the exception that safe.Close will
// close the underlying file.
func Open(path string) (safe *File, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		safe = nil
		err = fmt.Errorf("reading ELF file panicked: %s", r)
	}()

	file, err := elf.Open(path)
	if err != nil {
		return nil, err
	}

	return &File{file}, nil
}

// Symbols is the safe version of elf.File.Symbols.
func (se *File) Symbols() (syms []Symbol, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		syms = nil
		err = fmt.Errorf("reading ELF symbols panicked: %s", r)
	}()

	syms, err = se.File.Symbols()
	return
}

// DynamicSymbols is the safe version of elf.File.DynamicSymbols.
func (se *File) DynamicSymbols() (syms []Symbol, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		syms = nil
		err = fmt.Errorf("reading ELF dynamic symbols panicked: %s", r)
	}()

	syms, err = se.File.DynamicSymbols()
	return
}

// DWARF is the safe version of elf.File.DWARF.
func (se *File) DWARF() (d *dwarf.Data, err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}

		d = nil
		err = fmt.Errorf("reading ELF DWARF panicked: %s", r)
	}()

	d, err = se.File.DWARF()
	return
}
