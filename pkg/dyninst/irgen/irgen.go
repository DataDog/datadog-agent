// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package irgen generates an IR program from an object file and a list of
// probes.
//
// The irgen package is responsible for creating intermediate representation
// (IR) programs from binary files and configuration. It handles DWARF parsing,
// type analysis, and probe instrumentation planning.
//
// The core function is GenerateIR, which takes a binary object file and a
// list of probe configurations and produces an IR program that contains the
// information needed for dynamic instrumentation.
//
// The package also handles error collection for probes that fail during
// generation, allowing successful probes to continue processing while
// collecting failures for reporting.
package irgen

import (
	"cmp"
	"debug/dwarf"
	"errors"
	"fmt"
	"iter"
	"maps"
	"math"
	"reflect"
	"runtime/debug"
	"slices"
	"strings"

	pkgerrors "github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
)

// TODO: Validate the probes in the config and report things that are
// not supported without just bailing out.

// TODO: This code creates a lot of allocations, but we could greatly reduce
// the number of distinct allocations by using a batched allocation scheme.
// Such an approach makes sense because we know the lifetimes of all the
// objects are going to be the same.

// TODO: Handle creating return events.

// TODO: Properly set up the presence bitset.

// TODO: Support hmaps.

// Generator is used to generate IR programs from binary files and probe
// configurations.
type Generator struct {
	config config
}

// NewGenerator creates a new Generator with the given options.
func NewGenerator(options ...Option) *Generator {
	g := &Generator{
		config: defaultConfig,
	}
	for _, option := range options {
		option.apply(&g.config)
	}
	return g
}

// GenerateIR generates an IR program from a binary and a list of probes.
// It returns a GeneratedProgram containing both the successful IR program
// and any probes that failed during generation.
func (g *Generator) GenerateIR(
	programID ir.ProgramID,
	binaryPath string,
	probeDefs []ir.ProbeDefinition,
) (*ir.Program, error) {
	elfFile, err := g.config.objectLoader.Load(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load elf file: %w", err)
	}
	defer elfFile.Close()
	return generateIR(g.config, programID, elfFile, probeDefs)
}

// GenerateIR generates an IR program from a binary and a list of probes.
// It returns a GeneratedProgram containing both the successful IR program
// and any probes that failed during generation.
func GenerateIR(
	programID ir.ProgramID,
	objFile object.FileWithDwarf,
	probeDefs []ir.ProbeDefinition,
	options ...Option,
) (_ *ir.Program, retErr error) {
	cfg := defaultConfig
	for _, option := range options {
		option.apply(&cfg)
	}
	return generateIR(cfg, programID, objFile, probeDefs)
}

func generateIR(
	cfg config,
	programID ir.ProgramID,
	objFile object.FileWithDwarf,
	probeDefs []ir.ProbeDefinition,
) (_ *ir.Program, retErr error) {
	// Ensure deterministic output.
	slices.SortFunc(probeDefs, func(a, b ir.ProbeDefinition) int {
		return cmp.Compare(a.GetID(), b.GetID())
	})

	// Given that the go dwarf library is not intentionally safe when
	// used with untrusted inputs, let's at least recover from panics and
	// return them as errors. Perhaps there are malicious inputs that will
	// cause infinite loops, but that's a risk we'll have to deal with elsewhere
	// when evaluating dwarf expressions and line programs.
	defer func() {
		r := recover()
		switch r := r.(type) {
		case nil:
		case error:
			retErr = pkgerrors.Wrap(r, "GenerateIR: panic")
		default:
			retErr = pkgerrors.Errorf("GenerateIR: panic: %v\n%s", r, debug.Stack())
		}
	}()

	// Build the initial set of interests from the provided probe definitions.
	interests, issues := makeInterests(probeDefs)

	// Prepare the main DWARF visitor that will gather all the information we
	// need from the binary.
	ptrSize := uint8(objFile.Architecture().PointerSize())
	d := objFile.DwarfData()
	typeCatalog := newTypeCatalog(
		d,
		ptrSize,
		cfg.maxDynamicTypeSize,
		cfg.maxHashBucketsSize,
	)
	pendingSubprograms, err := processDwarf(interests, d, typeCatalog, objFile)
	if err != nil {
		return nil, err
	}

	// Find prologues need to determine injection points. We make an assumption
	// that prologue, if function has a frame, should be contained within the
	// first pc range of a subprogram. This simplifies the logic slightly.
	prologueSearch := make([]prologueSeachParams, 0, len(pendingSubprograms))
	for _, sp := range pendingSubprograms {
		if len(sp.subprogram.OutOfLinePCRanges) > 0 {
			prologueSearch = append(prologueSearch, prologueSeachParams{
				unit:    sp.unit,
				pcRange: sp.subprogram.OutOfLinePCRanges[0],
			})
		}
		for _, inlined := range sp.subprogram.InlinePCRanges {
			prologueSearch = append(prologueSearch, prologueSeachParams{
				unit:    sp.unit,
				pcRange: inlined.RootRanges[0],
			})
		}
	}

	prologueLocs, err := findProloguesEnds(objFile, prologueSearch)
	if err != nil {
		return nil, err
	}

	// Instantiate probes and gather any probe-related issues.
	probes, subprograms, probeIssues, err := createProbes(
		objFile, pendingSubprograms, prologueLocs,
	)
	if err != nil {
		return nil, err
	}
	issues = append(issues, probeIssues...)

	// Finalize type information now that we have all referenced types.
	if err := finalizeTypes(typeCatalog, subprograms); err != nil {
		return nil, err
	}

	// Populate event root expressions for every probe.
	probes, eventIssues := populateProbeEventsExpressions(probes, typeCatalog)
	issues = append(issues, eventIssues...)

	// Detect probe definitions that did not match any symbol in the binary.
	unused := findUnusedConfigs(probes, issues, probeDefs)
	for _, probe := range unused {
		issues = append(issues, ir.ProbeIssue{
			ProbeDefinition: probe,
			Issue: ir.Issue{
				Kind:    ir.IssueKindTargetNotFoundInBinary,
				Message: "target for probe not found in binary",
			},
		})
	}
	slices.SortFunc(issues, ir.CompareProbeIDs)

	return &ir.Program{
		ID:          programID,
		Subprograms: subprograms,
		Probes:      probes,
		Types:       typeCatalog.typesByID,
		MaxTypeID:   typeCatalog.idAlloc.alloc,
		Issues:      issues,
	}, nil
}

