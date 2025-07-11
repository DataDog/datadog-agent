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
	"slices"
	"strings"

	pkgerrors "github.com/pkg/errors"

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
	objFile *object.ElfFile,
	probeDefs []ir.ProbeDefinition,
) (*ir.Program, error) {
	return generateIR(g.config, programID, objFile, probeDefs)
}

// GenerateIR generates an IR program from a binary and a list of probes.
// It returns a GeneratedProgram containing both the successful IR program
// and any probes that failed during generation.
func GenerateIR(
	programID ir.ProgramID,
	objFile object.File,
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
	objFile object.File,
	probeDefs []ir.ProbeDefinition,
) (_ *ir.Program, retErr error) {
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
			retErr = pkgerrors.Errorf("GenerateIR: panic: %v", r)
		}
	}()

	interests, issues := makeInterests(probeDefs)

	ptrSize := objFile.PointerSize()

	d := objFile.DwarfData()
	loclistReader, err := objFile.LoclistReader()
	if err != nil {
		return nil, fmt.Errorf("failed to get loclist reader: %w", err)
	}
	v := &rootVisitor{
		interests:           interests,
		dwarf:               d,
		eventIDAlloc:        idAllocator[ir.EventID]{},
		subprogramIDAlloc:   idAllocator[ir.SubprogramID]{},
		subprograms:         nil,
		abstractSubprograms: make(map[dwarf.Offset]*abstractSubprogram),
		inlinedSubprograms:  make(map[*dwarf.Entry][]*inlinedSubprogram),
		typeCatalog: newTypeCatalog(
			d, ptrSize,
			cfg.maxDynamicTypeSize,
			cfg.maxHashBucketsSize,
		),
		object:        objFile,
		loclistReader: loclistReader,
		issues:        issues,
	}
	if err := visitDwarf(d.Reader(), v); err != nil {
		return nil, err
	}
	{
		probes, issues, err := v.instantiateAbstractSubprograms()
		if err != nil {
			return nil, err
		}
		v.issues = append(v.issues, issues...)
		v.probes = append(v.probes, probes...)
	}
	rewritePlaceholderReferences(v.typeCatalog)
	if err := completeGoTypes(v.typeCatalog); err != nil {
		return nil, err
	}

	// Rewrite the variable types to use the complete types.
	for _, subprogram := range v.subprograms {
		for _, variable := range subprogram.Variables {
			variable.Type = v.typeCatalog.typesByID[variable.Type.GetID()]
		}
	}

	probes, issues := populateProbeEventsExpressions(
		v.probes, v.typeCatalog,
	)
	issues = append(v.issues, issues...)

	// Note that findUnusedConfigs will sort the issues and probes.
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
		Subprograms: v.subprograms,
		Probes:      probes,
		Types:       v.typeCatalog.typesByID,
		MaxTypeID:   v.typeCatalog.idAlloc.alloc,
		Issues:      issues,
	}, nil
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

func (v *rootVisitor) instantiateAbstractSubprograms() (
	successes []*ir.Probe, failures []ir.ProbeIssue, err error,
) {
	// For every inlined instance of an abstract subprogram, we will add
	// variable locations from that instantiation. Sort the entries so that the
	// output is deterministic.
	inlinedSubprogramsByUnit := iterMapSorted(v.inlinedSubprograms, cmpEntry)
	for unit, inlinedSubprograms := range inlinedSubprogramsByUnit {
		for _, inlinedSubprogram := range inlinedSubprograms {
			abstractSubprogram, ok := v.abstractSubprograms[inlinedSubprogram.abstractOrigin]
			// Not interesting inlined instance, or already reported an issue.
			//
			// Note that we might want to collect up multiple issues per probe,
			// but the API upstream of us doesn't support that. We short-circuit
			// for simplicity.
			if !ok || !abstractSubprogram.issue.IsNone() {
				continue
			}
			issue := applyInlineToAbstractSubprogram(v, abstractSubprogram, inlinedSubprogram, unit)
			if !issue.IsNone() {
				abstractSubprogram.issue = issue
			}
		}
	}

	abstractSubprograms := iterMapSorted(v.abstractSubprograms, cmp.Compare)
	for _, abstractSubprogram := range abstractSubprograms {
		var hasAtLeastOneProbe bool
		for _, probeCfg := range abstractSubprogram.probesCfgs {
			probe, issue, err := v.newProbe(
				probeCfg, abstractSubprogram.unit, abstractSubprogram.subprogram,
			)
			if err != nil {
				return nil, nil, err // propagate global error
			}
			if !issue.IsNone() {
				failures = append(failures, ir.ProbeIssue{
					ProbeDefinition: probeCfg,
					Issue:           issue,
				})
			} else {
				successes = append(successes, probe)
				hasAtLeastOneProbe = true
			}
		}
		if hasAtLeastOneProbe {
			v.subprograms = append(v.subprograms, abstractSubprogram.subprogram)
		}
	}
	return successes, failures, nil
}

