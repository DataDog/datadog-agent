// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package loclist supports parsing DWARF loclist instructions.
package loclist

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// DWARF opcodes
// https://dwarfstd.org/doc/DWARF5.pdf
// All range constants are inclusive on both ends.
//
//revive:disable:var-naming
const (
	dw_op_addr  = 0x03
	dw_op_deref = 0x06

	// Range for DW_OP_const* ops.
	dw_const_op_lo = 0x08
	dw_const_op_hi = 0x11

	// Range for various stack ops
	dw_stack_op_lo = 0x12
	dw_stack_op_hi = 0x2f

	// Range for DW_OP_lit0 through DW_OP_lit31
	dw_op_lit0  = 0x30
	dw_op_lit31 = 0x4f

	// Range for DW_OP_reg0 through DW_OP_reg31
	dw_op_reg0  = 0x50
	dw_op_reg31 = 0x6f

	// Range for DW_OP_breg0 through DW_OP_breg31
	dw_op_breg0  = 0x70
	dw_op_breg31 = 0x8f

	dw_op_regx  = 0x90
	dw_op_fbreg = 0x91
	dw_op_bregx = 0x92

	dw_op_piece          = 0x93
	dw_op_call_frame_cfa = 0x9c
	// Range for evaluation ops, including preceding piece and call_frame_cfa.
	dw_eval_op_lo = 0x93
	dw_eval_op_hi = 0xa9

	dw_user_op_lo = 0xe0
	dw_user_op_hi = 0xff
)

//revive:enable:var-naming

// ParseInstructions parses DWARF loclist instruction bytes.
func ParseInstructions(data []byte, ptrSize uint8, totalByteSize uint32) ([]ir.Piece, error) {
	reader := bytes.NewBuffer(data)
	op := ir.PieceOp(nil)
	pieces := []ir.Piece{}
	instCnt := 0

	for reader.Len() > 0 {
		instCnt++
		opcode, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if opcode == dw_op_piece {
			size, err := readULEB128(reader)
			if err != nil {
				return nil, err
			}
			if size > math.MaxUint32 {
				return nil, fmt.Errorf("piece size too large: %d", size)
			}
			pieces = append(pieces, ir.Piece{Size: uint32(size), Op: op})
			op = nil
			continue
		}

		if op != nil {
			return nil, fmt.Errorf("unconsumed op: %v", op)
		}

		switch {
		case opcode == dw_op_addr:
			offset, err := readFixedSize(reader, ptrSize)
			if err != nil {
				return nil, err
			}
			op = ir.Addr{Addr: offset}

		case opcode == dw_op_deref:
			return nil, fmt.Errorf("unsupported DW_OP_deref")

		case dw_const_op_lo <= opcode && opcode <= dw_const_op_hi:
			return nil, fmt.Errorf("unsupported DW_OP_const* opcode: 0x%x", opcode)

		case dw_stack_op_lo <= opcode && opcode <= dw_stack_op_hi:
			return nil, fmt.Errorf("unsupported stack-manipulation opcode: 0x%x", opcode)

		case dw_op_lit0 <= opcode && opcode <= dw_op_lit31:
			return nil, fmt.Errorf("unsupported DW_OP_lit* opcode: 0x%x", opcode)

		case dw_op_reg0 <= opcode && opcode <= dw_op_reg31:
			op = ir.Register{RegNo: opcode - dw_op_reg0, Shift: 0}

		case dw_op_breg0 <= opcode && opcode <= dw_op_breg31:
			shift, err := readSLEB128(reader)
			if err != nil {
				return nil, err
			}
			op = ir.Register{RegNo: opcode - dw_op_breg0, Shift: shift}

		case opcode == dw_op_regx:
			idx, err := readULEB128(reader)
			if err != nil {
				return nil, err
			}
			if idx > math.MaxUint8 {
				return nil, fmt.Errorf("DW_OP_regx index too large: %d", idx)
			}
			op = ir.Register{RegNo: uint8(idx), Shift: 0}

		case opcode == dw_op_fbreg:
			offset, err := readSLEB128(reader)
			if err != nil {
				return nil, err
			}
			if offset > math.MaxInt32 {
				return nil, fmt.Errorf("DW_OP_fbreg offset too large: %d", offset)
			}
			op = ir.Cfa{CfaOffset: int32(offset)}

		case opcode == dw_op_bregx:
			idx, err := readULEB128(reader)
			if err != nil {
				return nil, err
			}
			shift, err := readSLEB128(reader)
			if err != nil {
				return nil, err
			}
			if idx > math.MaxUint8 {
				return nil, fmt.Errorf("DW_OP_bregx index too large: %d", idx)
			}
			op = ir.Register{RegNo: uint8(idx), Shift: shift}

		case opcode == dw_op_call_frame_cfa:
			op = ir.Cfa{CfaOffset: 0}

		case dw_eval_op_lo <= opcode && opcode <= dw_eval_op_hi:
			return nil, fmt.Errorf("unsupported stack-evaluation opcode: 0x%x", opcode)

		case dw_user_op_lo <= opcode && opcode <= dw_user_op_hi:
			return nil, fmt.Errorf("unsupported user-defined opcode: 0x%x", opcode)
		default:
			return nil, fmt.Errorf("unknown opcode: 0x%x", opcode)
		}
	}

	// Single op instruction list has an implicit piece.
	if instCnt == 1 {
		pieces = append(pieces, ir.Piece{Size: totalByteSize, Op: op})
		op = nil
	}

	if op != nil {
		return nil, fmt.Errorf("unconsumed last op: %v", op)
	}

	return pieces, nil
}

