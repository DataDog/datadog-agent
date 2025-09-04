// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"debug/buildinfo"
	"debug/dwarf"
	"errors"
	"fmt"
	"iter"
	"math"
	"runtime/debug"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"golang.org/x/time/rate"

	dwarf2 "github.com/DataDog/datadog-agent/pkg/dyninst/dwarf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var loclistErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 1)

// PackagesIterator returns an iterator over the packages in the binary.
//
// PackagesIterator can only be used if the packagesIterator was configured with
// ExtractOptions.IncludeInlinedFunctions=false (i.e. if we're ignoring inlined
// functions), since inlined functions can appear in different compile units
// than their package.
func PackagesIterator(binaryPath string, opt ExtractOptions) (iter.Seq2[Package, error], error) {
	if opt.IncludeInlinedFunctions {
		return nil, fmt.Errorf("cannot overate over packages when IncludeInlinedFunctions is set")
	}

	bin, err := openBinary(binaryPath, opt)
	if err != nil {
		return nil, err
	}

	b := newPackagesIterator(bin, opt)
	return b.iterator(), nil
}

// ExtractSymbols walks the DWARF data and accumulates the symbols to send to
// SymDB.
func ExtractSymbols(binaryPath string, opt ExtractOptions) (Symbols, error) {
	bin, err := openBinary(binaryPath, opt)
	if err != nil {
		return Symbols{}, err
	}
	b := newPackagesIterator(bin, opt)

	packages := make(map[string]*Package)
	for pkg, err := range b.iterator() {
		if err != nil {
			return Symbols{}, err
		}
		if existingPkg, ok := packages[pkg.Name]; ok {
			existingPkg.Functions = append(existingPkg.Functions, pkg.Functions...)
			existingPkg.Types = append(existingPkg.Types, pkg.Types...)
		} else {
			packages[pkg.Name] = &pkg
		}
	}

	res := Symbols{
		MainModule: b.mainModule,
		Packages:   nil,
	}
	if b.options.IncludeInlinedFunctions {
		err := b.addAbstractFunctions(packages)
		if err != nil {
			return Symbols{}, err
		}
	}

	for pkgName, types := range b.types.packages {
		anyNonEmpty := slices.ContainsFunc(types, func(t *Type) bool {
			return len(t.Fields) > 0 || len(t.Methods) > 0
		})
		if !anyNonEmpty {
			continue
		}

		pkg, ok := packages[pkgName]
		if !ok {
			pkg = &Package{
				Name:      pkgName,
				Functions: nil,
				Types:     nil,
			}
			packages[pkgName] = pkg
		}
		for _, t := range types {
			// Don't add empty types to the output.
			if len(t.Fields) == 0 && len(t.Methods) == 0 {
				continue
			}
			pkg.Types = append(pkg.Types, *t)
		}
	}
	for _, pkg := range packages {
		if len(pkg.Types) > 0 || len(pkg.Functions) > 0 {
			res.Packages = append(res.Packages, *pkg)
		}
	}
	// Sort packages so that output is stable.
	sort.Slice(res.Packages, func(i, j int) bool {
		return res.Packages[i].Name < res.Packages[j].Name
	})
	return res, nil
}

