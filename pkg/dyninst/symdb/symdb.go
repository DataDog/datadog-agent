// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"debug/dwarf"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"

	dwarf2 "github.com/DataDog/datadog-agent/pkg/dyninst/dwarf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
)

// Symbols models the symbols from a binary that get exported to SymDB.
type Symbols struct {
	Packages []Package
}

// Package describes a Go package for SymDB.
type Package struct {
	// The fully-qualified name of the package (e.g. "github.com/foo/bar").
	Name string
	// The functions in this package. Note that methods (i.e. functions with
	// receivers) are not represented here; they are represented on their
	// receiver Type.
	Functions []Function
	Types     []Type
}

// Type describes a Go type for SymDB.
type Type struct {
	Name    string
	Methods []Function
	// Fields contains the struct fields. Empty if the type is not a struct.
	Fields []Field

	// NOTE: Go debug info doesn't have file:line info for types. For structs,
	// we could perhaps use the file:line info of the first method.
}

// Field describes a field of a struct type.
type Field struct {
	Name string
	// The fully-qualified name of the field's type. Pointer types start with a
	// "*" (e.g. *main.bigStruct).
	Type string
}

// Function models a Go function or method. It is represented in SymDB as a
// scope.
type Function struct {
	// The function's unqualified name.
	Name string
	// The function's fully-qualified name, including module, package and
	// receiver. When creating probes based on a function name coming from
	// SymDB, the qualified name is how the function is identified to the
	// prober.
	QualifiedName string
	File          string
	// The function itself represents a lexical block, with variables and
	// sub-scopes.
	Scope
}

func (f Function) empty() bool {
	return f.Name == ""
}

// Scope represents a function or another lexical block.
type Scope struct {
	StartLine int
	EndLine   int
	Scopes    []Scope
	Variables []Variable
}

// Variable represents a variable or function argument.
type Variable struct {
	Name     string
	TypeName string
	// FunctionArgument indicates if this variable is a function argument, as
	// opposed to a local variable. Function arguments can only appear inside
	// Function, never inside Scope.
	FunctionArgument bool
	// The line on which the variable was declared in the source code. Note that
	// the variable is not necessarily actually available to be captured by the
	// debugger from that line to the end of the lexical block; see
	// AvailableLineRanges.
	DeclLine            int
	AvailableLineRanges []LineRange
}

// LineRange represents a range of source lines, inclusive of both ends.
type LineRange [2]int

// SymDBBuilder walks the DWARF data for a binary, extracting symbols in the
// SymDB format.
// nolint:revive  // ignore stutter rule
type SymDBBuilder struct {
	// The DWARF data to extract symbols from.
	dwarfData *dwarf.Data
	// The Go symbol table for the binary, used to resolve PC addresses to
	// functions.
	sym *gosym.GoSymbolTable
	// The location list reader used to read location lists for variables.
	loclistReader *loclist.Reader
	// The size of pointers for the binary's architecture, in bytes.
	pointerSize int

	// The compile unit currently being processed by explore* functions.
	currentCompileUnit        *dwarf.Entry
	filesInCurrentCompileUnit []string

	types typesCollection

	// Stack of blocks currently being explored. Variable location lists can
	// make references to the current block and its PC ranges; they will look at
	// the top of this stack.
	blockStack []codeBlock

	// cleanupFuncs holds functions to be called on Close() to clean up resources.
	cleanupFuncs []func()
}

type typesCollection struct {
	// typesCache will accumulate types as we look them up to resolve variables
	// and functions. The cache is indexed by DWARF offset.
	typesCache *dwarfutils.TypeFinder

	// Map from the type's name, as it appears in DWARF, to the type info. Only
	// some of the types from typesCache are to be exported to SymDB and thus
	// are present here -- we ignore some types we don't support, and we
	// dereference pointers. The map holds pointers to allow the Type's to
	// change over time (in particular, they accumulate methods).
	types map[string]*Type
	// Map from package qualified name to the types in that package.
	packages map[string][]*Type
}