type prologueSeachParams struct {
	unit    *dwarf.Entry
	pcRange ir.PCRange
}

type prologueEndLocation struct {
	err       error
	frameless bool
	// If not frameless, the pc of the prologue end, otherwise the first pc
	// of the subprogram.
	pc uint64
}

// findPrologues searches for prologue for each given search param. Results
// are indexed by the start of the pc range. Provided ranges must be non-overlapping
// but may contain duplicates.
func findProloguesEnds(
	objFile object.Dwarf,
	searchParams []prologueSeachParams,
) (map[uint64]prologueEndLocation, error) {
	if len(searchParams) == 0 {
		return make(map[uint64]prologueEndLocation), nil
	}
	slices.SortFunc(searchParams, func(a, b prologueSeachParams) int {
		return cmp.Compare(a.pcRange[0], b.pcRange[0])
	})
	// Remove duplicates.
	i := 1
	for j := 1; j < len(searchParams); j++ {
		if searchParams[i-1].pcRange != searchParams[j].pcRange {
			searchParams[i] = searchParams[j]
			i++
		}
	}
	searchParams = searchParams[:i]

	res := make(map[uint64]prologueEndLocation, len(searchParams))
	var prevUnit *dwarf.Entry
	var lineReader *dwarf.LineReader
	for _, sp := range searchParams {
		if prevUnit != sp.unit {
			prevUnit = sp.unit
			lr, err := objFile.DwarfData().LineReader(prevUnit)
			if err != nil {
				return nil, fmt.Errorf("failed to get line reader: %w", err)
			}
			lineReader = lr
		}

		pc, ok, err := findPrologueEnd(lineReader, sp.pcRange)
		switch {
		case err != nil:
			res[sp.pcRange[0]] = prologueEndLocation{err: err}
		case ok:
			res[sp.pcRange[0]] = prologueEndLocation{
				frameless: false,
				pc:        pc,
			}
		default:
			res[sp.pcRange[0]] = prologueEndLocation{
				frameless: true,
				pc:        sp.pcRange[0],
			}
		}
	}
	return res, nil
}

// createProbes instantiates probes for each pending sub-program and gathers any
// probe-specific issues encountered in the process.
func createProbes(
	objFile object.FileWithDwarf,
	pending []*pendingSubprogram,
	prologueLocs map[uint64]prologueEndLocation,
) ([]*ir.Probe, []*ir.Subprogram, []ir.ProbeIssue, error) {
	var (
		probes       []*ir.Probe
		subprograms  []*ir.Subprogram
		issues       []ir.ProbeIssue
		eventIDAlloc idAllocator[ir.EventID]
	)

	for _, p := range pending {
		if !p.issue.IsNone() {
			for _, cfg := range p.probesCfgs {
				issues = append(issues, ir.ProbeIssue{ProbeDefinition: cfg, Issue: p.issue})
			}
			continue
		}

		var haveProbe bool
		for _, cfg := range p.probesCfgs {
			probe, iss, err := newProbe(objFile, cfg, p.subprogram, &eventIDAlloc, prologueLocs)
			if err != nil {
				return nil, nil, nil, err
			}
			if !iss.IsNone() {
				issues = append(issues, ir.ProbeIssue{ProbeDefinition: cfg, Issue: iss})
				continue
			}
			probes = append(probes, probe)
			haveProbe = true
		}

		if haveProbe {
			subprograms = append(subprograms, p.subprogram)
		}
	}

	return probes, subprograms, issues, nil
}

// finalizeTypes resolves placeholder references, computes Go-specific type
// metadata, and rewrites variable type references so that each variable points
// at the fully-resolved type instance.
func finalizeTypes(tc *typeCatalog, subprograms []*ir.Subprogram) error {
	rewritePlaceholderReferences(tc)
	if err := completeGoTypes(tc); err != nil {
		return err
	}

	for _, sp := range subprograms {
		for _, v := range sp.Variables {
			v.Type = tc.typesByID[v.Type.GetID()]
		}
	}
	return nil
}

// processDwarf walks the DWARF data, collects all subprograms we care about
// (both concrete and abstract), propagates information from inlined instances
// to their abstract origins, and returns the resulting slice together with the
// index of the first abstract sub-program in that slice.
func processDwarf(
	interests interests,
	d *dwarf.Data,
	typeCatalog *typeCatalog,
	objFile object.FileWithDwarf,
) ([]*pendingSubprogram, error) {
	v := &rootVisitor{
		interests:           interests,
		dwarf:               d,
		subprogramIDAlloc:   idAllocator[ir.SubprogramID]{},
		abstractSubprograms: make(map[dwarf.Offset]*abstractSubprogram),
		inlinedSubprograms:  make(map[*dwarf.Entry][]*inlinedSubprogram),
		typeCatalog:         typeCatalog,
		pointerSize:         uint8(objFile.Architecture().PointerSize()),
		loclistReader:       objFile.LoclistReader(),
	}

	// Visit the entire DWARF tree.
	if err := visitDwarf(d.Reader(), v); err != nil {
		return nil, err
	}

	// Concrete subprograms are already in v.pendingSubprograms.
	pending := v.subprograms

	// Propagate details from each inlined instance to its abstract origin.
	inlinedByUnit := iterMapSorted(v.inlinedSubprograms, cmpEntry)
	for unit, inlinedSubs := range inlinedByUnit {
		for _, inlined := range inlinedSubs {
			abs, ok := v.abstractSubprograms[inlined.abstractOrigin]
			if !ok || !abs.issue.IsNone() {
				continue
			}
			issue := applyInlineToAbstractSubprogram(
				abs,
				inlined,
				unit,
				v.loclistReader,
				v.pointerSize,
				v.typeCatalog,
			)
			if !issue.IsNone() {
				abs.issue = issue
			}
		}
	}

	// Append the abstract sub-programs in deterministic order.
	abstractSubs := iterMapSorted(v.abstractSubprograms, cmp.Compare)
	for _, abs := range abstractSubs {
		pending = append(pending, &pendingSubprogram{
			subprogram: abs.subprogram,
			unit:       abs.unit,
			probesCfgs: abs.probesCfgs,
			issue:      abs.issue,
		})
	}

	return pending, nil
}

