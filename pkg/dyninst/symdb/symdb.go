// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"debug/dwarf"
	"debug/gosym"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
)

// Symbols models the symbols from a binary that get exported to SymDB.
type Symbols struct {
	Packages []Package
}

type Package struct {
	Name      string
	Functions []Function
	Types     []Type
}

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
	File string
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
	AvailableLineRanges [][2]int
}

// SymDBBuilder walks the DWARF data for a binary, extracting symbols in the
// SymDB format.
type SymDBBuilder struct {
	// The DWARF data to extract symbols from.
	dwarfData *dwarf.Data
	// The Go symbol table for the binary, used to resolve PC addresses to
	// functions.
	sym *gosym.Table
	// The location list reader used to read location lists for variables.
	loclistReader *loclist.Reader
	// The size of pointers for the binary's architecture, in bytes.
	pointerSize int

	// typesCache will accumulate types as we look them up to resolve variables
	// and functions.
	typesCache *dwarfutils.TypeFinder

	// typesInfo maps from type qualified name to the Type being built for the
	// exploration results. Used to gradually add methods to Types as we
	// discover them.
	types map[string]Type

	// The compile unit currently being processed by explore* functions.
	currentCompileUnit        *dwarf.Entry
	filesInCurrentCompileUnit []string

	// Stack of blocks currently being explored. Variable location lists can
	// make references to the current block and its PC ranges; they will look at
	// the top of this stack.
	blockStack []codeBlock
}

// PCRange corresponds to a range of program counters (PCs). Ranges are
// inclusive of the start and exclusive of the end.
type PCRange = [2]uint64

// codeBlock abstracts LexicalBlocks, Subprograms and InlinedSubroutines. They
// all correspond to a list of PC ranges.
type codeBlock interface {
	resolvePCRanges(dwarfData *dwarf.Data) ([]PCRange, error)
}

// dwarfBlock is the implementation of codeBlock for lexical blocks.
type dwarfBlock struct {
	// The DWARF entry for the lexical block.
	entry    *dwarf.Entry
	pcRanges []PCRange
	// pcRangesResolved is set if pcRanges has been already calculated.
	pcRangesResolved bool
}

// dwarfBlock implements codeBlock.
var _ codeBlock = (*dwarfBlock)(nil)

