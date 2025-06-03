// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package irgen generates an IR program from an object file and a list of
// probes.
package irgen

import (
	"debug/dwarf"
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/loclist"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/dyninst/config"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils/locexpr"
)

// TODO: Validate the probes in the config and report things that are
// not supported without just bailing out.

// TODO: This code creates a lot of allocations, but we could greatly reduce
// the number of distinct allocations by using a batched allocation scheme.
// Such an approach makes sense because we know the lifetimes of all the
// objects are going to be the same.

// TODO: Handle creating return events.

// TODO: Handle inline subprograms.

// TODO: Properly set up the presence bitset.

// TODO: Support hmaps.

// GenerateIR generates an IR program from a binary and a list of probes.
func GenerateIR(
	programID ir.ProgramID,
	objFile object.File,
	config []config.Probe,
) (_ *ir.Program, retErr error) {
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
			retErr = errors.Wrap(r, "GenerateIR: panic")
		default:
			retErr = errors.Errorf("GenerateIR: panic: %v", r)
		}
	}()

	// TODO: Rather than failing hard, this should collect up
	// the errors for diagnostics.
	interests, err := makeInterests(config)
	if err != nil {
		return nil, fmt.Errorf("failed to make interests: %w", err)
	}

	ptrSize := objFile.PointerSize()

	d := objFile.DwarfData()
	loclistReader, err := objFile.LoclistReader()
	if err != nil {
		return nil, fmt.Errorf("failed to get loclist reader: %w", err)
	}
	v := &rootVisitor{
		interests:         interests,
		dwarf:             d,
		eventIDAlloc:      idAllocator[ir.EventID]{},
		subprogramIDAlloc: idAllocator[ir.SubprogramID]{},
		subprograms:       make([]*ir.Subprogram, 0),
		typeCatalog:       newTypeCatalog(d, ptrSize),
		object:            objFile,
		loclistReader:     loclistReader,
	}
	if err := visitDwarf(d.Reader(), v); err != nil {
		return nil, err
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

	if err := populateEventsRootExpressions(
		v.probes, v.typeCatalog,
	); err != nil {
		return nil, err
	}
	return &ir.Program{
		ID:          programID,
		Subprograms: v.subprograms,
		Probes:      v.probes,
		Types:       v.typeCatalog.typesByID,
		MaxTypeID:   v.typeCatalog.idAlloc.alloc,
	}, nil
}