func findUnusedConfigs(
	successes []*ir.Probe,
	failures []ir.ProbeIssue,
	configs []ir.ProbeDefinition,
) (unused []ir.ProbeDefinition) {
	slices.SortFunc(successes, ir.CompareProbeIDs)
	slices.SortFunc(failures, ir.CompareProbeIDs)
	slices.SortFunc(configs, ir.CompareProbeIDs)
	for _, config := range configs {
		var inSuccesses, inFailures bool
		successes, inSuccesses = skipPast(successes, config)
		failures, inFailures = skipPast(failures, config)
		if !inSuccesses && !inFailures {
			unused = append(unused, config)
		}
	}
	return unused
}

func skipPast[A, B ir.ProbeIDer](items []A, target B) (_ []A, found bool) {
	idx, found := slices.BinarySearchFunc(items, target, ir.CompareProbeIDs)
	if found {
		idx++
	}
	return items[idx:], found
}

func applyInlineToAbstractSubprogram(
	abstractSubprogram *abstractSubprogram,
	inlinedSubprogram *inlinedSubprogram,
	unit *dwarf.Entry,
	loclistReader *loclist.Reader,
	pointerSize uint8,
	typeCatalog *typeCatalog,
) ir.Issue {
	var ranges []ir.PCRange
	if inlinedSubprogram.outOfLinePCRanges != nil {
		// Out-of-line instance.
		if abstractSubprogram.subprogram.OutOfLinePCRanges != nil {
			return ir.Issue{
				Kind:    ir.IssueKindMalformedExecutable,
				Message: "multiple out-of-line instances of abstract subprogram",
			}
		}
		abstractSubprogram.subprogram.OutOfLinePCRanges = inlinedSubprogram.outOfLinePCRanges
		ranges = inlinedSubprogram.outOfLinePCRanges
	} else {
		// Inlined instance.
		abstractSubprogram.subprogram.InlinePCRanges = append(
			abstractSubprogram.subprogram.InlinePCRanges, inlinedSubprogram.inlinedPCRanges)
		ranges = inlinedSubprogram.inlinedPCRanges.Ranges
	}
	for _, inlinedVariable := range inlinedSubprogram.variables {
		// Inlined subprograms usually have variables with abstract origin
		// pointing at the abstract subprogram variable.  Sometimes, they will
		// have fully defined variables (observed to be return values in
		// out-of-line instantations).
		var variable *ir.Variable
		abstractOrigin, ok, err := maybeGetAttr[dwarf.Offset](
			inlinedVariable, dwarf.AttrAbstractOrigin)
		if err != nil {
			return ir.Issue{
				Kind: ir.IssueKindMalformedExecutable,
				Message: fmt.Sprintf(
					"failed to get abstract origin for inlined variable: %v",
					err,
				),
			}
		}
		if ok {
			variable, ok = abstractSubprogram.variables[abstractOrigin]
			if !ok {
				return ir.Issue{
					Kind:    ir.IssueKindMalformedExecutable,
					Message: "abstract variable not found for inlined variable",
				}
			}
			var locations []ir.Location
			locField := inlinedVariable.AttrField(dwarf.AttrLocation)
			if locField != nil {
				locations, err = computeLocations(
					unit, ranges, variable.Type, locField,
					loclistReader, pointerSize,
				)
				if err != nil {
					return ir.Issue{
						Kind: ir.IssueKindMalformedExecutable,
						Message: fmt.Sprintf(
							"failed to compute locations for inlined variable %q: %v",
							variable.Name, err,
						),
					}
				}
				variable.Locations = append(variable.Locations, locations...)
			}
		} else {
			var isParameter bool
			switch inlinedVariable.Tag {
			case dwarf.TagFormalParameter:
				isParameter = true
			case dwarf.TagVariable:
				isParameter = false
			default:
				return ir.Issue{
					Kind: ir.IssueKindMalformedExecutable,
					Message: fmt.Sprintf(
						"unexpected tag for inlined variable: %v",
						inlinedVariable.Tag,
					),
				}
			}
			variable, err = processVariable(
				unit, inlinedVariable, isParameter,
				true, /* parseLocations */
				ranges,
				loclistReader, pointerSize, typeCatalog,
			)
			if err != nil {
				return ir.Issue{
					Kind:    ir.IssueKindMalformedExecutable,
					Message: fmt.Sprintf("failed to process variable: %v", err),
				}
			}
			abstractSubprogram.subprogram.Variables = append(
				abstractSubprogram.subprogram.Variables, variable)
		}
	}
	return ir.Issue{}
}

func completeGoTypes(tc *typeCatalog) error {
	for _, t := range iterMapSorted(tc.typesByID, cmp.Compare) {
		switch t := t.(type) {
		case *ir.StructureType:
			switch t.GoTypeAttributes.GoKind {
			case reflect.String:
				if err := completeGoStringType(tc, t); err != nil {
					return err
				}
			case reflect.Slice:
				if err := completeGoSliceType(tc, t); err != nil {
					return err
				}
			case reflect.Struct:
				// Nothing to do.
			default:
				return fmt.Errorf(
					"unexpected Go kind for structure type: %v",
					t.GoTypeAttributes.GoKind,
				)
			}
		case *ir.GoMapType:
			if err := completeGoMapType(tc, t); err != nil {
				return err
			}
		}
	}
	visitTypeReferences(tc, func(t *ir.Type) {
		if *t == nil {
			return
		}
		(*t) = tc.typesByID[(*t).GetID()]
	})
	return nil
}

// iterMapSorted is a helper function that iterates over a map and yields
// the keys and values in sorted order using the provided comparator.
func iterMapSorted[
	K comparable, V any, M ~map[K]V,
](m M, f func(K, K) int) iter.Seq2[K, V] {
	keys := make([]K, 0, len(m))
	keys = slices.AppendSeq(keys, maps.Keys(m))
	slices.SortFunc(keys, f)
	return func(yield func(K, V) bool) {
		for _, k := range keys {
			if !yield(k, m[k]) {
				return
			}
		}
	}
}

func cmpEntry(a, b *dwarf.Entry) int {
	return cmp.Compare(a.Offset, b.Offset)
}