// resolvePCRanges parses and memoizes the ranges attribute of the dwarf entry.
func (d *dwarfBlock) resolvePCRanges(dwarfData *dwarf.Data) ([]PCRange, error) {
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

// resolveLine computes the start and end lines of the lexical block by
// combining PC ranges from DWARF with pclinetab data. Returns [0,0] if the
// block has no address ranges, and thus no lines.
func (d *dwarfBlock) resolveLines(dwarfData *dwarf.Data, funcTable *gosym.Func) ([2]int, error) {
	// Find the start and end lines of the block by going through the address
	// ranges, resolving them to lines, and selecting the minimum and maximum.
	ranges, err := d.resolvePCRanges(dwarfData)
	if err != nil {
		return [2]int{}, err
	}
	if len(ranges) == 0 {
		return [2]int{}, nil
	}
	startLine := math.MaxInt
	endLine := 0
	for _, r := range ranges {
		rangeStartLine := funcTable.LineTable.PCToLine(r[0])
		rangeEndLine := funcTable.LineTable.PCToLine(r[1])
		if rangeStartLine < startLine {
			startLine = rangeStartLine
		}
		if rangeEndLine > endLine {
			endLine = rangeEndLine
		}
	}
	return [2]int{startLine, endLine}, nil
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
func (s subprogBlock) resolvePCRanges(*dwarf.Data) ([]PCRange, error) {
	return []PCRange{{s.lowpc, s.highpc}}, nil
}

func NewSymDBBuilder(file *object.ElfFile) (*SymDBBuilder, error) {
	dwarfData, err := file.DWARF()
	if err != nil {
		return nil, err
	}

	lineTableSect := file.Section(".gopclntab")
	if lineTableSect == nil {
		return nil, fmt.Errorf("missing .gopclntab section")
	}
	textSect := file.Section(".text")
	if textSect == nil {
		return nil, fmt.Errorf("missing required sections")
	}
	lineTableData, err := lineTableSect.Data()
	if err != nil {
		return nil, fmt.Errorf("read .gopclntab: %w", err)
	}
	textAddr := file.Section(".text").Addr
	lineTable := gosym.NewLineTable(lineTableData, textAddr)
	symTable, err := gosym.NewTable([]byte{}, lineTable)
	if err != nil {
		return nil, fmt.Errorf("failed to parse .gosymtab: %w", err)
	}
	loclistReader, err := file.LoclistReader()
	if err != nil {
		return nil, fmt.Errorf("failed to create loclist reader: %w", err)
	}

	return &SymDBBuilder{
		dwarfData:     dwarfData,
		sym:           symTable,
		loclistReader: loclistReader,
		pointerSize:   int(file.Architecture().PointerSize()),
		typesCache:    dwarfutils.NewTypeFinder(dwarfData),
		types:         make(map[string]Type),
	}, nil
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
		res.Packages = append(res.Packages, pkg)
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

func (b *SymDBBuilder) resolveType(offset dwarf.Offset) (godwarf.Type, error) {
	typ, err := b.typesCache.FindTypeByOffset(offset)
	if err != nil {
		return nil, err
	}

	// Unwrap pointer types until we reach a non-pointer type.
	for t, ok := typ.(*godwarf.PtrType); ok; t, ok = typ.(*godwarf.PtrType) {
		typ = t.Type
	}

	// Check if we've seen this type before. If we haven't, add it to the types
	// map.
	name := typ.Common().Name
	if _, ok := b.types[name]; !ok {
		t := Type{
			Name:    name,
			Methods: nil,
		}
		if s, ok := typ.(*godwarf.StructType); ok {
			for _, field := range s.Field {
				t.Fields = append(t.Fields, Field{
					Name: field.Name,
					Type: field.Type.Common().Name,
				})
			}
		}
		b.types[name] = t
	}

	return typ, nil
}

// exploreCompileUnit processes a compile unit entry (entry's tag is TagCompileUnit).
func (b *SymDBBuilder) exploreCompileUnit(entry *dwarf.Entry, reader *dwarf.Reader) (Package, error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return Package{}, fmt.Errorf("expected TagCompileUnit, got %s", entry.Tag)
	}

	b.currentCompileUnit = entry
	defer func() { b.currentCompileUnit = nil }()

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
			files = append(files, strings.Clone(file.Name))
		}
	}
	b.filesInCurrentCompileUnit = files

	// We recognize subprograms and types.
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return Package{}, err
		}
		if IsEntryNull(child) {
			break // End of children for this compile unit.
		}

		if child.Tag == dwarf.TagSubprogram {
			function, err := b.exploreSubprogram(child, reader)
			if err != nil {
				return Package{}, err
			}
			if !function.empty() {
				res.Functions = append(res.Functions, function)
			}
		} else if child.Tag == dwarf.TagTypedef ||
			child.Tag == dwarf.TagClassType ||
			child.Tag == dwarf.TagUnionType ||
			child.Tag == dwarf.TagPointerType ||
			child.Tag == dwarf.TagStructType {
			// TODO: deal with more type tags above

			typeName, ok := child.Val(dwarf.AttrName).(string)
			if !ok {
				continue // Skip if no name
			}
			t := Type{Name: typeName}

			// TODO: read the struct fields
			// TODO: deduplicate pointer types and their pointees.

			b.types[typeName] = t

			res.Types = append(res.Types, t)
		} else {
			reader.SkipChildren()
		}
	}

	// !!! copy the types from this current package to the output (since we now
	// know all the methods of their types).
	//for _, t := range b.types {
	//	res.Types = append(res.Types, t)
	//}

	return res, nil
}

// DW_INL_inlined is the value for AttrInline marking the function as inlined.
const DW_INL_inlined = 1

