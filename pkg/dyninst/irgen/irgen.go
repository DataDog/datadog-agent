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
	"errors"
	"fmt"
	"strings"

	"github.com/go-delve/delve/pkg/dwarf/loclist"

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
			retErr = fmt.Errorf("GenerateIR: panic: %w", r)
		default:
			retErr = fmt.Errorf("GenerateIR: panic: %v", r)
		}
	}()

	interests := makeInterests(config)

	d := objFile.DwarfData()
	loclistReader, err := objFile.LoclistReader()
	if err != nil {
		return nil, fmt.Errorf("failed to get loclist reader: %w", err)
	}
	v := &rootVisitor{
		interests:     interests,
		dwarf:         d,
		eventIDAlloc:  0,
		subprograms:   make([]*ir.Subprogram, 0),
		typeCatalog:   newTypeCatalog(d),
		object:        objFile,
		loclistReader: loclistReader,
	}
	if err := visitDwarf(d.Reader(), v); err != nil {
		return nil, err
	}
	rewriteTypeReferences(v.typeCatalog)
	populateEventsRootExpressions(v.probes, v.typeCatalog)
	return &ir.Program{
		ID:          programID,
		Subprograms: v.subprograms,
		Probes:      v.probes,
		Types:       v.typeCatalog.typesByID,
		MaxTypeID:   v.typeCatalog.idAlloc,
	}, nil
}