var (
	// The parsing goes as follows:
	// - optionally one or more starting '*', for pointer types. We discard these.
	// - consume eagerly up to the last slash, if any. This is part of the
	// package path.
	// - after the last slash, consume up to the next dot. This completes the
	// package name.
	parsePkgFromTypeNameRE = regexp.MustCompile(`^(\*)*(?P<pkg>(.*\/)?[^.]*)\.`)
	typePkgIdx             = parsePkgFromTypeNameRE.SubexpIndex("pkg")
)

func (c *typesCollection) resolveType(offset dwarf.Offset) (typeInfo, error) {
	typ, err := c.typesCache.FindTypeByOffset(offset)
	if err != nil {
		return typeInfo{}, err
	}
	typeName := typ.Common().Name
	size := typ.Common().Size()

	// Unwrap pointer types and typedefs.
	for {
		if t, ok := typ.(*godwarf.PtrType); ok {
			typ = t.Type
			continue
		}
		if t, ok := typ.(*godwarf.TypedefType); ok {
			typ = t.Type
			continue
		}
		break
	}

	if err := c.addType(typ); err != nil {
		return typeInfo{}, err
	}

	return typeInfo{
		name: typeName,
		size: int(size),
	}, nil
}

func (c *typesCollection) getType(name string) *Type {
	return c.types[name]
}

// addType adds a type to the collection if it is not already present.
// Unsupported types are ignored and no error is returned.
func (c *typesCollection) addType(t godwarf.Type) error {
	name := t.Common().Name
	// Check if the type is already present.
	if _, ok := c.types[name]; ok {
		return nil
	}

	// We don't support parametric types (generics).
	if _, ok := t.(*godwarf.ParametricType); ok {
		return nil
	}

	// Assert that we were not given a pointer type.
	if _, ok := t.(*godwarf.PtrType); ok {
		return fmt.Errorf("ptr type expected to have been unwrapped: %s", name)
	}
	if strings.HasPrefix(name, "*") {
		return fmt.Errorf("type name for non-pointer unexpectedly starting with '*': %s", name)
	}

	// Skip anonymous types, generic types, array types and structs
	// corresponding to slices.
	if strings.ContainsAny(name, "{<[") {
		return nil
	}

	// Figure out the type's package.
	groups := parsePkgFromTypeNameRE.FindStringSubmatch(name)
	if groups == nil {
		// Base types like "int" don't have a package. We don't care about these
		// types anyway.
		return nil
	}
	pkg := groups[typePkgIdx]
	if pkg == "" {
		return fmt.Errorf("failed to parse package from type %s (type: %s)", name, t)
	}
	typ := &Type{
		Name:   name,
		Fields: nil,
		// Methods will be populated later, as we discover them in DWARF.
		Methods: nil,
	}
	if s, isStruct := t.(*godwarf.StructType); isStruct {
		for _, field := range s.Field {
			typ.Fields = append(typ.Fields, Field{
				Name: field.Name,
				Type: field.Type.Common().Name,
			})
		}
	}

	c.types[name] = typ
	c.packages[pkg] = append(c.packages[pkg], typ)
	return nil
}

// codeBlock abstracts LexicalBlocks and Subprograms. They all correspond to a
// list of PC ranges.
type codeBlock interface {
	resolvePCRanges(dwarfData *dwarf.Data) ([]dwarfutil.PCRange, error)
}

// dwarfBlock is the implementation of codeBlock for lexical blocks.
type dwarfBlock struct {
	// The DWARF entry for the lexical block.
	entry    *dwarf.Entry
	pcRanges []dwarfutil.PCRange
	// pcRangesResolved is set if pcRanges has been already calculated.
	pcRangesResolved bool
}

// dwarfBlock implements codeBlock.
var _ codeBlock = (*dwarfBlock)(nil)

// resolvePCRanges parses and memoizes the ranges attribute of the dwarf entry.
func (d *dwarfBlock) resolvePCRanges(dwarfData *dwarf.Data) ([]dwarfutil.PCRange, error) {
	if d.pcRangesResolved {
		return d.pcRanges, nil
	}

	ranges, err := dwarfData.Ranges(d.entry)
	if err != nil {
		return nil, err
	}
	d.pcRanges = ranges
	d.pcRangesResolved = true
	return d.pcRanges, nil
}

