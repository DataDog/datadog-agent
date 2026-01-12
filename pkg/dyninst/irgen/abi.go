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

// abiConfig holds architecture-specific ABI details.
type abiConfig struct {
	// intRegs maps logical register numbers (0, 1, 2...) to physical DWARF
	// register numbers.
	intRegs []uint8
	// maxIntRegs is the number of integer registers available for
	// arguments/returns.
	maxIntRegs int
	// maxFPRegs is the number of floating-point registers available for
	// arguments/returns.
	maxFPRegs int
	// stackBase is the architecture-specific offset from CFA to the start
	// of the caller-allocated argument space.
	stackBase int32
}

// amd64ABI defines the ABI configuration for amd64.
// See https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L390
var amd64ABI = &abiConfig{
	intRegs:    []uint8{0, 3, 2, 5, 4, 8, 9, 10, 11},
	maxIntRegs: 9,  // RAX, RBX, RCX, RDI, RSI, R8, R9, R10, R11
	maxFPRegs:  15, // X0-X14
	stackBase:  0,
}

// arm64ABI defines the ABI configuration for arm64.
// See https://github.com/golang/go/blob/62deaf4f/src/cmd/compile/abi-internal.md?plain=1#L516
// R0-R15 are used for integer arguments/returns.
var arm64ABI = &abiConfig{
	intRegs:    []uint8{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	maxIntRegs: 16,
	maxFPRegs:  16, // F0-F15
	stackBase:  8,  // Link register storage
}

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
	// Select architecture-specific ABI configuration.
	var abi *abiConfig
	switch arch {
	case "amd64":
		abi = amd64ABI
	case "arm64":
		abi = arm64ABI
	default:
		return fmt.Errorf("unsupported architecture: %s", arch)
	}
	ptrSize := int32(arch.PointerSize())

	align := func(offset int32) int32 {
		return ((offset + ptrSize - 1) / ptrSize) * ptrSize
	}

	// Collect return PCs from all return events in probes.
	var returnPCs []uint64
	for _, probe := range probes {
		for _, event := range probe.Events {
			if event.Kind != ir.EventKindReturn {
				continue
			}
			for _, ip := range event.InjectionPoints {
				returnPCs = append(returnPCs, ip.PC)
			}
		}
	}
	if len(returnPCs) == 0 {
		return nil
	}

	// Sort return PCs for efficient range operations.
	slices.Sort(returnPCs)
	returnPCs = slices.Compact(returnPCs)

	// Stack layout:
	//  ┌─────────────────────────────────────┐  Lower addresses ↑
	//  │ Callee's local variables            │  ← Callee's frame
	//  │ (negative CFA offsets in DWARF)     │
	//  ├─────────────────────────────────────┤
	//  │ Frame pointer                       │
	//  ├─────────────────────────────────────┤
	//  │ CFA                                 │  ← Offset 0
	//  │ [link register] (ARM64 only)        │
	//  │ [stack-assigned receiver & args]    │
	//  │ [pointer-alignment]                 │
	//  │ [stack-assigned results]            │
	//  │ [pointer-alignment]                 │
	//  │ [spill space for reg-assigned args] │
	//  ├─────────────────────────────────────┤
	//  │ Caller's local variables            │  ← Caller's frame
	//  └─────────────────────────────────────┘  Higher addresses ↓

	// Pass 1: compute stack-assigned argument size.
	// We don't augment arguments, but need this to compute result offsets.
	argRegs := &registerState{}
	var argStackSize int32
	for _, v := range subprogram.Variables {
		if v.Role != ir.VariableRoleParameter {
			continue
		}

		_, ok, err := tryRegisterAssign(v.Type, argRegs, abi)
		if err != nil {
			return fmt.Errorf(
				"computing ABI for argument %q: %w", v.Name, err,
			)
		}
		if !ok {
			// Stack-assigned argument.
			argStackSize += align(int32(v.Type.GetByteSize()))
		}
		// Note: we don't track spill space for register-assigned arguments
		// because it comes AFTER results in the frame layout, so it doesn't
		// affect result offsets.
	}
	argStackSize = align(argStackSize)

	// Pass 2: process return values and determine assignment.
	type returnInfo struct {
		variable *ir.Variable
		pieces   []abiPiece
		onStack  bool
	}
	var returnVars []returnInfo

	resultRegs := &registerState{} // reset per ABI step 4
	var resultStackSize int32
	for _, v := range subprogram.Variables {
		if v.Role != ir.VariableRoleReturn {
			continue
		}

		pieces, ok, err := tryRegisterAssign(v.Type, resultRegs, abi)
		if err != nil {
			return fmt.Errorf("computing ABI for return %q: %w", v.Name, err)
		}

		info := returnInfo{variable: v}
		if ok {
			info.pieces = pieces
		} else {
			info.onStack = true
			resultStackSize += align(int32(v.Type.GetByteSize()))
		}
		returnVars = append(returnVars, info)
	}

	// Pass 3: create IR pieces and update variable locations.
	cfaOffset := abi.stackBase + align(argStackSize)

	containsFloatingPointRegisters := func(pieces []abiPiece) bool {
		return slices.ContainsFunc(pieces, func(p abiPiece) bool {
			return !p.isIntReg
		})
	}
	for _, info := range returnVars {
		if !info.onStack && containsFloatingPointRegisters(info.pieces) {
			// Skip it, as we don't support partial struct availability yet.
			continue
		}

		var pieces []ir.Piece
		if info.onStack {
			// Stack-assigned: CFA offset is always available.
			byteSize := info.variable.Type.GetByteSize()
			pieces = []ir.Piece{{
				Size: byteSize,
				Op:   ir.Cfa{CfaOffset: cfaOffset},
			}}
			cfaOffset += align(int32(byteSize))
		} else {
			// Register-assigned with all pieces available.
			pieces = make([]ir.Piece, len(info.pieces))
			for i, p := range info.pieces {
				pieces[i] = ir.Piece{
					Size: p.size,
					Op:   ir.Register{RegNo: abi.intRegs[p.regNo]},
				}
			}
		}

		// Remove existing locations that overlap with return PCs.
		locs := make(
			[]ir.Location, 0, len(info.variable.Locations)+len(returnPCs),
		)
		for _, loc := range info.variable.Locations {
			locs = append(locs, subtractPCsFromLocation(loc, returnPCs)...)
		}

		// Add ABI-derived locations at each return PC.
		for _, pc := range returnPCs {
			locs = append(locs, ir.Location{
				Range: ir.PCRange{pc, pc + 1}, Pieces: pieces,
			})
		}

		// Sort by start PC for efficient lookups.
		slices.SortFunc(locs, func(a, b ir.Location) int {
			return cmp.Compare(a.Range[0], b.Range[0])
		})
		info.variable.Locations = locs
	}

	return nil
}