func completeGoMapType(tc *typeCatalog, t *ir.GoMapType) error {
	// Convert the header type from a structure type to the appropriate
	// Go-specific type.
	headerType, ok := t.HeaderType.(*ir.StructureType)
	if !ok {
		return fmt.Errorf(
			"header type for map type %q is not a pointer type %T",
			t.Name, t.HeaderType,
		)
	}
	// Use the type name to determine whether this is an hmap or a swiss map.
	// We could alternatively use the go version or the structure field layout.
	// This works for now.
	switch {
	case strings.HasPrefix(headerType.Name, "map<"):
		return completeSwissMapHeaderType(tc, headerType)
	case strings.HasPrefix(headerType.Name, "hash<"):
		return completeHMapHeaderType(tc, headerType)
	default:
		return fmt.Errorf(
			"unexpected header type for map type %q: %T",
			t.Name, t.HeaderType,
		)
	}
}

func field(st *ir.StructureType, name string) (*ir.Field, error) {
	offset := slices.IndexFunc(st.RawFields, func(f ir.Field) bool {
		return f.Name == name
	})
	if offset == -1 {
		return nil, fmt.Errorf("type %q has no %s field", st.Name, name)
	}
	return &st.RawFields[offset], nil
}

func fieldType[T ir.Type](st *ir.StructureType, name string) (T, error) {
	f, err := field(st, name)
	if err != nil {
		return *new(T), err
	}
	fieldType, ok := f.Type.(T)
	if !ok {
		ret := *new(T)
		err := fmt.Errorf(
			"field %q of type %q is not a %T, got %T",
			name, st.Name, ret, f.Type,
		)
		return ret, err
	}
	return fieldType, nil
}

func pointeeType[T ir.Type](t ir.Type) (T, error) {
	ptrType, ok := t.(*ir.PointerType)
	if !ok {
		return *new(T), fmt.Errorf("type %q is not a pointer type, got %T", t.GetName(), t)
	}
	pointee, ok := ptrType.Pointee.(T)
	if !ok {
		return *new(T), fmt.Errorf(
			"pointee type %q is not a %T, got %T",
			ptrType.Pointee.GetName(), new(T), ptrType.Pointee,
		)
	}
	return pointee, nil
}

func completeSwissMapHeaderType(tc *typeCatalog, st *ir.StructureType) error {
	var tablePtrType *ir.PointerType
	var groupReferenceType *ir.StructureType
	var groupType *ir.StructureType
	{
		dirPtrType, err := fieldType[*ir.PointerType](st, "dirPtr")
		if err != nil {
			return err
		}
		tablePtrType, err = pointeeType[*ir.PointerType](dirPtrType)
		if err != nil {
			return err
		}
		tableType, err := pointeeType[*ir.StructureType](tablePtrType)
		if err != nil {
			return err
		}
		groupReferenceType, err = fieldType[*ir.StructureType](tableType, "groups")
		if err != nil {
			return err
		}
		groupPtrType, err := fieldType[*ir.PointerType](groupReferenceType, "data")
		if err != nil {
			return err
		}
		groupType, err = pointeeType[*ir.StructureType](groupPtrType)
		if err != nil {
			return err
		}
	}

	tablePtrSliceDataType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("[]%s.array", tablePtrType.GetName()),
			ByteSize: tc.maxHashBucketsSize,
		},
		Element: tablePtrType,
	}
	tc.typesByID[tablePtrSliceDataType.ID] = tablePtrSliceDataType

	groupSliceType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("[]%s.array", groupType.GetName()),
			ByteSize: uint32(tc.maxDynamicTypeSize),
		},
		Element: groupType,
	}
	tc.typesByID[groupSliceType.ID] = groupSliceType

	mapHeaderType := &ir.GoSwissMapHeaderType{
		StructureType:     st,
		TablePtrSliceType: tablePtrSliceDataType,
		GroupType:         groupType,
	}
	tc.typesByID[mapHeaderType.ID] = mapHeaderType

	groupsType := &ir.GoSwissMapGroupsType{
		StructureType:  groupReferenceType,
		GroupType:      groupType,
		GroupSliceType: groupSliceType,
	}
	tc.typesByID[groupsType.ID] = groupsType
	return nil
}

func completeHMapHeaderType(tc *typeCatalog, st *ir.StructureType) error {
	bucketsField, err := field(st, "buckets")
	if err != nil {
		return err
	}
	bucketsStructType, err := pointeeType[*ir.StructureType](bucketsField.Type)
	if err != nil {
		return err
	}
	keysArrayType, err := fieldType[*ir.ArrayType](bucketsStructType, "keys")
	if err != nil {
		return err
	}
	keyType := keysArrayType.Element
	valuesArrayType, err := fieldType[*ir.ArrayType](bucketsStructType, "values")
	if err != nil {
		return err
	}
	valueType := valuesArrayType.Element
	bucketsType := &ir.GoHMapBucketType{
		StructureType: bucketsStructType,
		KeyType:       keyType,
		ValueType:     valueType,
	}
	bucketsSliceDataType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("[]%s.array", bucketsType.GetName()),
			ByteSize: tc.maxDynamicTypeSize,
		},
		Element: bucketsType,
	}
	headerType := &ir.GoHMapHeaderType{
		StructureType: st,
		BucketType:    bucketsType,
		BucketsType:   bucketsSliceDataType,
	}
	tc.typesByID[bucketsSliceDataType.ID] = bucketsSliceDataType
	tc.typesByID[headerType.ID] = headerType
	tc.typesByID[bucketsType.ID] = bucketsType
	return nil
}

func completeGoStringType(tc *typeCatalog, st *ir.StructureType) error {
	strField, err := field(st, "str")
	if err != nil {
		return err
	}
	strDataType := &ir.GoStringDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("%s.str", st.Name),
			ByteSize: tc.maxDynamicTypeSize,
		},
	}
	tc.typesByID[strDataType.ID] = strDataType
	strDataPtrType := &ir.PointerType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("*%s.str", st.Name),
			ByteSize: uint32(tc.ptrSize),
		},
		Pointee: strDataType,
	}
	tc.typesByID[strDataPtrType.ID] = strDataPtrType
	strField.Type = strDataPtrType
	tc.typesByID[st.ID] = &ir.GoStringHeaderType{
		StructureType: st,
		Data:          strDataType,
	}

	return nil
}