func completeGoTypes(tc *typeCatalog) error {
	ids := make([]ir.TypeID, 0, len(tc.typesByID))
	for id := range tc.typesByID {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		t := tc.typesByID[id]
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
				// nothing to do
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

func completeGoMapType(tc *typeCatalog, t *ir.GoMapType) error {
	// Okay we need to convert the header type from a structure type to
	// the appropriate Go-specific type.
	headerType, ok := t.HeaderType.(*ir.StructureType)
	if !ok {
		return fmt.Errorf("header type for map type %q is not a pointer type %T", t.Name, t.HeaderType)
	}
	// Use the type name to determine whether this is an hmap or a swiss map.
	// We could altnatively use the go version or the structure field layout.
	// This works for now.
	switch {
	case strings.HasPrefix(headerType.Name, "map<"):
		return completeSwissMapHeaderType(tc, headerType)
	case strings.HasPrefix(headerType.Name, "hash<"):
		return completeHMapHeaderType(tc, headerType)
	default:
		return fmt.Errorf("unexpected header type for map type %q: %T", t.Name, t.HeaderType)
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
		return *new(T), fmt.Errorf("pointee type %q is not a %T, got %T", ptrType.Pointee.GetName(), new(T), ptrType.Pointee)
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
			ByteSize: 0, // variable size
		},
		Element: tablePtrType,
	}
	tc.typesByID[tablePtrSliceDataType.ID] = tablePtrSliceDataType

	groupSliceType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:       tc.idAlloc.next(),
			Name:     fmt.Sprintf("[]%s.array", groupType.GetName()),
			ByteSize: 0, // variable size
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
			ByteSize: 0, // variable size
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
			ByteSize: 0, // variable size
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

func populateEventsRootExpressions(probes []*ir.Probe, typeCatalog *typeCatalog) error {
	for _, probe := range probes {
		for _, event := range probe.Events {
			id := typeCatalog.idAlloc.next()
			byteSize := uint64(0)
			var expressions []*ir.RootExpression
			for _, variable := range probe.Subprogram.Variables {
				if !variable.IsParameter || variable.IsReturn {
					continue
				}
				variableSize := variable.Type.GetByteSize()
				expr := &ir.RootExpression{
					Name:   variable.Name,
					Offset: uint32(byteSize),
					Expression: ir.Expression{
						Type: variable.Type,
						Operations: []ir.Op{
							&ir.LocationOp{
								Variable: variable,
								Offset:   0,
								Size:     uint32(variableSize),
							},
						},
					},
				}
				expressions = append(expressions, expr)
				byteSize += uint64(variableSize)
			}
			if byteSize > math.MaxUint32 {
				return fmt.Errorf(
					"event %q has too many bytes: %d",
					probe.ID, byteSize,
				)
			}
			event.Type = &ir.EventRootType{
				TypeCommon: ir.TypeCommon{
					ID: id,
					// TODO: Give this a better name.
					Name:     "ProbeEvent",
					ByteSize: uint32(byteSize),
				},
				// TODO: Populate the presence bitset size and expressions.
				PresenseBitsetSize: 0,
				Expressions:        expressions,
			}
			typeCatalog.typesByID[event.Type.ID] = event.Type
		}
	}
	return nil
}

type rootVisitor struct {
	object            object.File
	interests         interests
	dwarf             *dwarf.Data
	eventIDAlloc      idAllocator[ir.EventID]
	subprogramIDAlloc idAllocator[ir.SubprogramID]
	subprograms       []*ir.Subprogram
	probes            []*ir.Probe
	typeCatalog       *typeCatalog
	loclistReader     *object.LoclistReader

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
	name, ok, err := maybeGetAttr[string](entry, dwarf.AttrName)
	if err != nil {
		return nil, fmt.Errorf("failed to get name for compile unit: %w", err)
	}
	if !ok {
		return nil, nil
	}
	if _, ok := v.interests.compileUnits[name]; !ok {
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
	unitVisitor.unitEntry = entry
	return unitVisitor
}

func (v *rootVisitor) putUnitVisitor(unitVisitor *unitChildVisitor) {
	if v.freeUnitChildVisitor == nil {
		unitVisitor.unitEntry = nil
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
	root      *rootVisitor
	unitEntry *dwarf.Entry

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
			// TODO: Handle out-of-line instances of inline
			// subprograms.
			_, ok, err := maybeGetAttr[dwarf.Offset](
				entry, dwarf.AttrAbstractOrigin,
			)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf(
					"expected subprogram without name or abstract origin",
				)
			}
			return nil, nil
		}
		inline, ok, err := maybeGetAttr[int64](entry, dwarf.AttrInline)
		if err != nil {
			return nil, err
		}
		// TODO: Handle inline subprograms.
		if ok && inline == dwInlInlined {
			return nil, nil
		}

		cfgProbes, ok := v.root.interests.subprograms[name]
		if !ok {
			return nil, nil
		}
		ranges, err := v.root.dwarf.Ranges(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to parse pc ranges: %w", err)
		}
		return &subprogramChildVisitor{
			root:            v.root,
			subprogramEntry: entry,
			unitEntry:       v.unitEntry,
			cfgProbes:       cfgProbes,
			ranges:          ranges,
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
		name, err := getAttr[string](t.subprogramEntry, dwarf.AttrName)
		if err != nil {
			return fmt.Errorf("failed to get subprogram name: %w", err)
		}

		subprogram := &ir.Subprogram{
			ID:                v.root.subprogramIDAlloc.next(),
			Name:              name,
			Variables:         t.variables,
			OutOfLinePCRanges: t.ranges,
		}

		// Here we want to convert the config probes into IR probes.
		cfgProbes := t.cfgProbes
		for _, cfgProbe := range cfgProbes {
			probe, err := t.newProbe(cfgProbe, subprogram)
			if err != nil {
				// TODO: We should collect up all the errors rather than
				// returning the first one.
				return fmt.Errorf(
					"failed to create probe %s: %w",
					cfgProbe.GetID(), err,
				)
			}
			v.root.probes = append(v.root.probes, probe)
		}
		v.root.subprograms = append(v.root.subprograms, subprogram)
		return nil
	default:
		return fmt.Errorf("unexpected visitor type for unit child: %T", t)
	}
}

func (v *subprogramChildVisitor) newProbe(
	cfgProbe config.Probe,
	subprogram *ir.Subprogram,
) (*ir.Probe, error) {
	kind, err := getProbeKind(cfgProbe)
	if err != nil {
		return nil, fmt.Errorf("failed to get probe kind: %w", err)
	}
	var captureSnapshot bool
	if lp, ok := cfgProbe.(*config.LogProbe); ok {
		captureSnapshot = lp.CaptureSnapshot
	}

	lineReader, err := v.root.dwarf.LineReader(v.unitEntry)
	if err != nil {
		return nil, fmt.Errorf("failed to get line reader: %w", err)
	}
	prologueEnd, err := findPrologueEnd(lineReader, v.ranges)
	if err != nil {
		return nil, err
	}

	// TODO: Find the return locations and add a return event.
	events := []*ir.Event{
		{
			ID:           v.root.eventIDAlloc.next(),
			InjectionPCs: []uint64{prologueEnd},
			Condition:    nil,
			// Will be populated after all the types have been resolved
			// and placeholders have been filled in.
			Type: nil,
		},
	}
	probe := &ir.Probe{
		ID:         cfgProbe.GetID(),
		Subprogram: subprogram,
		Kind:       kind,
		Version:    cfgProbe.GetVersion(),
		Tags:       cfgProbe.GetTags(),
		Events:     events,
		Snapshot:   captureSnapshot,
	}
	return probe, nil
}

func findPrologueEnd(
	lineReader *dwarf.LineReader, ranges []ir.PCRange,
) (prologueEnd uint64, err error) {
	var lineEntry dwarf.LineEntry
	// Note: this is assuming that the ranges are sorted.
	if len(ranges) == 0 {
		return 0, fmt.Errorf("expected at least one range for subprogram")
	}
	// Frameless subprograms have no prologue and so they'll have
	// no prologue end; use the first PC as the prologue end.
	prologueEnd = ranges[0][0]
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
				prologueEnd = lineEntry.Address
				break
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
	return prologueEnd, nil
}

func getProbeKind(cfgProbe config.Probe) (ir.ProbeKind, error) {
	switch ty := cfgProbe.GetType(); ty {
	case config.TypeLogProbe:
		return ir.ProbeKindLog, nil
	case config.TypeMetricProbe:
		return ir.ProbeKindMetric, nil
	case config.TypeSpanProbe:
		return ir.ProbeKindSpan, nil
	default:
		return 0, fmt.Errorf("unexpected probe type: %s", ty)
	}
}

type subprogramChildVisitor struct {
	root            *rootVisitor
	unitEntry       *dwarf.Entry
	subprogramEntry *dwarf.Entry
	variables       []*ir.Variable
	cfgProbes       []config.Probe
	ranges          []ir.PCRange
}

func (v *subprogramChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	var isParameter bool
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		// TODO: Traverse appropriately into inlined subroutines. There's some
		// work to do here because we'll need to know whether we should even have
		// travesed into this parent subprogram in the first place. Today we're
		// doing that based on interests. If we had a way to know where all the
		// instances of a subprogram are, then we could go and populate the
		// inlined instances outside of the core visitation loop.
		return nil, nil
	case dwarf.TagFormalParameter:
		isParameter = true
		fallthrough
	case dwarf.TagVariable:
		name, err := getAttr[string](entry, dwarf.AttrName)
		if err != nil {
			return nil, err
		}
		typeOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
		if err != nil {
			return nil, err
		}
		typ, err := v.root.typeCatalog.addType(typeOffset)
		if err != nil {
			return nil, err
		}
		var locations []ir.Location
		if locField := entry.AttrField(dwarf.AttrLocation); locField != nil {
			// Note that it's a bit wasteful to compute all the locations
			// here: we only really need to locations for some specific
			// PCs (such as the prologue end), but we don't know what
			// those PCs are here, and figuring them out can be expensive.
			var err error
			locations, err = computeLocations(typ, v, locField)
			if err != nil {
				return nil, err
			}
		}
		isReturn, _, err := maybeGetAttr[bool](entry, dwarf.AttrVarParam)
		if err != nil {
			return nil, err
		}
		variable := &ir.Variable{
			Name:        name,
			Type:        typ,
			Locations:   locations,
			IsParameter: isParameter,
			IsReturn:    isReturn,
		}
		v.variables = append(v.variables, variable)
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

func computeLocations(
	typ ir.Type,
	v *subprogramChildVisitor,
	locField *dwarf.Field,
) ([]ir.Location, error) {
	totalSize := int64(typ.GetByteSize())
	pointerSize := int(v.root.object.PointerSize())
	var locations []ir.Location
	switch locField.Class {
	case dwarf.ClassLocListPtr:
		offset, ok := locField.Val.(int64)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		if err := v.root.loclistReader.Seek(v.unitEntry, offset); err != nil {
			return nil, err
		}
		var entry loclist.Entry
		for v.root.loclistReader.Next(&entry) {
			locationPieces, err := locexpr.Exec(
				entry.Instr, totalSize, pointerSize,
			)
			if err != nil {
				return nil, err
			}
			locations = append(locations, ir.Location{
				Range:  ir.PCRange{entry.LowPC, entry.HighPC},
				Pieces: locationPieces,
			})
		}

	case dwarf.ClassExprLoc:
		locationExpression, ok := locField.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf(
				"unexpected location field type: %T", locField.Val,
			)
		}
		locationPieces, err := locexpr.Exec(
			locationExpression, totalSize, pointerSize,
		)
		if err != nil {
			return nil, err
		}
		for _, r := range v.ranges {
			locations = append(locations, ir.Location{
				Range:  r,
				Pieces: locationPieces,
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
	subprograms  map[string][]config.Probe
}

func makeInterests(cfg []config.Probe) (interests, error) {
	i := interests{
		compileUnits: make(map[string]struct{}),
		subprograms:  make(map[string][]config.Probe),
	}
	for _, probe := range cfg {
		where := probe.GetWhere()

		if where == nil {
			return interests{}, fmt.Errorf(
				"no interests found for probe %s", probe,
			)
		}
		methodName := probe.GetWhere().MethodName
		i.compileUnits[compileUnitFromName(methodName)] = struct{}{}
		i.subprograms[methodName] = append(i.subprograms[methodName], probe)
	}
	return i, nil
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