// Symbols models the symbols from a binary that get exported to SymDB.
type Symbols struct {
	// MainModule is the path of the module containing the "main" function.
	MainModule string
	Packages   []Package
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

// PackageStats represents statistics about the symbols collected for a package.
type PackageStats struct {
	// NumTypes is the number of types in this package represented in the
	// collected symbols.
	NumTypes int
	// NumFunctions is the number of functions in this package represented in
	// the collected symbols. This includes methods.
	NumFunctions int
}

func (s PackageStats) String() string {
	return fmt.Sprintf("Types: %d, Functions: %d", s.NumTypes, s.NumFunctions)
}

// Stats computes statistics about the package's symbols.
//
// sourceFiles will be populated with files encountered while going through this
// package's compile unit. Nil can be passed if the caller is not interested.
// Note that it's possible for multiple compile units to reference the same file
// due to inlined functions; in such cases, the file will arbitrarily count
// towards the stats of the first package that adds it to the map.
func (p Package) Stats() PackageStats {
	var res PackageStats
	res.NumTypes += len(p.Types)
	res.NumFunctions += len(p.Functions)
	for _, t := range p.Types {
		res.NumFunctions += len(t.Methods)
	}
	return res
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
	// receiver, as it appears in DWARF. When creating probes based on a
	// function name coming from SymDB, the qualified name is how the function
	// is identified to the prober.
	QualifiedName string
	// The source file containing the function. This is an absolute path local
	// to the build machine, as recorded in DWARF.
	File string
	// The function itself represents a lexical block, with variables and
	// sub-scopes.
	Scope
}

func (f Function) empty() bool {
	return f.Name == ""
}

// Serialize serializes the symbols as a human-readable string.
//
// If packageFilter is non-empty, only packages that start with this prefix are
// included in the output. The "main" package is always present, and is placed
// first in the output.
func (s Symbols) Serialize(w StringWriter) {
	w.WriteString("Main module: ")
	w.WriteString(s.MainModule)
	w.WriteString("\n")
	// Serialize the "main" package before all the others.
	mainPkgIdx := 0
	for i, pkg := range s.Packages {
		if pkg.Name == mainPackageName {
			mainPkgIdx = i
			pkg.Serialize(w)
		}
	}
	for i, pkg := range s.Packages {
		if i == mainPkgIdx {
			continue
		}
		pkg.Serialize(w)
	}
}

// Serialize serializes the symbols in the package as a human-readable string.
func (p Package) Serialize(w StringWriter) {
	w.WriteString("Package: ")
	w.WriteString(p.Name)
	w.WriteString("\n")
	for _, fn := range p.Functions {
		fn.Serialize(w, "\t")
	}
	for _, t := range p.Types {
		t.Serialize(w, "\t")
	}
}

// Serialize serializes the function as a human-readable string.
func (f Function) Serialize(w StringWriter, indent string) {
	w.WriteString(indent)
	w.WriteString("Function: ")
	w.WriteString(f.Name)
	w.WriteString(" (")
	w.WriteString(f.QualifiedName)
	w.WriteString(")")
	w.WriteString(" in ")
	file := f.File
	w.WriteString(file)
	w.WriteString(fmt.Sprintf(" [%d:%d]", f.StartLine, f.EndLine))
	w.WriteString("\n")

	childIndent := indent + "\t"
	for _, v := range f.Variables {
		v.Serialize(w, childIndent)
	}
	for _, s := range f.Scopes {
		s.Serialize(w, childIndent)
	}
}

// Serialize serializes the scope as a human-readable string. It looks like:
// Scope: <startLine>-<endLine>
func (s Scope) Serialize(w StringWriter, indent string) {
	w.WriteString(indent)
	w.WriteString("Scope: ")
	w.WriteString(fmt.Sprintf("%d-%d", s.StartLine, s.EndLine))
	w.WriteString("\n")
	childIndent := indent + "\t"
	for _, v := range s.Variables {
		v.Serialize(w, childIndent)
	}
}

// Serialize serializes the type as a human-readable string. It looks like:
//
//	Type: <name>
//		Field: <fieldName>: <fieldType>
//		Field: <fieldName>: <fieldType>
//	 	Method: <methodName> ...
func (t Type) Serialize(w StringWriter, indent string) {
	w.WriteString(indent)
	w.WriteString("Type: ")
	w.WriteString(t.Name)
	w.WriteString("\n")
	childIndent := indent + "\t"
	for _, f := range t.Fields {
		w.WriteString(childIndent)
		w.WriteString("Field: ")
		w.WriteString(f.Name)
		w.WriteString(": ")
		w.WriteString(f.Type)
		w.WriteString("\n")
	}
	for _, m := range t.Methods {
		m.Serialize(w, childIndent)
	}
}

// Serialize serializes the variable as a human-readable string. It looks like:
// Var: <name>: <type> (declared at line <declLine>, available: [<startLine>-<endLine>], [<startLine>-<endLine>], ...)
func (v Variable) Serialize(w StringWriter, indent string) {
	w.WriteString(indent)
	if v.FunctionArgument {
		w.WriteString("Arg: ")
	} else {
		w.WriteString("Var: ")
	}
	w.WriteString(v.Name)
	w.WriteString(": ")
	w.WriteString(v.TypeName)
	w.WriteString(" (declared at line ")
	w.WriteString(fmt.Sprintf("%d", v.DeclLine))
	w.WriteString(", available: ")
	for i, r := range v.AvailableLineRanges {
		if i > 0 {
			w.WriteString(", ")
		}
		w.WriteString(fmt.Sprintf("[%d-%d]", r[0], r[1]))
	}
	w.WriteString(")\n")
}

// StringWriter is like io.StringWriter, but writes panic on errors instead of
// returning errors. See symdbutil.PanickingWriter for an implementation.
type StringWriter interface {
	WriteString(s string)
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

const mainPackageName = "main"

// packagesIterator walks the DWARF data for a binary, extracting symbols in the
// SymDB format.
// nolint:revive  // ignore stutter rule
type packagesIterator struct {
	// The DWARF data to extract symbols from.
	dwarfData *dwarf.Data
	// The Go symbol table for the binary, used to resolve PC addresses to
	// functions.
	sym *gosym.GoSymbolTable
	// The location list reader used to read location lists for variables.
	loclistReader *loclist.Reader
	// The size of pointers for the binary's architecture, in bytes.
	pointerSize int
	options     ExtractOptions

	// The module path of the Go module containing the main function. Empty if
	// unknown.
	mainModule string
	// The prefix of package paths for first party packages. Empty if not used for filtering.
	firstPartyPkgPrefix string
	// Filter files starting with these path prefixes when exploring the DWARF.
	// Used to skip 3rd party code in certain cases.
	filesFilter []string

	// The compile unit currently being processed by explore* functions.
	currentCompileUnit        *dwarf.Entry
	currentCompileUnitName    string
	filesInCurrentCompileUnit []string

	abstractFunctions map[dwarf.Offset]*abstractFunction

	// typesCache will accumulate types as we look them up to resolve variables
	// and functions. The cache is indexed by DWARF offset.
	typesCache *dwarfutils.TypeFinder
	types      typesCollection

	// Stack of blocks currently being explored. Variable location lists can
	// make references to the current block and its PC ranges; they will look at
	// the top of this stack.
	blockStack []codeBlock

	// cleanupFuncs holds functions to be called on Close() to clean up resources.
	cleanupFuncs []func()
}

// abstractFunction aggregates data for an inlined function.
type abstractFunction struct {
	// Set on initialization, by parsing abstract definition.
	interesting   bool
	pkg           string
	name          string
	receiver      string
	qualifiedName string
	startLine     int

	// Updated when inlined instances are encountered.
	file    string
	endLine uint32

	// The variables map is generated by parsing abstract definition.
	// The AvailableLineRanges field is updated when inlined instances are
	// encountered.
	variables map[dwarf.Offset]*abstractVariable
}

type abstractVariable struct {
	Variable
	typeSize uint32
}

type typesCollection struct {
	scopeFilter         ExtractScope
	mainModule          string
	firstPartyPkgPrefix string

	// Map from the type's name, as it appears in DWARF, to the type info. Only
	// some of the types from typesCache are to be exported to SymDB and thus
	// are present here -- we ignore some types we don't support, and we
	// dereference pointers. The map holds pointers to allow the Type's to
	// change over time (in particular, they accumulate methods).
	types map[string]*Type
	// Map from package qualified name to the types in that package.
	packages map[string][]*Type
}

func (b *packagesIterator) resolveType(offset dwarf.Offset) (typeInfo, error) {
	typ, err := b.typesCache.FindTypeByOffset(offset)
	if err != nil {
		return typeInfo{}, err
	}
	// The package import path in the type name might be escaped. We want
	// unescaped paths for SymDB.
	typeName, err := unescapeSymbol(typ.Common().Name)
	if err != nil {
		return typeInfo{}, fmt.Errorf("failed to unescape type name %q: %w", typ.Common().Name, err)
	}
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

	pkgFilter := ""
	if !b.options.IncludeInlinedFunctions {
		// Only add types from the current package; we're accumulating types for
		// the purpose of adding methods to them and, if we ignore inlined
		// functions, this compile unit only contains methods for types in the
		// current package.
		pkgFilter = b.currentCompileUnitName
	}
	if err := b.types.maybeAddType(typ, pkgFilter); err != nil {
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

// maybeAddType adds a type to the collection if it is not already present and
// if it's from a package that's not filtered. Unsupported types are ignored and
// no error is returned. If pkgFilter is not
// empty, then only types from that package are added.
func (c *typesCollection) maybeAddType(t godwarf.Type, pkgFilter string) error {
	pkg, sym, wasEscaped, err := parseLinkFuncName(t.Common().Name)
	if err != nil {
		return fmt.Errorf("failed to split package for %s : %w", t.Common().Name, err)
	}

	if !interestingPackage(pkg, c.mainModule, c.firstPartyPkgPrefix, c.scopeFilter) {
		return nil
	}

	var unescapedName string
	if wasEscaped {
		unescapedName = pkg + "." + sym
	} else {
		unescapedName = t.Common().Name
	}

	// Check if the type is already present.
	if _, ok := c.types[unescapedName]; ok {
		return nil
	}

	// We don't support parametric types (generics).
	if _, ok := t.(*godwarf.ParametricType); ok {
		return nil
	}

	// Assert that we were not given a pointer type.
	if _, ok := t.(*godwarf.PtrType); ok {
		return fmt.Errorf("ptr type expected to have been unwrapped: %s", unescapedName)
	}
	if strings.HasPrefix(unescapedName, "*") {
		return fmt.Errorf("type unescapedName for non-pointer unexpectedly starting with '*': %s", unescapedName)
	}

	// Skip anonymous types, generic types, array types and structs
	// corresponding to slices.
	if strings.ContainsAny(unescapedName, "{<[") {
		return nil
	}

	if pkgFilter != "" && pkg != pkgFilter {
		return nil
	}

	typ := &Type{
		Name:   unescapedName,
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

	c.types[unescapedName] = typ
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
func (d *dwarfBlock) resolveLines(dwarfData *dwarf.Data, lines []gosym.LineRange) (LineRange, error) {
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
		lineRange, ok := pcRangeToLines(r, lines)
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

// ExtractScope defines which symbols to extract from the binary.
type ExtractScope int

const (
	// ExtractScopeAllSymbols extracts all symbols from the binary.
	ExtractScopeAllSymbols ExtractScope = iota
	// ExtractScopeMainModuleOnly extracts only symbols from the main module,
	// i.e. the Go module containing the main() function.
	ExtractScopeMainModuleOnly
	// ExtractScopeModulesFromSameOrg extracts symbols from the main module and
	// other modules from the same GitHub organization or GitLab namespace. For
	// example, if the main module is "github.com/DataDog/datadog-agent", this
	// will extract symbols from all "github.com/DataDog/*" modules.
	ExtractScopeModulesFromSameOrg
)

// newPackagesIterator creates a new packagesIterator for the given binary. The
// packagesIterator takes ownership of the ELF file.
//
// iterator() must be called to get an iterator and the iterator itself must
// then be called so that it eventually releases resources.
func newPackagesIterator(bin binaryInfo, opt ExtractOptions) *packagesIterator {
	b := &packagesIterator{
		dwarfData:           bin.obj.DwarfData(),
		sym:                 bin.symTable,
		loclistReader:       bin.obj.LoclistReader(),
		pointerSize:         int(bin.obj.PointerSize()),
		options:             opt,
		mainModule:          bin.mainModule,
		firstPartyPkgPrefix: bin.firstPartyPkgPrefix,
		filesFilter:         bin.filesFilter,
		abstractFunctions:   make(map[dwarf.Offset]*abstractFunction),
		typesCache:          dwarfutils.NewTypeFinder(bin.obj.DwarfData()),
		types: typesCollection{
			scopeFilter:         opt.Scope,
			mainModule:          bin.mainModule,
			firstPartyPkgPrefix: bin.firstPartyPkgPrefix,
			types:               make(map[string]*Type),
			packages:            make(map[string][]*Type),
		},
		cleanupFuncs: []func(){func() { _ = bin.goDebugSections.Close() }, func() { _ = bin.obj.Close() }},
	}
	return b
}

type binaryInfo struct {
	obj                 *object.ElfFileWithDwarf
	mainModule          string
	goDebugSections     *object.GoDebugSections
	symTable            *gosym.GoSymbolTable
	firstPartyPkgPrefix string
	filesFilter         []string
}

func openBinary(binaryPath string, opt ExtractOptions) (binaryInfo, error) {
	obj, err := object.OpenElfFileWithDwarf(binaryPath)
	if err != nil {
		return binaryInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	// Parse the binary's build info to figure out the URL of the main module.
	// Note that we'll get an empty URL for binaries built with Bazel.
	binfo, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		return binaryInfo{}, fmt.Errorf("failed to read build info: %w", err)
	}
	mainModule := binfo.Main.Path

	symTable, err := object.ParseGoSymbolTable(obj)
	if err != nil {
		return binaryInfo{}, err
	}

	// Figure out the package path prefix that corresponds to "1st party code"
	// (as opposed to 3rd party dependencies), in case we're asked to filter for
	// 1st party modules. We are only able to make this distinction if the
	// binary was built with Go modules, recent versions of the compiler, and
	// not with Bazel. We recognize modules hosted on GitHub and GitLab and
	// consider anything in their organization as 1st party code.
	//
	// Binaries built by Bazel don't have module information, which means they
	// don't have information on the main module. In that case, we can still
	// identify most of the 3rd party code by the decl_file attribute of the
	// subprograms -- third party functions are tagged as coming from
	// "external/..." files and standard library functions as coming from "GOROOT/...".
	var firstPartyPkgPrefix string
	var filesFilter []string
	if mainModule != "" {
		parts := strings.Split(mainModule, "/")
		if len(parts) >= 3 && (parts[0] == "github.com" || parts[0] == "gitlab.com") {
			firstPartyPkgPrefix = parts[0] + "/" + parts[1] + "/"
		}
	} else if opt.Scope == ExtractScopeMainModuleOnly || opt.Scope == ExtractScopeModulesFromSameOrg {
		filesFilter = []string{"external/", "GOROOT/"}
	}
	return binaryInfo{
		obj:                 obj,
		mainModule:          mainModule,
		goDebugSections:     &symTable.GoDebugSections,
		symTable:            &symTable.GoSymbolTable,
		firstPartyPkgPrefix: firstPartyPkgPrefix,
		filesFilter:         filesFilter,
	}, nil
}

// close frees resources associated with the iterator.
func (b *packagesIterator) close() {
	for _, f := range b.cleanupFuncs {
		f()
	}
}

// iterator returns a Go iterator that yields packages one by one. The returned
// iterator takes ownership of the Elf file, so it must be called (or used in a
// range loop) in order to eventually release resources.
func (b *packagesIterator) iterator() iter.Seq2[Package, error] {
	var err error
	return func(yield func(pkg Package, err error) bool) {
		defer b.close()
		entryReader := b.dwarfData.Reader()

		// Recognize compile units, which are the top-level entries in the DWARF
		// data corresponding to Go packages.
		var entry *dwarf.Entry
		for entry, err = entryReader.Next(); entry != nil; entry, err = entryReader.Next() {
			if err != nil {
				break
			}

			if entry.Tag != dwarf.TagCompileUnit {
				entryReader.SkipChildren()
				continue
			}

			var pkg Package
			pkg, err = b.exploreCompileUnit(entry, entryReader)
			if err != nil {
				break
			}
			if pkg.Name == "" {
				continue
			}

			// If we're not dealing with inlined functions, then move all
			// accumulated types to the output package. If we are dealing with
			// inlined functions, then this will happen later, once we've
			// discovered all the abstract functions and their inlined
			// instances, both of which can be in different compile units.
			if !b.options.IncludeInlinedFunctions {
				numPkgs := len(b.types.packages)
				if numPkgs > 1 {
					pkgNames := make([]string, 0, 2)
					for k := range b.types.packages {
						pkgNames = append(pkgNames, k)
						if len(pkgNames) == 2 {
							break
						}
					}
					err = fmt.Errorf(
						"types from multiple packages in compile unit %s (0x%x); examples: %v",
						pkg.Name, entry.Offset, pkgNames)
					break
				}

				var types []*Type
				if pkg.Name != "main" {
					types = b.types.packages[pkg.Name]
				} else {
					for _, v := range b.types.packages {
						types = v
						break
					}
				}
				if numPkgs == 1 && len(types) == 0 {
					err = fmt.Errorf(
						"types from unexpected package in compile unit: 0x%x",
						entry.Offset)
					break
				}
				for _, t := range types {
					pkg.Types = append(pkg.Types, *t)
				}
				clear(b.types.packages)
				clear(b.types.types)
			}

			if !yield(pkg, nil) {
				break
			}
		}
		if err != nil {
			yield(Package{}, err)
		}
	}
}

func interestingPackage(pkgName string, mainModule string, firstPartyPkgPrefix string, scopeFilter ExtractScope) bool {
	// We don't know what the main module is, so we can't filter out
	// anything.
	if mainModule == "" || scopeFilter == ExtractScopeAllSymbols {
		return true
	}
	// The "main" package is always included.
	if pkgName == mainPackageName {
		return true
	}
	switch scopeFilter {
	case ExtractScopeMainModuleOnly:
		return strings.HasPrefix(pkgName, mainModule)
	case ExtractScopeModulesFromSameOrg:
		return strings.HasPrefix(pkgName, firstPartyPkgPrefix)
	default:
		panic(fmt.Sprintf("unsupported extract scope: %d", scopeFilter))
	}
}

// addAbstractFunctions takes the aggregated data about inlined functions and
// adds the functions to the corresponding packages and types.
func (b *packagesIterator) addAbstractFunctions(packages map[string]*Package) error {
	// Sort abstract functions so that output is stable.
	abstractFunctions := make([]*abstractFunction, 0, len(b.abstractFunctions))
	for _, af := range b.abstractFunctions {
		abstractFunctions = append(abstractFunctions, af)
	}
	sort.Slice(abstractFunctions, func(i, j int) bool {
		return abstractFunctions[i].name < abstractFunctions[j].name
	})
	for _, af := range abstractFunctions {
		variables := make([]Variable, 0, len(af.variables))
		for _, v := range af.variables {
			v.Variable.AvailableLineRanges = coalesceLineRanges(v.AvailableLineRanges)
			variables = append(variables, v.Variable)
		}
		// Sort variables so that output is stable.
		sort.Slice(variables, func(i, j int) bool {
			return variables[i].Name < variables[j].Name
		})
		f := Function{
			Name:          af.name,
			QualifiedName: af.qualifiedName,
			File:          af.file,
			Scope: Scope{
				StartLine: af.startLine,
				EndLine:   int(af.endLine),
				Scopes:    nil,
				Variables: variables,
			},
		}
		if af.receiver != "" {
			t := b.types.getType(af.receiver)
			if t == nil {
				// Some types are empty structures, and functions that
				// use them as receivers don't actually have a parameter
				// of the receiver type. Thus we end up without a type.
				// Just make one up.
				t = &Type{
					Name: af.receiver,
				}
				b.types.types[af.receiver] = t
				b.types.packages[af.pkg] = append(b.types.packages[af.pkg], t)
			}
			t.Methods = append(t.Methods, f)
		} else {
			p := packages[af.pkg]
			if p != nil {
				p.Functions = append(p.Functions, f)
			}
		}
	}
	return nil
}

func (b *packagesIterator) currentBlock() codeBlock {
	return b.blockStack[len(b.blockStack)-1]
}

func (b *packagesIterator) pushBlock(block codeBlock) {
	b.blockStack = append(b.blockStack, block)
}

func (b *packagesIterator) popBlock() {
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

// ExtractOptions contains knobs controlling what symbols collected from a
// binary.
type ExtractOptions struct {
	Scope ExtractScope
	// If set, abstract functions and their inlined instances are not explored.
	// The produced
	IncludeInlinedFunctions bool
}

// exploreCompileUnit processes a compile unit entry (entry's tag is
// TagCompileUnit).
//
// Returns a zero value if the compile unit is not a Go package.
func (b *packagesIterator) exploreCompileUnit(
	entry *dwarf.Entry, reader *dwarf.Reader,
) (Package, error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return Package{}, fmt.Errorf("expected TagCompileUnit, got %s", entry.Tag)
	}

	name, ok := entry.Val(dwarf.AttrName).(string)
	if !ok {
		return Package{}, errors.New("compile unit without name")
	}
	if !interestingPackage(name, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
		reader.SkipChildren()
		return Package{}, nil
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
	b.currentCompileUnitName = name
	defer func() {
		b.currentCompileUnit = nil
		b.currentCompileUnitName = ""
		b.filesInCurrentCompileUnit = nil
	}()

	var res Package
	res.Name = name
	start := time.Now()

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

	// Go through the children, looking for subprograms.
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
	duration := time.Since(start)
	if duration > 5*time.Second {
		log.Warnf("Processing package %s took %s: %s", name, duration, res.Stats())
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
func (b *packagesIterator) exploreSubprogram(
	entry *dwarf.Entry, reader *dwarf.Reader,
) (Function, error) {
	// When returning early, we need to consume all the children of the subprogram
	// entry. Note that this utility can only be used before the reader is advanced.
	earlyExit := func() (Function, error) {
		reader.SkipChildren()
		return Function{}, nil
	}

	if entry.Tag != dwarf.TagSubprogram {
		return Function{}, fmt.Errorf("expected TagSubprogram, got %s", entry.Tag)
	}

	// Ignore auto-generated functions, marked as trampolines. There are a few
	// cases where the compiler generates such functions:
	// - methods with value receivers that are called through an interface that
	// boxes a pointer value.
	// - "deferwrap" functions
	if entry.AttrField(dwarf.AttrTrampoline) != nil {
		return earlyExit()
	}

	inline, ok := entry.Val(dwarf.AttrInline).(int64)
	if ok && inline == dwarf2.DW_INL_inlined {
		// Abstract function definition, nothing to do here, we parse
		// them on-demand when encountering inlined instances.
		return earlyExit()
	}

	if entry.AttrField(dwarf.AttrAbstractOrigin) != nil {
		// "out-of-line" instance of an abstract subprogram.
		lowpc, ok := entry.Val(dwarf.AttrLowpc).(uint64)
		if !ok {
			return Function{}, fmt.Errorf("subprogram without lowpc at 0x%x", entry.Offset)
		}
		lines, err := b.sym.FunctionLines(lowpc)
		if err != nil {
			return Function{}, fmt.Errorf("failed to resolve function lines for function pc 0x%x: %w", lowpc, err)
		}
		return Function{}, b.exploreInlinedInstance(entry, reader, lines)
	}

	funcQualifiedName, funcName, recognized, err := b.parseFunctionName(entry)
	if err != nil {
		return Function{}, err
	}
	if !recognized {
		return earlyExit()
	}

	fileIdx, ok := entry.Val(dwarf.AttrDeclFile).(int64)
	if !ok || fileIdx == 0 { // fileIdx == 0 means unknown file, as per DWARF spec
		// TODO: log if this ever happens. I haven't seen it.
		return earlyExit()
	}
	if fileIdx < 0 || int(fileIdx) >= len(b.filesInCurrentCompileUnit) {
		return Function{}, fmt.Errorf(
			"subprogram at 0x%x has invalid file index %d, expected in range [0, %d)",
			entry.Offset, fileIdx, len(b.filesInCurrentCompileUnit),
		)
	}
	fileName := b.filesInCurrentCompileUnit[fileIdx]
	// If configured with a filter, check if the file should be ignored.
	for _, filter := range b.filesFilter {
		if strings.HasPrefix(fileName, filter) {
			return earlyExit()
		}
	}
	// There are some funky auto-generated functions that we can't deal with
	// because we can't parse their names (e.g.
	// type:.eq.sync/atomic.Pointer[os.dirInfo]). Skip everything
	// auto-generated.
	if fileName == "<autogenerated>" {
		return earlyExit()
	}

	lowpc, ok := entry.Val(dwarf.AttrLowpc).(uint64)
	if !ok {
		return Function{}, fmt.Errorf("subprogram without lowpc: %s @ 0x%x", funcQualifiedName, entry.Offset)
	}
	highPCField := entry.AttrField(dwarf.AttrHighpc)
	if highPCField == nil {
		return Function{}, fmt.Errorf("subprogram without highpc: %s @ 0x%x", funcQualifiedName, entry.Offset)
	}
	// The highpc can either be an absolute value, or a delta relative to the
	// lowpc. We distinguish based on the field's class.
	var highpc uint64
	switch highPCField.Class {
	case dwarf.ClassAddress:
		highpc = highPCField.Val.(uint64)
	case dwarf.ClassConstant:
		highpc = lowpc + uint64(highPCField.Val.(int64))
	default:
		return Function{}, fmt.Errorf("unrecognized highpc class: %d for %s @ 0x%x", highPCField.Class, funcQualifiedName, entry.Offset)
	}

	lines, err := b.sym.FunctionLines(lowpc)
	if err != nil {
		return Function{}, fmt.Errorf("failed to resolve function lines for function %s at PC 0x%x: %w", funcQualifiedName, lowpc, err)
	}
	selfLines, ok := lines[funcQualifiedName]
	if !ok {
		return Function{}, fmt.Errorf("missing self function lines for function %s at PC 0x%x", funcQualifiedName, lowpc)
	}

	firstLine := uint32(0)
	maxLine := uint32(0)

	for _, lineRange := range selfLines.Lines {
		if firstLine == 0 {
			firstLine = lineRange.Line
		}
		if lineRange.Line > maxLine {
			maxLine = lineRange.Line
		}
	}

	// From now on, location lists that reference the current block will
	// reference this function.
	b.pushBlock(subprogBlock{lowpc: lowpc, highpc: highpc})
	defer b.popBlock()

	// Explore the function's body. Besides collecting the information that we
	// want to export to SymDB, this also has the side effect of resolving the
	// types of all the function arguments; in particular, we rely below on the
	// type of the receiver having been resolved.
	inner, err := b.exploreCode(reader, funcQualifiedName, lines)
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
				funcQualifiedName, typeQualifiedName, entry.Offset,
			)
		}
		typ.Methods = append(typ.Methods, res)
		// We don't return a Function for methods.
		return Function{}, nil
	}
	return res, nil
}

// Explores inlined instances of an abstract function (both InlinedSubroutines and out-of-line Subprogram instances).
func (b *packagesIterator) exploreInlinedInstance(
	entry *dwarf.Entry,
	reader *dwarf.Reader,
	lines map[string]gosym.FunctionLines,
) error {
	earlyExit := func() error {
		reader.SkipChildren()
		return nil
	}

	origin, ok := entry.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset)
	if !ok {
		return fmt.Errorf("inlined instance without abstract origin at 0x%x", entry.Offset)
	}

	if !b.options.IncludeInlinedFunctions {
		return earlyExit()
	}

	// Parse the abstract definition eagerly, and cache it.
	af, ok := b.abstractFunctions[origin]
	if !ok {
		var err error
		af, err = b.parseAbstractFunction(origin)
		if err != nil {
			return err
		}
		b.abstractFunctions[origin] = af
	}
	if !af.interesting {
		return earlyExit()
	}

	// Update file and endLine which are not present on abstract definition.
	selfLines, ok := lines[af.qualifiedName]
	if !ok {
		return fmt.Errorf("missing self function lines for function %s at PC 0x%x", af.qualifiedName, entry.Offset)
	}
	if af.file == "" {
		af.file = selfLines.File
	}
	for _, line := range selfLines.Lines {
		if line.Line > af.endLine {
			af.endLine = line.Line
		}
	}

	return b.exploreInlinedCode(entry, reader, lines, af)
}

func (b *packagesIterator) exploreInlinedCode(
	entry *dwarf.Entry,
	reader *dwarf.Reader,
	lines map[string]gosym.FunctionLines,
	af *abstractFunction,
) error {
	block := &dwarfBlock{entry: entry}
	b.pushBlock(block)
	defer b.popBlock()

	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return nil
		}
		if dwarfutil.IsEntryNull(child) {
			break // end of child nodes
		}

		switch child.Tag {
		case dwarf.TagFormalParameter, dwarf.TagVariable:
			origin, ok := child.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset)
			if !ok {
				// Inlined instance sometimes have non-abstract variables, like
				// named return parameters. We don't collect these currently.
				continue
			}
			av, ok := af.variables[origin]
			if !ok {
				return fmt.Errorf("inlined variable with unknown abstract origin at 0x%x", child.Offset)
			}
			av.AvailableLineRanges, err = b.parseVariableLocations(
				b.currentCompileUnit,
				b.currentBlock(),
				child,
				av.typeSize,
				lines[af.qualifiedName].Lines,
				av.AvailableLineRanges,
			)
			if err != nil {
				return fmt.Errorf("error parsing variable locations for inlined variable at 0x%x: %w", child.Offset, err)
			}

		case dwarf.TagLexDwarfBlock:
			err = b.exploreInlinedCode(child, reader, lines, af)
			if err != nil {
				return err
			}

		case dwarf.TagInlinedSubroutine:
			err := b.exploreInlinedInstance(child, reader, lines)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected child tag %s in inlined instance at 0x%x", child.Tag, child.Offset)
		}
	}
	return nil
}

// exploreLexicalBlock processes a lexical block entry. If the block contains
// any variables, it returns one Scope with those variables and any sub-blocks.
// If the block does not contain any variables, it returns any sub-blocks that
// do contain variables (if any).
func (b *packagesIterator) exploreLexicalBlock(
	blockEntry *dwarf.Entry,
	reader *dwarf.Reader,
	functionName string,
	lines map[string]gosym.FunctionLines,
) ([]Scope, error) {
	if blockEntry.Tag != dwarf.TagLexDwarfBlock {
		return nil, fmt.Errorf("expected TagLexDwarfBlock, got %s", blockEntry.Tag)
	}
	currentBlock := &dwarfBlock{entry: blockEntry}
	// From now on, location lists that reference the current block will
	// reference this block.
	b.pushBlock(currentBlock)
	defer b.popBlock()

	inner, err := b.exploreCode(reader, functionName, lines)
	if err != nil {
		return nil, fmt.Errorf("error exploring code in lexical block: %w", err)
	}

	// If the block has any variables, then we create a scope for it. If it
	// doesn't, then inner scopes (if any), are returned directly, to be added
	// as direct children of the caller's block.
	if len(inner.vars) != 0 {
		blockLineRange, err := currentBlock.resolveLines(b.dwarfData, lines[functionName].Lines)
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

func pcRangeToLines(r dwarfutil.PCRange, lines []gosym.LineRange) (LineRange, bool) {
	// Heuristic - assume that we are going to instrument each beginning of a pc range.
	// Collect all lines where that beginning falls into the given pc range.
	i := sort.Search(len(lines), func(i int) bool {
		return lines[i].PCLo >= r[0]
	})
	lineLo := uint32(0)
	lineHi := uint32(0)
	for i < len(lines) && lines[i].PCLo <= r[1] {
		if lineLo == 0 || lines[i].Line < lineLo {
			lineLo = lines[i].Line
		}
		if lineHi == 0 || lines[i].Line > lineHi {
			lineHi = lines[i].Line
		}
		i++
	}
	if lineLo == 0 {
		return LineRange{}, false
	}
	return LineRange{int(lineLo), int(lineHi)}, true
}

// exploreVariable processes a variable or formal parameter entry.
func (b *packagesIterator) exploreVariable(entry *dwarf.Entry, lines []gosym.LineRange) (Variable, error) {
	v, typ, err := b.parseAbstractVariable(entry)
	if err != nil {
		return Variable{}, err
	}
	availableLineRanges, err := b.parseVariableLocations(
		b.currentCompileUnit,
		b.currentBlock(),
		entry,
		uint32(typ.size),
		lines,
		nil,
	)
	if err != nil {
		return Variable{}, err
	}
	v.AvailableLineRanges = coalesceLineRanges(availableLineRanges)
	return v, nil
}

func (b *packagesIterator) parseFunctionName(entry *dwarf.Entry) (
	qualifiedName string,
	parsedName funcName,
	recognized bool,
	err error,
) {
	var ok bool
	qualifiedName, ok = entry.Val(dwarf.AttrName).(string)
	if !ok {
		err = fmt.Errorf("subprogram without name at 0x%x (%s)", entry.Offset, entry.Tag)
		return
	}
	parseRes, err := parseFuncName(qualifiedName)
	if err != nil {
		err = nil
		// Note that we do not return an error here; we simply ignore the
		// function so that we don't choke on weird function names that we
		// haven't anticipated.
		// TODO: log the error, but don't spam the logs.
		return
	}
	if parseRes.failureReason != parseFuncNameFailureReasonUndefined {
		return
	}
	parsedName = parseRes.funcName
	recognized = true
	return
}

func (b *packagesIterator) parseAbstractFunction(offset dwarf.Offset) (*abstractFunction, error) {
	// TODO: once we switch to Go 1.25, instead of constructing a new Reader, we
	// should take one in and Seek() to the desired offset; that would be more
	// efficient when seeking within the same compilation unit as the one we're
	// already in. Unfortunately, seeking across compilation units is broken
	// until Go 1.25 (see https://go-review.googlesource.com/c/go/+/655976).
	reader := b.dwarfData.Reader()
	reader.Seek(offset)
	entry, err := reader.Next()
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("expected abstract function entry at 0x%x, got nil", offset)
	}

	funcQualifiedName, funcName, recognized, err := b.parseFunctionName(entry)
	if err != nil {
		return nil, err
	}
	if !recognized || !interestingPackage(funcName.Package, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
		return &abstractFunction{
			interesting: false,
		}, nil
	}

	var receiver string
	if funcName.Type != "" {
		receiver = funcName.Package + "." + funcName.Type
	}

	startLine := int(entry.Val(dwarf.AttrDeclLine).(int64))

	variables := make(map[dwarf.Offset]*abstractVariable)
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return nil, err
		}
		if dwarfutil.IsEntryNull(child) {
			break // end of child nodes
		}

		switch child.Tag {
		case dwarf.TagFormalParameter, dwarf.TagVariable:
			v, typ, err := b.parseAbstractVariable(child)
			if err != nil {
				return nil, err
			}
			variables[child.Offset] = &abstractVariable{
				Variable: v,
				typeSize: uint32(typ.size),
			}
		default:
			return nil, fmt.Errorf("unexpected child tag %s in abstract function at 0x%x", child.Tag, child.Offset)
		}
	}

	return &abstractFunction{
		interesting:   true,
		pkg:           funcName.Package,
		name:          funcName.Name,
		receiver:      receiver,
		qualifiedName: funcQualifiedName,
		startLine:     startLine,
		variables:     variables,
	}, nil
}

func (b *packagesIterator) parseAbstractVariable(entry *dwarf.Entry) (Variable, typeInfo, error) {
	name, ok := entry.Val(dwarf.AttrName).(string)
	if !ok {
		debug.PrintStack()
		return Variable{}, typeInfo{}, fmt.Errorf("variable without name at 0x%x", entry.Offset)
	}
	declLine, ok := entry.Val(dwarf.AttrDeclLine).(int64)
	if !ok {
		declLine = 0
	}
	typeOffset, ok := entry.Val(dwarf.AttrType).(dwarf.Offset)
	if !ok {
		return Variable{}, typeInfo{}, fmt.Errorf("variable without type at 0x%x", entry.Offset)
	}
	typ, err := b.resolveType(typeOffset)
	if err != nil {
		return Variable{}, typeInfo{}, err
	}

	// Decide whether this variable is a function argument or a regular variable.
	// Note that return values are represented DW_TAG_formal_parameter with
	// attribute DW_AT_variable_parameter = 1. We don't mark return values as
	// function arguments.
	functionArgument := entry.Tag == dwarf.TagFormalParameter
	if functionArgument {
		if varParamField := entry.AttrField(dwarf.AttrVarParam); varParamField != nil {
			varParamValue, ok := varParamField.Val.(int)
			if ok && varParamValue == 1 {
				functionArgument = false
			}
		}
	}

	return Variable{
		Name:             name,
		DeclLine:         int(declLine),
		TypeName:         typ.name,
		FunctionArgument: functionArgument,
	}, typ, nil
}

// The parsed locations are appended to the out slice, that is then returned.
func (b *packagesIterator) parseVariableLocations(
	unit *dwarf.Entry,
	block codeBlock,
	entry *dwarf.Entry,
	typeSize uint32,
	lines []gosym.LineRange,
	out []LineRange,
) ([]LineRange, error) {
	locField := entry.AttrField(dwarf.AttrLocation)
	if locField == nil {
		return out, nil
	}
	pcRanges, err := b.processLocations(unit, block, entry.Offset, locField, typeSize)
	if err != nil {
		return nil, fmt.Errorf(
			"error processing locations for variable at 0x%x: %w", entry.Offset, err,
		)
	}
	for _, r := range pcRanges {
		lineRange, ok := pcRangeToLines(r, lines)
		if ok {
			out = append(out, lineRange)
		}
	}
	return out, nil
}

// Coalesce the available lines into disjoint ranges.
func coalesceLineRanges(ranges []LineRange) []LineRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i][0] < ranges[j][0]
	})
	merged := make([]LineRange, 0, len(ranges))
	merged = append(merged, ranges[0])
	for _, curr := range ranges[1:] {
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
	return merged
}

