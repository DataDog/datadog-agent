// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

type abiRegs []uint8

// RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
// See https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L390
// https://gitlab.com/x86-psABIs/x86-64-ABI/-/blob/e1ce098331da5dbd66e1ffc74162380bcc213236/x86-64-ABI/low-level-sys-info.tex#L2508-2516
var amd64AbiRegs = abiRegs{0, 3, 2, 5, 4, 8, 9, 10, 11}

// https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L516
var arm64AbiRegs = abiRegs{0, 1, 2, 3, 4, 5, 6, 7, 8}

// augmentReturnLocationsFromABI adds ABI-derived location information for
// return variables at return probe points. It computes register and stack
// assignments based on the Go internal ABI and updates variable locations
// to include this information at return PCs.
//
// See https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md#function-call-argument-and-result-passing
func augmentReturnLocationsFromABI(
	arch object.Architecture,
	subprogram *ir.Subprogram,
	probes []*ir.Probe,
) error {
	// Determine which register set to use based on architecture.
	var intRegs abiRegs
	switch arch {
	case "amd64":
		intRegs = amd64AbiRegs
	case "arm64":
		intRegs = arm64AbiRegs
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}

	// Collect return PCs from all return events in probes.
	var returnPCs []uint64
	for _, probe := range probes {
		for _, event := range probe.Events {
			if event.Kind == ir.EventKindReturn {
				for _, ip := range event.InjectionPoints {
					returnPCs = append(returnPCs, ip.PC)
				}
			}
		}
	}
	if len(returnPCs) == 0 {
		return nil
	}

	// Sort return PCs for efficient range operations.
	slices.Sort(returnPCs)
	returnPCs = slices.Compact(returnPCs)

	// Track stack offset for spill space and stack-assigned returns.
	// Per the Go ABI, the stack frame layout is:
	// [stack-assigned arguments, stack-assigned returns, pointer-align, spill space for reg returns]
	var stackOffset int32
	switch arch {
	case "amd64":
		stackOffset = 0
	case "arm64":
		stackOffset = 8
	}

	// Zeroth pass: compute stack offset due to arguments.
	inRegs := &registerState{}
	for _, variable := range subprogram.Variables {
		if !variable.IsParameter || variable.IsReturn {
			continue
		}

		// Try to register-assign this return value.
		// Per the Go ABI, if any piece can't fit in registers, the entire
		// value goes on the stack.
		_, ok := tryRegisterAssign(variable.Type, inRegs)
		if ok {
			continue
		}
		// Stack-assigned: entire value goes on the stack.
		stackSize := variable.Type.GetByteSize()
		stackOffset = alignInt32(stackOffset+int32(stackSize), 8)
	}
	stackAssignedOffset := stackOffset

	// First pass: compute locations and determine what goes on stack.
	type returnVarInfo struct {
		variable  *ir.Variable
		pieces    []abiPiece
		onStack   bool
		stackSize uint32
	}
	var returnVars []returnVarInfo

	regs := &registerState{}
	for _, variable := range subprogram.Variables {
		if !variable.IsReturn {
			continue
		}

		// Try to register-assign this return value.
		// Per the Go ABI, if any piece can't fit in registers, the entire
		// value goes on the stack.
		pieces, ok := tryRegisterAssign(variable.Type, regs)
		if !ok {
			// Stack-assigned: entire value goes on the stack.
			stackSize := variable.Type.GetByteSize()
			returnVars = append(returnVars, returnVarInfo{
				variable:  variable,
				pieces:    nil,
				onStack:   true,
				stackSize: stackSize,
			})
			// Align stack offset to pointer size for this return value.
			stackOffset = alignInt32(stackOffset+int32(stackSize), 8)
			continue
		}

		// Successfully register-assigned.
		returnVars = append(returnVars, returnVarInfo{
			variable:  variable,
			pieces:    pieces,
			onStack:   false,
			stackSize: variable.Type.GetByteSize(),
		})
	}

	// Add pointer alignment after stack-assigned returns.
	stackOffset = alignInt32(stackOffset, 8)

	// Reserve spill space for register-assigned returns.
	// Note: Per the Go ABI, spill space is reserved but not currently used
	// in our implementation. We track it here for correctness and potential
	// future use.
	for _, info := range returnVars {
		if !info.onStack {
			// spillOffset := stackOffset (would be used if we needed it)
			stackOffset = alignInt32(stackOffset+int32(info.stackSize), 8)
		}
	}

	// Second pass: create IR pieces and update locations.
	for _, info := range returnVars {
		var irPieces []ir.Piece

		if info.onStack {
			// Stack-assigned: use CFA offset.
			irPieces = []ir.Piece{{
				Size: info.stackSize,
				Op: ir.Cfa{
					CfaOffset: stackAssignedOffset,
				},
			}}
			stackAssignedOffset = alignInt32(
				stackAssignedOffset+int32(info.stackSize), 8,
			)
		} else {
			// Register-assigned: convert pieces to register assignments.
			// Check if this variable has any integer register pieces we can
			// actually provide information for.
			hasIntReg := false
			for _, piece := range info.pieces {
				if piece.isIntReg {
					hasIntReg = true
					break
				}
			}

			if !hasIntReg {
				// All pieces are FP registers (which we don't track).
				// Don't augment this variable - keep DWARF locations.
				continue
			}

			irPieces = make([]ir.Piece, 0, len(info.pieces))
			for _, piece := range info.pieces {
				irPiece := ir.Piece{Size: piece.size}
				if piece.isIntReg {
					// Integer register: map to physical register.
					irPiece.Op = ir.Register{
						RegNo: intRegs[piece.regNo],
						Shift: 0,
					}
				}
				// Floating-point register: leave Op as nil (unavailable).
				// We don't track FP registers in our implementation.
				irPieces = append(irPieces, irPiece)
			}
		}

		// Subtract return PC ranges from existing locations to avoid
		// overlaps.
		var newLocations []ir.Location
		for _, loc := range info.variable.Locations {
			newLocations = append(
				newLocations,
				subtractPCsFromLocation(loc, returnPCs)...,
			)
		}

		// Add ABI-derived locations for each return PC.
		for _, pc := range returnPCs {
			pcRange := ir.PCRange{pc, pc + 1}
			newLocations = append(newLocations, ir.Location{
				Range:  pcRange,
				Pieces: irPieces,
			})
		}

		// Sort locations by start PC to maintain ordering and make lookups
		// efficient.
		slices.SortFunc(newLocations, func(a, b ir.Location) int {
			return cmp.Compare(a.Range[0], b.Range[0])
		})

		info.variable.Locations = newLocations
	}

	return nil
}