// subtractPCsFromLocation removes PCs from a location's range, potentially
// splitting it into multiple non-overlapping ranges.
func subtractPCsFromLocation(
	loc ir.Location, pcs []uint64,
) []ir.Location {
	if len(pcs) == 0 {
		return []ir.Location{loc}
	}

	var result []ir.Location
	start, end := loc.Range[0], loc.Range[1]

	for _, pc := range pcs {
		if pc+1 <= start || pc >= end {
			continue
		}
		// Split: add segment before PC.
		if pc > start {
			result = append(result, ir.Location{
				Range:  ir.PCRange{start, pc},
				Pieces: loc.Pieces,
			})
		}
		start = pc + 1
	}

	// Add remaining segment after last PC.
	if start < end {
		result = append(result, ir.Location{
			Range:  ir.PCRange{start, end},
			Pieces: loc.Pieces,
		})
	}

	return result
}

// abiPiece represents a piece of a value in the ABI.
type abiPiece struct {
	size  uint32
	regNo int
	// isIntReg indicates whether this is an integer register (true) or
	// floating-point register (false).
	isIntReg bool
}

// tryRegisterAssign attempts to register-assign a type per the Go ABI.
// Returns (pieces, true, nil) on success, (nil, false, nil) if assignment
// fails per ABI rules, or (nil, false, error) on unexpected conditions.
// Register assignment is all-or-nothing per value.
func tryRegisterAssign(
	t ir.Type, regs *registerState, abi *abiConfig,
) ([]abiPiece, bool, error) {
	saved := *regs
	pieces, ok, err := registerAssign(t, regs, abi)
	if !ok && err == nil {
		*regs = saved // backtrack on normal failure
	}
	return pieces, ok, err
}

