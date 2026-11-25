// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package elfstream plans and writes bounded-memory debug ELF streams.
package elfstream

import (
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
	// ELF64 record sizes.
	ehdrSize = 64 // Elf64_Ehdr
	phdrSize = 56 // Elf64_Phdr
	shdrSize = 64 // Elf64_Shdr

	// maxSectionNames bounds the section-name table read into memory. It matches
	// the profiler's own ELF reader (pfelf's maxBytesLargeSection).
	maxSectionNames = 16 << 20
)

// outputSection is one output section: its header and where its bytes come from.
type outputSection struct {
	header      elf.Section64 // Output header; Off is the output offset.
	name        string
	inputOffset uint64 // Start of the input range to copy.
}

// ELFStream holds the input's metadata, which later steps turn into a plan.
type ELFStream struct {
	input          io.ReaderAt
	byteOrder      binary.ByteOrder
	elfHeader      elf.Header64
	programHeaders []elf.Prog64
	sections       []outputSection
}