// resolveLines computes the start and end lines of the lexical block by
// combining PC ranges from DWARF with pclinetab data. Returns [0,0] if the
// block has no address ranges, and thus no lines.
func (d *dwarfBlock) resolveLines(dwarfData *dwarf.Data, pcIt *gosym.PCIterator) (LineRange, error) {
	// Find the start and end lines of the block by going through the address
	// ranges, resolving them to lines, and selecting the minimum and maximum.
	pcRanges, err := d.resolvePCRanges(dwarfData)
	if err != nil {
		return LineRange{}, err
	}
	if len(pcRanges) == 0 {
		return LineRange{}, nil
	}
	startLine := math.MaxInt
	endLine := 0
	for _, r := range pcRanges {
		lineRange, ok := pcRangeToLines(r, pcIt)
		if !ok {
			continue
		}
		rangeStartLine := lineRange[0]
		rangeEndLine := lineRange[1]
		if rangeStartLine < startLine {
			startLine = rangeStartLine
		}
		if rangeEndLine > endLine {
			endLine = rangeEndLine
		}
	}
	return LineRange{startLine, endLine}, nil
}

// subprogBlock is the implementation of codeBlock for subprograms. The block's
// PC ranges are resolved trivially to a single range.
type subprogBlock struct {
	lowpc  uint64
	highpc uint64
}

// subprogBlock implements codeBlock.
var _ codeBlock = subprogBlock{}

// resolvePCRanges implements the codeBlock interface.
func (s subprogBlock) resolvePCRanges(*dwarf.Data) ([]dwarfutil.PCRange, error) {
	return []dwarfutil.PCRange{{s.lowpc, s.highpc}}, nil
}

// NewSymDBBuilder creates a new SymDBBuilder for the given ELF file. The
// SymDBBuilder takes ownership of the ELF file.
// Close() needs to be called .
func NewSymDBBuilder(obj *object.ElfFile) (*SymDBBuilder, error) {
	moduledata, err := object.ParseModuleData(obj.Underlying)
	if err != nil {
		return nil, err
	}
	goVersion, err := object.ReadGoVersion(obj.Underlying)
	if err != nil {
		return nil, err
	}

	goDebugSections, err := moduledata.GoDebugSections(obj.Underlying)
	if err != nil {
		return nil, err
	}

	// goDebugSections cannot be Close()'ed while symTable is in use. Ownership of
	// goDebugSections is transferred to the SymDBBuilder.

	symTable, err := gosym.ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	if err != nil {
		return nil, err
	}

	b := &SymDBBuilder{
		dwarfData:     obj.DwarfData(),
		sym:           symTable,
		loclistReader: obj.LoclistReader(),
		pointerSize:   int(obj.PointerSize()),
		cleanupFuncs:  []func(){func() { _ = goDebugSections.Close() }, func() { _ = obj.Close() }},
	}
	b.types.typesCache = dwarfutils.NewTypeFinder(obj.DwarfData())
	b.types.packages = make(map[string][]*Type)
	b.types.types = make(map[string]*Type)
	return b, nil
}

// Close frees resources associated with the builder.
func (b *SymDBBuilder) Close() {
	for _, f := range b.cleanupFuncs {
		f()
	}
}

// ExtractSymbols walks the DWARF data and accumulates the symbols to send to
// SymDB.
func (b *SymDBBuilder) ExtractSymbols() (Symbols, error) {
	var res Symbols
	entryReader := b.dwarfData.Reader()

	// Recognize compile units, which are the top-level entries in the DWARF
	// data corresponding to Go packages.
	for entry, err := entryReader.Next(); entry != nil; entry, err = entryReader.Next() {
		if err != nil {
			return Symbols{}, err
		}

		if entry.Tag != dwarf.TagCompileUnit {
			entryReader.SkipChildren()
			continue
		}

		pkg, err := b.exploreCompileUnit(entry, entryReader)
		if err != nil {
			return Symbols{}, err
		}
		if pkg.Name != "" {
			res.Packages = append(res.Packages, pkg)
		}
	}

	return res, nil
}

