// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package bininspect

import (
	"debug/dwarf"
	"debug/elf"
	"debug/gosym"
	"fmt"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"github.com/go-delve/delve/pkg/dwarf/loclist"
	"github.com/go-delve/delve/pkg/goversion"

	"github.com/DataDog/datadog-agent/pkg/network/go/asmscan"
	"github.com/DataDog/datadog-agent/pkg/network/go/binversion"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils/locexpr"
)

type inspectionState struct {
	elfFile *elf.File
	config  Config
	symbols []elf.Symbol
	arch    GoArch

	// This field is only included if the binary has debug info attached
	dwarfInspectionState *dwarfInspectionState

	// The rest of the fields will be extracted
	abi       GoABI
	goVersion goversion.GoVersion
}

type dwarfInspectionState struct {
	dwarfData      *dwarf.Data
	debugInfoBytes []byte
	loclist2       *loclist.Dwarf2Reader
	loclist5       *loclist.Dwarf5Reader
	debugAddr      *godwarf.DebugAddrSection
	typeFinder     *dwarfutils.TypeFinder
	compileUnits   *dwarfutils.CompileUnits
}

// Inspect attempts to scan through a Golang ELF binary
// to find a variety of information useful for attaching eBPF uprobes to certain functions.
// Some information, such as struct offsets and function parameter locations,
// is only available when the binary has not had its debug information stripped.
// In such cases, it is recommended to construct a lookup table of well-known values
// (keyed by the Go version) to use instead.
func Inspect(elfFile *elf.File, config Config) (*Result, error) {
	// Determine the architecture of the binary
	arch, err := getArchitecture(elfFile)
	if err != nil {
		return nil, err
	}

	// Try to load in the ELF symbols.
	// This might fail if the binary was stripped.
	symbols, err := elfFile.Symbols()
	if err != nil {
		symbols = nil
	}

	// Determine if the binary has debug symbols,
	// and if it does, initialize the dwarf inspection state.
	var dwarfInspection *dwarfInspectionState
	if dwarfData, ok := HasDwarfInfo(elfFile); ok {
		debugInfoBytes, err := godwarf.GetDebugSectionElf(elfFile, "info")
		if err != nil {
			return nil, err
		}

		compileUnits, err := dwarfutils.LoadCompileUnits(dwarfData, debugInfoBytes)
		if err != nil {
			return nil, err
		}

		debugLocBytes, _ := godwarf.GetDebugSectionElf(elfFile, "loc")
		loclist2 := loclist.NewDwarf2Reader(debugLocBytes, int(arch.PointerSize()))
		debugLoclistBytes, _ := godwarf.GetDebugSectionElf(elfFile, "loclists")
		loclist5 := loclist.NewDwarf5Reader(debugLoclistBytes)
		debugAddrBytes, _ := godwarf.GetDebugSectionElf(elfFile, "addr")
		debugAddr := godwarf.ParseAddr(debugAddrBytes)

		dwarfInspection = &dwarfInspectionState{
			dwarfData:      dwarfData,
			debugInfoBytes: debugInfoBytes,
			loclist2:       loclist2,
			loclist5:       loclist5,
			debugAddr:      debugAddr,
			typeFinder:     dwarfutils.NewTypeFinder(dwarfData),
			compileUnits:   compileUnits,
		}
	}

	insp := &inspectionState{
		elfFile:              elfFile,
		symbols:              symbols,
		config:               config,
		dwarfInspectionState: dwarfInspection,
		arch:                 arch,
		// The rest of the fields will be extracted
	}
	result, err := insp.run()
	if err != nil {
		return nil, err
	}

	return result, nil
}

// HasDwarfInfo attempts to parse the DWARF data and look for any records.
// If it cannot be parsed or if there are no DWARF info records,
// then it assumes that the binary has been stripped.
func HasDwarfInfo(binary *elf.File) (*dwarf.Data, bool) {
	dwarfData, err := binary.DWARF()
	if err != nil {
		return nil, false
	}

	infoReader := dwarfData.Reader()
	if firstEntry, err := infoReader.Next(); err == nil && firstEntry != nil {
		return dwarfData, true
	}

	return nil, false
}

