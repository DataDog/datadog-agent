// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package symdb

import (
	"cmp"
	"debug/buildinfo"
	"debug/dwarf"
	"errors"
	"fmt"
	"iter"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-delve/delve/pkg/dwarf/godwarf"
	"golang.org/x/time/rate"

	dwarf2 "github.com/DataDog/datadog-agent/pkg/dyninst/dwarf"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosym"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var loclistErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 1)

// PackagesIterator returns an iterator over the packages in the binary.
//
// The last package yielded by the iterator has its Final field set to true. No
// more packages will be yielded after that, but an error may still be yielded.
//
// PackagesIterator can only be used if the packagesIterator was configured with
// ExtractOptions.AccumulateInlineInfoAcrossCompileUnits=false.
func PackagesIterator(binaryPath string, loader object.Loader, opt ExtractOptions) (iter.Seq2[PackageWithFinal, error], error) {
	bin, err := openBinary(binaryPath, loader, opt)
	if err != nil {
		return nil, err
	}

	b := newPackagesIterator(bin, opt)
	return b.iterator(), nil
}

// ExtractSymbols walks the DWARF data and accumulates the symbols to send to
// SymDB.
func ExtractSymbols(binaryPath string, loader object.Loader, opt ExtractOptions) (Symbols, error) {
	bin, err := openBinary(binaryPath, loader, opt)
	if err != nil {
		return Symbols{}, err
	}
	b := newPackagesIterator(bin, opt)

	packages := make(map[string]*Package)
	it := b.iterator()
	for pkg, err := range it {
		if err != nil {
			return Symbols{}, err
		}
		if existingPkg, ok := packages[pkg.Name]; ok {
			existingPkg.Functions = append(existingPkg.Functions, pkg.Functions...)
			for name, t := range pkg.Types {
				existingPkg.Types[name] = t
			}
		} else {
			packages[pkg.Name] = &pkg.Package
		}
	}

	res := Symbols{
		MainModule: b.mainModule,
		Packages:   nil,
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
	// The types in the package, indexed by fully-qualified name.
	Types map[string]*Type
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
	// Source code lines suitable for line probing.
	InjectibleLines []LineRange
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
func (p *Package) Serialize(w StringWriter) {
	w.WriteString("Package: ")
	w.WriteString(p.Name)
	w.WriteString("\n")
	// Serialize functions sorted by name for stable output.
	sort.Slice(p.Functions, func(i, j int) bool { return p.Functions[i].Name < p.Functions[j].Name })
	for _, fn := range p.Functions {
		fn.Serialize(w, "\t")
	}
	// Serialize types sorted by name for stable output.
	typeNames := make([]string, 0, len(p.Types))
	for name := range p.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		p.Types[name].Serialize(w, "\t")
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
	w.WriteString(" injectible: ")
	for i, r := range f.InjectibleLines {
		if i > 0 {
			w.WriteString(", ")
		}
		w.WriteString(fmt.Sprintf("[%d-%d]", r[0], r[1]))
	}
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
	w.WriteString(strconv.Itoa(v.DeclLine))
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
	currentCompileUnit compileUnitInfo

	abstractFunctions map[dwarf.Offset]*abstractFunction

	// displacedFunctions accumulates functions that were found in a compile
	// unit different from where they are defined (e.g. generic shape functions
	// instantiated in a different package). Keyed by the function's actual
	// package name.
	displacedFunctions map[string]*Package

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

	// Information about compile units, indexed by the unit's offset (the offset
	// of the corresponding DIE).
	offsetToUnit map[dwarf.Offset]dwarfutil.CompileUnitHeader
	// sortedCUOffsets is the sorted slice of CU offsets from offsetToUnit,
	// used for binary search to find which CU contains a given DWARF offset.
	sortedCUOffsets []dwarf.Offset

	// genericIndex maps canonicalized qualified names to DWARF offsets for
	// generic shape functions found across all compile units. Built during a
	// lightweight pre-pass in buildGenericIndex().
	genericIndex genericFuncIndex
	// genericTypeIndex maps canonicalized qualified type names to DWARF offsets
	// for generic struct type instantiations. Used to populate fields on
	// displaced generic types.
	genericTypeIndex genericFuncIndex
	// cuContextCache caches CU context (file table, etc.) for foreign compile
	// units accessed when processing displaced generic functions.
	cuContextCache map[dwarf.Offset]*cachedCUContext
}

type compileUnitInfo struct {
	entry *dwarf.Entry
	name  string
	// The length of the compile unit, not including the header.
	length uint64
	files  []string

	// The Package being constructed based on the current compile unit's data.
	outputPkg *Package
}

// cachedCUContext stores a compile unit's context for processing displaced
// generic functions that live in a foreign CU.
type cachedCUContext struct {
	entry  *dwarf.Entry
	name   string
	length uint64
	files  []string
}

// hasFunctionQName returns true if the compile unit's output package already
// has a function with the given qualified name. Used to dedup generic shape
// instantiations that canonicalize to the same name.
func (c *compileUnitInfo) hasFunctionQName(qname string) bool {
	return slices.ContainsFunc(c.outputPkg.Functions, func(f Function) bool {
		return f.QualifiedName == qname
	})
}

// addFunctionToPackage adds a function to the correct package. If the
// function's package (extracted from its qualified name) matches the current
// compile unit, it goes into the compile unit's output package. Otherwise it
// is accumulated in displacedFunctions for later yielding under the correct
// package name. This handles generic shape functions that the Go compiler
// places in the instantiating package's compile unit rather than the defining
// package's compile unit.
func (b *packagesIterator) addFunctionToPackage(function Function) {
	// Extract the function's package from its qualified name.
	sym := gosymname.Parse(function.QualifiedName, gosymname.SourceDWARF)
	funcPkg := sym.Package()

	if funcPkg == "" || funcPkg == b.currentCompileUnit.name {
		b.currentCompileUnit.outputPkg.Functions = append(
			b.currentCompileUnit.outputPkg.Functions, function)
		return
	}

	// Function belongs to a different package. If it's a canonicalized
	// generic (contains "[...]"), the generic index will handle it via
	// augmentWithDisplacedGenerics — skip accumulating it here.
	if b.genericIndex != nil && strings.Contains(function.QualifiedName, "[...]") {
		return
	}

	// Non-generic function from a different package — accumulate it in
	// displacedFunctions for merging when that package's CU is processed.
	pkg, ok := b.displacedFunctions[funcPkg]
	if !ok {
		pkg = &Package{
			Name:  funcPkg,
			Types: make(map[string]*Type),
		}
		b.displacedFunctions[funcPkg] = pkg
	}
	// Dedup by qualified name.
	if !slices.ContainsFunc(pkg.Functions, func(f Function) bool {
		return f.QualifiedName == function.QualifiedName
	}) {
		pkg.Functions = append(pkg.Functions, function)
	}
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
	file            string
	endLine         uint32
	injectibleLines []LineRange

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

	if err := b.currentCompileUnit.outputPkg.maybeAddType(typ); err != nil {
		return typeInfo{}, err
	}

	return typeInfo{
		name: typeName,
		size: int(size),
	}, nil
}

// maybeAddType adds a type to the collection if it is not already present and
// if the type belongs to the package. Unsupported types are ignored and no
// error is returned.
func (p *Package) maybeAddType(t godwarf.Type) error {
	pkg, sym, wasEscaped, err := parseLinkFuncName(t.Common().Name)
	if err != nil {
		return fmt.Errorf("failed to split package for %s : %w", t.Common().Name, err)
	}

	// Ignore types from other packages.
	if pkg != p.Name {
		return nil
	}

	var unescapedName string
	if wasEscaped {
		unescapedName = pkg + "." + sym
	} else {
		unescapedName = t.Common().Name
	}

	canonicalName := gosymname.CanonicalizeGenerics(unescapedName)

	if existing, ok := p.Types[canonicalName]; ok {
		if s, isStruct := t.(*godwarf.StructType); isStruct {
			newFields := structFields(s)
			if len(existing.Fields) == 0 {
				// Type was created bare (e.g. by method-receiver fallback).
				existing.Fields = newFields
			} else {
				// Merge: compare fields from this instantiation with existing.
				// Any field whose type differs is a type-parameter-dependent
				// field — mark it with a generic placeholder.
				mergeStructFields(existing.Fields, newFields)
			}
		}
		return nil
	}

	// Skip parametric types (uninstantiated generics with type params).
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

	// Skip anonymous types, array types, and structs corresponding to slices.
	if strings.ContainsAny(unescapedName, "{<") {
		return nil
	}

	typ := &Type{
		Name:   canonicalName,
		Fields: nil,
		// Methods will be populated later, as we discover them in DWARF.
		Methods: nil,
	}
	if s, isStruct := t.(*godwarf.StructType); isStruct {
		typ.Fields = structFields(s)
	}

	p.Types[canonicalName] = typ
	return nil
}

// structFields extracts fields from a DWARF struct type, canonicalizing
// generic type names in field types so that fields from a specific
// instantiation (e.g. lib.Box[float64]) show as lib.Box[...].
func structFields(s *godwarf.StructType) []Field {
	fields := make([]Field, 0, len(s.Field))
	for _, field := range s.Field {
		fieldType := field.Type.Common().Name
		// Only canonicalize names containing generic shape instantiations.
		// DWARF type names can also contain brackets for slices ([]T),
		// arrays ([N]T), and maps (map[K]V) which should not be touched.
		if strings.Contains(fieldType, "[go.shape.") {
			fieldType = gosymname.CanonicalizeGenerics(fieldType)
		}
		fields = append(fields, Field{
			Name: field.Name,
			Type: fieldType,
		})
	}
	return fields
}

// mergeStructFields compares two field lists from different instantiations of
// the same generic type. For each field, if the type differs between
// instantiations, the field type is replaced with a generic placeholder.
// This lets us deduce which fields are type-parameter-dependent by observing
// multiple concrete instantiations.
func mergeStructFields(existing, incoming []Field) {
	n := min(len(existing), len(incoming))
	for i := 0; i < n; i++ {
		if existing[i].Type == genericFieldType {
			continue // already marked as generic
		}
		if existing[i].Type != incoming[i].Type {
			existing[i].Type = genericFieldType
		}
	}
}

// genericFieldType is the placeholder used for struct fields whose type
// varies across instantiations, indicating the field depends on a type
// parameter.
const genericFieldType = "<T>"

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
		pointerSize:         int(bin.obj.Architecture().PointerSize()),
		options:             opt,
		mainModule:          bin.mainModule,
		firstPartyPkgPrefix: bin.firstPartyPkgPrefix,
		filesFilter:         bin.filesFilter,
		abstractFunctions:   make(map[dwarf.Offset]*abstractFunction),
		displacedFunctions:  make(map[string]*Package),
		typesCache:          dwarfutils.NewTypeFinder(bin.obj.DwarfData()),
		types: typesCollection{
			scopeFilter:         opt.Scope,
			mainModule:          bin.mainModule,
			firstPartyPkgPrefix: bin.firstPartyPkgPrefix,
		},
		cleanupFuncs:   []func(){func() { _ = bin.goDebugSections.Close() }, func() { _ = bin.obj.Close() }},
		offsetToUnit:   make(map[dwarf.Offset]dwarfutil.CompileUnitHeader), // filled in below
		cuContextCache: make(map[dwarf.Offset]*cachedCUContext),
	}
	headers := bin.obj.UnitHeaders()
	b.sortedCUOffsets = make([]dwarf.Offset, 0, len(headers))
	for _, h := range headers {
		b.offsetToUnit[h.Offset] = h
		b.sortedCUOffsets = append(b.sortedCUOffsets, h.Offset)
	}
	slices.Sort(b.sortedCUOffsets)
	return b
}

type binaryInfo struct {
	obj                 object.FileWithDwarf
	mainModule          string
	goDebugSections     *object.GoDebugSections
	symTable            *gosym.GoSymbolTable
	firstPartyPkgPrefix string
	filesFilter         []string
}

func openBinary(binaryPath string, loader object.Loader, opt ExtractOptions) (binaryInfo, error) {
	obj, err := loader.Load(binaryPath)
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

type PackageWithFinal struct {
	Package
	// Final is set if this is the last package in the binary.
	Final bool
}

// iterator returns a Go iterator that yields packages one by one. The returned
// iterator takes ownership of the Elf file, so it must be called (or used in a
// range loop) in order to eventually release resources.
//
// Packages with no types or functions are not yielded.
//
// For simplicity, the iterator never returns both a package and an error.
//
// The implementation wraps innerIterator() and yields packages with a delay of
// one so that it can set the Final field of the last package.
func (b *packagesIterator) iterator() iter.Seq2[PackageWithFinal, error] {
	return func(yield func(pkg PackageWithFinal, err error) bool) {
		var prev *Package
		var iterErr error
		var stopped bool
		// Call innerIterator's closure directly instead of using range-over-func
		// to avoid nested range-over-func, which can panic when the consumer
		// breaks out of the outer range loop mid-iteration (Go runtime issue
		// with nested iterator coroutine teardown).
		b.innerIterator()(func(pkg Package, err error) bool {
			if err != nil {
				iterErr = err
				return false
			}
			if prev != nil {
				if !yield(PackageWithFinal{Package: *prev, Final: false}, nil) {
					stopped = true
					return false
				}
			}
			prev = &pkg
			return true
		})
		if stopped {
			return
		}
		if prev != nil {
			if !yield(PackageWithFinal{
				Package: *prev,
				// NOTE: We set the Final field even if the iteration terminated
				// because of an error.
				Final: true,
			}, nil) {
				return
			}
		}
		if iterErr != nil {
			yield(PackageWithFinal{}, iterErr)
		}
	}
}

// iterator returns a Go iterator that yields packages one by one. The returned
// iterator takes ownership of the Elf file, so it must be called (or used in a
// range loop) in order to eventually release resources.
//
// Packages with no types or functions are not yielded.
//
// For simplicity, the iterator never returns both a package and an error.
func (b *packagesIterator) innerIterator() iter.Seq2[Package, error] {
	var err error
	// Keep track of packages we've seen so that we ignore compile units
	// belonging to packages we've already yielded. This happens for packages
	// that have assembly sources: each assembly file gets its own compile unit,
	// and they all have the same name (the name of the Go package). In these
	// cases, the iterator only yields the first compile unit for each package
	// name; empirically the first unit corresponds to the non-assembly sources.
	seenPackages := make(map[string]struct{})
	return func(yield func(pkg Package, err error) bool) {
		defer b.close()

		// Build generic indexes via a lightweight pre-scan of all DWARF
		// compile units. These indexes let us find displaced generic shape
		// functions and struct types regardless of CU ordering.
		b.genericIndex, b.genericTypeIndex, err = b.buildGenericIndexes()
		if err != nil {
			yield(Package{}, fmt.Errorf("failed to build generic indexes: %w", err))
			return
		}
		defer func() {
			if b.genericIndex != nil {
				_ = b.genericIndex.Close()
				b.genericIndex = nil
			}
			if b.genericTypeIndex != nil {
				_ = b.genericTypeIndex.Close()
				b.genericTypeIndex = nil
			}
		}()

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

			var pkg *Package
			pkg, err = b.exploreCompileUnit(entry, entryReader)
			if err != nil {
				break
			}
			if pkg == nil {
				continue
			}
			if _, ok := seenPackages[pkg.Name]; ok {
				continue
			}

			// Move all accumulated abstract functions to the output package.
			b.addAbstractFunctions(pkg)

			// Augment the package with displaced generic shape functions
			// found in other compile units via the pre-built index.
			unitHeader := b.offsetToUnit[entry.Offset]
			if err = b.augmentWithDisplacedGenerics(pkg, entry.Offset, unitHeader.Length); err != nil {
				break
			}

			// Merge in any displaced functions that belong to this package.
			// This handles non-generic abstract functions from other CUs.
			if displaced, ok := b.displacedFunctions[pkg.Name]; ok {
				pkg.Functions = append(pkg.Functions, displaced.Functions...)
				delete(b.displacedFunctions, pkg.Name)
			}

			// Yield the package if it's not empty.
			pkgEmpty := len(pkg.Functions) == 0 && len(pkg.Types) == 0
			if !pkgEmpty {
				if !yield(*pkg, nil /* error */) {
					return
				}
				seenPackages[pkg.Name] = struct{}{}
			}
		}
		if err != nil {
			yield(Package{}, err)
			return
		}

		// Yield packages for functions that were found in a compile unit
		// different from their defining package (e.g. abstract inlined
		// functions from other packages).
		for _, pkg := range b.displacedFunctions {
			if len(pkg.Functions) > 0 {
				if !yield(*pkg, nil) {
					return
				}
			}
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

// findCUForOffset returns the CU offset of the compile unit that contains the
// given DWARF offset. It binary searches the sorted CU offsets for the largest
// CU offset ≤ the given offset.
func (b *packagesIterator) findCUForOffset(offset dwarf.Offset) (dwarf.Offset, bool) {
	// Find the rightmost CU offset that is ≤ offset.
	i, _ := slices.BinarySearch(b.sortedCUOffsets, offset)
	// BinarySearch returns the insertion point. If offset matches exactly, i
	// points at it. Otherwise i points one past the last element < offset.
	if i < len(b.sortedCUOffsets) && b.sortedCUOffsets[i] == offset {
		return b.sortedCUOffsets[i], true
	}
	if i == 0 {
		return 0, false
	}
	return b.sortedCUOffsets[i-1], true
}

// getCUContext returns the cached CU context for the given CU offset, building
// it if necessary by seeking to the CU and reading its file table.
func (b *packagesIterator) getCUContext(cuOffset dwarf.Offset) (*cachedCUContext, error) {
	if ctx, ok := b.cuContextCache[cuOffset]; ok {
		return ctx, nil
	}

	reader := b.dwarfData.Reader()
	reader.Seek(cuOffset)
	entry, err := reader.Next()
	if err != nil {
		return nil, fmt.Errorf("failed to read CU entry at 0x%x: %w", cuOffset, err)
	}
	if entry == nil || entry.Tag != dwarf.TagCompileUnit {
		return nil, fmt.Errorf("expected TagCompileUnit at 0x%x, got %v", cuOffset, entry)
	}

	name, _ := entry.Val(dwarf.AttrName).(string)

	unitHeader, ok := b.offsetToUnit[cuOffset]
	if !ok {
		return nil, fmt.Errorf("header missing for CU at 0x%x", cuOffset)
	}

	cuLineReader, err := b.dwarfData.LineReader(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to get line reader for CU at 0x%x: %w", cuOffset, err)
	}
	var files []string
	if cuLineReader != nil {
		files = make([]string, 0, len(cuLineReader.Files()))
		for i, file := range cuLineReader.Files() {
			if file == nil {
				if i != 0 {
					return nil, fmt.Errorf("CU at 0x%x has invalid nil file entry at index %d", cuOffset, i)
				}
				files = append(files, "")
				continue
			}
			files = append(files, file.Name)
		}
	}

	ctx := &cachedCUContext{
		entry:  entry,
		name:   name,
		length: unitHeader.Length,
		files:  files,
	}
	b.cuContextCache[cuOffset] = ctx
	return ctx, nil
}

// buildGenericIndexes performs a lightweight pre-scan of all DWARF compile
// units, recording the canonicalized qualified name and DWARF offset of every
// generic shape function and struct type. Returns sorted indexes for
// prefix-based package lookup.
func (b *packagesIterator) buildGenericIndexes() (funcIdx genericFuncIndex, typeIdx genericFuncIndex, retErr error) {
	newBuilder := func(suffix string) (genericFuncIndexBuilder, error) {
		if b.options.DiskCache != nil {
			return newOnDiskGenericFuncIndexBuilder(b.options.DiskCache, suffix)
		}
		return &inMemGenericFuncIndexBuilder{}, nil
	}

	funcBuilder, err := newBuilder("funcs")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create generic func index builder: %w", err)
	}
	defer funcBuilder.Close()

	typeBuilder, err := newBuilder("types")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create generic type index builder: %w", err)
	}
	defer typeBuilder.Close()

	reader := b.dwarfData.Reader()
	for {
		entry, err := reader.Next()
		if err != nil {
			return nil, nil, fmt.Errorf("generic index pre-scan: %w", err)
		}
		if entry == nil {
			break
		}

		switch entry.Tag {
		case dwarf.TagCompileUnit:
			// Process children of all CUs — don't skip any.
			continue

		case dwarf.TagSubprogram:
			// Skip trampolines.
			if entry.AttrField(dwarf.AttrTrampoline) != nil {
				reader.SkipChildren()
				continue
			}

			name, ok := entry.Val(dwarf.AttrName).(string)
			if !ok || !strings.Contains(name, "[go.shape.") {
				reader.SkipChildren()
				continue
			}

			// Parse the function's package from its qualified name.
			result, err := parseFuncName(name)
			if err != nil || result.failureReason != parseFuncNameFailureReasonUndefined {
				reader.SkipChildren()
				continue
			}

			pkg := result.funcName.Package
			if !interestingPackage(pkg, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
				reader.SkipChildren()
				continue
			}

			// parseFuncName already canonicalizes generics in QualifiedName.
			if err := funcBuilder.add(result.funcName.QualifiedName, entry.Offset); err != nil {
				return nil, nil, err
			}
			reader.SkipChildren()

		case dwarf.TagStructType:
			name, ok := entry.Val(dwarf.AttrName).(string)
			if !ok || !strings.Contains(name, "[go.shape.") {
				reader.SkipChildren()
				continue
			}

			// Extract the package from the struct type name.
			typePkg, _, _, err := parseLinkFuncName(name)
			if err != nil || typePkg == "" {
				reader.SkipChildren()
				continue
			}
			if !interestingPackage(typePkg, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
				reader.SkipChildren()
				continue
			}

			canonicalName := gosymname.CanonicalizeGenerics(name)
			// Unescape the package path in the canonical name.
			if unescaped, err := unescapeSymbol(canonicalName); err == nil {
				canonicalName = unescaped
			}
			if err := typeBuilder.add(canonicalName, entry.Offset); err != nil {
				return nil, nil, err
			}
			reader.SkipChildren()

		default:
			reader.SkipChildren()
		}
	}

	funcIdx, err = funcBuilder.build()
	if err != nil {
		return nil, nil, err
	}
	typeIdx, err = typeBuilder.build()
	if err != nil {
		_ = funcIdx.Close()
		return nil, nil, err
	}
	return funcIdx, typeIdx, nil
}

// augmentWithDisplacedGenerics queries the generic index for all shape
// functions belonging to pkg and processes any that weren't already found in
// the compile unit identified by cuOffset/cuLength.
func (b *packagesIterator) augmentWithDisplacedGenerics(pkg *Package, cuOffset dwarf.Offset, cuLength uint64) error {
	if b.genericIndex == nil {
		return nil
	}

	cuEnd := uint64(cuOffset) + cuLength

	for name, funcOffset := range b.genericIndex.forPackage(pkg.Name) {
		// Skip functions that are within the current CU — they were already
		// processed during normal exploration.
		if uint64(funcOffset) >= uint64(cuOffset) && uint64(funcOffset) < cuEnd {
			continue
		}

		// Skip if the package already has this function.
		if packageHasFunctionQName(pkg, name) {
			continue
		}

		// Find the CU containing this function.
		foreignCUOffset, ok := b.findCUForOffset(funcOffset)
		if !ok {
			continue
		}

		// Get or build the foreign CU context.
		foreignCU, err := b.getCUContext(foreignCUOffset)
		if err != nil {
			return fmt.Errorf("augmentWithDisplacedGenerics: %w", err)
		}

		// Save the current CU context and set up the foreign one.
		savedCU := b.currentCompileUnit
		b.currentCompileUnit = compileUnitInfo{
			entry:     foreignCU.entry,
			name:      foreignCU.name,
			length:    foreignCU.length,
			files:     foreignCU.files,
			outputPkg: pkg, // methods land in the target package's types
		}

		// Seek to the function and process it.
		funcReader := b.dwarfData.Reader()
		funcReader.Seek(funcOffset)
		funcEntry, err := funcReader.Next()
		if err != nil {
			b.currentCompileUnit = savedCU
			return fmt.Errorf("augmentWithDisplacedGenerics: failed to read entry at 0x%x: %w", funcOffset, err)
		}

		function, err := b.exploreSubprogram(funcEntry, funcReader)

		// Restore the original CU context.
		b.currentCompileUnit = savedCU

		if err != nil {
			return fmt.Errorf("augmentWithDisplacedGenerics: %w", err)
		}

		// Methods return an empty Function since they're added to Types
		// inside exploreSubprogram. Freestanding functions are returned.
		if !function.empty() && !packageHasFunctionQName(pkg, function.QualifiedName) {
			pkg.Functions = append(pkg.Functions, function)
		}
	}

	// Process displaced generic struct types. Multiple instantiations of
	// the same type (e.g. lib.Box[go.shape.int] and lib.Box[go.shape.float64])
	// are compared field-by-field in maybeAddType to deduce which fields are
	// type-parameter-dependent.
	if b.genericTypeIndex != nil {
		for _, typeOffset := range b.genericTypeIndex.forPackage(pkg.Name) {
			// Skip types within the current CU.
			if uint64(typeOffset) >= uint64(cuOffset) && uint64(typeOffset) < cuEnd {
				continue
			}

			// Resolve the DWARF type and call maybeAddType which will either
			// create the type, populate fields on a bare type, or merge
			// fields from a different instantiation.
			typ, err := b.typesCache.FindTypeByOffset(typeOffset)
			if err != nil {
				continue // type resolution can fail for unsupported types
			}
			if err := pkg.maybeAddType(typ); err != nil {
				return fmt.Errorf("augmentWithDisplacedGenerics type: %w", err)
			}
		}
	}

	return nil
}

// packageHasFunctionQName returns true if the package already has a function
// with the given qualified name.
func packageHasFunctionQName(pkg *Package, qname string) bool {
	return slices.ContainsFunc(pkg.Functions, func(f Function) bool {
		return f.QualifiedName == qname
	})
}

// addAbstractFunctions takes the aggregated data about inlined functions
// accumulated in b.abstractFunctions and adds the functions to targetPackage
// and methods to types in `b.types`. `b.abstractFunctions` is reset.
//
// targetPackage is the package in which all freestanding abstract functions
// will go, regardless of the package they really belong to. Abstract *methods*
// that don't belong to this single package are ignored: methods are added to
// types, and we only ever report a type in the package that it belongs to -- we
// don't move types to the a different package as we do with freestanding
// functions. That's because types might not be complete yet (they might have
// methods that can only be discovered when exploring other packages in which
// they were inlined) and we don't want to report incomplete types or report a
// type multiple times.
func (b *packagesIterator) addAbstractFunctions(targetPackage *Package) {
	// Sort abstract functions so that output is stable.
	abstractFunctions := make([]*abstractFunction, 0, len(b.abstractFunctions))
	for _, af := range b.abstractFunctions {
		if !af.interesting {
			continue
		}
		abstractFunctions = append(abstractFunctions, af)
	}
	// Reset the map.
	clear(b.abstractFunctions)
	sort.Slice(abstractFunctions, func(i, j int) bool {
		return abstractFunctions[i].name < abstractFunctions[j].name
	})
	for _, af := range abstractFunctions {
		// Ignore methods that don't belong to the target package; see function
		// comment.
		if af.pkg != targetPackage.Name && af.receiver != "" {
			continue
		}

		variables := make([]Variable, 0, len(af.variables))
		for _, v := range af.variables {
			v.Variable.AvailableLineRanges = coalesceLineRanges(v.AvailableLineRanges)
			variables = append(variables, v.Variable)
		}
		// Sort variables so that output is stable.
		sort.Slice(variables, func(i, j int) bool {
			if variables[i].Name != variables[j].Name {
				return variables[i].Name < variables[j].Name
			}
			return variables[i].DeclLine < variables[j].DeclLine
		})
		f := Function{
			Name:            af.name,
			QualifiedName:   af.qualifiedName,
			File:            af.file,
			InjectibleLines: af.injectibleLines,
			Scope: Scope{
				StartLine: af.startLine,
				EndLine:   int(af.endLine),
				Scopes:    nil,
				Variables: variables,
			},
		}
		if af.receiver != "" {
			t, ok := targetPackage.Types[af.receiver]
			if !ok {
				// Some types are empty structures, and functions that
				// use them as receivers don't actually have a parameter
				// of the receiver type. Thus, we end up without a type.
				// Just make one up.
				t = &Type{
					Name: af.receiver,
				}
				targetPackage.Types[af.receiver] = t
			}
			t.Methods = append(t.Methods, f)
		} else if af.pkg != targetPackage.Name {
			// Freestanding function from a different package. If it's a
			// canonicalized generic, the generic index handles it — skip.
			if b.genericIndex != nil && strings.Contains(f.QualifiedName, "[...]") {
				continue
			}
			// Non-generic displaced function: route to displacedFunctions.
			pkg, ok := b.displacedFunctions[af.pkg]
			if !ok {
				pkg = &Package{
					Name:  af.pkg,
					Types: make(map[string]*Type),
				}
				b.displacedFunctions[af.pkg] = pkg
			}
			if !slices.ContainsFunc(pkg.Functions, func(existing Function) bool {
				return existing.QualifiedName == f.QualifiedName
			}) {
				pkg.Functions = append(pkg.Functions, f)
			}
		} else {
			targetPackage.Functions = append(targetPackage.Functions, f)
		}
	}
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
	// DiskCache, if non-nil, enables the on-disk generic function index which
	// reduces memory usage for large binaries. When nil, the in-memory index
	// is used instead.
	DiskCache *object.DiskCache
}

// exploreCompileUnit processes a compile unit entry (entry's tag is
// TagCompileUnit).
//
// Returns (nil, nil) if the compile unit is not a Go package.
func (b *packagesIterator) exploreCompileUnit(
	entry *dwarf.Entry, reader *dwarf.Reader,
) (*Package, error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return nil, fmt.Errorf("expected TagCompileUnit, got %s", entry.Tag)
	}

	name, ok := entry.Val(dwarf.AttrName).(string)
	if !ok {
		return nil, errors.New("compile unit without name")
	}
	if !interestingPackage(name, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
		reader.SkipChildren()
		return nil, nil
	}

	// Filter out non-Go compile units.
	langField := entry.AttrField(dwarf.AttrLanguage)
	if langField == nil {
		reader.SkipChildren()
		return nil, nil
	}
	langCode, ok := langField.Val.(int64)
	if !ok || langCode != dwarf2.DW_LANG_Go {
		reader.SkipChildren()
		return nil, nil
	}

	// Some compile units are empty; we ignore them (for example, compile units
	// corresponding to assembly code).
	if !entry.Children {
		return nil, nil
	}

	unitHeader, ok := b.offsetToUnit[entry.Offset]
	if !ok {
		return nil, fmt.Errorf("header missing for compile unit %s (0x%x)", name, entry.Offset)
	}
	b.currentCompileUnit = compileUnitInfo{
		entry:  entry,
		name:   name,
		length: unitHeader.Length,
		files:  nil, // filled in below
	}
	defer func() {
		b.currentCompileUnit = compileUnitInfo{}
	}()

	b.currentCompileUnit.outputPkg = &Package{
		Name:  name,
		Types: make(map[string]*Type),
	}
	start := time.Now()

	cuLineReader, err := b.dwarfData.LineReader(entry)
	if err != nil {
		return nil, fmt.Errorf("could not get file line reader for compile unit %s: %w", name, err)
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
					return nil, fmt.Errorf(
						"compile unit %s has invalid nil file entry at index %d", name, i)
				}
				files = append(files, "")
				continue
			}
			files = append(files, file.Name)
		}
	}
	b.currentCompileUnit.files = files

	// Go through the children, looking for subprograms.
	for child, err := reader.Next(); child != nil; child, err = reader.Next() {
		if err != nil {
			return nil, err
		}
		if dwarfutil.IsEntryNull(child) {
			break // End of children for this compile unit.
		}

		switch child.Tag {
		case dwarf.TagSubprogram:
			function, err := b.exploreSubprogram(child, reader)
			if err != nil {
				return nil, err
			}
			if !function.empty() && !b.currentCompileUnit.hasFunctionQName(function.QualifiedName) {
				b.addFunctionToPackage(function)
			}
		default:
			reader.SkipChildren()
		}
	}
	duration := time.Since(start)
	if duration > 5*time.Second {
		log.Warnf("Processing package %s took %s: %s", name, duration, b.currentCompileUnit.outputPkg.Stats())
	}

	return b.currentCompileUnit.outputPkg, nil
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

	inlineAttr, ok := entry.Val(dwarf.AttrInline).(int64)
	// The attribute DW_AT_inline with a value of DW_INL_inlined means that
	// this is an "abstract definition" of the function, which is then
	// referenced by inlined instances (and also possibly by an out-of-line
	// instance) through their AttrAbstractOrigin attribute which will point
	// to this entry.
	if abstractFunc := ok && inlineAttr == dwarf2.DW_INL_inlined; abstractFunc {
		if _, ok := b.abstractFunctions[entry.Offset]; !ok {
			af, err := b.parseAbstractFunction(entry.Offset, reader)
			if err != nil {
				return Function{}, err
			}
			// Keep around information about this abstract function. It will be
			// updated by every subsequent inlined instance of the function.
			b.abstractFunctions[entry.Offset] = af
		}

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

	// Check if the function's own package is interesting. This is needed
	// because generic shape functions can appear in a compile unit different
	// from where they are defined (they end up in the instantiating package's
	// compile unit). Without this check, a dependency's generic function
	// instantiated in first-party code would be incorrectly included.
	if !interestingPackage(funcName.Package, b.mainModule, b.firstPartyPkgPrefix, b.options.Scope) {
		return earlyExit()
	}

	fileIdx, ok := entry.Val(dwarf.AttrDeclFile).(int64)
	if !ok || fileIdx == 0 { // fileIdx == 0 means unknown file, as per DWARF spec
		// TODO: log if this ever happens. I haven't seen it.
		return earlyExit()
	}
	if fileIdx < 0 || int(fileIdx) >= len(b.currentCompileUnit.files) {
		return Function{}, fmt.Errorf(
			"subprogram at 0x%x has invalid file index %d, expected in range [0, %d)",
			entry.Offset, fileIdx, len(b.currentCompileUnit.files),
		)
	}
	fileName := b.currentCompileUnit.files[fileIdx]
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

	lineRanges := coalesceLines(selfLines.Lines)
	var startLine, endLine int
	if len(lineRanges) > 0 {
		startLine = lineRanges[0][0]
		endLine = lineRanges[len(lineRanges)-1][1]
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
		Name:            funcName.Name,
		QualifiedName:   funcName.QualifiedName,
		File:            fileName,
		InjectibleLines: lineRanges,
		Scope: Scope{
			StartLine: startLine,
			EndLine:   endLine,
			Variables: inner.vars,
			Scopes:    inner.scopes,
		},
	}

	// If this function is a method (i.e. it has a receiver), we add it to the
	// respective type instead of returning it as a stand-alone function.
	if funcName.Type != "" {
		typeQualifiedName := funcName.Package + "." + funcName.Type

		// Skip displaced generic methods — they belong to a different
		// package and will be processed by augmentWithDisplacedGenerics
		// when that package is yielded. This prevents creating a foreign
		// type (e.g. lib.Box[...]) under the wrong package (e.g. main).
		if b.genericIndex != nil &&
			funcName.Package != b.currentCompileUnit.outputPkg.Name &&
			strings.Contains(typeQualifiedName, "[...]") {
			return Function{}, nil
		}

		// We generally expect the type of the receiver to have been populated
		// by the exploreCode() call above.
		t, ok := b.currentCompileUnit.outputPkg.Types[typeQualifiedName]
		if !ok {
			// Some types are empty structures, and functions that use them as
			// receivers don't actually have a parameter of the receiver type.
			// Thus, we end up without a type. Just make one up.
			t = &Type{
				Name: typeQualifiedName,
			}
			b.currentCompileUnit.outputPkg.Types[typeQualifiedName] = t
		}
		// For generic types, dedup methods by name (first shape wins).
		for _, m := range t.Methods {
			if m.Name == res.Name {
				return Function{}, nil
			}
		}
		t.Methods = append(t.Methods, res)
		// We don't return a Function for methods.
		return Function{}, nil
	}
	return res, nil
}