// subtractPCsFromLocation removes the given PCs from a location's range,
// potentially splitting the location into multiple non-overlapping ranges.
func subtractPCsFromLocation(
	loc ir.Location, pcs []uint64,
) []ir.Location {
	if len(pcs) == 0 {
		return []ir.Location{loc}
	}

	rangeStart := loc.Range[0]
	rangeEnd := loc.Range[1]

	var result []ir.Location
	currentStart := rangeStart

	for _, pc := range pcs {
		// PC is completely before current segment.
		if pc+1 <= currentStart {
			continue
		}
		// PC is completely after the location range.
		if pc >= rangeEnd {
			break
		}

		// PC overlaps with [currentStart, rangeEnd). We need to exclude
		// [pc, pc+1).
		if pc > currentStart {
			// Add the range before the PC.
			result = append(result, ir.Location{
				Range:  ir.PCRange{currentStart, pc},
				Pieces: loc.Pieces,
			})
		}
		// Move past the excluded PC.
		currentStart = pc + 1
	}

	// Add any remaining range after the last PC.
	if currentStart < rangeEnd {
		result = append(result, ir.Location{
			Range:  ir.PCRange{currentStart, rangeEnd},
			Pieces: loc.Pieces,
		})
	}

	return result
}

// alignInt32 aligns an offset up to the given alignment.
func alignInt32(offset, alignment int32) int32 {
	return ((offset + alignment - 1) / alignment) * alignment
}

// abiPiece represents a piece of a value in the ABI.
type abiPiece struct {
	size  uint32
	regNo int
	// isIntReg indicates whether this is an integer register (true) or
	// floating-point register (false).
	isIntReg bool
}

// registerState tracks available registers during ABI assignment.
type registerState struct {
	intReg int
	fpReg  int
}

// tryRegisterAssign attempts to assign a type to registers according to the
// Go ABI. Returns the pieces and true if successful, or nil and false if the
// type cannot be register-assigned (e.g., too large, too complex, or not
// enough registers available). Per the ABI, register assignment is
// all-or-nothing per value.
func tryRegisterAssign(
	t ir.Type, regs *registerState,
) ([]abiPiece, bool) {
	// Remember register state in case we need to backtrack.
	savedRegs := *regs
	pieces, ok := computeABILocations(t, regs)
	if !ok {
		// Restore register state since assignment failed.
		*regs = savedRegs
	}
	return pieces, ok
}

