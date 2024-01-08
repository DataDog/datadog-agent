// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package asmscan provides functions for scanning the machine code of functions.
package asmscan

import (
	"debug/elf"
	"fmt"

	"golang.org/x/arch/arm64/arm64asm"
	"golang.org/x/arch/x86/x86asm"
)

// ScanFunction finds the program counter (PC) positions of machine code instructions
// within a specific range of the text section of a binary,
// using the provided callback to disassemble and scan the buffer of machine code.
// The callback should return a slice of indices into the buffer
// that point to positions within the larger binary.
// These positions will then be adjusted based on the offset of the text section
// to provide the same positions as PC positions,
// which will be returned from the outer function.
//
// lowPC, highPC forms an interval that contains all machine code bytes
// up to but not including high PC.
// In practice, this works well for scanning the instructions of functions,
// since highPC (in both DWARF parlance and in the Go symbol table),
// seems to refer to the address of the first location *past* the last instruction of the function.
//
// This function was intended to be used to find return instructions for functions in Go binaries,
// for the purpose of then attaching eBPF uprobes to these locations.
// This is needed because uretprobes don't work well with Go.
// See the following links for more info:
//   - https://github.com/iovisor/bcc/issues/1320
//   - https://github.com/iovisor/bcc/issues/1320#issuecomment-407927542
//     (which describes how this approach works as a workaround)
//   - https://github.com/golang/go/issues/22008
func ScanFunction(textSection *elf.Section, sym elf.Symbol, functionOffset uint64, scanInstructions func(data []byte) ([]uint64, error)) ([]uint64, error) {
	// Determine the offset in the section that the function starts at
	lowPC := sym.Value
	highPC := lowPC + sym.Size
	offset := lowPC - textSection.Addr
	buf := make([]byte, int(highPC-lowPC))

	readBytes, err := textSection.ReadAt(buf, int64(offset))
	if err != nil {
		return nil, fmt.Errorf("could not read text section: %w", err)
	}
	data := buf[:readBytes]

	// instructionIndices contains indices into `buf`.
	instructionIndices, err := scanInstructions(data)
	if err != nil {
		return nil, fmt.Errorf("error while scanning instructions in text section: %w", err)
	}

	// Add the function lowPC to each index to obtain the actual locations
	adjustedLocations := make([]uint64, len(instructionIndices))
	for i, instructionIndex := range instructionIndices {
		adjustedLocations[i] = instructionIndex + functionOffset
	}

	return adjustedLocations, nil
}

// FindX86_64ReturnInstructions is a callback for ScanFunction
// that scans for all x86_64 return instructions (RET)
// contained in the given buffer of machine code.
// On success, this function returns the index into the buffer
// of the start of each return instruction.
//
// Note that this may not behave well with panics or defer statements.
// See the following links for more context:
// - https://github.com/go-delve/delve/pull/2704/files#diff-fb7b7a020e32bf8bf477c052ac2d2857e7e587478be6039aebc7135c658417b2R769
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L86-L95
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L374
func FindX86_64ReturnInstructions(data []byte) ([]uint64, error) {
	// x86_64 => mode is 64
	mode := 64
	returnOffsets := []uint64{}
	cursor := 0
	for cursor < len(data) {
		instruction, err := x86asm.Decode(data[cursor:], mode)
		if err != nil {
			return nil, fmt.Errorf("failed to decode x86-64 instruction at offset %d within function machine code: %w", cursor, err)
		}

		if instruction.Op == x86asm.RET {
			returnOffsets = append(returnOffsets, uint64(cursor))
		}

		cursor += instruction.Len
	}

	return returnOffsets, nil
}

// FindARM64ReturnInstructions is a callback for ScanFunction
// that scans for all ARM 64-bit return instructions (RET, not RETAA/RETAB)
// contained in the given buffer of machine code.
// On success, this function returns the index into the buffer
// of the start of each return instruction.
//
// Note that this may not behave well with panics or defer statements.
// See the following links for more context:
// - https://github.com/go-delve/delve/pull/2704/files#diff-fb7b7a020e32bf8bf477c052ac2d2857e7e587478be6039aebc7135c658417b2R769
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L86-L95
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L374
func FindARM64ReturnInstructions(data []byte) ([]uint64, error) {
	// It seems like we only need to look for RET instructions
	// (and not RETAA/RETAB, since gc doesn't support pointer authentication:
	// https://github.com/golang/go/issues/39208)
	returnOffsets := []uint64{}
	cursor := 0
	for cursor < len(data) {
		instruction, err := arm64asm.Decode(data[cursor:])
		if err != nil {
			// ARM64 instructions are 4 bytes long so we just advance
			// the cursor accordingly in case we run into a decoding error
			cursor += 4
			continue
		}

		if instruction.Op == arm64asm.RET {
			returnOffsets = append(returnOffsets, uint64(cursor))
		}

		// Each instruction is 4 bytes long
		cursor += 4
	}

	return returnOffsets, nil
}
