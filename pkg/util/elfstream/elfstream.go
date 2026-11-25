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

	// ELF64 record sizes. debug/elf only exports the symbol's.
	ehdrSize = 64            // Elf64_Ehdr
	phdrSize = 56            // Elf64_Phdr
	shdrSize = 64            // Elf64_Shdr
	symSize  = elf.Sym64Size // Elf64_Sym
	chdrSize = 24            // Elf64_Chdr

	// shtRela is elf.SHT_RELA under a name the misspell linter accepts, so the
	// nolint stays in one place instead of on every use.
	shtRela = elf.SHT_RELA //nolint:misspell

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

// symbolOverride replaces one entry of the input symbol table while it is copied.
type symbolOverride struct {
	offset uint64 // Byte offset of the entry within the input symbol table.
	entry  elf.Sym64
}

// outputSection describes the bytes to emit for one section, not a copy of them:
// the input range with its symbol overrides, then its own bytes, then the
// appended bytes.
type outputSection struct {
	header          elf.Section64 // Output header; Off is the output offset.
	name            string
	inputOffset     uint64           // Start of the input range to copy.
	inputSize       uint64           // Length of the input range; 0 copies nothing.
	ownBytes        []byte           // Supplied section bytes, or the rebuilt name table.
	appended        []byte           // Symbol entries or names added after the input range.
	symbolOverrides []symbolOverride // Entries written instead of the input's, in offset order.
}

func (sec *outputSection) typ() elf.SectionType { return elf.SectionType(sec.header.Type) }

// ELFStream writes a debug ELF in one forward pass. Keep its input, and any
// supplied StreamSection.Data, unchanged until writing is done. Code and data
// bytes are omitted unless selected; retained section bytes, including existing
// compressed sections, are copied unchanged.
type ELFStream struct {
	input             io.ReaderAt
	inputSectionNames []byte // Input section-name table; the output appends to a copy.
	byteOrder         binary.ByteOrder
	elfHeader         elf.Header64
	programHeaders    []elf.Prog64
	sections          []outputSection
	outputSize        uint64
}

// NewELFStream validates the input and plans every output offset. It reads
// headers and tables, not section payloads.
func NewELFStream(ctx context.Context, input io.ReaderAt, inputSize int64, opts StreamOptions) (*ELFStream, error) {
	s, err := readELF64(ctx, input, inputSize)
	if err != nil {
		return nil, err
	}
	if err := s.selectSections(opts.KeepSections); err != nil {
		return nil, err
	}
	if err := s.applySections(opts.Sections); err != nil {
		return nil, err
	}
	if err := s.checkRetainedSections(inputSize); err != nil {
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
	// Some steps read nothing more and don't check ctx: don't return a plan
	// once its deadline passed.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ELFStream) sectionIndex(name string) int {
	for i := range s.sections {
		if s.sections[i].name == name {
			return i
		}
	}
	return -1
}

// Size is the exact number of ELF bytes Write will emit.
func (s *ELFStream) Size() int64 { return int64(s.outputSize) }

// Write emits the planned ELF with a fixed-size buffer. The plan can be written
// again, including concurrently if the input supports concurrent ReadAt.
func (s *ELFStream) Write(ctx context.Context, output io.Writer) error {
	out := &streamWriter{ctx: ctx, output: output}
	if err := binary.Write(out, s.byteOrder, s.elfHeader); err != nil {
		return err
	}
	if err := binary.Write(out, s.byteOrder, s.programHeaders); err != nil {
		return err
	}
	for _, sec := range s.sections {
		if err := binary.Write(out, s.byteOrder, sec.header); err != nil {
			return err
		}
	}
	copyBuffer := make([]byte, streamBufferSize)
	for i := 1; i < len(s.sections); i++ {
		sec := &s.sections[i]
		if sec.typ() == elf.SHT_NOBITS {
			continue
		}
		clear(copyBuffer) // Pad with zeros, not the previous section's bytes.
		for out.position < sec.header.Off {
			if _, err := out.Write(copyBuffer[:min(uint64(len(copyBuffer)), sec.header.Off-out.position)]); err != nil {
				return err
			}
		}
		copyInput := func(offset, length uint64) error {
			n, err := io.CopyBuffer(out, io.NewSectionReader(s.input, int64(sec.inputOffset+offset), int64(length)), copyBuffer)
			// CopyBuffer treats EOF as success; a truncated input range must fail.
			if err == nil && uint64(n) != length {
				err = io.ErrUnexpectedEOF
			}
			if err == nil && out.ctx.Err() != nil {
				return out.ctx.Err()
			}
			return err
		}
		var copied uint64
		for _, override := range sec.symbolOverrides {
			if err := copyInput(copied, override.offset-copied); err != nil {
				return err
			}
			if err := binary.Write(out, s.byteOrder, override.entry); err != nil {
				return err
			}
			copied = override.offset + symSize
		}
		if err := copyInput(copied, sec.inputSize-copied); err != nil {
			return fmt.Errorf("copy section %q: %w", sec.name, err)
		}
		if _, err := out.Write(sec.ownBytes); err != nil {
			return err
		}
		if _, err := out.Write(sec.appended); err != nil {
			return err
		}
	}
	if out.position != s.outputSize {
		return fmt.Errorf("incomplete ELF output: wrote %d of %d bytes", out.position, s.outputSize)
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