func readFixedSize(reader *bytes.Buffer, size uint8) (uint64, error) {
	switch size {
	case 1:
		val, err := reader.ReadByte()
		return uint64(val), err
	case 2:
		return uint64(binary.LittleEndian.Uint16(reader.Next(2))), nil
	case 4:
		return uint64(binary.LittleEndian.Uint32(reader.Next(4))), nil
	case 8:
		return binary.LittleEndian.Uint64(reader.Next(8)), nil
	default:
		return 0, fmt.Errorf("unsupported reading %d-byte integer", size)
	}
}

func readULEB128(reader *bytes.Buffer) (uint64, error) {
	var val uint64
	var shift uint
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		val |= uint64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	return val, nil
}

func readSLEB128(reader *bytes.Buffer) (int64, error) {
	var val int64
	var shift uint
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		val |= int64(b&0x7f) << shift
		shift += 7
		if b&0x80 == 0 {
			break
		}
	}
	if val&0x40 != 0 {
		val -= 1 << shift
	}
	return val, nil
}

// FixLoclists mitigates bug in go that can cause incorrect location lists to be generated.
// The bug occurs because of some behavior in ssa where different pieces of a
// variable can be interpreted as different types and can end up having
// duplicated location lists. Empirically this has primarily been observed with
// contexts and when it occurs, the location lists are doubled. Below is a
// somewhat crude heuristic to detect this case and fix it.
//
// See https://github.com/golang/go/issues/61700
func FixLoclists(loclists []ir.Location, expectedByteSize uint64) ([]ir.Location, error) {
	currentByteSize := func(loclists []ir.Location) uint64 {
		if len(loclists) == 0 || len(loclists[0].Pieces) == 0 {
			return 0
		}
		var sum uint64
		for _, piece := range loclists[0].Pieces {
			sum += uint64(piece.Size)
		}
		return sum
	}

	// We want to find any pair of adjacent pieces that are in the same location
	for currentByteSize(loclists) > expectedByteSize {
		if len(loclists) == 0 || len(loclists[0].Pieces) == 0 {
			break
		}

		numPieces := len(loclists[0].Pieces)
		var idx int
		var found bool

		// First strategy: find adjacent pieces with same byte size and same operation
		for _, loclist := range loclists {
			for i := 0; i < len(loclist.Pieces)-1; i++ {
				a, b := &loclist.Pieces[i], &loclist.Pieces[i+1]
				if a.Size == b.Size &&
					a.Op != nil && b.Op != nil &&
					a.Op == b.Op {
					idx = i
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		// Second strategy: find pieces where there are no conflicts in their location
		// because it's always either one is present or the other but not both
		if !found {
			for i := 1; i < numPieces; i++ {
				allMatch := true
				for _, loclist := range loclists {
					if i >= len(loclist.Pieces) {
						allMatch = false
						break
					}
					a, b := &loclist.Pieces[i-1], &loclist.Pieces[i]
					matched := a.Size == b.Size && (a.Op == nil) != (b.Op == nil)
					if !matched {
						allMatch = false
						break
					}
				}
				if allMatch {
					idx = i - 1
					found = true
					break
				}
			}
		}

		// Third strategy: find adjacent pieces with same byte size where both ops are nil
		// and the next piece has a non-nil op
		if !found {
			for i := 1; i < numPieces; i++ {
				allMatch := true
				for _, loclist := range loclists {
					if i >= len(loclist.Pieces) {
						allMatch = false
						break
					}
					a, b := &loclist.Pieces[i-1], &loclist.Pieces[i]
					matched := a.Size == b.Size && a.Op == nil && b.Op == nil
					if !matched {
						allMatch = false
						break
					}
					// Check if next piece has non-nil op (or we're at the end)
					if i+1 < len(loclist.Pieces) {
						next := &loclist.Pieces[i+1]
						if next.Op == nil {
							allMatch = false
							break
						}
					}
				}
				if allMatch {
					idx = i - 1
					found = true
					break
				}
			}
		}

		if !found {
			return nil, fmt.Errorf("could not compact pieces")
		}

		// Remove and merge pieces at the found index
		for i := range loclists {
			if idx >= len(loclists[i].Pieces) || idx+1 >= len(loclists[i].Pieces) {
				continue
			}

			removed := loclists[i].Pieces[idx]
			// Remove the piece at foundIndex
			loclists[i].Pieces = append(loclists[i].Pieces[:idx], loclists[i].Pieces[idx+1:]...)

			// Merge with the piece that's now at foundIndex (was at foundIndex+1)
			if idx < len(loclists[i].Pieces) {
				other := &loclists[i].Pieces[idx]
				if removed.Op != nil {
					other.Op = removed.Op
				}
			}
		}
	}

	return loclists, nil
}