func applyInlineToAbstractSubprogram(
	v *rootVisitor,
	abstractSubprogram *abstractSubprogram,
	inlinedSubprogram *inlinedSubprogram,
	unit *dwarf.Entry,
) ir.Issue {
	if inlinedSubprogram.outOfLineInstance {
		if abstractSubprogram.subprogram.OutOfLinePCRanges != nil {
			return ir.Issue{
				Kind:    ir.IssueKindMalformedExecutable,
				Message: "multiple out-of-line instances of abstract subprogram",
			}
		}
		abstractSubprogram.subprogram.OutOfLinePCRanges = inlinedSubprogram.ranges
	} else {
		abstractSubprogram.subprogram.InlinePCRanges = append(
			abstractSubprogram.subprogram.InlinePCRanges, inlinedSubprogram.ranges)
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
				locations, err = v.computeLocations(
					unit, inlinedSubprogram.ranges, variable.Type, locField)
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
			variable, err = v.processVariable(
				unit, inlinedVariable, isParameter,
				true /* parseLocations */, inlinedSubprogram.ranges)
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
	offset := slices.IndexFunc(st.Fields, func(f ir.Field) bool {
		return f.Name == name
	})
	if offset == -1 {
		return nil, fmt.Errorf("type %q has no %s field", st.Name, name)
	}
	return &st.Fields[offset], nil
}

func fieldType[T ir.Type](st *ir.StructureType, name string) (T, error) {
	f, err := field(st, name)
	if err != nil {
		return *new(T), err
	}
	fieldType, ok := f.Type.(T)
	if !ok {
		return *new(T), fmt.Errorf("field %q is not a %T, got %T", name, new(T), f.Type)
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

func completeHMapHeaderType(_ *typeCatalog, _ *ir.StructureType) error {
	// TODO: Implement this and test it on older versions of Go.
	return fmt.Errorf("hmap support is not implemented")
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
	object              object.File
	interests           interests
	dwarf               *dwarf.Data
	eventIDAlloc        idAllocator[ir.EventID]
	subprogramIDAlloc   idAllocator[ir.SubprogramID]
	subprograms         []*ir.Subprogram
	abstractSubprograms map[dwarf.Offset]*abstractSubprogram
	// InlinedSubprograms grouped by the compilation unit entry.
	inlinedSubprograms map[*dwarf.Entry][]*inlinedSubprogram
	probes             []*ir.Probe
	typeCatalog        *typeCatalog
	loclistReader      *loclist.Reader
	issues             []ir.ProbeIssue

	// This is used to avoid allocations of unitChildVisitor for each
	// compile unit.
	freeUnitChildVisitor *unitChildVisitor
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
			childVisitor, err = processInlinedSubroutineEntry(v.root, v.unit, entry, true /* outOfLineInstance */)
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
		if len(probesCfgs) > 0 {
			ranges, err := v.root.dwarf.Ranges(entry)
			if err != nil {
				return nil, fmt.Errorf("failed to parse pc ranges: %w", err)
			}
			subprogram = &ir.Subprogram{
				ID:                v.root.subprogramIDAlloc.next(),
				Name:              name,
				OutOfLinePCRanges: ranges,
			}
		}
		return &subprogramChildVisitor{
			root:            v.root,
			subprogramEntry: entry,
			unit:            v.unit,
			subprogram:      subprogram,
			probesCfgs:      probesCfgs,
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
		if t.subprogram == nil {
			return nil
		}

		// Here we want to convert the config probes into IR probes.
		for _, probeCfg := range t.probesCfgs {
			probe, issue, err := v.root.newProbe(probeCfg, t.unit, t.subprogram)
			if err != nil {
				return err
			}
			if issue != (ir.Issue{}) {
				v.root.issues = append(v.root.issues, ir.ProbeIssue{
					ProbeDefinition: probeCfg,
					Issue:           issue,
				})
				continue
			}
			v.root.probes = append(v.root.probes, probe)
		}
		v.root.subprograms = append(v.root.subprograms, t.subprogram)
		return nil
	case *inlinedSubroutineChildVisitor:
		return nil
	case *abstractSubprogramVisitor:
		return nil
	default:
		return fmt.Errorf("unexpected visitor type for unit child: %T", t)
	}
}

func (v *rootVisitor) newProbe(
	probeCfg ir.ProbeDefinition,
	unit *dwarf.Entry,
	subprogram *ir.Subprogram,
) (*ir.Probe, ir.Issue, error) {
	kind := probeCfg.GetKind()
	if !kind.IsValid() {
		return nil, ir.Issue{
			Kind:    ir.IssueKindInvalidProbeDefinition,
			Message: fmt.Sprintf("invalid probe kind: %v", kind),
		}, nil
	}

	lineReader, err := v.dwarf.LineReader(unit)
	if err != nil {
		return nil, ir.Issue{}, fmt.Errorf("failed to get line reader: %w", err)
	}
	var injectionPoints []ir.InjectionPoint
	if subprogram.OutOfLinePCRanges == nil && len(subprogram.InlinePCRanges) == 0 {
		return nil, ir.Issue{
			Kind:    ir.IssueKindMalformedExecutable,
			Message: fmt.Sprintf("subprogram %s has no pc ranges", subprogram.Name),
		}, nil
	}
	if subprogram.OutOfLinePCRanges != nil {
		prologueEnd, ok, err := findPrologueEnd(lineReader, subprogram.OutOfLinePCRanges)
		if err != nil {
			return nil, ir.Issue{
				Kind:    ir.IssueKindInvalidDWARF,
				Message: err.Error(),
			}, nil
		}
		if !ok {
			// Frameless subprogram, first PC should be suitable for injection.
			injectionPoints = append(injectionPoints, ir.InjectionPoint{
				PC:        subprogram.OutOfLinePCRanges[0][0],
				Frameless: true,
			})
		} else {
			injectionPoints = append(injectionPoints, ir.InjectionPoint{
				PC:        prologueEnd,
				Frameless: false,
			})
		}
	}
	for _, inlinedInstanceRanges := range subprogram.InlinePCRanges {
		injectionPoints = append(injectionPoints, ir.InjectionPoint{
			PC: inlinedInstanceRanges[0][0],
			// TODO: We need to determine from the inlined parent whether the
			// inlined instance is frameless.
			Frameless: true,
		})
	}

	// TODO: Find the return locations and add a return event.
	events := []*ir.Event{
		{
			ID:              v.eventIDAlloc.next(),
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
	lineReader *dwarf.LineReader, ranges []ir.PCRange,
) (injectionPC uint64, ok bool, err error) {
	var lineEntry dwarf.LineEntry
	// Note: this is assuming that the ranges are sorted.
	if len(ranges) == 0 {
		return 0, false, fmt.Errorf("expected at least one range for subprogram")
	}
	for _, r := range ranges {
		// In general, SeekPC is not the function we're looking for.  We
		// want to seek to the next line entry that's in the range but
		// not necessarily the first one. We add some hacks here that
		// work unless we're at the beginning of a sequence.
		//
		// TODO: Find a way to seek to the first entry in a range rather
		// than just
		err := lineReader.SeekPC(r[0], &lineEntry)
		// If we find that we have a hole, then we'll have our hands on
		// a reader that's positioned after our PC. We can then seek to
		// the instruction prior to that which should be in range of a
		// real sequence. This is grossly inefficient.
		if err != nil &&
			errors.Is(err, dwarf.ErrUnknownPC) &&
			lineEntry.Address < r[0] {
			nextErr := lineReader.Next(&lineEntry)
			if nextErr == nil {
				nextErr = lineReader.SeekPC(lineEntry.Address-1, &lineEntry)
			}
			if nextErr == nil && lineEntry.Address >= r[0] {
				err = nil
			}
		}
		if err != nil {
			// TODO(XXX): We hit this whenever the function prologue
			// begins.
			break
		}
		for lineEntry.Address < r[1] {
			if lineEntry.PrologueEnd {
				return lineEntry.Address, true, nil
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
	probesCfgs []ir.ProbeDefinition
}

func (v *subprogramChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	var isParameter bool
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		return processInlinedSubroutineEntry(v.root, v.unit, entry, false /* outOfLineInstance */)
	case dwarf.TagFormalParameter:
		isParameter = true
		fallthrough
	case dwarf.TagVariable:
		if v.subprogram != nil {
			variable, err := v.root.processVariable(
				v.unit, entry, isParameter,
				true /* parseLocations */, v.subprogram.OutOfLinePCRanges)
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
	subroutine *dwarf.Entry,
	outOfLineInstance bool,
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
		outOfLineInstance: outOfLineInstance,
		abstractOrigin:    abstractOrigin,
		ranges:            ranges,
	}
	root.inlinedSubprograms[unit] = append(root.inlinedSubprograms[unit], sp)
	return &inlinedSubroutineChildVisitor{
		root: root,
		unit: unit,
		sp:   sp,
	}, nil
}

func (v *rootVisitor) processVariable(
	unit, entry *dwarf.Entry,
	isParameter, parseLocations bool,
	subprogramPCRanges []ir.PCRange,
) (*ir.Variable, error) {
	name, err := getAttr[string](entry, dwarf.AttrName)
	if err != nil {
		return nil, err
	}
	typeOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
	if err != nil {
		return nil, err
	}
	typ, err := v.typeCatalog.addType(typeOffset)
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
			locations, err = v.computeLocations(unit, subprogramPCRanges, typ, locField)
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
		variable, err := v.root.processVariable(
			v.unit, entry, isParameter,
			false /* parseLocations */, nil /* subprogramPCRanges */)
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
	outOfLineInstance bool
	abstractOrigin    dwarf.Offset
	ranges            []ir.PCRange
	variables         []*dwarf.Entry
}

type inlinedSubroutineChildVisitor struct {
	root *rootVisitor
	unit *dwarf.Entry
	sp   *inlinedSubprogram
}

func (v *inlinedSubroutineChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		return processInlinedSubroutineEntry(v.root, v.unit, entry, false /* outOfLineInstance */)
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

func (v *rootVisitor) computeLocations(
	unit *dwarf.Entry,
	subprogramRanges []ir.PCRange,
	typ ir.Type,
	locField *dwarf.Field,
) ([]ir.Location, error) {
	totalSize := typ.GetByteSize()
	var locations []ir.Location
	switch locField.Class {
	case dwarf.ClassLocListPtr:
		offset, ok := locField.Val.(int64)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		loclist, err := v.loclistReader.Read(unit, offset, totalSize)
		if err != nil {
			return nil, err
		}
		if len(loclist.Default) > 0 {
			return nil, fmt.Errorf("unexpected default location pieces")
		}
		locations = loclist.Locations

	case dwarf.ClassExprLoc:
		instr, ok := locField.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		pieces, err := loclist.ParseInstructions(instr, v.object.PointerSize(), totalSize)
		if err != nil {
			return nil, err
		}
		// BUG: This should take into consideration the ranges of the current
		// block, not necessarily the ranges of the subprogram.
		for _, r := range subprogramRanges {
			locations = append(locations, ir.Location{
				Range:  r,
				Pieces: pieces,
			})
		}
	default:
		return nil, fmt.Errorf(
			"unexpected %s class: %s",
			locField.Attr, locField.Class,
		)
	}

	return locations, nil
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
