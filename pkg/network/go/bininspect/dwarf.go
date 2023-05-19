// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build linux

package bininspect

import (
	"debug/dwarf"
	"debug/elf"
	"errors"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils/locexpr"
	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/dwarf/loclist"
)

// dwarfInspector is used to keep common data for the dwarf inspection functions.
type dwarfInspector struct {
	elf       elfMetadata
	dwarfData *dwarf.Data
}

// InspectWithDWARF returns the offsets of the given functions and fields in the given elf file.
// It also returns some additional relevant metadata about the given file.
// It is using the DWARF debug data to obtain information, and therefore should be run on elf files that contain debug
// data, like our test binaries.
func InspectWithDWARF(elfFile *elf.File, functions []string, structFields []FieldIdentifier) (*Result, error) {
	if elfFile == nil {
		return nil, ErrNilElf
	}

	// Determine the architecture of the binary
	arch, err := GetArchitecture(elfFile)
	if err != nil {
		return nil, err
	}

	dwarfData, ok := HasDwarfInfo(elfFile)

	if !ok || dwarfData == nil {
		return nil, errors.New("expected dwarf data")
	}

	inspector := dwarfInspector{
		elf: elfMetadata{
			file: elfFile,
			arch: arch,
		},
		dwarfData: dwarfData,
	}

	// Scan for functions and struct offsets
	functionsMetadata, err := inspector.findFunctionsUsingDWARF(functions)
	if err != nil {
		return nil, err
	}

	structOffsets, err := inspector.findStructOffsets(structFields)
	if err != nil {
		return nil, err
	}

	goVersion, err := FindGoVersion(elfFile)
	if err != nil {
		return nil, err
	}

	abi, err := FindABI(goVersion, arch)
	if err != nil {
		return nil, err
	}

	return &Result{
		Arch:          arch,
		ABI:           abi,
		GoVersion:     goVersion,
		Functions:     functionsMetadata,
		StructOffsets: structOffsets,
	}, nil

}

func (d dwarfInspector) findFunctionsUsingDWARF(functions []string) (map[string]FunctionMetadata, error) {
	// Find each function's dwarf entry
	functionEntries, err := d.findFunctionDebugInfoEntries(functions)
	if err != nil {
		return nil, err
	}

	// Inspect each function individually
	functionMetadataMap := make(map[string]FunctionMetadata, len(functionEntries))
	for functionName, entry := range functionEntries {
		metadata, err := d.inspectSingleFunctionUsingDWARF(entry)
		if err != nil {
			return nil, err
		}

		functionMetadataMap[functionName] = metadata
	}

	return functionMetadataMap, nil
}

func (d dwarfInspector) findFunctionDebugInfoEntries(functions []string) (map[string]*dwarf.Entry, error) {
	// Convert the function config slice to a set of names
	functionsToSearch := make(map[string]struct{}, len(functions))
	for _, function := range functions {
		functionsToSearch[function] = struct{}{}
	}

	functionEntries := make(map[string]*dwarf.Entry)
	entryReader := d.dwarfData.Reader()
	for entry, err := entryReader.Next(); entry != nil; entry, err = entryReader.Next() {
		if err != nil {
			return nil, err
		}

		// Check if this entry is a function
		if entry.Tag != dwarf.TagSubprogram {
			continue
		}

		funcName, _ := entry.Val(dwarf.AttrName).(string)

		// See if the func name is one of the search functions
		if _, ok := functionsToSearch[funcName]; !ok {
			continue
		}

		delete(functionsToSearch, funcName)
		functionEntries[funcName] = entry
	}

	if len(functionsToSearch) != 0 {
		return nil, errors.New("not all functions found")
	}

	return functionEntries, nil
}