func (b *SymDBBuilder) currentBlock() codeBlock {
	return b.blockStack[len(b.blockStack)-1]
}

func (b *SymDBBuilder) pushBlock(block codeBlock) {
	b.blockStack = append(b.blockStack, block)
}

func (b *SymDBBuilder) popBlock() {
	if len(b.blockStack) == 0 {
		panic("popBlock called on empty block stack")
	}
	b.blockStack = b.blockStack[:len(b.blockStack)-1]
}

type typeInfo struct {
	// The type's name, including a leading '*' for pointer types.
	name string
	// The size of the type, in bytes.
	size int
}

// exploreCompileUnit processes a compile unit entry (entry's tag is
// TagCompileUnit).
//
// Returns a zero value if the compile unit is not a Go package.
func (b *SymDBBuilder) exploreCompileUnit(entry *dwarf.Entry, reader *dwarf.Reader) (Package, error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return Package{}, fmt.Errorf("expected TagCompileUnit, got %s", entry.Tag)
	}

	// Filter out non-Go compile units.
	langField := entry.AttrField(dwarf.AttrLanguage)
	if langField == nil {
		reader.SkipChildren()
		return Package{}, nil
	}
	langCode, ok := langField.Val.(int64)
	if !ok || langCode != dwarf2.DW_LANG_Go {
		reader.SkipChildren()
		return Package{}, nil
	}

	// Some compile units are empty; we ignore them (for example, compile units
	// corresponding to assembly code).
	if !entry.Children {
		return Package{}, nil
	}

	b.currentCompileUnit = entry
	defer func() {
		b.currentCompileUnit = nil
		b.filesInCurrentCompileUnit = nil
	}()

	var res Package
	name, ok := entry.Val(dwarf.AttrName).(string)
	if !ok {
		return Package{}, errors.New("compile unit without name")
	}
	res.Name = name

	cuLineReader, err := b.dwarfData.LineReader(entry)
	if err != nil {
		return Package{}, fmt.Errorf("could not get file line reader for compile unit %s: %w", name, err)
	}
	var files []string
	if cuLineReader != nil {
		files = make([]string, 0, len(cuLineReader.Files()))
		for i, file := range cuLineReader.Files() {
			if file == nil {
				// Each compile unit starts with a nil entry at position 0; 0 is
				// used as a sentinel by file references to indicate that the
				// file is not known.
				if i != 0 {
					return Package{}, fmt.Errorf(
						"compile unit %s has invalid nil file entry at index %d", name, i)
				}
				files = append(files, "")
				continue
			}
			files = append(files, file.Name)
		}
	}
	b.filesInCurrentCompileUnit = files

	// We recognize subprograms and types.
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return Package{}, err
		}
		if dwarfutil.IsEntryNull(child) {
			break // End of children for this compile unit.
		}

		switch child.Tag {
		case dwarf.TagSubprogram:
			function, err := b.exploreSubprogram(child, reader)
			if err != nil {
				return Package{}, err
			}
			if !function.empty() {
				res.Functions = append(res.Functions, function)
			}
		default:
			reader.SkipChildren()
		}
	}

	// Copy the types in this package from b.types. By now these types have all
	// their methods populated, since their methods have to be defined in this
	// compile unit.
	res.Types = make([]Type, len(b.types.packages[res.Name]))
	for i, t := range b.types.packages[res.Name] {
		res.Types[i] = *t
	}
	return res, nil
}