func completeGoSliceType(tc *typeCatalog, st *ir.StructureType) error {
	arrayField, err := field(st, "array")
	if err != nil {
		return err
	}
	elementType, err := pointeeType[ir.Type](arrayField.Type)
	if err != nil {
		return err
	}
	arrayDataType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("%s.array", st.Name),
			ByteSize: tc.maxDynamicTypeSize,
		},
		Element: elementType,
	}
	tc.typesByID[arrayDataType.ID] = arrayDataType
	arrayDataPtrType := &ir.PointerType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("*%s.array", st.Name),
			ByteSize: uint32(tc.ptrSize),
		},
		Pointee: arrayDataType,
	}
	tc.typesByID[arrayDataPtrType.ID] = arrayDataPtrType
	tc.typesByID[st.ID] = &ir.GoSliceHeaderType{
		StructureType: st,
		Data:          arrayDataType,
	}
	return nil
}

func populateProbeEventsExpressions(
	probes []*ir.Probe,
	typeCatalog *typeCatalog,
) (successful []*ir.Probe, failed []ir.ProbeIssue) {
	for _, probe := range probes {
		if issue := populateProbeExpressions(probe, typeCatalog); !issue.IsNone() {
			failed = append(failed, ir.ProbeIssue{
				ProbeDefinition: probe.ProbeDefinition,
				Issue:           issue,
			})
		} else {
			successful = append(successful, probe)
		}
	}
	return successful, failed
}

func populateProbeExpressions(
	probe *ir.Probe,
	typeCatalog *typeCatalog,
) ir.Issue {
	for _, event := range probe.Events {
		issue := populateEventExpressions(probe, event, typeCatalog)
		if !issue.IsNone() {
			return issue
		}
	}
	return ir.Issue{}
}

func populateEventExpressions(
	probe *ir.Probe,
	event *ir.Event,
	typeCatalog *typeCatalog,
) ir.Issue {
	id := typeCatalog.idAlloc.next()
	var expressions []*ir.RootExpression
	for _, variable := range probe.Subprogram.Variables {
		if !variable.IsParameter || variable.IsReturn {
			continue
		}
		variableSize := variable.Type.GetByteSize()
		expr := &ir.RootExpression{
			Name:   variable.Name,
			Offset: uint32(0),
			Expression: ir.Expression{
				Type: variable.Type,
				Operations: []ir.ExpressionOp{
					&ir.LocationOp{
						Variable: variable,
						Offset:   0,
						ByteSize: uint32(variableSize),
					},
				},
			},
		}
		expressions = append(expressions, expr)
	}
	presenceBitsetSize := uint32((len(expressions) + 7) / 8)
	byteSize := uint64(presenceBitsetSize)
	for _, e := range expressions {
		e.Offset = uint32(byteSize)
		byteSize += uint64(e.Expression.Type.GetByteSize())
	}
	if byteSize > math.MaxUint32 {
		return ir.Issue{
			Kind:    ir.IssueKindUnsupportedFeature,
			Message: fmt.Sprintf("root data type too large: %d bytes", byteSize),
		}
	}
	event.Type = &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       id,
			Name:     fmt.Sprintf("Probe[%s]", probe.Subprogram.Name),
			ByteSize: uint32(byteSize),
		},
		PresenceBitsetSize: presenceBitsetSize,
		Expressions:        expressions,
	}
	typeCatalog.typesByID[event.Type.ID] = event.Type
	return ir.Issue{}
}

type rootVisitor struct {
	pointerSize         uint8
	interests           interests
	dwarf               *dwarf.Data
	subprogramIDAlloc   idAllocator[ir.SubprogramID]
	subprograms         []*pendingSubprogram
	abstractSubprograms map[dwarf.Offset]*abstractSubprogram
	// InlinedSubprograms grouped by the compilation unit entry.
	inlinedSubprograms map[*dwarf.Entry][]*inlinedSubprogram
	typeCatalog        *typeCatalog
	loclistReader      *loclist.Reader

	// This is used to avoid allocations of unitChildVisitor for each
	// compile unit.
	freeUnitChildVisitor *unitChildVisitor
}

type pendingSubprogram struct {
	subprogram *ir.Subprogram
	unit       *dwarf.Entry
	probesCfgs []ir.ProbeDefinition
	issue      ir.Issue
}

func (v *rootVisitor) push(entry *dwarf.Entry) (childVisitor visitor, err error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return nil, nil
	}

	language, ok, err := maybeGetAttr[int64](entry, dwarf.AttrLanguage)
	if err != nil {
		return nil, fmt.Errorf("failed to get language for compile unit: %w", err)
	}
	if !ok || language != dwLangGo {
		return nil, nil
	}
	return v.getUnitVisitor(entry), nil
}

func (v *rootVisitor) getUnitVisitor(entry *dwarf.Entry) (unitVisitor *unitChildVisitor) {
	if v.freeUnitChildVisitor != nil {
		unitVisitor, v.freeUnitChildVisitor = v.freeUnitChildVisitor, nil
	} else {
		unitVisitor = &unitChildVisitor{
			root: v,
		}
	}
	unitVisitor.unit = entry
	return unitVisitor
}

func (v *rootVisitor) putUnitVisitor(unitVisitor *unitChildVisitor) {
	if v.freeUnitChildVisitor == nil {
		unitVisitor.unit = nil
		v.freeUnitChildVisitor = unitVisitor
	}
}

func (v *rootVisitor) pop(_ *dwarf.Entry, childVisitor visitor) error {
	switch t := childVisitor.(type) {
	case *unitChildVisitor:
		v.putUnitVisitor(t)
	}
	return nil
}

type unitChildVisitor struct {
	root *rootVisitor
	unit *dwarf.Entry

	// TODO: Reuse the subprogramChildVisitor.
}