// inspectSingleFunctionUsingDWARF receives a DWARf entry representing a function and extracts its address in the binary
// and the parameters locations.
func (d dwarfInspector) inspectSingleFunctionUsingDWARF(entry *dwarf.Entry) (FunctionMetadata, error) {
	lowPC, _ := entry.Val(dwarf.AttrLowpc).(uint64)

	// Get all child leaf entries of the function entry
	// that have the type "formal parameter".
	// This includes parameters (both method receivers and normal arguments)
	// and return values.
	entryReader := d.dwarfData.Reader()
	formalParameterEntries, err := dwarfutils.GetChildLeafEntries(entryReader, entry.Offset, dwarf.TagFormalParameter)
	if err != nil {
		return FunctionMetadata{}, fmt.Errorf("failed getting formal parameter children: %w", err)
	}

	// Iterate through each formal parameter entry and classify/inspect them
	params := []ParameterMetadata{}
	for _, formalParamEntry := range formalParameterEntries {
		isReturn, _ := formalParamEntry.Val(dwarf.AttrVarParam).(bool)
		if isReturn {
			// Return parameters have empty locations,
			// so there is no point in trying to execute their location expressions.
			continue
		}

		parameter, err := d.getParameterLocationAtPC(formalParamEntry, lowPC)
		if err != nil {
			paramName, _ := formalParamEntry.Val(dwarf.AttrName).(string)
			return FunctionMetadata{}, fmt.Errorf("could not inspect param %q on function: %w", paramName, err)
		}

		params = append(params, parameter)
	}

	return FunctionMetadata{
		// This should really probably be the location of the end of the prologue
		// (which might help with parameter locations being half-spilled),
		// but so far using the first PC position in the function has worked
		// for the functions we're tracing.
		// See:
		// - https://github.com/go-delve/delve/pull/2704#issuecomment-944374511
		//   (which implies that the instructions in the prologue
		//   might get executed multiple times over the course of a single function call,
		//   though I'm not sure under what circumstances this might be true)
		EntryLocation: lowPC,
		Parameters:    params,
	}, nil
}

func (d dwarfInspector) getParameterLocationAtPC(parameterDIE *dwarf.Entry, pc uint64) (ParameterMetadata, error) {
	typeOffset, ok := parameterDIE.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return ParameterMetadata{}, fmt.Errorf("no type offset attribute in parameter entry")
	}

	// Find the location field on the entry
	locationField := parameterDIE.AttrField(dwarf.AttrLocation)
	if locationField == nil {
		return ParameterMetadata{}, fmt.Errorf("no location field in parameter entry")
	}

	typ, err := dwarfutils.NewTypeFinder(d.dwarfData).FindTypeByOffset(typeOffset)
	if err != nil {
		return ParameterMetadata{}, fmt.Errorf("could not find parameter type by offset: %w", err)
	}

	// The location field can be one of two things:
	// (See DWARF v4 spec section 2.6)
	// 1. Single location descriptions,
	//    which specifies a location expression as the direct attribute value.
	//    This has a DWARF class of `exprloc`,
	//    and the value is a `[]byte` that can be directly interpreted.
	// 2. Location lists, which gives an index into the loclists section.
	//    This has a DWARF class of `loclistptr`,
	//    which is used to index into the location list
	//    and to get the location expression that corresponds to
	//    the given program counter
	//    (in this case, that is the entry of the function, where we will attach the uprobe).
	var locationExpression []byte
	switch locationField.Class {
	case dwarf.ClassExprLoc:
		if locationValAsBytes, ok := locationField.Val.([]byte); ok {
			locationExpression = locationValAsBytes
		} else {
			return ParameterMetadata{}, fmt.Errorf("formal parameter entry contained invalid value for location attribute: locationField=%#v", locationField)
		}
	case dwarf.ClassLocListPtr:
		locationAsLocListIndex, ok := locationField.Val.(int64)
		if !ok {
			return ParameterMetadata{}, fmt.Errorf("could not interpret location attribute in formal parameter entry as location list pointer: locationField=%#v", locationField)
		}

		loclistEntry, err := d.getLoclistEntry(locationAsLocListIndex, pc)
		if err != nil {
			return ParameterMetadata{}, fmt.Errorf("could not find loclist entry at %#x for PC %#x: %w", locationAsLocListIndex, pc, err)
		}
		locationExpression = loclistEntry.Instr
	default:
		return ParameterMetadata{}, fmt.Errorf("unexpected field class on formal parameter's location attribute: locationField=%#v", locationField)
	}

	totalSize := typ.Size()
	pieces, err := locexpr.Exec(locationExpression, totalSize, int(d.elf.arch.PointerSize()))
	if err != nil {
		return ParameterMetadata{}, fmt.Errorf("error executing location expression for parameter: %w", err)
	}
	inspectPieces := make([]ParameterPiece, len(pieces))
	for i, piece := range pieces {
		inspectPieces[i] = ParameterPiece{
			Size:        piece.Size,
			InReg:       piece.InReg,
			StackOffset: piece.StackOffset,
			Register:    piece.Register,
		}
	}
	return ParameterMetadata{
		TotalSize: totalSize,
		Kind:      typ.Common().ReflectKind,
		Pieces:    inspectPieces,
	}, nil
}