// exploreSubprogram processes a subprogram entry, corresponding to a Go
// function (entry's tag is TagSubprogram). It accumulates the function's
// lexical blocks and variables. If the function is free-standing (not a
// method), the accumulated info is returned. If the function is a method (i.e.
// has a receiver), it is appended to the types methods in the types collection
// and NOT returned.
//
// Returns a zero Function if the kind of function is currently unsupported and
// should be ignored.
//
// If no error is returned, the reader is positioned after the subprogram's
// children. If an error is returned, the reader is left at an undefined
// position inside the program.
func (b *SymDBBuilder) exploreSubprogram(
	subprogEntry *dwarf.Entry, reader *dwarf.Reader,
) (Function, error) {
	// When returning early, we need to consume all the children of the subprogram
	// entry. Note that this utility can only be used before the reader is advanced.
	earlyExit := func() (Function, error) {
		reader.SkipChildren()
		return Function{}, nil
	}

	if subprogEntry.Tag != dwarf.TagSubprogram {
		return Function{}, fmt.Errorf("expected TagSubprogram, got %s", subprogEntry.Tag)
	}

	// Ignore auto-generated functions, marked as trampolines. There are a few
	// cases where the compiler generates such functions:
	// - methods with value receivers that are called through an interface that
	// boxes a pointer value.
	// - "deferwrap" functions
	if subprogEntry.AttrField(dwarf.AttrTrampoline) != nil {
		return earlyExit()
	}

	// If this is a "concrete-out-of-line" instance of a subprogram, the
	// variables inside it will reference the abstract origin. We don't handle
	// that at the moment, so skip these functions.
	// TODO: handle concrete-out-of-line instances of functions.
	if subprogEntry.AttrField(dwarf.AttrAbstractOrigin) != nil {
		return earlyExit()
	}

	inline, ok := subprogEntry.Val(dwarf.AttrInline).(int64)
	if ok && inline == dwarf2.DW_INL_inlined {
		// This function is inlined; its variables don't have location lists here;
		// instead, each inlined instance will have them. Nothing to do.
		//
		// Note that, if the function is not *always* inlined, there will also be a
		// "concrete out-of-line" instance of this subprogram, which will have
		// AttrAbstractOrigin pointing here.
		return earlyExit()
	}

	funcQualifiedName, ok := subprogEntry.Val(dwarf.AttrName).(string)
	if !ok {
		return Function{}, fmt.Errorf("subprogram without name at 0x%x (%s)", subprogEntry.Offset, subprogEntry.Tag)
	}

	fileIdx, ok := subprogEntry.Val(dwarf.AttrDeclFile).(int64)
	if !ok || fileIdx == 0 { // fileIdx == 0 means unknown file, as per DWARF spec
		// TODO: log if this ever happens. I haven't seen it.
		return earlyExit()
	}
	if fileIdx < 0 || int(fileIdx) >= len(b.filesInCurrentCompileUnit) {
		return Function{}, fmt.Errorf(
			"subprogram %s has invalid file index %d, expected in range [0, %d)",
			funcQualifiedName, fileIdx, len(b.filesInCurrentCompileUnit),
		)
	}
	fileName := b.filesInCurrentCompileUnit[fileIdx]

	// There are some funky auto-generated functions that we can't deal with
	// because we can't parse their names (e.g.
	// type:.eq.sync/atomic.Pointer[os.dirInfo]). Skip everything
	// auto-generated.
	if fileName == "<autogenerated>" {
		return earlyExit()
	}

	parseRes, err := parseFuncName(funcQualifiedName)
	if err != nil {
		// Note that we do not return an error here; we simply ignore the
		// function so that we don't choke on weird function names that we
		// haven't anticipated.
		// TODO: log the error, but don't spam the logs.
		return earlyExit()
	}
	if parseRes.failureReason != parseFuncNameFailureReasonUndefined {
		return earlyExit()
	}
	funcName := parseRes.funcName

	lowpc, ok := subprogEntry.Val(dwarf.AttrLowpc).(uint64)
	if !ok {
		return Function{}, fmt.Errorf("subprogram without lowpc: %s", funcQualifiedName)
	}
	highpc, ok := subprogEntry.Val(dwarf.AttrHighpc).(uint64)
	if !ok {
		return Function{}, errors.New("subprogram without highpc")
	}

	// Do a pass through the subprogram's DWARF to find all the inlined
	// subroutines. We need to know the PC ranges of inlined subroutines to figure
	// out the source lines of this function.
	//
	// TODO: do away with this separate pass; defer resolving any PCs until
	// after we parse the whole subprogram, at which point we can also resolve
	// PCs in order, which is more efficient.
	inlinedPcRanges, err := dwarfutil.ExploreInlinedPcRangesInSubprogram(reader, b.dwarfData)
	if err != nil {
		return Function{}, fmt.Errorf("error exploring inlined subroutines in function %s: %w", funcQualifiedName, err)
	}
	// Reset the reader to the start of the subprogram.
	reader.Seek(subprogEntry.Offset)
	// Skip the subprogram entry, to position the reader where it was before
	// ExploreInlinedPcRangesInSubprogram.
	_, err = reader.Next()
	if err != nil {
		return Function{}, err
	}

	// Figure out the function's line range. The start line corresponds to the
	// function's start PC. For figuring out the end line, we iterate through the
	// function's PC-to-line mapping and keep track of the maximum line number
	// (the function's highpc does not map to the function's end in the source
	// code for Go functions; the last instructions have to do with stack growth
	// and map to the function's start).
	f := b.sym.PCToFunction(lowpc)
	if f == nil {
		return Function{}, fmt.Errorf("failed to resolve function for subprogram %s at PC 0x%x", funcQualifiedName, lowpc)
	}
	pcIt, err := f.PCIterator(inlinedPcRanges)
	if err != nil {
		return Function{}, fmt.Errorf("failed to create PC iterator for function %s: %w", funcQualifiedName, err)
	}
	firstLine := uint32(0)
	maxLine := uint32(0)
	for {
		line, ok := pcIt.Next()
		if !ok {
			break
		}
		if firstLine == 0 {
			firstLine = line.Line
		}
		if line.Line > maxLine {
			maxLine = line.Line
		}
	}
	pcIt.Reset()

	// From now on, location lists that reference the current block will
	// reference this function.
	b.pushBlock(subprogBlock{lowpc: lowpc, highpc: highpc})
	defer b.popBlock()

	// Explore the function's body. Besides collecting the information that we
	// want to export to SymDB, this also has the side effect of resolving the
	// types of all the function arguments; in particular, we rely below on the
	// type of the receiver having been resolved.
	inner, err := b.exploreCode(reader, &pcIt)
	if err != nil {
		return Function{}, err
	}

	res := Function{
		Name:          funcName.Name,
		QualifiedName: funcQualifiedName,
		File:          fileName,
		Scope: Scope{
			StartLine: int(firstLine),
			EndLine:   int(maxLine),
			Variables: inner.vars,
			Scopes:    inner.scopes,
		},
	}

	// If this function is a method (i.e. it has a receiver), we add it to the
	// respective type instead of returning it as a stand-alone function.
	if funcName.Type != "" {
		typeQualifiedName := funcName.Package + "." + funcName.Type
		// We expect the type of the receiver to have been populated by the
		// exploreCode() call above.
		typ := b.types.getType(typeQualifiedName)
		if typ == nil {
			return Function{}, fmt.Errorf(
				"%s is a method of type %s, but that type is missing from the cache. DWARF offset: 0x%x",
				funcQualifiedName, typeQualifiedName, subprogEntry.Offset,
			)
		}
		typ.Methods = append(typ.Methods, res)
		// We don't return a Function for methods.
		return Function{}, nil
	}
	return res, nil
}