// Explores inlined instances of an abstract function (both InlinedSubroutines
// and out-of-line Subprogram instances). Modify the data associated with the
// abstract function definition based on the variable availability in this
// instance.
func (b *packagesIterator) exploreInlinedInstance(
	entry *dwarf.Entry,
	reader *dwarf.Reader,
	lines map[string]gosym.FunctionLines,
) error {
	earlyExit := func() error {
		reader.SkipChildren()
		return nil
	}
	// resetReader repositions the reader back to entry after parseAbstractFunction
	// has moved it.
	resetReader := func() error {
		reader.Seek(entry.Offset)
		e, err := reader.Next()
		if err != nil {
			return err
		}
		if e.Tag != dwarf.TagSubprogram && e.Tag != dwarf.TagInlinedSubroutine {
			return fmt.Errorf("unexpected tag %v at 0x%x, expected TagSubprogram or TagInlinedSubroutine", e.Tag, entry.Offset)
		}
		return nil
	}

	// Lookup the abstract function definition referenced by this inlined
	// instance.
	originOffset, ok := entry.Val(dwarf.AttrAbstractOrigin).(dwarf.Offset)
	if !ok {
		return fmt.Errorf("inlined instance without abstract origin at 0x%x", entry.Offset)
	}

	af, ok := b.abstractFunctions[originOffset]
	if !ok {
		// We only explore the abstract definition if it's in the current
		// compilation unit; we don't accumulate data across compile units in
		// b.abstractFunctions.
		inCurrentUnit := originOffset >= b.currentCompileUnit.entry.Offset &&
			uint64(originOffset) < uint64(b.currentCompileUnit.entry.Offset)+b.currentCompileUnit.length
		if !inCurrentUnit {
			return earlyExit()
		}

		var err error
		af, err = b.parseAbstractFunction(originOffset, reader)
		if err != nil {
			return err
		}
		b.abstractFunctions[originOffset] = af
		if err := resetReader(); err != nil {
			return err
		}
	}
	if !af.interesting {
		return earlyExit()
	}

	// Update properties that are not present on abstract definition.
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
	if af.injectibleLines == nil {
		af.injectibleLines = coalesceLines(selfLines.Lines)
	} else {
		af.injectibleLines = intersectRanges(af.injectibleLines, coalesceLines(selfLines.Lines))
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
				b.currentCompileUnit.entry,
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
		case dwarf.TagTypedef:
			// Typedefs in generic shape functions for dictionary type
			// parameters (.param0, .param1). Skip.
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
		b.currentCompileUnit.entry,
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

// parseAbstractFunction parses the abstract function definition at the given
// offset. It moves the reader; the caller is responsible for resetting it
// afterwards.
func (b *packagesIterator) parseAbstractFunction(offset dwarf.Offset, reader *dwarf.Reader) (*abstractFunction, error) {
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
		case dwarf.TagTypedef:
			// Typedefs in generic shape functions for dictionary type
			// parameters (.param0, .param1). Skip.
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

func coalesceLines(linePcRanges []gosym.LineRange) []LineRange {
	if len(linePcRanges) == 0 {
		return nil
	}
	lines := make([]int, 0, len(linePcRanges))
	for _, linePcRange := range linePcRanges {
		lines = append(lines, int(linePcRange.Line))
	}
	slices.Sort(lines)
	var lineRanges []LineRange
	start := lines[0]
	end := start
	for _, line := range lines[1:] {
		if line == end || line == end+1 {
			end = line
		} else {
			lineRanges = append(lineRanges, LineRange{start, end})
			start = line
			end = start
		}
	}
	lineRanges = append(lineRanges, LineRange{start, end})
	return lineRanges
}

// To calculate intersection of two sets of ranges, we use a sweep algorithm,
// handling events that mark the beginning and end of ranges (inclusive).
type intersectEvent struct {
	Val int
	// +1 for beginning of range, -1 for end of range.
	Mod int
}

func intersectRanges(a, b []LineRange) []LineRange {
	events := make([]intersectEvent, 0, 2*len(a)+2*len(b))
	for _, r := range a {
		events = append(events, intersectEvent{Val: r[0], Mod: 1})
		events = append(events, intersectEvent{Val: r[1], Mod: -1})
	}
	for _, r := range b {
		events = append(events, intersectEvent{Val: r[0], Mod: 1})
		events = append(events, intersectEvent{Val: r[1], Mod: -1})
	}
	slices.SortFunc(events, func(a, b intersectEvent) int {
		return cmp.Or(cmp.Compare(a.Val, b.Val), -cmp.Compare(a.Mod, b.Mod))
	})
	intersected := make([]LineRange, 0, len(a)+len(b))
	active := 0
	start := 0
	for _, e := range events {
		active += e.Mod
		if active == 2 {
			start = e.Val
		} else if active == 1 && start != 0 {
			intersected = append(intersected, LineRange{start, e.Val})
			start = 0
		}
	}
	return intersected
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
	loclists, err := loclist.ProcessLocations(locField, unit, b.loclistReader, pcRanges, totalSize, uint8(b.pointerSize))
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