func (v *unitChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	// For now, we're going to skip types and just visit other parts of
	// subprograms.
	switch entry.Tag {
	case dwarf.TagSubprogram:
		name, ok, err := maybeGetAttr[string](entry, dwarf.AttrName)
		if err != nil {
			return nil, err
		}
		if !ok {
			// This is expected to be an out-of-line instance of an abstract program.
			childVisitor, err = processInlinedSubroutineEntry(v.root, v.unit, true /* outOfLineInstance */, nil, entry)
			if err != nil {
				return nil, fmt.Errorf("unnamed, non-inline subprogram: %w", err)
			}
			return childVisitor, nil
		}
		probesCfgs := v.root.interests.subprograms[name]
		inline, ok, err := maybeGetAttr[int64](entry, dwarf.AttrInline)
		if err != nil {
			return nil, err
		}
		if ok && inline == dwInlInlined {
			if len(probesCfgs) > 0 {
				abstractSubprogram := &abstractSubprogram{
					unit:       v.unit,
					probesCfgs: probesCfgs,
					subprogram: &ir.Subprogram{
						ID:   v.root.subprogramIDAlloc.next(),
						Name: name,
					},
					variables: make(map[dwarf.Offset]*ir.Variable),
				}
				v.root.abstractSubprograms[entry.Offset] = abstractSubprogram
				return &abstractSubprogramVisitor{
					root:               v.root,
					unit:               v.unit,
					abstractSubprogram: abstractSubprogram,
				}, nil
			}
			return nil, nil
		}

		var subprogram *ir.Subprogram
		var subprogramPCRanges []ir.PCRange
		if len(probesCfgs) > 0 {
			var err error
			subprogramPCRanges, err = v.root.dwarf.Ranges(entry)
			if err != nil {
				return nil, fmt.Errorf("failed to parse pc ranges: %w", err)
			}
			subprogram = &ir.Subprogram{
				ID:                v.root.subprogramIDAlloc.next(),
				Name:              name,
				OutOfLinePCRanges: subprogramPCRanges,
			}
		}
		return &subprogramChildVisitor{
			root:                     v.root,
			subprogramEntry:          entry,
			unit:                     v.unit,
			subprogram:               subprogram,
			cachedSubprogramPCRanges: subprogramPCRanges,
			probesCfgs:               probesCfgs,
		}, nil

	case dwarf.TagUnspecifiedType:
		// Go defines one of these but doesn't use it. Skip it.
		return nil, nil

	case dwarf.TagPointerType,
		dwarf.TagBaseType,
		dwarf.TagArrayType,
		dwarf.TagStructType,
		dwarf.TagTypedef,
		dwarf.TagSubroutineType:
		// TODO: We've already parsed this node, it's wasteful to parse it
		// again, but we're not going to know whether we need it until later. so
		// for now we'll just skip over all types and come back to them lazily.
		return nil, nil

	case dwarf.TagVariable:
		// TODO: Handle variables.
		return nil, nil
	case dwarf.TagConstant:
		// TODO: Handle constants.
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected tag for unit child: %s", entry.Tag)
	}
}

func (v *unitChildVisitor) pop(_ *dwarf.Entry, childVisitor visitor) error {
	switch t := childVisitor.(type) {
	case nil:
		return nil
	case *subprogramChildVisitor:
		if t.subprogram != nil {
			v.root.subprograms = append(v.root.subprograms, &pendingSubprogram{
				subprogram: t.subprogram,
				unit:       t.unit,
				probesCfgs: t.probesCfgs,
			})
		}
		return nil
	case *inlinedSubroutineChildVisitor:
		return nil
	case *abstractSubprogramVisitor:
		return nil
	default:
		return fmt.Errorf("unexpected visitor type for unit child: %T", t)
	}
}

func newProbe(
	objFile object.FileWithDwarf,
	probeCfg ir.ProbeDefinition,
	subprogram *ir.Subprogram,
	eventIDAlloc *idAllocator[ir.EventID],
	prologueLocs map[uint64]prologueEndLocation,
) (*ir.Probe, ir.Issue, error) {
	kind := probeCfg.GetKind()
	if !kind.IsValid() {
		return nil, ir.Issue{
			Kind:    ir.IssueKindInvalidProbeDefinition,
			Message: fmt.Sprintf("invalid probe kind: %v", kind),
		}, nil
	}

	var injectionPoints []ir.InjectionPoint
	if subprogram.OutOfLinePCRanges == nil && len(subprogram.InlinePCRanges) == 0 {
		return nil, ir.Issue{
			Kind:    ir.IssueKindMalformedExecutable,
			Message: fmt.Sprintf("subprogram %s has no pc ranges", subprogram.Name),
		}, nil
	}
	if subprogram.OutOfLinePCRanges != nil {
		pc := subprogram.OutOfLinePCRanges[0][0]
		loc, ok := prologueLocs[pc]
		if !ok {
			return nil, ir.Issue{}, fmt.Errorf("missing prologue loc for: %d", pc)
		}
		if loc.err != nil {
			return nil, ir.Issue{
				Kind:    ir.IssueKindInvalidDWARF,
				Message: loc.err.Error(),
			}, nil
		}
		arch := objFile.Architecture()
		switch arch {
		case "amd64":
			// Do nothing
		case "arm64":
			// On arm, the prologue end is marked before the stack allocation.
			loc.frameless = true
		default:
			return nil, ir.Issue{}, fmt.Errorf("unsupported architecture: %s", arch)
		}
		injectionPoints = append(injectionPoints, ir.InjectionPoint{
			PC:        loc.pc,
			Frameless: loc.frameless,
		})
	}
	for _, inlined := range subprogram.InlinePCRanges {
		pc := inlined.RootRanges[0][0]
		loc, ok := prologueLocs[pc]
		if !ok {
			return nil, ir.Issue{}, fmt.Errorf("missing prologue loc for: %d", pc)
		}
		injectionPoints = append(injectionPoints, ir.InjectionPoint{
			PC:        inlined.Ranges[0][0],
			Frameless: loc.frameless,
		})
	}

	// TODO: Find the return locations and add a return event.
	events := []*ir.Event{
		{
			ID:              eventIDAlloc.next(),
			InjectionPoints: injectionPoints,
			Condition:       nil,
			// Will be populated after all the types have been resolved
			// and placeholders have been filled in.
			Type: nil,
		},
	}
	probe := &ir.Probe{
		ProbeDefinition: probeCfg,
		Subprogram:      subprogram,
		Events:          events,
	}
	return probe, ir.Issue{}, nil
}