func (d dwarfInspector) findStructOffsets(structFields []FieldIdentifier) (map[FieldIdentifier]uint64, error) {
	structOffsets := make(map[FieldIdentifier]uint64)
	typeReader := dwarfutils.NewTypeFinder(d.dwarfData)
	for _, fieldID := range structFields {
		offset, err := typeReader.FindStructFieldOffset(fieldID.StructName, fieldID.FieldName)
		if err != nil {
			return nil, fmt.Errorf("could not find offset of \"%s.%s\": %w", fieldID.StructName, fieldID.FieldName, err)
		}
		structOffsets[fieldID] = offset
	}
	return structOffsets, nil
}

// getLoclistEntry returns the loclist entry in the loclist
// starting at offset, for address pc.
// Adapted from github.com/go-delve/delve/pkg/proc.(*BinaryInfo).loclistEntry
func (d dwarfInspector) getLoclistEntry(offset int64, pc uint64) (*loclist.Entry, error) {
	debugInfoBytes, err := godwarf.GetDebugSectionElf(d.elf.file, "info")
	if err != nil {
		return nil, err
	}

	compileUnits, err := dwarfutils.LoadCompileUnits(d.dwarfData, debugInfoBytes)
	if err != nil {
		return nil, err
	}

	debugLocBytes, _ := godwarf.GetDebugSectionElf(d.elf.file, "loc")
	loclist2 := loclist.NewDwarf2Reader(debugLocBytes, int(d.elf.arch.PointerSize()))
	debugLoclistBytes, _ := godwarf.GetDebugSectionElf(d.elf.file, "loclists")
	loclist5 := loclist.NewDwarf5Reader(debugLoclistBytes)
	debugAddrBytes, _ := godwarf.GetDebugSectionElf(d.elf.file, "addr")
	debugAddrSection := godwarf.ParseAddr(debugAddrBytes)

	var base uint64
	compileUnit := compileUnits.FindCompileUnit(pc)
	if compileUnit != nil {
		base = compileUnit.LowPC
	}

	var loclist loclist.Reader = loclist2
	var debugAddr *godwarf.DebugAddr
	if compileUnit != nil && compileUnit.Version >= 5 && loclist5 != nil {
		loclist = loclist5
		if addrBase, ok := compileUnit.Entry.Val(dwarf.AttrAddrBase).(int64); ok {
			debugAddr = debugAddrSection.GetSubsection(uint64(addrBase))
		}
	}

	if loclist.Empty() {
		return nil, fmt.Errorf("no loclist found for the given program counter")
	}

	// Use 0x0 as the static base
	var staticBase uint64 = 0x0
	entry, err := loclist.Find(int(offset), staticBase, base, pc, debugAddr)
	if err != nil {
		return nil, fmt.Errorf("error reading loclist section: %w", err)
	}
	if entry != nil {
		return entry, nil
	}

	return nil, fmt.Errorf("no loclist entry found")
}