func (i *inspectionState) run() (*Result, error) {
	// First, find the Go version and ABI to use in other stages of the inspection:
	var err error
	i.goVersion, i.abi, err = i.findGoVersionAndABI()
	if err != nil {
		return nil, err
	}

	// Scan for functions and struct offsets
	functions, err := i.findFunctions()
	if err != nil {
		return nil, err
	}
	structOffsets, err := i.findStructOffsets()
	if err != nil {
		return nil, err
	}

	return &Result{
		Arch:                 i.arch,
		ABI:                  i.abi,
		GoVersion:            i.goVersion,
		IncludesDebugSymbols: i.dwarfInspectionState != nil,
		Functions:            functions,
		StructOffsets:        structOffsets,
	}, nil
}

// getArchitecture returns the `runtime.GOARCH`-compatible names of the architecture.
// Only returns a value for supported architectures.
func getArchitecture(elfFile *elf.File) (GoArch, error) {
	switch elfFile.FileHeader.Machine {
	case elf.EM_X86_64:
		return GoArchX86_64, nil
	case elf.EM_AARCH64:
		return GoArchARM64, nil
	}

	return "", fmt.Errorf("unsupported architecture")
}

// findGoVersionAndABI attempts to determine the Go version
// from the embedded string inserted in the binary by the linker.
// The implementation is available in src/cmd/go/internal/version/version.go:
// https://cs.opensource.google/go/go/+/refs/tags/go1.17.2:src/cmd/go/internal/version/version.go
// The main logic was pulled out to a sub-package, `binversion`
func (i *inspectionState) findGoVersionAndABI() (goversion.GoVersion, GoABI, error) {
	version, _, err := binversion.ReadElfBuildInfo(i.elfFile)
	if err != nil {
		return goversion.GoVersion{}, "", fmt.Errorf("could not get Go toolchain version from ELF binary file: %w", err)
	}

	parsed, ok := goversion.Parse(version)
	if !ok {
		return goversion.GoVersion{}, "", fmt.Errorf("failed to parse Go toolchain version %q", version)
	}

	// Statically assume the ABI based on the Go version and architecture
	var abi GoABI
	switch i.arch {
	case GoArchX86_64:
		if parsed.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 17}) {
			abi = GoABIRegister
		} else {
			abi = GoABIStack
		}
	case GoArchARM64:
		if parsed.AfterOrEqual(goversion.GoVersion{Major: 1, Minor: 18}) {
			abi = GoABIRegister
		} else {
			abi = GoABIStack
		}
	}

	return parsed, abi, nil
}

func (i *inspectionState) findFunctions() ([]FunctionMetadata, error) {
	// If the binary has debug symbols, we can traverse the debug info entries (DIEs)
	// to look for the functions.
	// Otherwise, fall-back to a go symbol table-based implementation
	// (see https://pkg.go.dev/debug/gosym).
	if i.dwarfInspectionState != nil {
		return i.findFunctionsUsingDWARF()
	}

	return i.findFunctionsUsingGoSymTab()
}

func (i *inspectionState) findFunctionsUsingDWARF() ([]FunctionMetadata, error) {
	// Find each function's dwarf entry
	functionEntries, err := i.findFunctionDebugInfoEntries()
	if err != nil {
		return nil, err
	}

	// Convert the configs to a map, keyed by the name
	configsByNames := make(map[string]FunctionConfig, len(i.config.Functions))
	for _, config := range i.config.Functions {
		configsByNames[config.Name] = config
	}

	// Inspect each function individually
	functions := []FunctionMetadata{}
	for functionName, entry := range functionEntries {
		if config, ok := configsByNames[functionName]; ok {
			metadata, err := i.inspectFunctionUsingDWARF(entry, config)
			if err != nil {
				return nil, err
			}

			functions = append(functions, metadata)
		}
	}

	return functions, nil
}

func (i *inspectionState) findFunctionDebugInfoEntries() (map[string]*dwarf.Entry, error) {
	// Convert the function config slice to a set of names
	searchFunctions := make(map[string]struct{}, len(i.config.Functions))
	for _, config := range i.config.Functions {
		searchFunctions[config.Name] = struct{}{}
	}

	functionEntries := make(map[string]*dwarf.Entry)
	entryReader := i.dwarfInspectionState.dwarfData.Reader()
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
		if _, ok := searchFunctions[funcName]; !ok {
			continue
		}

		delete(searchFunctions, funcName)
		functionEntries[funcName] = entry
	}

	return functionEntries, nil
}