func findPrologueEnd(
	lineReader *dwarf.LineReader, r ir.PCRange,
) (injectionPC uint64, ok bool, err error) {
	var lineEntry dwarf.LineEntry
	prevPos := lineReader.Tell()
	// In general, SeekPC is not the function we're looking for.  We
	// want to seek to the next line entry that's in the range but
	// not necessarily the first one. We add some hacks here that
	// work unless we're at the beginning of a sequence.
	//
	// TODO: Find a way to seek to the first entry in a range rather
	// than just
	err = lineReader.SeekPC(r[0], &lineEntry)
	// If we find that we have a hole, then we'll have our hands on
	// a reader that's positioned after our PC. We can then seek to
	// the instruction prior to that which should be in range of a
	// real sequence. This is grossly inefficient.
	if err != nil &&
		errors.Is(err, dwarf.ErrUnknownPC) &&
		lineEntry.Address < r[0] {
		nextErr := lineReader.Next(&lineEntry)
		if nextErr == nil {
			lineReader.Seek(prevPos)
			nextErr = lineReader.SeekPC(lineEntry.Address-1, &lineEntry)
		}
		if nextErr == nil && lineEntry.Address >= r[0] {
			err = nil
		}
	}
	if err != nil {
		// TODO(XXX): We hit this whenever the function prologue
		// begins.
		lineReader.Seek(prevPos)
		return
	}
	// for whatever reason the entrypoint of a function is marked as a
	// statement and then should come the prologue end. If we see two
	// statements in a row then we're not going to find the prologue end.
	stmtsSeen := 0
	for lineEntry.Address < r[1] && stmtsSeen < 2 {
		if lineEntry.PrologueEnd {
			return lineEntry.Address, true, nil
		}
		if lineEntry.IsStmt {
			stmtsSeen++
		}
		if err := lineReader.Next(&lineEntry); err != nil {
			// TODO(XXX): Should this bail out?
			// In general, if we don't have the proper prologue end
			// and it's not a frameless subprogram, then we're going
			// to have a problem on x86 because we won't know the
			// real cfa. On ARM things may be better.
			break
		}
	}
	return 0, false, nil
}

type subprogramChildVisitor struct {
	root            *rootVisitor
	unit            *dwarf.Entry
	subprogramEntry *dwarf.Entry
	// May be nil if the subprogram is not interesting. We still need to visit it
	// to collect possibly interesting inlined subprograms instances.
	subprogram *ir.Subprogram
	// Cached pc ranges of the subprogram. Calculated on demand from subprogramEntry
	// or when subprogram is set.
	cachedSubprogramPCRanges []ir.PCRange
	probesCfgs               []ir.ProbeDefinition
}

func (v *subprogramChildVisitor) subprogramPCRanges() ([]ir.PCRange, error) {
	if v.cachedSubprogramPCRanges != nil {
		return v.cachedSubprogramPCRanges, nil
	}
	ranges, err := v.root.dwarf.Ranges(v.subprogramEntry)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pc ranges: %w", err)
	}
	v.cachedSubprogramPCRanges = ranges
	return ranges, nil
}

func (v *subprogramChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	var isParameter bool
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		rootPCRanges, err := v.subprogramPCRanges()
		if err != nil {
			return nil, err
		}
		return processInlinedSubroutineEntry(v.root, v.unit, false /* outOfLineInstance */, rootPCRanges, entry)
	case dwarf.TagFormalParameter:
		isParameter = true
		fallthrough
	case dwarf.TagVariable:
		if v.subprogram != nil {
			variable, err := processVariable(
				v.unit, entry, isParameter,
				true, /* parseLocations */
				v.subprogram.OutOfLinePCRanges,
				v.root.loclistReader,
				v.root.pointerSize,
				v.root.typeCatalog,
			)
			if err != nil {
				return nil, err
			}
			v.subprogram.Variables = append(v.subprogram.Variables, variable)
		}
		return nil, nil
	case dwarf.TagTypedef:
		// Typedefs occur for generic type parameters and carry their dictionary
		// index.
		return nil, nil
	case dwarf.TagLexDwarfBlock:
		return v, nil
	default:
		return nil, fmt.Errorf(
			"unexpected tag for subprogram child: %s", entry.Tag,
		)
	}
}

func (v *subprogramChildVisitor) pop(_ *dwarf.Entry, _ visitor) error {
	return nil
}

func processInlinedSubroutineEntry(
	root *rootVisitor,
	unit *dwarf.Entry,
	outOfLineInstance bool,
	rootPCRanges []ir.PCRange,
	subroutine *dwarf.Entry,
) (childVisitor visitor, err error) {
	abstractOrigin, err := getAttr[dwarf.Offset](subroutine, dwarf.AttrAbstractOrigin)
	if err != nil {
		return nil, err
	}
	ranges, err := root.dwarf.Ranges(subroutine)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pc ranges %w", err)
	}
	sp := &inlinedSubprogram{
		abstractOrigin: abstractOrigin,
		rootPCRanges:   rootPCRanges,
	}
	if outOfLineInstance {
		sp.outOfLinePCRanges = ranges
		rootPCRanges = ranges
	} else {
		sp.inlinedPCRanges = ir.InlinePCRanges{
			Ranges:     ranges,
			RootRanges: rootPCRanges,
		}
	}
	root.inlinedSubprograms[unit] = append(root.inlinedSubprograms[unit], sp)
	return &inlinedSubroutineChildVisitor{
		root:         root,
		unit:         unit,
		rootPCRanges: rootPCRanges,
		sp:           sp,
	}, nil
}

func processVariable(
	unit, entry *dwarf.Entry,
	isParameter, parseLocations bool,
	subprogramPCRanges []ir.PCRange,
	loclistReader *loclist.Reader,
	pointerSize uint8,
	typeCatalog *typeCatalog,
) (*ir.Variable, error) {
	name, err := getAttr[string](entry, dwarf.AttrName)
	if err != nil {
		return nil, err
	}
	typeOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
	if err != nil {
		return nil, err
	}
	typ, err := typeCatalog.addType(typeOffset)
	if err != nil {
		return nil, err
	}
	var locations []ir.Location
	if parseLocations {
		if locField := entry.AttrField(dwarf.AttrLocation); locField != nil {
			// Note that it's a bit wasteful to compute all the locations
			// here: we only really need to locations for some specific
			// PCs (such as the prologue end), but we don't know what
			// those PCs are here, and figuring them out can be expensive.
			locations, err = computeLocations(
				unit, subprogramPCRanges, typ, locField, loclistReader,
				pointerSize,
			)
			if err != nil {
				return nil, err
			}
		}
	}
	isReturn, _, err := maybeGetAttr[bool](entry, dwarf.AttrVarParam)
	if err != nil {
		return nil, err
	}
	return &ir.Variable{
		Name:        name,
		Type:        typ,
		Locations:   locations,
		IsParameter: isParameter,
		IsReturn:    isReturn,
	}, nil
}