// exploreSubprogram processes a subprogram entry, corresponding to a Go
// function (entry's tag is TagSubprogram). It accumulates the function's
// lexical blocks and variables. If the function is free-standing (not a
// method), returns the accumulated info. If the function is a method (i.e. has
// a receiver), it is appended to the types methods in the types collection.
//
// Returns a zero Function if the kind of function is currently unsupported and
// should be ignored.
//
// Upon return, the reader is positioned after the subprogram's children.
func (b *SymDBBuilder) exploreSubprogram(
	subprogEntry *dwarf.Entry, reader *dwarf.Reader,
) (Function, error) {
	earlyExit := func() (Function, error) {
		reader.SkipChildren()
		return Function{}, nil
	}

	if subprogEntry.Tag != dwarf.TagSubprogram {
		return Function{}, fmt.Errorf("expected TagSubprogram, got %s", subprogEntry.Tag)
	}

	// If this is a "concrete-out-of-line" instance of a subprogram, the
	// variables inside it will reference the abstract origin. We don't handle
	// that at the moment, so skip these functions.
	// TODO: handle concrete-out-of-line instances of functions.
	if subprogEntry.AttrField(dwarf.AttrAbstractOrigin) != nil {
		return earlyExit()
	}

	funcQualifiedName, ok := subprogEntry.Val(dwarf.AttrName).(string)
	if !ok {
		return Function{}, errors.New("subprogram without name")
	}
	inline, ok := subprogEntry.Val(dwarf.AttrInline).(int64)
	if ok {
		if inline == DW_INL_inlined {
			// This function is always inlined; its variables don't have
			// location lists here; instead, each inlined instance will have
			// them. Nothing to do.
			return earlyExit()
		}
	}

	fileIdx, ok := subprogEntry.Val(dwarf.AttrDeclFile).(int64)
	if !ok {
		// No DeclFile means the function is coming from the stdlib.
		fileIdx = 0
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

	// We cannot deal with the auto-generated trampoline functions because they
	// don't have debug info. The name looks like a method, but there's no info
	// on the pointer receiver. Also, we don't want to show these functions to
	// users anyway.
	if subprogEntry.AttrField(dwarf.AttrTrampoline) != nil {
		return earlyExit()
	}

	lowpc, ok := subprogEntry.Val(dwarf.AttrLowpc).(uint64)
	if !ok {
		return Function{}, fmt.Errorf("subprogram without lowpc: %s", funcQualifiedName)
	}
	highpc, ok := subprogEntry.Val(dwarf.AttrHighpc).(uint64)
	if !ok {
		return Function{}, errors.New("subprogram without highpc")
	}

	// From now on, location lists that reference the current block will
	// reference this function.
	b.pushBlock(subprogBlock{lowpc: lowpc, highpc: highpc})
	defer b.popBlock()

	// Resolve the address range to a function, and use the function's line
	// table to resolve addresses for all the scopes in the function. We
	// technically don't need to do this -- b.sym.PCToFunc can be used directly
	// to resolve any address across all the functions, but resolving the
	// function first and reusing that is more efficient.
	funcTable := b.sym.PCToFunc(lowpc)

	startLine := funcTable.LineTable.PCToLine(lowpc)
	endLine := funcTable.LineTable.PCToLine(highpc)

	// Explore the function's body. Besides collecting the information that we
	// want to explore to SymDB, this also has the side effect of resolving the
	// types of all the function arguments; in particular, we rely below on the
	// type of the receiver having been resolved.
	inner, err := b.exploreCode(reader, funcTable)
	if err != nil {
		return Function{}, err
	}

	funcName, err := ParseFuncName(funcQualifiedName)
	if err != nil {
		return Function{}, err
	}
	if funcName.Empty() {
		return Function{}, err
	}

	res := Function{
		Name: funcName.Name,
		File: fileName,
		Scope: Scope{
			StartLine: startLine,
			EndLine:   endLine,
			Variables: inner.vars,
			Scopes:    inner.scopes,
		},
	}

	if funcName.GenericFunction {
		return Function{}, err
	}
	// If this function is a method (i.e. it has a receiver), we add it to the
	// respective type instead of returning it as a stand-alone function.
	if funcName.Type != "" {
		typeQualifiedName := funcName.Package + "." + funcName.Type
		// We expect the type of the receiver to have been populated by the
		// exploreCode() call above.
		typ, ok := b.types[typeQualifiedName]
		if !ok {
			return Function{}, fmt.Errorf(
				"%s is a method of type %s, but that type is missing from the cache. DWARF offset: 0x%x",
				funcQualifiedName, typeQualifiedName, subprogEntry.Offset,
			)
		}
		typ.Methods = append(typ.Methods, res)
		b.types[typeQualifiedName] = typ
		return Function{}, nil
	} else {
		return res, nil
	}
}

func (b *SymDBBuilder) exploreInlinedSubroutine(inlinedEntry *dwarf.Entry, reader *dwarf.Reader) error {
	if inlinedEntry.Tag != dwarf.TagInlinedSubroutine {
		return fmt.Errorf("expected TagInlinedSubroutine, got %s", inlinedEntry.Tag)
	}
	// TODO: accumulate variable availability data and join it with the other
	// inlined and out-of-line instances.
	reader.SkipChildren()
	return nil
}

// exploreLexicalBlock processes a lexical block entry. If the block contains
// any variables, it returns one Scope with those variables and any sub-blocks.
// If the block does not contain any variables, it returns any sub-blocks that
// do contain variables (if any).
func (b *SymDBBuilder) exploreLexicalBlock(
	blockEntry *dwarf.Entry, reader *dwarf.Reader, funcTable *gosym.Func,
) ([]Scope, error) {
	if blockEntry.Tag != dwarf.TagLexDwarfBlock {
		return nil, fmt.Errorf("expected TagLexDwarfBlock, got %s", blockEntry.Tag)
	}
	currentBlock := &dwarfBlock{entry: blockEntry}
	// From now on, location lists that reference the current block will
	// reference this block.
	b.pushBlock(currentBlock)
	defer b.popBlock()

	inner, err := b.exploreCode(reader, funcTable)
	if err != nil {
		return nil, fmt.Errorf("error exploring code in lexical block: %w", err)
	}

	// If the block has any variables, then we create a scope for it. If it
	// doesn't, then inner scopes (if any), are returned directly, to be added
	// as direct children of the caller's block.
	if len(inner.vars) != 0 {
		lineRange, err := currentBlock.resolveLines(b.dwarfData, funcTable)
		if err != nil {
			return nil, fmt.Errorf("error resolving lines for lexical block: %w (0x%x)",
				err, currentBlock.entry.Offset)
		}
		if lineRange == [2]int{} {
			// The block has no address ranges; let's ignore it. I've seen a
			// case where this happens even though the block has variables
			// inside it; in that case, the variables did not have availability
			// information either, so the block was useless.
			return nil, nil
		}
		// Replace all the accumulated scopes with a new scope that contains them as
		// children.
		return []Scope{{
			StartLine: lineRange[0],
			EndLine:   lineRange[1],
			Scopes:    inner.scopes,
			Variables: inner.vars,
		}}, nil
	} else {
		return inner.scopes, nil
	}
}

func (b *SymDBBuilder) parseVariable(varEntry *dwarf.Entry, funcTable *gosym.Func) (Variable, error) {
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
	typ, err := b.resolveType(typeOffset)
	if err != nil {
		return Variable{}, err
	}

	// Parse the line availability for the variable.
	var availableLineRanges [][2]int
	if locField := varEntry.AttrField(dwarf.AttrLocation); locField != nil {
		pcRanges, err := b.processLocations(locField, uint32(typ.Size()))
		if err != nil {
			return Variable{}, fmt.Errorf(
				"error processing locations for variable %q: %w", varName, err,
			)
		}
		for _, r := range pcRanges {
			rangeStartLine := funcTable.LineTable.PCToLine(r[0])
			rangeEndLine := funcTable.LineTable.PCToLine(r[1])
			availableLineRanges = append(availableLineRanges, [2]int{rangeStartLine, rangeEndLine})
		}
	}

	// Merge the available lines into disjoint ranges.
	if len(availableLineRanges) > 0 {
		sort.Slice(availableLineRanges, func(i, j int) bool {
			return availableLineRanges[i][0] < availableLineRanges[j][0]
		})
		merged := make([][2]int, 0, len(availableLineRanges))
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

	return Variable{
		Name:                varName,
		DeclLine:            int(declLine),
		TypeName:            typ.Common().Name,
		FunctionArgument:    false,
		AvailableLineRanges: availableLineRanges,
	}, nil
}

// processLocations goes through a list of location lists and returns the PC
// ranges for which the whole variable is available. Ranges for which the
// variable is only partially available are ignored.
func (b *SymDBBuilder) processLocations(
	locField *dwarf.Field,
	// The size of the type that this location list is describing.
	totalSize uint32,
) ([]PCRange, error) {
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
	res := make([]PCRange, len(loclists))
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
func (b *SymDBBuilder) exploreCode(
	reader *dwarf.Reader, funcTable *gosym.Func,
) (exploreCodeResult, error) {
	var res exploreCodeResult
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return exploreCodeResult{}, err
		}
		if IsEntryNull(child) {
			break // End of children for this subprogram.
		}

		// We recognize formal parameters, variables, lexical blocks and inlined subroutines.
		switch child.Tag {
		case dwarf.TagFormalParameter:
			v, err := b.parseVariable(child, funcTable)
			if err != nil {
				return exploreCodeResult{}, err
			}
			v.FunctionArgument = true
			res.vars = append(res.vars, v)
		case dwarf.TagVariable:
			v, err := b.parseVariable(child, funcTable)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.vars = append(res.vars, v)
		case dwarf.TagLexDwarfBlock:
			scopes, err := b.exploreLexicalBlock(child, reader, funcTable)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.scopes = append(res.scopes, scopes...)
		case dwarf.TagInlinedSubroutine:
			if err := b.exploreInlinedSubroutine(child, reader); err != nil {
				return exploreCodeResult{}, err
			}
		}
	}
	return res, nil
}

// IsEntryNull determines whether a *dwarf.Entry value is a "null" DIE, which
// are used to denote the end of sub-trees. See DWARF v4 spec, section 2.3.
func IsEntryNull(entry *dwarf.Entry) bool {
	// Every field is 0 in an empty entry.
	return !entry.Children &&
		len(entry.Field) == 0 &&
		entry.Offset == 0 &&
		entry.Tag == dwarf.Tag(0)
}