func (i *inspectionState) inspectFunctionUsingDWARF(entry *dwarf.Entry, config FunctionConfig) (FunctionMetadata, error) {
	lowPC, _ := entry.Val(dwarf.AttrLowpc).(uint64)

	// Get all child leaf entries of the function entry
	// that have the type "formal parameter".
	// This includes parameters (both method receivers and normal arguments)
	// and return values.
	entryReader := i.dwarfInspectionState.dwarfData.Reader()
	formalParameterEntries, err := dwarfutils.GetChildLeafEntries(entryReader, entry.Offset, dwarf.TagFormalParameter)
	if err != nil {
		return FunctionMetadata{}, fmt.Errorf("failed getting formal parameter children: %w", err)
	}

	// If enabled, find all return locations in the function's machine code.
	var returnLocations []uint64
	if config.IncludeReturnLocations {
		highPC, _ := entry.Val(dwarf.AttrHighpc).(uint64)
		locations, err := i.findReturnLocations(lowPC, highPC)
		if err != nil {
			return FunctionMetadata{}, fmt.Errorf("could not find return locations for function %q: %w", config.Name, err)
		}

		returnLocations = locations
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

		parameter, err := i.getParameterLocationAtPC(formalParamEntry, lowPC)
		if err != nil {
			paramName, _ := formalParamEntry.Val(dwarf.AttrName).(string)
			return FunctionMetadata{}, fmt.Errorf("could not inspect param %q on function %q: %w", paramName, config.Name, err)
		}

		params = append(params, parameter)
	}

	return FunctionMetadata{
		Name: config.Name,
		// This should really probably be the location of the end of the prologue
		// (which might help with parameter locations being half-spilled),
		// but so far using the first PC position in the function has worked
		// for the functions we're tracing.
		// See:
		// - https://github.com/go-delve/delve/pull/2704#issuecomment-944374511
		//   (which implies that the instructions in the prologue
		//   might get executed multiple times over the course of a single function call,
		//   though I'm not sure under what circumstances this might be true)
		EntryLocation:   lowPC,
		Parameters:      params,
		ReturnLocations: returnLocations,
	}, nil
}

func (i *inspectionState) getParameterLocationAtPC(parameterDIE *dwarf.Entry, pc uint64) (ParameterMetadata, error) {
	typeOffset, ok := parameterDIE.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return ParameterMetadata{}, fmt.Errorf("no type offset attribute in parameter entry")
	}

	// Find the location field on the entry
	locationField := parameterDIE.AttrField(dwarf.AttrLocation)
	if locationField == nil {
		return ParameterMetadata{}, fmt.Errorf("no location field in parameter entry")
	}

	typ, err := i.dwarfInspectionState.typeFinder.FindTypeByOffset(typeOffset)
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

		loclistEntry, err := i.getLoclistEntry(locationAsLocListIndex, pc)
		if err != nil {
			return ParameterMetadata{}, fmt.Errorf("could not find loclist entry at %#x for PC %#x: %w", locationAsLocListIndex, pc, err)
		}
		locationExpression = loclistEntry.Instr
	default:
		return ParameterMetadata{}, fmt.Errorf("unexpected field class on formal parameter's location attribute: locationField=%#v", locationField)
	}

	totalSize := typ.Size()
	pieces, err := locexpr.Exec(locationExpression, totalSize, int(i.arch.PointerSize()))
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

// Note that this may not behave well with panics or defer statements.
// See the following links for more context:
// - https://github.com/go-delve/delve/pull/2704/files#diff-fb7b7a020e32bf8bf477c052ac2d2857e7e587478be6039aebc7135c658417b2R769
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L86-L95
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/breakpoints.go#L374
func (i *inspectionState) findReturnLocations(lowPC, highPC uint64) ([]uint64, error) {
	textSection := i.elfFile.Section(".text")
	if textSection == nil {
		return nil, fmt.Errorf("no %q section found in binary file", ".text")
	}

	switch i.arch {
	case GoArchX86_64:
		return asmscan.ScanFunction(textSection, lowPC, highPC, asmscan.FindX86_64ReturnInstructions)
	case GoArchARM64:
		return asmscan.ScanFunction(textSection, lowPC, highPC, asmscan.FindARM64ReturnInstructions)
	default:
		return nil, fmt.Errorf("unsupported architecture %q", i.arch)
	}
}