type abstractSubprogram struct {
	unit       *dwarf.Entry
	probesCfgs []ir.ProbeDefinition
	subprogram *ir.Subprogram
	variables  map[dwarf.Offset]*ir.Variable
	issue      ir.Issue
}

type abstractSubprogramVisitor struct {
	root               *rootVisitor
	unit               *dwarf.Entry
	abstractSubprogram *abstractSubprogram
}

func (v *abstractSubprogramVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	var isParameter bool
	switch entry.Tag {
	case dwarf.TagFormalParameter:
		isParameter = true
		fallthrough
	case dwarf.TagVariable:
		variable, err := processVariable(
			v.unit, entry, isParameter,
			false /* parseLocations */, nil, /* subprogramPCRanges */
			v.root.loclistReader, v.root.pointerSize,
			v.root.typeCatalog,
		)
		if err != nil {
			return nil, err
		}
		v.abstractSubprogram.subprogram.Variables = append(
			v.abstractSubprogram.subprogram.Variables, variable)
		v.abstractSubprogram.variables[entry.Offset] = variable
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected tag for abstract subprogram child: %s", entry.Tag)
}

func (v *abstractSubprogramVisitor) pop(_ *dwarf.Entry, _ visitor) error {
	return nil
}

type inlinedSubprogram struct {
	abstractOrigin dwarf.Offset
	// Exactly one of the following is non-nil. If this is an out-of-line instance,
	// outOfLinePCRanges are set. Otherwise, inlinedPCRanges are set.
	outOfLinePCRanges []ir.PCRange
	inlinedPCRanges   ir.InlinePCRanges
	rootPCRanges      []ir.PCRange
	variables         []*dwarf.Entry
}

type inlinedSubroutineChildVisitor struct {
	root         *rootVisitor
	unit         *dwarf.Entry
	rootPCRanges []ir.PCRange
	sp           *inlinedSubprogram
}

func (v *inlinedSubroutineChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		return processInlinedSubroutineEntry(v.root, v.unit, false /* outOfLineInstance */, v.rootPCRanges, entry)
	case dwarf.TagFormalParameter:
		fallthrough
	case dwarf.TagVariable:
		v.sp.variables = append(v.sp.variables, entry)
		return nil, nil
	case dwarf.TagLexDwarfBlock:
		return v, nil
	case dwarf.TagTypedef:
		return v, nil
	}
	return nil, fmt.Errorf("unexpected tag for inlined subroutine child: %s", entry.Tag)
}

func (v *inlinedSubroutineChildVisitor) pop(_ *dwarf.Entry, _ visitor) error {
	return nil
}

func computeLocations(
	unit *dwarf.Entry,
	subprogramRanges []ir.PCRange,
	typ ir.Type,
	locField *dwarf.Field,
	loclistReader *loclist.Reader,
	pointerSize uint8,
) ([]ir.Location, error) {
	// BUG: We shouldn't pass subprogramRanges below; we should take into
	// consideration the ranges of the current block, not necessarily the ranges
	// of the subprogram.
	return dwarfutil.ProcessLocations(
		locField, unit, loclistReader, subprogramRanges, typ.GetByteSize(), pointerSize)
}

// maybeGetAttr is a helper function that returns the value of an attribute if
// it exists, and a boolean indicating whether the attribute exists.
//
// If the attribute exists but does not have type T, an error is returned.
func maybeGetAttr[T any](
	entry *dwarf.Entry, attr dwarf.Attr,
) (T, bool, error) {
	val := entry.Val(attr)
	if val == nil {
		return *new(T), false, nil
	}
	v, ok := val.(T)
	if !ok {
		return v, false, fmt.Errorf(
			"maybeGetAttrVal: expected %T for attribute %s, got %v (%T)",
			v, attr, val, val,
		)
	}
	return v, true, nil
}

// getAttr is like maybeGetAttrVal, but if the attribute does not exist, an
// error is returned.
func getAttr[T any](entry *dwarf.Entry, attr dwarf.Attr) (T, error) {
	v, ok, err := maybeGetAttr[T](entry, attr)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf(
			"getAttrVal: expected %T for attribute %s, got nil", v, attr,
		)
	}
	return v, nil
}

const runtimePackageName = "runtime"

// interests tracks what compile units and subprograms we're interested in.
type interests struct {
	compileUnits map[string]struct{}
	subprograms  map[string][]ir.ProbeDefinition
}

func makeInterests(cfg []ir.ProbeDefinition) (interests, []ir.ProbeIssue) {
	i := interests{
		compileUnits: make(map[string]struct{}),
		subprograms:  make(map[string][]ir.ProbeDefinition),
	}
	var issues []ir.ProbeIssue
	for _, probe := range cfg {
		switch where := probe.GetWhere().(type) {
		case ir.FunctionWhere:
			methodName := where.Location()
			i.compileUnits[compileUnitFromName(methodName)] = struct{}{}
			i.subprograms[methodName] = append(i.subprograms[methodName], probe)
		default:
			issues = append(issues, ir.ProbeIssue{
				ProbeDefinition: probe,
				Issue: ir.Issue{
					Kind:    ir.IssueKindInvalidProbeDefinition,
					Message: "no where clause specified",
				},
			})
			continue
		}
	}

	return i, issues
}

// Note that this heuristic is flawed: it doesn't handle generics, linkname
// symbols (as often used in the runtime), or symbols that are in assembly.
//
// TODO: Stop trying to guess which compile unit a symbol lives in. It already
// doesn't work for inlines. We'll need to make iterating dwarf more efficient.
func compileUnitFromName(name string) string {
	indexOrZero := func(i int) int {
		if i == -1 {
			return 0
		}
		return i
	}

	// Square brackets aren't allowed in import paths, but package
	// names can appear in generic types, so only look at symbol names
	// up to the first square bracket.
	if bracketIndex := strings.Index(name, "["); bracketIndex != -1 {
		name = name[:bracketIndex]
	}
	lastSlash := indexOrZero(strings.LastIndex(name, "/"))
	firstDotAfterSlash := indexOrZero(strings.Index(name[lastSlash:], "."))
	packageNameEnd := lastSlash + firstDotAfterSlash

	// If there's no dots and no slashes, we're in the runtime package.
	if packageNameEnd == 0 {
		return runtimePackageName
	}
	return name[:packageNameEnd]
}
