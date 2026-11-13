// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package elfstream plans and writes bounded-memory debug ELF streams.
package elfstream

import (
	"context"
	"debug/elf" //nolint:depguard // ELF encoding structs are not exposed by safeelf.
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// ErrUnsupported marks a well-formed ELF layout that this writer does not handle,
// as opposed to a malformed input.
var ErrUnsupported = errors.New("unsupported ELF")

func unsupported(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrUnsupported}, args...)...)
}

const (
	streamBufferSize = 64 * 1024

	// ELF64 record sizes.
	ehdrSize = 64 // Elf64_Ehdr
	phdrSize = 56 // Elf64_Phdr
	shdrSize = 64 // Elf64_Shdr
	symSize  = 24 // Elf64_Sym
	chdrSize = 24 // Elf64_Chdr

	// maxAlignment bounds the zero padding a retained section can add.
	maxAlignment = 1 << 20
	// maxSectionNames bounds the section-name table read into memory. It matches
	// the profiler's own ELF reader (pfelf's maxBytesLargeSection).
	maxSectionNames = 16 << 20
)

// StreamSection supplies an already-extracted section, without copying its data.
type StreamSection struct {
	Name    string
	Address uint64
	Data    []byte
}

// StreamSymbol supplies an absolute symbol value and its owning section.
type StreamSymbol struct {
	Name    string
	Section string
	Value   uint64
}

// StreamOptions selects extra content. Debug sections, notes and static symbol
// tables are always retained; GNU version payloads are always omitted.
type StreamOptions struct {
	KeepSections []string        // Extra input sections to retain, by name.
	Sections     []StreamSection // Sections to add or replace.
	Symbols      []StreamSymbol  // Symbols to add, or to patch when they exist.
}

type symbolPatch struct {
	offset uint64 // Byte offset within the input symbol table.
	value  elf.Sym64
}

// outputSection describes bytes to emit, not a copy of them: input bytes from
// source (with patches), then data, then tail.
type outputSection struct {
	header     elf.Section64 // Output header; Off is the output offset.
	name       string
	source     uint64 // Input offset.
	sourceSize uint64
	data       []byte // Supplied section bytes, or the generated name table.
	tail       []byte // Appended symbol names or entries.
	patches    []symbolPatch
}

func (s *outputSection) typ() elf.SectionType { return elf.SectionType(s.header.Type) }

// ELFStream writes a debug ELF in one forward pass. Keep its input, and any
// supplied StreamSection.Data, unchanged until writing is done. Code and data
// bytes are omitted unless selected; retained section bytes, including existing
// compressed sections, are copied unchanged.
type ELFStream struct {
	input    io.ReaderAt
	names    []byte // Input section-name table; output appends to it.
	order    binary.ByteOrder
	header   elf.Header64
	programs []elf.Prog64
	sections []outputSection
	size     uint64
}

// NewELFStream validates the input and plans every output offset. It reads
// headers and tables, not section payloads.
func NewELFStream(ctx context.Context, input io.ReaderAt, size int64, opts StreamOptions) (*ELFStream, error) {
	s, err := readELF64(ctx, input, size)
	if err != nil {
		return nil, err
	}
	if err := s.selectSections(opts.KeepSections); err != nil {
		return nil, err
	}
	if err := s.applySections(opts.Sections); err != nil {
		return nil, err
	}
	if err := s.checkRetainedSections(size); err != nil {
		return nil, err
	}
	if err := s.addSymbols(ctx, opts.Symbols); err != nil {
		return nil, err
	}
	if err := s.planProgramHeaders(); err != nil {
		return nil, err
	}
	if err := s.planLayout(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ELFStream) getSectionIndex(name string) int {
	for i := range s.sections {
		if s.sections[i].name == name {
			return i
		}
	}
	return -1
}

// Size is the exact number of ELF bytes Write will emit.
func (s *ELFStream) Size() int64 { return int64(s.size) }

// Write emits the planned ELF with a fixed-size buffer. The plan can be written
// again, including concurrently if the input supports concurrent ReadAt.
func (s *ELFStream) Write(ctx context.Context, output io.Writer) error {
	w := &streamWriter{ctx: ctx, output: output}
	if err := binary.Write(w, s.order, s.header); err != nil {
		return err
	}
	if err := binary.Write(w, s.order, s.programs); err != nil {
		return err
	}
	for _, sec := range s.sections {
		if err := binary.Write(w, s.order, sec.header); err != nil {
			return err
		}
	}
	buffer := make([]byte, streamBufferSize)
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if sec.typ() == elf.SHT_NOBITS {
			continue
		}
		clear(buffer) // Pad with zeros, not the previous section's bytes.
		for w.position < sec.header.Off {
			if _, err := w.Write(buffer[:min(uint64(len(buffer)), sec.header.Off-w.position)]); err != nil {
				return err
			}
		}
		copyRange := func(offset, length uint64) error {
			n, err := io.CopyBuffer(w, io.NewSectionReader(s.input, int64(sec.source+offset), int64(length)), buffer)
			// CopyBuffer treats EOF as success; a truncated source range must fail.
			if err == nil && uint64(n) != length {
				err = io.ErrUnexpectedEOF
			}
			if err == nil && w.ctx.Err() != nil {
				return w.ctx.Err()
			}
			return err
		}
		var start uint64
		for _, patch := range sec.patches {
			if err := copyRange(start, patch.offset-start); err != nil {
				return err
			}
			if err := binary.Write(w, s.order, patch.value); err != nil {
				return err
			}
			start = patch.offset + symSize
		}
		if err := copyRange(start, sec.sourceSize-start); err != nil {
			return fmt.Errorf("copy section %q: %w", sec.name, err)
		}
		if _, err := w.Write(sec.data); err != nil {
			return err
		}
		if _, err := w.Write(sec.tail); err != nil {
			return err
		}
	}
	if w.position != s.size {
		return fmt.Errorf("incomplete ELF output: wrote %d of %d bytes", w.position, s.size)
	}
	return ctx.Err()
}

// streamWriter tracks the output position and checks cancellation between chunks.
type streamWriter struct {
	ctx      context.Context
	output   io.Writer
	position uint64
}

func (w *streamWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) != 0 {
		if err := w.ctx.Err(); err != nil {
			return written, err
		}
		chunk := p[:min(len(p), streamBufferSize)]
		n, err := w.output.Write(chunk)
		written += n
		w.position += uint64(n)
		if err != nil {
			return written, err
		}
		if n != len(chunk) {
			return written, io.ErrShortWrite
		}
		p = p[n:]
	}
	return written, nil
}