func populateEventsRootExpressions(probes []*ir.Probe, typeCatalog *typeCatalog) {
	for _, probe := range probes {
		for _, event := range probe.Events {
			id := typeCatalog.idAlloc
			typeCatalog.idAlloc++
			byteSize := uint64(0)
			var expressions []*ir.RootExpression
			for _, variable := range probe.Subprogram.Variables {
				if !variable.IsParameter || variable.IsReturn {
					continue
				}
				variableSize := variable.Type.GetByteSize()
				expr := &ir.RootExpression{
					Name:   variable.Name,
					Offset: int(byteSize),
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
				byteSize += variableSize
			}
			event.Type = &ir.EventRootType{
				TypeCommon: ir.TypeCommon{
					ID: id,
					// TODO: Give this a better name.
					Name: "ProbeEvent",
				},
				// TODO: Populate the presence bitset size and expressions.
				PresenseBitsetSize: 0,
				Expressions:        expressions,
			}
		}
	}
}

type rootVisitor struct {
	object        object.File
	interests     interests
	dwarf         *dwarf.Data
	eventIDAlloc  ir.EventID
	subprograms   []*ir.Subprogram
	probes        []*ir.Probe
	typeCatalog   *typeCatalog
	loclistReader *object.LoclistReader

	freeUnitChildVisitor *unitChildVisitor
}

func (v *rootVisitor) allocEventID() ir.EventID {
	v.eventIDAlloc++
	return v.eventIDAlloc
}

func (v *rootVisitor) push(entry *dwarf.Entry) (childVisitor visitor, err error) {
	if entry.Tag != dwarf.TagCompileUnit {
		return nil, nil
	}

	language, ok, err := maybeGetAttrVal[int64](entry, dwarf.AttrLanguage)
	if err != nil {
		return nil, fmt.Errorf("failed to get language for compile unit: %w", err)
	}
	if !ok || language != dwLangGo {
		return nil, nil
	}
	name, ok, err := maybeGetAttrVal[string](entry, dwarf.AttrName)
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
	// For now, we're going to skip types and just visit other parts of subprograms.
	switch entry.Tag {
	case dwarf.TagSubprogram:
		name, ok, err := maybeGetAttrVal[string](entry, dwarf.AttrName)
		if err != nil {
			return nil, err
		}
		if !ok {
			// TODO: Handle out-of-line instances of inline
			// subprograms.
			_, ok, err := maybeGetAttrVal[dwarf.Offset](entry, dwarf.AttrAbstractOrigin)
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("expected subprogram without name or abstract origin")
			}
			return nil, nil
		}
		inline, ok, err := maybeGetAttrVal[int64](entry, dwarf.AttrInline)
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
		// TODO: We've already parsed this node, it's wasteful to parse it again, but
		// we're not going to know whether we need it until later. so for now we'll just skip
		// over all types and come back to them lazily.
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
		name, err := getAttrVal[string](t.subprogramEntry, dwarf.AttrName)
		if err != nil {
			return fmt.Errorf("failed to get subprogram name: %w", err)
		}

		subprogram := &ir.Subprogram{
			Name:              name,
			Variables:         t.variables,
			OutOfLinePCRanges: t.ranges,
		}

		// Here we want to convert the config probes into IR probes.
		cfgProbes := t.cfgProbes
		for _, cfgProbe := range cfgProbes {
			kind, err := getProbeKind(cfgProbe)
			if err != nil {
				return fmt.Errorf("failed to get probe kind: %w", err)
			}
			var captureSnapshot bool
			if lp, ok := cfgProbe.(*config.LogProbe); ok {
				captureSnapshot = lp.CaptureSnapshot
			}

			eventID := v.root.allocEventID()
			// Find the prologue end.
			lineReader, err := v.root.dwarf.LineReader(v.unitEntry)
			if err != nil {
				return fmt.Errorf("failed to get line reader: %w", err)
			}
			var lineEntry dwarf.LineEntry
			// Note: this is assuming that the ranges are sorted.
			if len(t.ranges) == 0 {
				return fmt.Errorf("expected at least one range for subprogram %s", name)
			}
			// Frameless subprograms have no prologue and so they'll have
			// no prologue end; use the first PC as the prologue end.
			prologueEnd := t.ranges[0][0]
			for _, r := range t.ranges {
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
			// TODO: Find the return locations and add a return event.
			events := []*ir.Event{
				{
					ID:           eventID,
					InjectionPCs: []uint64{prologueEnd},
					Condition:    nil,
					// Will be populated after all the types have been resolved
					// and placeholders have been filled in.
					Type: nil,
				},
			}

			v.root.probes = append(v.root.probes, &ir.Probe{
				ID:         cfgProbe.GetID(),
				Subprogram: subprogram,
				Type:       kind,
				Version:    cfgProbe.GetVersion(),
				Tags:       cfgProbe.GetTags(),
				Events:     events,
				Snapshot:   captureSnapshot,
			})
		}
		v.root.subprograms = append(v.root.subprograms, subprogram)
		return nil
	default:
		return fmt.Errorf("unexpected visitor type for unit child: %T", t)
	}
}

func getProbeKind(cfgProbe config.Probe) (ir.ProbeKind, error) {
	switch ty := cfgProbe.GetType(); ty {
	case config.ConfigTypeLogProbe:
		return ir.ProbeKindLog, nil
	case config.ConfigTypeMetricProbe:
		return ir.ProbeKindMetric, nil
	case config.ConfigTypeSpanProbe:
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

func maybeGetAttrVal[T any](entry *dwarf.Entry, attr dwarf.Attr) (T, bool, error) {
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

func getAttrVal[T any](entry *dwarf.Entry, attr dwarf.Attr) (T, error) {
	v, ok, err := maybeGetAttrVal[T](entry, attr)
	if err != nil {
		return v, err
	}
	if !ok {
		return v, fmt.Errorf("getAttrVal: expected %T for attribute %s, got nil", v, attr)
	}
	return v, nil
}

func (v *subprogramChildVisitor) push(entry *dwarf.Entry) (childVisitor visitor, err error) {
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
		name, err := getAttrVal[string](entry, dwarf.AttrName)
		if err != nil {
			return nil, err
		}
		typeOffset, err := getAttrVal[dwarf.Offset](entry, dwarf.AttrType)
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
		isReturn, _, err := maybeGetAttrVal[bool](entry, dwarf.AttrVarParam)
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
		return nil, fmt.Errorf("unexpected tag for subprogram child: %s", entry.Tag)
	}
}

func computeLocations(typ ir.Type, v *subprogramChildVisitor, locField *dwarf.Field) ([]ir.Location, error) {
	totalSize := int64(typ.GetByteSize())
	pointerSize := int(v.root.object.PointerSize())
	var locations []ir.Location
	switch locField.Class {
	case dwarf.ClassLocListPtr:
		offset, ok := locField.Val.(int64)
		if !ok {
			return nil, fmt.Errorf("unexpected location field type: %T", locField.Val)
		}
		if err := v.root.loclistReader.Seek(v.unitEntry, offset); err != nil {
			return nil, err
		}
		var entry loclist.Entry
		for v.root.loclistReader.Next(&entry) {
			locationPieces, err := locexpr.Exec(entry.Instr, totalSize, pointerSize)
			if err != nil {
				return nil, err
			}
			locations = append(locations, ir.Location{
				Range:    ir.PCRange{entry.LowPC, entry.HighPC},
				Location: locationPieces,
			})
		}

	case dwarf.ClassExprLoc:
		locationExpression, ok := locField.Val.([]byte)
		if !ok {
			return nil, fmt.Errorf("unexpected location field type: %T", locField.Val)
		}
		locationPieces, err := locexpr.Exec(locationExpression, totalSize, pointerSize)
		if err != nil {
			return nil, err
		}
		for _, r := range v.ranges {
			locations = append(locations, ir.Location{
				Range:    r,
				Location: locationPieces,
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

func (v *subprogramChildVisitor) pop(_ *dwarf.Entry, _ visitor) error {
	return nil
}

type visitErr struct {
	entry *dwarf.Entry
	cause error
}

func (e *visitErr) Error() string {
	return fmt.Sprintf("%s@0x%x: %v", e.entry.Tag, e.entry.Offset, e.cause)
}

func (e *visitErr) Unwrap() error {
	return e.cause
}

// Visit the DWARF reader, calling the visitor for each compile unit.
func visitDwarf(reader *dwarf.Reader, visitor visitor) (retErr error) {
	for {
		entry, err := reader.Next()
		if err != nil {
			return err
		}
		if entry == nil {
			break
		}
		if err := visitReader(entry, reader, visitor); err != nil {
			return err
		}
	}
	return nil
}

// Visit the current entry, and if it has children and the visitor has returned
// a child visitor, visit the children and then call pop with the visitor.
func visitReader(
	entry *dwarf.Entry,
	reader *dwarf.Reader,
	visitor visitor,
) (retErr error) {
	defer func() {
		if retErr == nil {
			return
		}
		retErr = &visitErr{
			entry: entry,
			cause: retErr,
		}
	}()
	childVisitor, err := visitor.push(entry)
	if err != nil {
		return err
	}
	if entry.Children && childVisitor == nil {
		reader.SkipChildren()
	} else if entry.Children {
		for {
			child, err := reader.Next()
			if err != nil {
				return fmt.Errorf("visitReader: failed to get DWARF child entry: %w", err)
			}
			if child == nil {
				return fmt.Errorf("visitReader: unexpected EOF while reading children")
			}
			if child.Tag == 0 {
				break
			}
			if err := visitReader(child, reader, childVisitor); err != nil {
				return err
			}
		}
	}
	return visitor.pop(entry, childVisitor)
}

type visitor interface {
	push(entry *dwarf.Entry) (childVisitor visitor, err error)
	pop(entry *dwarf.Entry, childVisitor visitor) error
}

const runtimePackageName = "runtime"

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

type interests struct {
	compileUnits map[string]struct{}
	subprograms  map[string][]config.Probe
}

func makeInterests(cfg []config.Probe) interests {
	interests := interests{
		compileUnits: make(map[string]struct{}),
		subprograms:  make(map[string][]config.Probe),
	}
	for _, probe := range cfg {
		methodName := probe.GetWhere().MethodName
		interests.compileUnits[compileUnitFromName(methodName)] = struct{}{}
		interests.subprograms[methodName] = append(interests.subprograms[methodName], probe)
	}
	return interests
}