func (i *inspectionState) findFunctionsUsingGoSymTab() ([]FunctionMetadata, error) {
	symbolTable, err := i.parseSymbolTable()
	if err != nil {
		return nil, err
	}

	functionMetadata := []FunctionMetadata{}
	for _, config := range i.config.Functions {
		f := symbolTable.LookupFunc(config.Name)
		if f == nil {
			return nil, fmt.Errorf("could not find func %q in symbol table", config.Name)
		}

		lowPC := f.Entry
		highPC := f.End

		var returnLocations []uint64
		if config.IncludeReturnLocations {
			locations, err := i.findReturnLocations(lowPC, highPC)
			if err != nil {
				return nil, fmt.Errorf("could not find return locations for function %q: %w", config.Name, err)
			}

			returnLocations = locations
		}

		// Parameter metadata cannot be determined without DWARF symbols,
		// so this is as much metadata as we can extract.
		functionMetadata = append(functionMetadata, FunctionMetadata{
			Name:            config.Name,
			EntryLocation:   lowPC,
			Parameters:      nil,
			ReturnLocations: returnLocations,
		})
	}

	return functionMetadata, nil
}

func (i *inspectionState) parseSymbolTable() (*gosym.Table, error) {
	pclntabSection := i.elfFile.Section(".gopclntab")
	if pclntabSection == nil {
		return nil, fmt.Errorf("no %q section found in binary file", ".gopclntab")
	}

	pclntabData, err := pclntabSection.Data()
	if err != nil {
		return nil, fmt.Errorf("error while reading pclntab data from binary: %w", err)
	}

	symtabSection := i.elfFile.Section(".gosymtab")
	if symtabSection == nil {
		return nil, fmt.Errorf("no %q section found in binary file", ".gosymtab")
	}

	symtabData, err := symtabSection.Data()
	if err != nil {
		return nil, fmt.Errorf("error while reading symtab data from binary: %w", err)
	}

	textSection := i.elfFile.Section(".text")
	if textSection == nil {
		return nil, fmt.Errorf("no %q section found in binary file", ".text")
	}

	lineTable := gosym.NewLineTable(pclntabData, textSection.Addr)
	table, err := gosym.NewTable(symtabData, lineTable)
	if err != nil {
		return nil, fmt.Errorf("error while parsing symbol table: %w", err)
	}

	return table, nil
}

func (i *inspectionState) findStructOffsets() ([]StructOffset, error) {
	if i.dwarfInspectionState == nil {
		// The binary has been stripped; we won't be able to find the struct offsets.
		return nil, nil
	}

	structOffsets := []StructOffset{}

	for _, config := range i.config.StructOffsets {
		offset, err := i.dwarfInspectionState.typeFinder.FindStructFieldOffset(config.StructName, config.FieldName)
		if err != nil {
			return nil, fmt.Errorf("could not find offset of %q . %q: %w", config.StructName, config.FieldName, err)
		}

		structOffsets = append(structOffsets, StructOffset{
			StructName: config.StructName,
			FieldName:  config.FieldName,
			Offset:     offset,
		})
	}

	return structOffsets, nil
}

// getLoclistEntry returns the loclist entry in the loclist
// starting at offset, for address pc.
// Adapted from github.com/go-delve/delve/pkg/proc.(*BinaryInfo).loclistEntry
func (i *inspectionState) getLoclistEntry(offset int64, pc uint64) (*loclist.Entry, error) {
	var base uint64
	compileUnit := i.dwarfInspectionState.compileUnits.FindCompileUnit(pc)
	if compileUnit != nil {
		base = compileUnit.LowPC
	}

	var loclist loclist.Reader = i.dwarfInspectionState.loclist2
	var debugAddr *godwarf.DebugAddr
	if compileUnit != nil && compileUnit.Version >= 5 && i.dwarfInspectionState.loclist5 != nil {
		loclist = i.dwarfInspectionState.loclist5
		if addrBase, ok := compileUnit.Entry.Val(dwarf.AttrAddrBase).(int64); ok {
			debugAddr = i.dwarfInspectionState.debugAddr.GetSubsection(uint64(addrBase))
		}
	}

	if loclist.Empty() {
		return nil, fmt.Errorf("no loclist found for the given program counter")
	}

	// Use 0x0 as the static base
	var staticBase uint64 = 0x0
	e, err := loclist.Find(int(offset), staticBase, base, pc, debugAddr)
	if err != nil {
		return nil, fmt.Errorf("error reading loclist section: %w", err)
	}
	if e != nil {
		return e, nil
	}

	return nil, fmt.Errorf("no loclist entry found")
}