func (b *SymDBBuilder) exploreInlinedSubroutine(inlinedEntry *dwarf.Entry, reader *dwarf.Reader) error {
	if inlinedEntry.Tag != dwarf.TagInlinedSubroutine {
		return fmt.Errorf("expected TagInlinedSubroutine, got %s", inlinedEntry.Tag)
	}
	// TODO: accumulate variable availability data and join it with the other
	// inlined and out-of-line instances.
	// TODO: Resolve the declaration file of the inlined function, if we haven't
	// already done it for another inlined instance of this function. Go doesn't
	// annotate the DWARF abstract function definitions with the declaration file,
	// so we have to figure it out from the line programs of one of the inlined
	// instances.
	reader.SkipChildren()
	return nil
}

// exploreLexicalBlock processes a lexical block entry. If the block contains
// any variables, it returns one Scope with those variables and any sub-blocks.
// If the block does not contain any variables, it returns any sub-blocks that
// do contain variables (if any).
func (b *SymDBBuilder) exploreLexicalBlock(
	blockEntry *dwarf.Entry, reader *dwarf.Reader, pcIt *gosym.PCIterator,
) ([]Scope, error) {
	if blockEntry.Tag != dwarf.TagLexDwarfBlock {
		return nil, fmt.Errorf("expected TagLexDwarfBlock, got %s", blockEntry.Tag)
	}
	currentBlock := &dwarfBlock{entry: blockEntry}
	// From now on, location lists that reference the current block will
	// reference this block.
	b.pushBlock(currentBlock)
	defer b.popBlock()

	inner, err := b.exploreCode(reader, pcIt)
	if err != nil {
		return nil, fmt.Errorf("error exploring code in lexical block: %w", err)
	}

	// If the block has any variables, then we create a scope for it. If it
	// doesn't, then inner scopes (if any), are returned directly, to be added
	// as direct children of the caller's block.
	if len(inner.vars) != 0 {
		blockLineRange, err := currentBlock.resolveLines(b.dwarfData, pcIt)
		if err != nil {
			return nil, fmt.Errorf("error resolving lines for lexical block: %w (0x%x)",
				err, currentBlock.entry.Offset)
		}
		if blockLineRange == (LineRange{}) {
			// The block has no address ranges; let's ignore it. I've seen a
			// case where this happens even though the block has variables
			// inside it; in that case, the variables did not have availability
			// information either, so the block was useless.
			return nil, nil
		}
		// Replace all the accumulated scopes with a new scope that contains them as
		// children.
		return []Scope{{
			StartLine: blockLineRange[0],
			EndLine:   blockLineRange[1],
			Scopes:    inner.scopes,
			Variables: inner.vars,
		}}, nil
	}
	return inner.scopes, nil
}