// computeABILocations recursively computes register assignment for a type.
// Returns pieces and true on success, or nil and false if assignment fails.
func computeABILocations(
	t ir.Type, regs *registerState,
) ([]abiPiece, bool) {
	const maxIntRegs = 9
	const maxFPRegs = 15

	switch typ := t.(type) {
	// Basic integer-like types, pointers, channels, functions, maps.
	case *ir.BaseType:
		switch kind, ok := typ.GetGoKind(); {
		case ok && (kind == reflect.Bool ||
			kind == reflect.Int || kind == reflect.Int8 ||
			kind == reflect.Int16 || kind == reflect.Int32 ||
			kind == reflect.Int64 ||
			kind == reflect.Uint || kind == reflect.Uint8 ||
			kind == reflect.Uint16 || kind == reflect.Uint32 ||
			kind == reflect.Uint64 || kind == reflect.Uintptr):
			if regs.intReg < maxIntRegs {
				regNo := regs.intReg
				regs.intReg++
				return []abiPiece{{
					size:     typ.GetByteSize(),
					regNo:    regNo,
					isIntReg: true,
				}}, true
			}
			// Not enough integer registers.
			return nil, false
		case ok && (kind == reflect.Float32 || kind == reflect.Float64):
			// Floating-point: assign to FP register.
			if regs.fpReg < maxFPRegs {
				regNo := regs.fpReg
				regs.fpReg++
				return []abiPiece{{
					size:     typ.GetByteSize(),
					regNo:    regNo,
					isIntReg: false,
				}}, true
			}
			// Not enough FP registers.
			return nil, false
		case ok && (kind == reflect.Complex64 || kind == reflect.Complex128):
			// Complex types: two FP pieces.
			if regs.fpReg+1 < maxFPRegs {
				reg0 := regs.fpReg
				reg1 := regs.fpReg + 1
				regs.fpReg += 2
				halfSize := typ.GetByteSize() / 2
				return []abiPiece{
					{size: halfSize, regNo: reg0, isIntReg: false},
					{size: halfSize, regNo: reg1, isIntReg: false},
				}, true
			}
			// Not enough FP registers.
			return nil, false
		case ok:
			// Unsupported base type kind - cannot register-assign.
			return nil, false
		default:
			// Base type has no Go kind - cannot register-assign.
			return nil, false
		}

	case *ir.PointerType, *ir.VoidPointerType:
		if regs.intReg < maxIntRegs {
			regNo := regs.intReg
			regs.intReg++
			return []abiPiece{{
				size:     typ.GetByteSize(),
				regNo:    regNo,
				isIntReg: true,
			}}, true
		}
		// Not enough integer registers.
		return nil, false

	case *ir.GoChannelType, *ir.GoSubroutineType:
		// Channels and functions are pointer-sized.
		if regs.intReg < maxIntRegs {
			regNo := regs.intReg
			regs.intReg++
			return []abiPiece{{
				size:     typ.GetByteSize(),
				regNo:    regNo,
				isIntReg: true,
			}}, true
		}
		// Not enough integer registers.
		return nil, false

	case *ir.GoStringHeaderType, *ir.GoEmptyInterfaceType, *ir.GoInterfaceType:
		// Strings and interfaces: two pointer-sized fields.
		if regs.intReg+1 < maxIntRegs {
			reg0 := regs.intReg
			reg1 := regs.intReg + 1
			regs.intReg += 2
			return []abiPiece{
				{size: 8, regNo: reg0, isIntReg: true},
				{size: 8, regNo: reg1, isIntReg: true},
			}, true
		}
		// Not enough integer registers.
		return nil, false

	case *ir.GoSliceHeaderType:
		// Slices: three pointer-sized fields.
		if regs.intReg+2 < maxIntRegs {
			reg0 := regs.intReg
			reg1 := regs.intReg + 1
			reg2 := regs.intReg + 2
			regs.intReg += 3
			return []abiPiece{
				{size: 8, regNo: reg0, isIntReg: true},
				{size: 8, regNo: reg1, isIntReg: true},
				{size: 8, regNo: reg2, isIntReg: true},
			}, true
		}
		// Not enough integer registers.
		return nil, false

	case *ir.StructureType:
		// Recursively assign each field.
		var pieces []abiPiece
		for _, field := range typ.RawFields {
			fieldPieces, ok := computeABILocations(field.Type, regs)
			if !ok {
				// Field can't be register-assigned, so entire struct fails.
				return nil, false
			}
			pieces = append(pieces, fieldPieces...)
		}
		return pieces, true

	case *ir.ArrayType:
		// Only arrays of length 0 or 1 are register-assignable.
		if !typ.HasCount {
			// Array with no count cannot be register-assigned.
			return nil, false
		}
		switch typ.Count {
		case 0:
			return []abiPiece{}, true
		case 1:
			return computeABILocations(typ.Element, regs)
		default:
			// Array too large for register assignment.
			return nil, false
		}

	case *ir.GoMapType:
		// Maps are pointer-sized.
		if regs.intReg < maxIntRegs {
			regNo := regs.intReg
			regs.intReg++
			return []abiPiece{{
				size:     typ.GetByteSize(),
				regNo:    regNo,
				isIntReg: true,
			}}, true
		}
		// Not enough integer registers.
		return nil, false

	case *ir.UnresolvedPointeeType,
		*ir.EventRootType,
		*ir.GoSliceDataType,
		*ir.GoStringDataType,
		*ir.GoHMapHeaderType,
		*ir.GoHMapBucketType,
		*ir.GoSwissMapHeaderType,
		*ir.GoSwissMapGroupsType:
		// These types cannot be register-assigned.
		return nil, false

	default:
		// Unknown type - cannot register-assign.
		return nil, false
	}
}