// registerAssign recursively assigns registers for a type per the Go ABI.
// Returns pieces and true on success, nil and false if assignment fails per
// ABI rules, or an error if an unexpected condition is encountered.
func registerAssign(
	t ir.Type, regs *registerState, abi *abiConfig,
) ([]abiPiece, bool, error) {
	maxIntRegs := abi.maxIntRegs
	maxFPRegs := abi.maxFPRegs

	switch typ := t.(type) {
	case *ir.BaseType:
		// Check if type has Go kind information.
		kind, ok := typ.GetGoKind()
		if !ok {
			// Unexpected: base type with no Go kind.
			return nil, false, fmt.Errorf(
				"base type %q has no Go kind", typ.GetName(),
			)
		}

		// Handle each kind appropriately.
		switch kind {
		case reflect.Bool, reflect.Int, reflect.Uint, reflect.Uintptr,
			reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			regNo, ok := regs.allocInt(maxIntRegs)
			if !ok {
				return nil, false, nil
			}
			return []abiPiece{{
				size:     typ.GetByteSize(),
				regNo:    regNo,
				isIntReg: true,
			}}, true, nil

		case reflect.Float32, reflect.Float64:
			regNo, ok := regs.allocFP(maxFPRegs)
			if !ok {
				return nil, false, nil
			}
			return []abiPiece{{
				size:     typ.GetByteSize(),
				regNo:    regNo,
				isIntReg: false,
			}}, true, nil

		case reflect.Complex64, reflect.Complex128:
			fpRegs, ok := regs.allocFPN(2, maxFPRegs)
			if !ok {
				return nil, false, nil
			}
			halfSize := typ.GetByteSize() / 2
			return []abiPiece{
				{size: halfSize, regNo: fpRegs[0], isIntReg: false},
				{size: halfSize, regNo: fpRegs[1], isIntReg: false},
			}, true, nil

		default:
			return nil, false, fmt.Errorf(
				"unsupported base type kind: %s (%v)", typ.GetName(), kind,
			)
		}

	case *ir.PointerType, *ir.VoidPointerType, *ir.GoChannelType,
		*ir.GoSubroutineType, *ir.GoMapType:
		regNo, ok := regs.allocInt(maxIntRegs)
		if !ok {
			return nil, false, nil
		}
		return []abiPiece{{
			size: typ.GetByteSize(), regNo: regNo, isIntReg: true,
		}}, true, nil

	case *ir.GoStringHeaderType, *ir.GoEmptyInterfaceType,
		*ir.GoInterfaceType:
		intRegs, ok := regs.allocIntN(2, maxIntRegs)
		if !ok {
			return nil, false, nil
		}
		return []abiPiece{
			{size: 8, regNo: intRegs[0], isIntReg: true},
			{size: 8, regNo: intRegs[1], isIntReg: true},
		}, true, nil

	case *ir.GoSliceHeaderType:
		intRegs, ok := regs.allocIntN(3, maxIntRegs)
		if !ok {
			return nil, false, nil
		}
		return []abiPiece{
			{size: 8, regNo: intRegs[0], isIntReg: true},
			{size: 8, regNo: intRegs[1], isIntReg: true},
			{size: 8, regNo: intRegs[2], isIntReg: true},
		}, true, nil

	case *ir.StructureType:
		var pieces []abiPiece
		for _, field := range typ.RawFields {
			fieldPieces, ok, err := registerAssign(field.Type, regs, abi)
			if err != nil {
				return nil, false, fmt.Errorf(
					"struct %s field %s: %w", typ.GetName(), field.Name, err,
				)
			}
			if !ok {
				return nil, false, nil
			}
			pieces = append(pieces, fieldPieces...)
		}
		return pieces, true, nil

	case *ir.ArrayType:
		if !typ.HasCount {
			return nil, false, fmt.Errorf(
				"array type %s has no count", typ.GetName(),
			)
		}
		switch typ.Count {
		case 0:
			return []abiPiece{}, true, nil
		case 1:
			return registerAssign(typ.Element, regs, abi)
		default:
			return nil, false, nil
		}

	case *ir.UnresolvedPointeeType:
		return nil, false, fmt.Errorf(
			"unresolved pointee type %s", typ.GetName(),
		)

	case *ir.EventRootType:
		return nil, false, fmt.Errorf(
			"unexpected event root type %s", typ.GetName(),
		)

	case *ir.GoSliceDataType, *ir.GoStringDataType:
		return nil, false, fmt.Errorf(
			"unexpected dynamically-sized type %s", typ.GetName(),
		)

	case *ir.GoHMapHeaderType, *ir.GoHMapBucketType,
		*ir.GoSwissMapHeaderType, *ir.GoSwissMapGroupsType:
		return nil, false, fmt.Errorf(
			"unexpected internal map type %s", typ.GetName(),
		)

	default:
		return nil, false, fmt.Errorf(
			"unknown type for ABI: %T (%s)", typ, t.GetName(),
		)
	}
}

// registerState tracks available registers during ABI assignment.
type registerState struct {
	intReg int
	fpReg  int
}

// allocInt allocates one integer register if available.
func (r *registerState) allocInt(max int) (int, bool) {
	if r.intReg >= max {
		return 0, false
	}
	reg := r.intReg
	r.intReg++
	return reg, true
}

// allocIntN allocates N consecutive integer registers if available.
func (r *registerState) allocIntN(n, max int) ([]int, bool) {
	if r.intReg+n-1 >= max {
		return nil, false
	}
	regs := make([]int, n)
	for i := 0; i < n; i++ {
		regs[i] = r.intReg + i
	}
	r.intReg += n
	return regs, true
}

// allocFP allocates one floating-point register if available.
func (r *registerState) allocFP(max int) (int, bool) {
	if r.fpReg >= max {
		return 0, false
	}
	reg := r.fpReg
	r.fpReg++
	return reg, true
}

// allocFPN allocates N consecutive floating-point registers if available.
func (r *registerState) allocFPN(n, max int) ([]int, bool) {
	if r.fpReg+n-1 >= max {
		return nil, false
	}
	regs := make([]int, n)
	for i := 0; i < n; i++ {
		regs[i] = r.fpReg + i
	}
	r.fpReg += n
	return regs, true
}