func pcRangeToLines(r dwarfutil.PCRange, pcIt *gosym.PCIterator) (LineRange, bool) {
	startLine, ok := pcIt.PCToLine(r[0])
	if !ok {
		return LineRange{}, false
	}

	endLine, ok := pcIt.PCToLine(r[1])
	if !ok {
		return LineRange{}, false
	}
	if startLine > endLine {
		return LineRange{}, false
	}

	return LineRange{int(startLine), int(endLine)}, true
}

// parseVariable parses a variable or formal parameter entry.
func (b *SymDBBuilder) parseVariable(varEntry *dwarf.Entry, pcIt *gosym.PCIterator) (Variable, error) {
	varName, ok := varEntry.Val(dwarf.AttrName).(string)
	if !ok {
		return Variable{}, fmt.Errorf("formal parameter without name at 0x%x", varEntry.Offset)
	}
	declLine, ok := varEntry.Val(dwarf.AttrDeclLine).(int64)
	if !ok {
		return Variable{}, errors.New("formal parameter without declaration line")
	}
	typeOffset, ok := varEntry.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return Variable{}, errors.New("formal parameter without type")
	}
	typ, err := b.types.resolveType(typeOffset)
	if err != nil {
		return Variable{}, err
	}

	// Parse the line availability for the variable.
	var availableLineRanges []LineRange
	if locField := varEntry.AttrField(dwarf.AttrLocation); locField != nil {
		pcRanges, err := b.processLocations(locField, uint32(typ.size))
		if err != nil {
			return Variable{}, fmt.Errorf(
				"error processing locations for variable %q: %w", varName, err,
			)
		}
		for _, r := range pcRanges {
			lineRange, ok := pcRangeToLines(r, pcIt)
			if ok {
				availableLineRanges = append(availableLineRanges, lineRange)
			}
		}
	}

	// Merge the available lines into disjoint ranges.
	if len(availableLineRanges) > 0 {
		sort.Slice(availableLineRanges, func(i, j int) bool {
			return availableLineRanges[i][0] < availableLineRanges[j][0]
		})
		merged := make([]LineRange, 0, len(availableLineRanges))
		merged = append(merged, availableLineRanges[0])
		for _, curr := range availableLineRanges[1:] {
			last := &merged[len(merged)-1]
			if curr[0] <= (*last)[1] {
				if curr[1] > last[1] {
					// curr extends last.
					last[1] = curr[1]
				}
			} else {
				// curr is disjoint from last, so add it.
				merged = append(merged, curr)
			}
		}
		availableLineRanges = merged
	}

	// Decide whether this variable is a function argument or a regular variable.
	// Note that return values are represented DW_TAG_formal_parameter with
	// attribute DW_AT_variable_parameter = 1. We don't mark return values as
	// function arguments.
	functionArgument := varEntry.Tag == dwarf.TagFormalParameter
	if functionArgument {
		if varParamField := varEntry.AttrField(dwarf.AttrVarParam); varParamField != nil {
			varParamValue, ok := varParamField.Val.(int)
			if ok && varParamValue == 1 {
				functionArgument = false
			}
		}
	}

	return Variable{
		Name:                varName,
		DeclLine:            int(declLine),
		TypeName:            typ.name,
		FunctionArgument:    functionArgument,
		AvailableLineRanges: availableLineRanges,
	}, nil
}

