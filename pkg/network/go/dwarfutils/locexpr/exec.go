// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package locexpr provides a function to statically execute a DWARF location expression.
package locexpr

import (
	"bytes"
	"fmt"

	"github.com/go-delve/delve/pkg/dwarf/op"
)

// LocationPiece is the result of `Exec` (returned as a list),
// and describes whether the piece of the location is in a register (and if so, which one)
// or if it is on the stack (and if so, at what offset).
//
// Note that a LocationPiece does not, by itself, tell you the offset of the
// piece within the variable that it describes (in case the variable is
// described by multiple pieces). A LocationPiece is always part of an array of
// pieces which describe the whole variable; if some pieces of the variable are
// unavailable, then the whole variable is considered unavailable and there are
// no LocationPieces to speak of.
type LocationPiece struct {
	// Size of this piece in bytes
	Size int64
	// True if this piece is contained in a register.
	InReg bool
	// Offset from the stackpointer.
	// Only given if the piece resides on the stack.
	StackOffset int64
	// Register number of the piece.
	// Only given if the piece resides in registers.
	Register int
}

// Format gives a pretty printing of the location expression
// for debugging/error reporting purposes
func Format(expression []byte) string {
	buf := bytes.NewBufferString("")
	op.PrettyPrint(buf, expression, nil)
	return buf.String()
}

// Exec statically executes a DWARF location expression (see DWARF v4 spec
// sections 2.5, 2.6, and 7.7 for more info), returning a description of the
// location that is either in registers or on the stack. If the variable is not
// fully available (i.e. if all or even some of the location pieces are
// unavailable), then returns nil, nil.
//
// This implementation was originally based on
// github.com/go-delve/delve/pkg/proc.(*BinaryInfo).Location:
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/bininfo.go#L1062
// which is licensed under MIT.
func Exec(expression []byte, totalSize int64, pointerSize int) ([]LocationPiece, error) {
	if len(expression) == 0 {
		// The location expression is empty;
		// this means that the object doesn't have a value.
		// From section 2.6.1.1.4 Empty Location Descriptions:
		// > An empty location description consists of a DWARF expression
		// > containing no operations. It represents a piece or all of an object
		// > that is present in the source but not in the object code
		// > (perhaps due to optimization)
		return []LocationPiece{}, nil
	}

	// Execute the location expression to find the offset and pieces.
	// Note: there are two slight complexities to how this could work.
	// First, a location expression could depend on the value
	// of the canonical frame address (CFA) at the current program counter
	// (by using the expression opcode `DW_OP_call_frame_cfa`,
	// which would pull in the value from `op.DwarfRegisters.CFA`),
	// which is determined using the Call frame information section.
	// However, this is would add significant complexity to this implementation,
	// so we make the assumption that the CFA is constant
	// upon the entry to the function,
	// and that it is equal to/at a constant offset to the current stack pointer.
	// However, because we're executing these programs in a static context,
	// we don't have an actual value for the CFA,
	// so instead we inject an arbitrary large value
	// that lets us detect if it was indeed used to determine the offset,
	// and if so, subtract the original CFA value to get the stack pointer offset.
	// Second, a location expression could depend on the frame base of the function
	// (by using the expression op `DW_OP_fbreg [offset]`,
	// which would pull in the value from `op.DwarfRegisters.FrameBase` and add the offset),
	// which is stored in the `DW_AT_frame_base` attribute on the function DIE.
	// However, because this can also be a DWARF expression,
	// we again make some assumptions to simplify the implementation.
	// Just like with the CFA, we inject a large number
	// and subtract it from the final offset if it was used.
	var fakeCFA int64 = 0x080000
	var fakeFrameBase int64 = 0x100000
	reg := op.DwarfRegisters{CFA: fakeCFA, FrameBase: fakeFrameBase}
	offset, opPieces, err := op.ExecuteStackProgram(reg, expression, pointerSize, op.ReadMemoryFunc(nil))
	if err != nil {
		return nil, fmt.Errorf("an error occurred while executing the location expression; expression=(%s): %w", Format(expression), err)
	}

	// translateOffset adjusts the offset if it depended on the CFA or frame base:
	// 1. If the offset is greater than the midpoint between between the fake CFA and FrameBase,
	//    assume it was derived from the frame base
	//    (this should reliably work unless the offset is itself derived from the CFA
	//    plus some very large number, which is unlikely)
	// 2. Otherwise, if the offset is greater than the midpoint between the fake CFA and 0,
	//    assume it was derived from the CFA
	//    (again, this should work unless the offset is some very large constant offset from 0)
	translateOffset := func(offset int64) int64 {
		if offset > ((fakeCFA + fakeFrameBase) / 2) {
			return offset - fakeFrameBase
		} else if offset > (fakeCFA / 2) {
			return offset - fakeCFA
		}
		return offset
	}

	if len(opPieces) == 0 {
		offset = translateOffset(offset)

		// This pointer-size offset was adapted from Delve code.
		offset += int64(pointerSize)

		// Return one large piece on the stack
		return []LocationPiece{{
			Size:        totalSize,
			InReg:       false,
			StackOffset: offset,
		}}, nil
	}

	// If there's a single piece, Delve returns it with size 0 when it really
	// means that it covers the whole variable.
	if len(opPieces) == 1 && opPieces[0].Size == 0 {
		opPieces[0].Size = int(totalSize)
	}

	// Convert the list of pieces returned by the op library
	// into the desired type.
	pieces := []LocationPiece{}
	for _, opPiece := range opPieces {
		switch opPiece.Kind {
		case op.RegPiece:
			pieces = append(pieces, LocationPiece{
				Size:     int64(opPiece.Size),
				InReg:    true,
				Register: int(opPiece.Val),
			})
		case op.AddrPiece:
			stackOffset := int64(opPiece.Val)
			stackOffset = translateOffset(stackOffset)

			// This pointer-size offset was adapted from Delve code.
			stackOffset += int64(pointerSize)

			pieces = append(pieces, LocationPiece{
				Size:        int64(opPiece.Size),
				InReg:       false,
				StackOffset: stackOffset,
			})
		case op.ImmPiece:
			// The Delve library reports unavailable pieces as 0-valued. For
			// simplicity, we'll consider all immediate pieces to be
			// unavailable.
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected piece kind %d in location expression", opPiece.Kind)
		}
	}

	// The Go compiler has bugs that result in invalid location lists;
	// empirically, such lists sometimes contain duplicate pieces; removing the
	// duplicates is sometimes enough to recover a valid list.
	dedupedPieces := []LocationPiece{}
	seenPices := make(map[LocationPiece]struct{})
	for _, piece := range pieces {
		if _, ok := seenPices[piece]; !ok {
			dedupedPieces = append(dedupedPieces, piece)
			seenPices[piece] = struct{}{}
		}
	}

	piecesSize := int64(0)
	for _, piece := range dedupedPieces {
		piecesSize += piece.Size
	}
	// If we ended up with more pieces that the variable's size, then the
	// location list is invalid (Go compiler bugs, which exist). We'll declare
	// the whole variable as unavailable.
	if piecesSize > totalSize {
		return nil, nil
	}
	// If some pieces are unavailable, we declare the whole variable as
	// unavailable.
	if piecesSize < totalSize {
		return nil, nil
	}

	return dedupedPieces, nil
}
