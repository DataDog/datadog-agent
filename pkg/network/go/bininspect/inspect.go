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
	"github.com/DataDog/datadog-agent/pkg/network/go/goid"
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

	// Scan for goroutine metadata, functions, struct offsets, and static itab entries
	goroutineIDMetadata, err := i.getGoroutineIDMetadata()
	if err != nil {
		return nil, err
	}
	functions, err := i.findFunctions()
	if err != nil {
		return nil, err
	}
	structOffsets, err := i.findStructOffsets()
	if err != nil {
		return nil, err
	}
	staticItabEntries, err := i.findStaticItabEntries()
	if err != nil {
		return nil, err
	}

	return &Result{
		Arch:                 i.arch,
		ABI:                  i.abi,
		GoVersion:            i.goVersion,
		IncludesDebugSymbols: i.dwarfInspectionState != nil,
		GoroutineIDMetadata:  goroutineIDMetadata,
		Functions:            functions,
		StructOffsets:        structOffsets,
		StaticItabEntries:    staticItabEntries,
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

// findStaticItabEntries scans the ELF symbols in the binary
// to find the desired itab index values for the given struct,interface pairs.
// This is used at runtime by Go to identify `interface{}` values.
func (i *inspectionState) findStaticItabEntries() ([]StaticItabEntry, error) {
	if len(i.symbols) == 0 {
		// The binary has no symbols; we won't be able to find any static itab entries.
		return nil, nil
	}

	staticItabEntries := []StaticItabEntry{}

	for _, config := range i.config.StaticItabEntries {
		itabSymbol := i.getELFSymbol(fmt.Sprintf("go.itab.%s,%s", config.StructName, config.InterfaceName))
		if itabSymbol == nil {
			return nil, fmt.Errorf("could not find %q <> %q's itab ELF symbol", config.StructName, config.InterfaceName)
		}

		staticItabEntries = append(staticItabEntries, StaticItabEntry{
			StructName:    config.StructName,
			InterfaceName: config.InterfaceName,
			EntryIndex:    itabSymbol.Value,
		})
	}

	return staticItabEntries, nil
}

// getELFSymbol searches for a symbol in the binary with a matching name.
// If no such symbol is found, returns `nil`.
func (i *inspectionState) getELFSymbol(name string) *elf.Symbol {
	for _, symbol := range i.symbols {
		if symbol.Name == name {
			s := symbol
			return &s
		}
	}
	return nil
}

// getGoroutineIDMetadata collects enough metadata about the binary
// to be able to reliably determine the goroutine ID from the context of an eBPF uprobe.
// This is accomplished by finding the offset of the `goid` field in the `runtime.g` struct,
// which is the goroutine context struct.
//
// A pointer to this struct is always stored in thread-local-strorage (TLS),
// but it might also be in a dedicated register (which is faster to access),
// depending on the ABI and architecture:
// 1. If it has a dedicated register, this function gives the register number
// 2. Otherwise, this function finds the offset in TLS that the pointer exists at.
//
// See:
// - https://go.googlesource.com/proposal/+/master/design/40724-register-calling.md#go_s-current-stack_based-abi
// - https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
// - https://github.com/golang/go/blob/61011de1af0bc6ab286c4722632719d3da2cf746/src/runtime/runtime2.go#L403
// - https://github.com/golang/go/blob/61011de1af0bc6ab286c4722632719d3da2cf746/src/runtime/runtime2.go#L436
func (i *inspectionState) getGoroutineIDMetadata() (GoroutineIDMetadata, error) {
	goroutineIDOffset, err := i.getGoroutineIDOffset()
	if err != nil {
		return GoroutineIDMetadata{}, fmt.Errorf("could not find goroutine ID offset in goroutine context struct: %w", err)
	}

	// On x86_64 and the register ABI, the runtime.g pointer (current goroutine context struct) is stored in a register (r14):
	// https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
	// Additionally, on all architectures other than x86_64 and x86 (in all Go versions),
	// the runtime.g pointer is stored on a register.
	// On x86_64 pre-Go 1.17 and on x86 (in all Go versions),
	// the runtime.g pointer is stored in the thread's thread-local-storage.
	var runtimeGInRegister bool
	if i.arch == GoArchX86_64 {
		runtimeGInRegister = i.abi == GoABIRegister
	} else {
		runtimeGInRegister = true
	}

	var runtimeGRegister int
	var runtimeGTLSAddrOffset uint64
	if runtimeGInRegister {
		switch i.arch {
		case GoArchX86_64:
			// https://go.googlesource.com/go/+/refs/heads/dev.regabi/src/cmd/compile/internal-abi.md#amd64-architecture
			runtimeGRegister = 14
		case GoArchARM64:
			// https://golang.org/doc/asm#arm64
			// TODO make sure this is valid
			runtimeGRegister = 27
		}
	} else {
		offset, err := i.getRuntimeGAddrTLSOffset()
		if err != nil {
			return GoroutineIDMetadata{}, fmt.Errorf("could not get offset of runtime.g offset in TLS: %w", err)
		}

		runtimeGTLSAddrOffset = offset
	}

	return GoroutineIDMetadata{
		GoroutineIDOffset:     goroutineIDOffset,
		RuntimeGInRegister:    runtimeGInRegister,
		RuntimeGRegister:      runtimeGRegister,
		RuntimeGTLSAddrOffset: runtimeGTLSAddrOffset,
	}, nil
}

// getRuntimeGAddrTLSOffset determines what the offset
// of the `runtime.g` value is in thread-local-storage.
//
// This implementation is based on github.com/go-delve/delve/pkg/proc.(*BinaryInfo).setGStructOffsetElf:
// - https://github.com/go-delve/delve/blob/75bbbbb60cecda0d65c63de7ae8cb8b8412d6fc3/pkg/proc/bininfo.go#L1413
// which is licensed under MIT.
func (i *inspectionState) getRuntimeGAddrTLSOffset() (uint64, error) {
	// This is a bit arcane. Essentially:
	// - If the program is pure Go, it can do whatever it wants, and puts the G
	//   pointer at %fs-8 on 64 bit.
	// - %Gs is the index of private storage in GDT on 32 bit, and puts the G
	//   pointer at -4(tls).
	// - Otherwise, Go asks the external linker to place the G pointer by
	//   emitting runtime.tlsg, a TLS symbol, which is relocated to the chosen
	//   offset in libc's TLS block.
	// - On ARM64 (but really, any architecture other than i386 and 86x64) the
	//   offset is calculate using runtime.tls_g and the formula is different.

	var tls *elf.Prog
	for _, prog := range i.elfFile.Progs {
		if prog.Type == elf.PT_TLS {
			tls = prog
			break
		}
	}

	switch i.arch {
	case GoArchX86_64:
		tlsg := i.getELFSymbol("runtime.tlsg")
		if tlsg == nil || tls == nil {
			return ^uint64(i.arch.PointerSize()) + 1, nil //-ptrSize
		}

		// According to https://reviews.llvm.org/D61824, linkers must pad the actual
		// size of the TLS segment to ensure that (tlsoffset%align) == (vaddr%align).
		// This formula, copied from the lld code, matches that.
		// https://github.com/llvm-mirror/lld/blob/9aef969544981d76bea8e4d1961d3a6980980ef9/ELF/InputSection.cpp#L643
		memsz := tls.Memsz + (-tls.Vaddr-tls.Memsz)&(tls.Align-1)

		// The TLS register points to the end of the TLS block, which is
		// tls.Memsz long. runtime.tlsg is an offset from the beginning of that block.
		return ^(memsz) + 1 + tlsg.Value, nil // -tls.Memsz + tlsg.Value

	case GoArchARM64:
		tlsg := i.getELFSymbol("runtime.tls_g")
		if tlsg == nil || tls == nil {
			return 2 * uint64(i.arch.PointerSize()), nil
		}

		return tlsg.Value + uint64(i.arch.PointerSize()*2) + ((tls.Vaddr - uint64(i.arch.PointerSize()*2)) & (tls.Align - 1)), nil

	default:
		return 0, fmt.Errorf("binary is for unsupported architecture")
	}
}

func (i *inspectionState) getGoroutineIDOffset() (uint64, error) {
	if i.dwarfInspectionState == nil {
		// The binary has been stripped; we won't be able to find the struct's offset.
		// Fall back to a static lookup table
		return goid.GetGoroutineIDOffset(i.goVersion, string(i.arch))
	}

	goroutineIDOffset, err := i.dwarfInspectionState.typeFinder.FindStructFieldOffset("runtime.g", "goid")
	if err != nil {
		return 0, err
	}

	return goroutineIDOffset, nil
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