// processLocations goes through a list of location lists and returns the PC
// ranges for which the whole variable is available. Ranges for which the
// variable is only partially available are ignored.
//
// totalSize is the size of the type that this location list is describing.
func (b *SymDBBuilder) processLocations(
	locField *dwarf.Field,
	totalSize uint32,
) ([]dwarfutil.PCRange, error) {
	// TODO: resolving these PC ranges is only necessary for variables that use
	// dwarf.ClassExprLoc; it's not necessary for other variables. It might be
	// worth it to make the computation lazy.
	pcRanges, err := b.currentBlock().resolvePCRanges(b.dwarfData)
	if err != nil {
		return nil, err
	}
	loclists, err := dwarfutil.ProcessLocations(locField, b.currentCompileUnit, b.loclistReader, pcRanges, totalSize, uint8(b.pointerSize))
	if err != nil {
		return nil, err
	}
	loclists = dwarfutil.FilterIncompleteLocationLists(loclists)
	res := make([]dwarfutil.PCRange, len(loclists))
	for i, loc := range loclists {
		res[i] = loc.Range
	}
	return res, nil
}

type exploreCodeResult struct {
	// vars contains all the variables that are defined by the outer scope (i.e.
	// not inside `scopes`).
	vars   []Variable
	scopes []Scope
}

// exploreCode contains shared logic for exploring blocks, subprograms, and
// inlined subprograms.
//
// pcIt is a PC iterator for the function containing this code.
func (b *SymDBBuilder) exploreCode(reader *dwarf.Reader, pcIt *gosym.PCIterator) (exploreCodeResult, error) {
	var res exploreCodeResult
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return exploreCodeResult{}, err
		}
		if dwarfutil.IsEntryNull(child) {
			break // end of child nodes
		}

		// We recognize formal parameters, variables, lexical blocks and inlined subroutines.
		switch child.Tag {
		case dwarf.TagFormalParameter:
			v, err := b.parseVariable(child, pcIt)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.vars = append(res.vars, v)
		case dwarf.TagVariable:
			v, err := b.parseVariable(child, pcIt)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.vars = append(res.vars, v)
		case dwarf.TagLexDwarfBlock:
			scopes, err := b.exploreLexicalBlock(child, reader, pcIt)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.scopes = append(res.scopes, scopes...)
		case dwarf.TagInlinedSubroutine:
			err := b.exploreInlinedSubroutine(child, reader)
			if err != nil {
				return exploreCodeResult{}, err
			}
		}
	}
	return res, nil
}