// processLocations goes through a list of location lists and returns the PC
// ranges for which the whole variable is available. Ranges for which the
// variable is only partially available are ignored.
//
// totalSize is the size of the type that this location list is describing.
func (b *packagesIterator) processLocations(
	unit *dwarf.Entry,
	block codeBlock,
	entryOffset dwarf.Offset,
	locField *dwarf.Field,
	totalSize uint32,
) ([]dwarfutil.PCRange, error) {
	// TODO: resolving these PC ranges is only necessary for variables that use
	// dwarf.ClassExprLoc; it's not necessary for other variables. It might be
	// worth it to make the computation lazy.
	pcRanges, err := block.resolvePCRanges(b.dwarfData)
	if err != nil {
		return nil, err
	}
	loclists, err := dwarfutil.ProcessLocations(locField, unit, b.loclistReader, pcRanges, totalSize, uint8(b.pointerSize))
	if err != nil {
		// Do not fail hard, just pretend the variable is not available.
		if loclistErrorLogLimiter.Allow() {
			log.Warnf(
				"ignoring locations for variable at 0x%x: %v", entryOffset, err,
			)
		}
		return nil, nil
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
func (b *packagesIterator) exploreCode(
	reader *dwarf.Reader,
	functionName string,
	lines map[string]gosym.FunctionLines,
) (exploreCodeResult, error) {
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
			v, err := b.exploreVariable(child, lines[functionName].Lines)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.vars = append(res.vars, v)
		case dwarf.TagVariable:
			v, err := b.exploreVariable(child, lines[functionName].Lines)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.vars = append(res.vars, v)
		case dwarf.TagLexDwarfBlock:
			scopes, err := b.exploreLexicalBlock(child, reader, functionName, lines)
			if err != nil {
				return exploreCodeResult{}, err
			}
			res.scopes = append(res.scopes, scopes...)
		case dwarf.TagInlinedSubroutine:
			err := b.exploreInlinedInstance(child, reader, lines)
			if err != nil {
				return exploreCodeResult{}, err
			}
		}
	}
	return res, nil
}
