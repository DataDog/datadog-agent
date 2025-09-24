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
	"container/heap"
	"debug/dwarf"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"math"
	"reflect"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var loclistErrorLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var invalidGoRuntimeTypeLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// TODO: This code creates a lot of allocations, but we could greatly reduce
// the number of distinct allocations by using a batched allocation scheme.
// Such an approach makes sense because we know the lifetimes of all the
// objects are going to be the same.

// TODO: Handle creating return events.

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

	cleanupCloser := func(c io.Closer, name string) func() {
		return func() {
			err := c.Close()
			if err == nil {
				return
			}
			err = fmt.Errorf("failed to close %s: %w", name, err)
			if retErr != nil {
				retErr = errors.Join(retErr, err)
			} else {
				retErr = err
			}
		}
	}

	// Prepare the main DWARF visitor that will gather all the information we
	// need from the binary.
	arch := objFile.Architecture()
	ptrSize := uint8(arch.PointerSize())
	d := objFile.DwarfData()

	typeTab, err := gotype.NewTable(objFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create type table: %w", err)
	}
	defer cleanupCloser(typeTab, "gotype.Table")()
	typeTabSize := typeTab.DataByteSize()

	typeIndexBuilder, err := cfg.typeIndexFactory.newGoTypeToOffsetIndexBuilder(
		programID, typeTabSize,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create type index builder: %w", err)
	}
	defer cleanupCloser(typeIndexBuilder, "type index builder")()

	processed, err := processDwarf(interests, d, arch, typeIndexBuilder)
	if err != nil {
		return nil, err
	}
	// Find prologues need to determine injection points. We make an assumption
	// that prologue, if function has a frame, should be contained within the
	// first pc range of a subprogram. This simplifies the logic slightly.
	prologueSearch := make([]prologueSeachParams, 0, len(processed.pendingSubprograms))
	for _, sp := range processed.pendingSubprograms {
		if len(sp.outOfLinePCRanges) > 0 {
			prologueSearch = append(prologueSearch, prologueSeachParams{
				unit:    sp.unit,
				pcRange: sp.outOfLinePCRanges[0],
			})
		}
		for _, inlined := range sp.inlinePCRanges {
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

	typeCatalog := newTypeCatalog(d, ptrSize)
	var commonTypes ir.CommonTypes
	for _, offset := range processed.interestingTypes {
		t, err := typeCatalog.addType(offset)
		if err != nil {
			return nil, fmt.Errorf("failed to add type at offset %#x: %w", offset, err)
		}
		ok := true
		switch t.GetName() {
		case "runtime.g":
			commonTypes.G, ok = t.(*ir.StructureType)
		case "runtime.m":
			commonTypes.M, ok = t.(*ir.StructureType)
		}
		if !ok {
			return nil, fmt.Errorf("expected structure type for %q, got %T", t.GetName(), t)
		}
	}
	if commonTypes.G == nil {
		return nil, fmt.Errorf("runtime.g not found")
	}
	if commonTypes.M == nil {
		return nil, fmt.Errorf("runtime.m not found")
	}

	// Materialize before creating probes so IR subprograms and vars exist.
	materializedSubprograms, err := materializePending(
		objFile.LoclistReader(), ptrSize, typeCatalog, processed.pendingSubprograms,
	)
	if err != nil {
		return nil, err
	}

	typeIndex, err := typeIndexBuilder.build()
	if err != nil {
		return nil, fmt.Errorf("failed to build type index: %w", err)
	}
	defer cleanupCloser(typeIndex, "type index")()

	ib, err := cfg.typeIndexFactory.newMethodToGoTypeIndexBuilder(programID, typeTabSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create method index builder: %w", err)
	}
	defer cleanupCloser(ib, "method index builder")()

	var methodBuf []gotype.Method
	for tid := range typeIndex.allGoTypes() {
		goType, err := typeTab.ParseGoType(tid)
		if err != nil {
			if !invalidGoRuntimeTypeLogLimiter.Allow() {
				continue
			}
			// We've seen situations where the GoRuntimeType values are bogus.
			// That shouldn't prevent us from generating IR.
			dwarfOffset, offsetOk := typeIndex.resolveDwarfOffset(tid)
			irTypeID, idOk := typeCatalog.typesByDwarfType[dwarfOffset]
			var typeName string
			if t, ok := typeCatalog.typesByID[irTypeID]; offsetOk && idOk && ok {
				typeName = t.GetName()
			}
			log.Warnf(
				"invalid go runtime type id for %q (%d) (dwarf offset: %#x): %v",
				typeName, irTypeID, dwarfOffset, err,
			)
			continue
		}
		methodBuf, err = goType.Methods(methodBuf[:0])
		if err != nil {
			return nil, fmt.Errorf("failed to get methods: %w", err)
		}
		for _, m := range methodBuf {
			if err := ib.addMethod(m, tid); err != nil {
				return nil, fmt.Errorf("failed to add method implementation: %w", err)
			}
		}
	}
	methodIndex, err := ib.build()
	if err != nil {
		return nil, fmt.Errorf("failed to build method index: %w", err)
	}
	defer cleanupCloser(methodIndex, "method index")()

	// Resolve placeholder types by a unified, budgeted expansion from
	// subprogram parameter roots. Container internals are zero-cost.
	{
		budgets := computeDepthBudgets(processed.pendingSubprograms)
		// Specialize any already-added container types before traversal.
		if err := completeGoTypes(typeCatalog, 1, typeCatalog.idAlloc.alloc); err != nil {
			return nil, err
		}
		if err := expandTypesWithBudgets(
			typeCatalog, typeTab, methodIndex, typeIndex, materializedSubprograms, budgets,
		); err != nil {
			return nil, err
		}
	}

	idToSub := make(map[ir.SubprogramID]*ir.Subprogram, len(materializedSubprograms))
	for _, sp := range materializedSubprograms {
		idToSub[sp.ID] = sp
	}
	// Instantiate probes and gather any probe-related issues.
	probes, subprograms, probeIssues, err := createProbes(
		arch, processed.pendingSubprograms, prologueLocs, idToSub,
	)
	if err != nil {
		return nil, err
	}
	issues = append(issues, probeIssues...)

	// Finalize type information now that we have all referenced types.
	if err := finalizeTypes(typeCatalog, materializedSubprograms); err != nil {
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
		ID:               programID,
		Subprograms:      subprograms,
		Probes:           probes,
		Types:            typeCatalog.typesByID,
		MaxTypeID:        typeCatalog.idAlloc.alloc,
		Issues:           issues,
		GoModuledataInfo: processed.goModuledataInfo,
		CommonTypes:      commonTypes,
	}, nil
}

// computeDepthBudgets returns the maximum reference depth per subprogram ID
// across all probes configured for that subprogram.
func computeDepthBudgets(pending []*pendingSubprogram) map[ir.SubprogramID]uint32 {
	budgets := make(map[ir.SubprogramID]uint32, len(pending))
	for _, p := range pending {
		var maxDepth uint32
		for _, cfg := range p.probesCfgs {
			maxDepth = max(maxDepth, cfg.GetCaptureConfig().GetMaxReferenceDepth())
		}
		budgets[p.id] = maxDepth
	}
	return budgets
}

type typeQueueEntry struct {
	id        ir.TypeID
	remaining uint32
}
type typeQueue struct {
	items []typeQueueEntry
	pos   map[ir.TypeID]uint32 // current index in items if present
}

var _ heap.Interface = (*typeQueue)(nil)

// heap.Interface
func (q *typeQueue) Len() int { return len(q.items) }
func (q *typeQueue) Less(i, j int) bool {
	// Explore the types with the highest remaining budget first.
	return cmp.Or(
		cmp.Compare(q.items[i].remaining, q.items[j].remaining),
		cmp.Compare(q.items[i].id, q.items[j].id),
	) < 0
}
func (q *typeQueue) Swap(i, j int) {
	q.items[i], q.items[j] = q.items[j], q.items[i]
	q.pos[q.items[i].id] = uint32(i)
	q.pos[q.items[j].id] = uint32(j)
}

// No-op the heap interface methods so we avoid allocations.
func (q *typeQueue) Push(any) {}
func (q *typeQueue) Pop() any { return nil }

func (q *typeQueue) push(e typeQueueEntry) {
	q.items = append(q.items, e)
	q.pos[e.id] = uint32(len(q.items) - 1)
	heap.Push(q, nil)
}
func (q *typeQueue) pop() typeQueueEntry {
	heap.Pop(q)
	n := len(q.items)
	e := q.items[n-1]
	q.items = q.items[:n-1]
	delete(q.pos, e.id)
	return e
}

func newTypeQueue() *typeQueue {
	return &typeQueue{
		pos: make(map[ir.TypeID]uint32),
	}
}

// expandTypesWithBudgets performs a unified graph expansion starting from all
// subprogram parameter roots, observing per-subprogram depth budgets. Only
// pointer dereferences consume depth; container internals (strings, slices,
// maps) are zero-cost. Newly materialized types are immediately completed to
// ensure correct container specialization.
func expandTypesWithBudgets(
	tc *typeCatalog,
	goTypes *gotype.Table,
	methodIndex methodToGoTypeIndex,
	gotypeTypeIndex goTypeToOffsetIndex,
	subprograms []*ir.Subprogram,
	budgets map[ir.SubprogramID]uint32,
) error {
	// Track the best (maximum) processed remaining depth per type.
	processedBest := make(map[ir.TypeID]uint32)

	q := newTypeQueue()

	// Initialize the type queue with all types with a depth budget of 0 to make
	// sure we properly explore every type. In general there's an invariant that
	// every non-placeholder type be explored, and this ensures that.
	for id, t := range tc.typesByID {
		if _, ok := t.(*pointeePlaceholderType); ok {
			continue
		}
		q.pos[id] = uint32(len(q.items))
		q.items = append(q.items, typeQueueEntry{id: id, remaining: 0})
	}

	// Update the budgets from the subprogram variables to that of the
	// corresponding subprogram.
	for _, sp := range subprograms {
		budget := budgets[sp.ID]
		for _, v := range sp.Variables {
			pos := q.pos[v.Type.GetID()]
			item := &q.items[pos]
			item.remaining = max(item.remaining, budget)
		}
	}

	// Initialize the heap now that everything has been updated.
	heap.Init(q)

	// push enqueues (or improves) only if strictly better than any
	// already processed or enqueued remaining budget.
	push := func(t ir.Type, remaining uint32) {
		id := t.GetID()
		if r, ok := processedBest[id]; ok && remaining <= r {
			return
		}
		if idx, ok := q.pos[id]; ok {
			if remaining <= q.items[idx].remaining {
				return
			}
			q.items[idx].remaining = remaining
			heap.Fix(q, int(idx))
			return
		}
		q.push(typeQueueEntry{id: id, remaining: remaining})
	}

	// Local helper to complete a just-added type ID.
	ensureCompleted := func(id ir.TypeID) error {
		return completeGoTypes(tc, id, id)
	}

	var methodBuf []gotype.IMethod
	ii := makeImplementorIterator(methodIndex)
	for q.Len() > 0 {
		wi := q.pop()
		if r, ok := processedBest[wi.id]; ok && wi.remaining <= r {
			continue
		}

		// Ensure the current type is specialized before visiting.
		if err := ensureCompleted(wi.id); err != nil {
			return err
		}

		t := tc.typesByID[wi.id]

		switch tt := t.(type) {

		// Nothing to do for these types.
		case *ir.BaseType,
			*ir.EventRootType,
			*ir.GoChannelType,
			*ir.GoEmptyInterfaceType,
			*ir.GoStringDataType,
			*ir.GoSubroutineType,
			*ir.UnresolvedPointeeType,
			*ir.VoidPointerType:

		case *ir.GoInterfaceType:
			if wi.remaining <= 0 {
				break
			}
			// Now we need to iterate through the implementations of the
			// interface.
			grtID, ok := tt.GetGoRuntimeType()
			if !ok {
				break
			}
			grt, err := goTypes.ParseGoType(gotype.TypeID(grtID))
			if err != nil {
				return fmt.Errorf("failed to parse go type for interface %q: %w", tt.GetName(), err)
			}
			iface, ok := grt.Interface()
			if !ok {
				return fmt.Errorf("go type for interface %q is not an interface: %v", tt.GetName(), grt.Kind())
			}
			methods, err := iface.Methods(methodBuf[:0])
			if err != nil {
				return fmt.Errorf("failed to get methods for interface %q: %w", tt.GetName(), err)
			}
			for ii.seek(methods); ii.valid(); ii.next() {
				impl := ii.cur()
				var t ir.Type
				if tid, ok := tc.typesByGoRuntimeType[impl]; ok {
					t = tc.typesByID[tid]
				} else {
					implOffset, ok := gotypeTypeIndex.resolveDwarfOffset(impl)
					if !ok {
						// This is suspicious, but not obviously worth failing out
						// over.
						continue
					}
					if tid, ok := tc.typesByDwarfType[implOffset]; ok {
						t = tc.typesByID[tid]
					} else {
						var err error
						t, err = tc.addType(implOffset)
						if err != nil {
							return fmt.Errorf("failed to add type for implementation of %q: %w", tt.GetName(), err)
						}
						if err := ensureCompleted(t.GetID()); err != nil {
							return fmt.Errorf("failed to complete type for implementation of %q: %w", tt.GetName(), err)
						}
					}
				}
				if ppt, ok := t.(*pointeePlaceholderType); ok {
					var err error
					t, err = tc.addType(ppt.offset)
					if err != nil {
						return fmt.Errorf("failed to add type for implementation of %q: %w", tt.GetName(), err)
					}
					if err := ensureCompleted(t.GetID()); err != nil {
						return fmt.Errorf("failed to complete type for implementation of %q: %w", tt.GetName(), err)
					}
				}
				push(t, wi.remaining-1)
			}

		// Zero-cost neighbors (do not dereference pointers here).
		case *ir.StructureType:
			for i := range tt.RawFields {
				push(tt.RawFields[i].Type, wi.remaining)
			}
		case *ir.GoSliceHeaderType:
			push(tt.Data, wi.remaining)
		case *ir.GoStringHeaderType:
			push(tt.Data, wi.remaining)
		case *ir.ArrayType:
			push(tt.Element, wi.remaining)
		case *ir.GoMapType:
			push(tt.HeaderType, wi.remaining)
		case *ir.GoHMapHeaderType:
			push(tt.BucketsType, wi.remaining)
			push(tt.BucketType, wi.remaining)
		case *ir.GoSwissMapHeaderType:
			push(tt.TablePtrSliceType, wi.remaining)
			push(tt.GroupType, wi.remaining)
		case *ir.GoSliceDataType:
			push(tt.Element, wi.remaining)
		case *ir.GoHMapBucketType:
			push(tt.KeyType, wi.remaining)
			push(tt.ValueType, wi.remaining)
		case *ir.GoSwissMapGroupsType:
			push(tt.GroupType, wi.remaining)
			push(tt.GroupSliceType, wi.remaining)

		// Depth-cost step: pointer dereference.
		case *ir.PointerType:
			if wi.remaining <= 0 {
				break
			}
			if placeholder, ok := tt.Pointee.(*pointeePlaceholderType); ok {
				newT, err := tc.addType(placeholder.offset)
				if err != nil {
					return err
				}
				tt.Pointee = newT
				if err := ensureCompleted(newT.GetID()); err != nil {
					return err
				}
			}
			push(tt.Pointee, wi.remaining-1)

		default:
			return fmt.Errorf("unexpected ir.Type %[1]T: %#[1]v", tt)
		}

		// Mark processed with this best remaining.
		processedBest[wi.id] = wi.remaining
	}

	// Rewrite placeholders to unresolved pointee types.
	var r *dwarf.Reader
	for i := ir.TypeID(1); i <= ir.TypeID(tc.idAlloc.alloc); i++ {
		ppt, ok := tc.typesByID[i].(*pointeePlaceholderType)
		if !ok {
			continue
		}
		if r == nil {
			r = tc.dwarf.Reader()
		}
		r.Seek(ppt.offset)
		entry, err := r.Next()
		if err != nil {
			return fmt.Errorf("failed to get next entry: %w", err)
		}
		if entry == nil {
			return fmt.Errorf("unexpected EOF while reading type")
		}
		name, err := getAttr[string](entry, dwarf.AttrName)
		if err != nil {
			return fmt.Errorf("failed to get name for type: %w", err)
		}
		tc.typesByID[ppt.id] = &ir.UnresolvedPointeeType{
			TypeCommon: ir.TypeCommon{
				ID:       ppt.id,
				Name:     name,
				ByteSize: uint32(tc.ptrSize),
			},
		}
	}
	return nil
}

func materializePending(
	loclistReader *loclist.Reader,
	pointerSize uint8,
	tc *typeCatalog,
	pending []*pendingSubprogram,
) ([]*ir.Subprogram, error) {
	subprograms := make([]*ir.Subprogram, 0, len(pending))
	for _, p := range pending {
		// Build IR subprogram from discovery state.
		sp := &ir.Subprogram{
			ID:                p.id,
			Name:              p.name,
			OutOfLinePCRanges: p.outOfLinePCRanges,
			InlinePCRanges:    p.inlinePCRanges,
		}
		// First, create variables defined directly under the subprogram/abstract DIEs.
		variableByOffset := make(map[dwarf.Offset]*ir.Variable, len(p.variables))
		for _, die := range p.variables {
			parseLocs := !p.abstract
			var ranges []ir.PCRange
			if parseLocs {
				ranges = p.outOfLinePCRanges
			}
			isParameter := die.Tag == dwarf.TagFormalParameter
			if !isParameter {
				continue
			}
			v, err := processVariable(
				p.unit, die, isParameter,
				parseLocs, ranges,
				loclistReader, pointerSize, tc,
			)
			if err != nil {
				return nil, err
			}
			sp.Variables = append(sp.Variables, v)
			variableByOffset[die.Offset] = v
		}
		// Then, propagate locations and define additional vars from inlined instances.
		for _, inl := range p.inlined {
			var ranges []ir.PCRange
			if inl.outOfLinePCRanges != nil {
				ranges = inl.outOfLinePCRanges
			} else {
				ranges = inl.inlinedPCRanges.Ranges
			}
			for _, inlVar := range inl.variables {
				isParameter := inlVar.Tag == dwarf.TagFormalParameter
				if !isParameter {
					continue
				}
				abstractOrigin, ok, err := maybeGetAttr[dwarf.Offset](
					inlVar, dwarf.AttrAbstractOrigin,
				)
				if err != nil {
					return nil, err
				}
				if ok {
					baseVar, found := variableByOffset[abstractOrigin]
					if !found {
						return nil, fmt.Errorf("abstract variable not found for inlined variable")
					}
					if locField := inlVar.AttrField(dwarf.AttrLocation); locField != nil {
						locs := computeLocations(
							p.unit, inlVar.Offset, ranges, baseVar.Type, locField,
							loclistReader, pointerSize,
						)
						baseVar.Locations = append(baseVar.Locations, locs...)
					}
				} else {

					// Fully defined var in the inlined instance.
					v, err := processVariable(
						p.unit, inlVar, isParameter,
						true /* parseLocations */, ranges,
						loclistReader, pointerSize, tc,
					)
					if err != nil {
						return nil, err
					}
					sp.Variables = append(sp.Variables, v)
				}
			}
		}
		subprograms = append(subprograms, sp)
	}

	return subprograms, nil
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
	arch object.Architecture,
	pending []*pendingSubprogram,
	prologueLocs map[uint64]prologueEndLocation,
	idToSubprogram map[ir.SubprogramID]*ir.Subprogram,
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

		sp := idToSubprogram[p.id]
		var haveProbe bool
		for _, cfg := range p.probesCfgs {
			probe, iss, err := newProbe(arch, cfg, sp, &eventIDAlloc, prologueLocs)
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
			subprograms = append(subprograms, sp)
		}
	}

	return probes, subprograms, issues, nil
}

// finalizeTypes resolves placeholder references, computes Go-specific type
// metadata, and rewrites variable type references so that each variable points
// at the fully-resolved type instance.
func finalizeTypes(tc *typeCatalog, subprograms []*ir.Subprogram) error {
	rewritePlaceholderReferences(tc)
	if err := completeGoTypes(tc, 1, tc.idAlloc.alloc); err != nil {
		return err
	}

	visitTypeReferences(tc, func(t *ir.Type) {
		if *t == nil {
			return
		}
		(*t) = tc.typesByID[(*t).GetID()]
	})

	for _, sp := range subprograms {
		for _, v := range sp.Variables {
			v.Type = tc.typesByID[v.Type.GetID()]
		}
	}
	return nil
}

type processedDwarf struct {
	pendingSubprograms []*pendingSubprogram
	goModuledataInfo   ir.GoModuledataInfo
	interestingTypes   []dwarf.Offset
}

// processDwarf walks the DWARF data, collects all subprograms we care about
// (both concrete and abstract), propagates information from inlined instances
// to their abstract origins, and returns the resulting slice together with the
// index of the first abstract sub-program in that slice.
func processDwarf(
	interests interests,
	d *dwarf.Data,
	arch object.Architecture,
	typeIndexBuilder goTypeToOffsetIndexBuilder,
) (processedDwarf, error) {
	v := &rootVisitor{
		interests:           interests,
		dwarf:               d,
		subprogramIDAlloc:   idAllocator[ir.SubprogramID]{},
		abstractSubprograms: make(map[dwarf.Offset]*abstractSubprogram),
		inlinedSubprograms:  make(map[*dwarf.Entry][]*inlinedSubprogram),
		pointerSize:         uint8(arch.PointerSize()),
		typeIndexBuilder:    typeIndexBuilder,
	}

	// Visit the entire DWARF tree.
	if err := visitDwarf(d.Reader(), v); err != nil {
		return processedDwarf{}, err
	}

	// Concrete subprograms are already in v.pendingSubprograms.
	pending := v.subprograms

	// Propagate details from each inlined instance to its abstract origin.
	inlinedByUnit := iterMapSorted(v.inlinedSubprograms, cmpEntry)
	for _, inlinedSubs := range inlinedByUnit {
		for _, inlined := range inlinedSubs {
			abs, ok := v.abstractSubprograms[inlined.abstractOrigin]
			if !ok || !abs.issue.IsNone() {
				continue
			}

			if inlined.outOfLinePCRanges != nil {
				// Out-of-line instance.
				if len(abs.outOfLinePCRanges) != 0 {
					abs.issue = ir.Issue{
						Kind:    ir.IssueKindMalformedExecutable,
						Message: "multiple out-of-line instances of abstract subprogram",
					}
				} else {
					abs.outOfLinePCRanges = inlined.outOfLinePCRanges
				}
			} else {
				// Inlined instance.
				abs.inlinePCRanges = append(abs.inlinePCRanges, inlined.inlinedPCRanges)
			}
			abs.inlined = append(abs.inlined, inlined)
		}
	}

	// Append the abstract sub-programs in deterministic order.
	abstractSubs := iterMapSorted(v.abstractSubprograms, cmp.Compare)
	for _, abs := range abstractSubs {
		// Collect abstract variable DIEs in deterministic order.
		varVars := make([]*dwarf.Entry, 0, len(abs.variables))
		if len(abs.variables) > 0 {
			keys := make([]dwarf.Offset, 0, len(abs.variables))
			for k := range abs.variables {
				keys = append(keys, k)
			}
			slices.SortFunc(keys, func(a, b dwarf.Offset) int { return cmp.Compare(a, b) })
			for _, k := range keys {
				varVars = append(varVars, abs.variables[k])
			}
		}
		pending = append(pending, &pendingSubprogram{
			unit:              abs.unit,
			subprogramEntry:   nil,
			name:              abs.name,
			outOfLinePCRanges: abs.outOfLinePCRanges,
			inlinePCRanges:    abs.inlinePCRanges,
			inlined:           abs.inlined,
			variables:         varVars,
			probesCfgs:        abs.probesCfgs,
			issue:             abs.issue,
			id:                v.subprogramIDAlloc.next(),
			abstract:          true,
		})
	}

	if v.goRuntimeInformation == (ir.GoModuledataInfo{}) {
		return processedDwarf{}, fmt.Errorf("runtime.firstmoduledata not found")
	}
	return processedDwarf{
		pendingSubprograms: pending,
		goModuledataInfo:   v.goRuntimeInformation,
		interestingTypes:   v.interestingTypes,
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

func completeGoTypes(tc *typeCatalog, minID, maxID ir.TypeID) error {
	for i := minID; i <= maxID; i++ {
		switch t := tc.typesByID[i].(type) {
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
					"unexpected Go kind for structure type %q: %v",
					t.Name, t.GoTypeAttributes.GoKind,
				)
			}
		case *ir.GoMapType:
			if err := completeGoMapType(tc, t); err != nil {
				return err
			}
		}
	}
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
	headerType := tc.typesByID[t.HeaderType.GetID()]
	var headerStructureType *ir.StructureType
	switch headerType := headerType.(type) {
	case *ir.StructureType:
		headerStructureType = headerType
	case *ir.GoHMapHeaderType:
		return nil
	case *ir.GoSwissMapHeaderType:
		return nil
	default:
		return fmt.Errorf(
			"header type for map type %q has been completed to %T, expected %T",
			t.Name, headerType, headerStructureType,
		)
	}
	// Convert the header type from a structure type to the appropriate
	// Go-specific type.
	//
	// Use the type name to determine whether this is an hmap or a swiss map.
	// We could alternatively use the go version or the structure field layout.
	// This works for now.
	switch {
	case strings.HasPrefix(headerStructureType.Name, "map<"):
		return completeSwissMapHeaderType(tc, headerStructureType)
	case strings.HasPrefix(headerStructureType.Name, "hash<"):
		return completeHMapHeaderType(tc, headerStructureType)
	default:
		return fmt.Errorf(
			"unexpected header type for map type %q: %q %T",
			t.Name, t.HeaderType.GetName(), t.HeaderType,
		)
	}
}

func field(tc *typeCatalog, st *ir.StructureType, name string) (*ir.Field, error) {
	offset := slices.IndexFunc(st.RawFields, func(f ir.Field) bool {
		return f.Name == name
	})
	if offset == -1 {
		return nil, fmt.Errorf("type %q has no %s field", st.Name, name)
	}
	f := &st.RawFields[offset]
	f.Type = tc.typesByID[f.Type.GetID()]
	return &st.RawFields[offset], nil
}

func fieldType[T ir.Type](tc *typeCatalog, st *ir.StructureType, name string) (T, error) {
	f, err := field(tc, st, name)
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

func resolvePointeeType[T ir.Type](tc *typeCatalog, t ir.Type) (T, error) {
	ptrType, ok := t.(*ir.PointerType)
	if !ok {
		return *new(T), fmt.Errorf("type %q is not a pointer type, got %T", t.GetName(), t)
	}
	ptrType.Pointee = tc.typesByID[ptrType.Pointee.GetID()]
	if ppt, ok := ptrType.Pointee.(*pointeePlaceholderType); ok {
		pointee, err := tc.addType(ppt.offset)
		if err != nil {
			return *new(T), err
		}
		ptrType.Pointee = pointee
	}
	pointee, ok := ptrType.Pointee.(T)
	if !ok {
		return *new(T), fmt.Errorf(
			"pointee type %d %q of %d (%q) is not a %T, got %T",
			ptrType.ID, ptrType.Pointee.GetName(), ptrType.ID, ptrType.Name, new(T), ptrType.Pointee,
		)
	}
	return pointee, nil
}

func completeSwissMapHeaderType(tc *typeCatalog, st *ir.StructureType) error {
	var tablePtrType *ir.PointerType
	var groupReferenceType *ir.StructureType
	var groupType *ir.StructureType
	{
		dirPtrType, err := fieldType[*ir.PointerType](tc, st, "dirPtr")
		if err != nil {
			return err
		}
		tablePtrType, err = resolvePointeeType[*ir.PointerType](tc, dirPtrType)
		if err != nil {
			return err
		}
		tableType, err := resolvePointeeType[*ir.StructureType](tc, tablePtrType)
		if err != nil {
			return err
		}
		groupReferenceType, err = fieldType[*ir.StructureType](tc, tableType, "groups")
		if err != nil {
			return err
		}
		groupPtrType, err := fieldType[*ir.PointerType](tc, groupReferenceType, "data")
		if err != nil {
			return err
		}
		groupType, err = resolvePointeeType[*ir.StructureType](tc, groupPtrType)
		if err != nil {
			return err
		}
	}

	tablePtrSliceDataType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:               tc.idAlloc.next(),
			Name:             fmt.Sprintf("[]%s.array", tablePtrType.GetName()),
			DynamicSizeClass: ir.DynamicSizeHashmap,
			ByteSize:         tablePtrType.GetByteSize(),
		},
		Element: tablePtrType,
	}
	tc.typesByID[tablePtrSliceDataType.ID] = tablePtrSliceDataType

	groupSliceType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:               tc.idAlloc.next(),
			Name:             fmt.Sprintf("[]%s.array", groupType.GetName()),
			DynamicSizeClass: ir.DynamicSizeHashmap,
			ByteSize:         groupType.GetByteSize(),
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
	bucketsField, err := field(tc, st, "buckets")
	if err != nil {
		return err
	}
	bucketsStructType, err := resolvePointeeType[*ir.StructureType](tc, bucketsField.Type)
	if err != nil {
		return err
	}
	keysArrayType, err := fieldType[*ir.ArrayType](tc, bucketsStructType, "keys")
	if err != nil {
		return err
	}
	keyType := keysArrayType.Element
	valuesArrayType, err := fieldType[*ir.ArrayType](tc, bucketsStructType, "values")
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
			ID:               tc.idAlloc.next(),
			Name:             fmt.Sprintf("[]%s.array", bucketsType.GetName()),
			DynamicSizeClass: ir.DynamicSizeHashmap,
			ByteSize:         bucketsType.GetByteSize(),
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
	strField, err := field(tc, st, "str")
	if err != nil {
		return err
	}
	strDataType := &ir.GoStringDataType{
		TypeCommon: ir.TypeCommon{
			ID:               tc.idAlloc.next(),
			Name:             fmt.Sprintf("%s.str", st.Name),
			DynamicSizeClass: ir.DynamicSizeString,
			ByteSize:         1,
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
	arrayField, err := field(tc, st, "array")
	if err != nil {
		return err
	}
	elementType, err := resolvePointeeType[ir.Type](tc, arrayField.Type)
	if err != nil {
		return err
	}
	arrayDataType := &ir.GoSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:               tc.idAlloc.next(),
			Name:             fmt.Sprintf("%s.array", st.Name),
			DynamicSizeClass: ir.DynamicSizeSlice,
			ByteSize:         elementType.GetByteSize(),
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
	interestingTypes   []dwarf.Offset
	typeIndexBuilder   goTypeToOffsetIndexBuilder

	goRuntimeInformation ir.GoModuledataInfo

	// This is used to avoid allocations of unitChildVisitor for each
	// compile unit.
	freeUnitChildVisitor *unitChildVisitor
}

// pendingSubprogram collects DWARF discovery for a subprogram without building
// IR. It holds the DIEs and ranges needed for materialization.
type pendingSubprogram struct {
	unit              *dwarf.Entry
	subprogramEntry   *dwarf.Entry
	variables         []*dwarf.Entry
	name              string
	outOfLinePCRanges []ir.PCRange
	inlinePCRanges    []ir.InlinePCRanges

	// Inlined instances associated with this (abstract) subprogram.
	inlined    []*inlinedSubprogram
	abstract   bool
	id         ir.SubprogramID
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
					name:       name,
					variables:  make(map[dwarf.Offset]*dwarf.Entry),
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

		var subprogramPCRanges []ir.PCRange
		if len(probesCfgs) > 0 {
			var err error
			subprogramPCRanges, err = v.root.dwarf.Ranges(entry)
			if err != nil {
				return nil, fmt.Errorf("failed to parse pc ranges: %w", err)
			}
		}
		return &subprogramChildVisitor{
			root:                     v.root,
			subprogramEntry:          entry,
			unit:                     v.unit,
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

		// Use this as a heuristic to determine if this is a base type or a
		// pointer to a base type. This is handy because these things are
		// sometimes found underneath interfaces. Also, generally it's nice to
		// have some types eagerly added.
		name, ok, err := maybeGetAttr[string](entry, dwarf.AttrName)
		if err != nil || !ok {
			return nil, err
		}

		goAttrs, err := getGoTypeAttributes(entry)
		if err != nil {
			return nil, fmt.Errorf("failed to get go type attributes: %w", err)
		}
		if gt, ok := goAttrs.GetGoRuntimeType(); ok {
			if err := v.root.typeIndexBuilder.addType(
				gotype.TypeID(gt), entry.Offset,
			); err != nil {
				return nil, fmt.Errorf("failed to add type %q: %w", name, err)
			}
		}

		interesting := false
		if entry.Tag != dwarf.TagTypedef && (name == "runtime.g" || name == "runtime.m") {
			interesting = true
		}
		if !interesting {
			nameWithoutStar := name
			if entry.Tag == dwarf.TagPointerType {
				nameWithoutStar = name[1:]
			}
			interesting = primitiveTypeNameRegexp.MatchString(nameWithoutStar)
		}
		if interesting {
			v.root.interestingTypes = append(v.root.interestingTypes, entry.Offset)
		}
		return nil, nil

	case dwarf.TagVariable:
		name, ok, err := maybeGetAttr[string](entry, dwarf.AttrName)
		if err != nil {
			return nil, err
		}
		if !ok || name != "runtime.firstmoduledata" {
			return nil, nil
		}

		typeOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
		if err != nil {
			return nil, fmt.Errorf("failed to get type for runtime.firstmoduledata: %w", err)
		}

		// See https://github.com/golang/go/blob/5a56d884/src/runtime/symtab.go#L414
		byteSize, memberOffset, err := findStructSizeAndMemberOffset(v.root.dwarf, typeOffset, "types")
		if err != nil {
			return nil, fmt.Errorf("failed to find struct size and member offset: %w", err)
		}
		location, err := getAttr[[]byte](entry, dwarf.AttrLocation)
		if err != nil {
			return nil, fmt.Errorf("failed to get location for runtime.firstmoduledata: %w", err)
		}
		instructions, err := loclist.ParseInstructions(location, v.root.pointerSize, byteSize)
		if err != nil {
			return nil, fmt.Errorf("failed to parse location for runtime.firstmoduledata: %w", err)
		}
		if len(instructions) != 1 {
			return nil, fmt.Errorf("runtime.firstmoduledata has %d instructions, expected 1", len(instructions))
		}
		addr, ok := instructions[0].Op.(ir.Addr)
		if !ok {
			return nil, fmt.Errorf("runtime.firstmoduledata is not an address, got %T", instructions[0].Op)
		}
		v.root.goRuntimeInformation = ir.GoModuledataInfo{
			FirstModuledataAddr: addr.Addr,
			TypesOffset:         memberOffset,
		}
		return nil, nil

	case dwarf.TagConstant:
		// TODO: Handle constants.
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected tag for unit child: %s", entry.Tag)
	}
}

// primitiveTypeNameRegexp is a regex that matches the names of primitive types.
// It doesn't match anything with a package or any of the odd internal types
// used by the runtime like sudog<T>.
var primitiveTypeNameRegexp = regexp.MustCompile(`^[a-z]+[0-9]*$`)

// findStructMemberOffset finds the offset of a member in a struct type.
func findStructSizeAndMemberOffset(
	dwarfData *dwarf.Data,
	typeOffset dwarf.Offset,
	memberName string,
) (size uint32, memberOffset uint32, retErr error) {
	reader := dwarfData.Reader()
	reader.Seek(typeOffset)
	entry, err := reader.Next()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get entry: %w", err)
	}
	if entry.Tag != dwarf.TagTypedef {
		return 0, 0, fmt.Errorf("expected typedef type, got %s", entry.Tag)
	}
	underlyingOffset, err := getAttr[dwarf.Offset](entry, dwarf.AttrType)
	if err != nil {
		return 0, 0, fmt.Errorf("missing type for typedef: %w", err)
	}
	reader.Seek(underlyingOffset)
	entry, err = reader.Next()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get entry: %w", err)
	}
	if entry.Tag != dwarf.TagStructType {
		return 0, 0, fmt.Errorf("expected struct type, got %s", entry.Tag)
	}
	if !entry.Children {
		return 0, 0, fmt.Errorf("struct type has no children")
	}
	structSize, err := getAttr[int64](entry, dwarf.AttrByteSize)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get size for struct type: %w", err)
	}
	if structSize < 0 || structSize > math.MaxUint32 {
		return 0, 0, fmt.Errorf("invalid struct size %d", structSize)
	}
	size = uint32(structSize)
	for {
		child, err := reader.Next()
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get next child: %w", err)
		}
		if child == nil {
			return 0, 0, fmt.Errorf("unexpected EOF while reading struct type")
		}
		if child.Tag == 0 {
			break
		}
		if child.Tag == dwarf.TagMember {
			name, err := getAttr[string](child, dwarf.AttrName)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to get name for member: %w", err)
			}
			if name != memberName {
				continue
			}
			offset, err := getAttr[int64](child, dwarf.AttrDataMemberLoc)
			if err != nil {
				return 0, 0, fmt.Errorf("failed to get offset for member: %w", err)
			}
			if offset > math.MaxUint32 {
				return 0, 0, fmt.Errorf("member offset is too large: %d", offset)
			}
			return size, uint32(offset), nil
		}
		if child.Children {
			reader.SkipChildren()
		}
	}
	return 0, 0, fmt.Errorf("member %q not found", memberName)
}

func (v *unitChildVisitor) pop(_ *dwarf.Entry, childVisitor visitor) error {
	switch t := childVisitor.(type) {
	case nil:
		return nil
	case *subprogramChildVisitor:
		if len(t.probesCfgs) > 0 {
			var spName string
			if n, ok, _ := maybeGetAttr[string](t.subprogramEntry, dwarf.AttrName); ok {
				spName = n
			}
			spID := v.root.subprogramIDAlloc.next()
			v.root.subprograms = append(v.root.subprograms, &pendingSubprogram{
				unit:              t.unit,
				subprogramEntry:   t.subprogramEntry,
				name:              spName,
				outOfLinePCRanges: t.cachedSubprogramPCRanges,
				variables:         t.variableEntries,
				probesCfgs:        t.probesCfgs,
				id:                spID,
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
	arch object.Architecture,
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
		// Reset the reader to the previous position which is more efficient
		// than starting from 0 for the next seek given the caller is exploring
		// in PC order.
		lineReader.Seek(prevPos)
		return 0, false, err
	}
	// For whatever reason the entrypoint of a function is marked as a
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
			// Should this return an error?
			//
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
	// Cached pc ranges of the subprogram. Calculated on demand from subprogramEntry.
	cachedSubprogramPCRanges []ir.PCRange
	probesCfgs               []ir.ProbeDefinition
	// Discovery: collect variable DIEs for later materialization.
	variableEntries []*dwarf.Entry
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
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		rootPCRanges, err := v.subprogramPCRanges()
		if err != nil {
			return nil, err
		}
		return processInlinedSubroutineEntry(v.root, v.unit, false /* outOfLineInstance */, rootPCRanges, entry)
	case dwarf.TagFormalParameter:
		fallthrough
	case dwarf.TagVariable:
		// Collect variables only if this subprogram is interesting to us.
		if len(v.probesCfgs) > 0 {
			v.variableEntries = append(v.variableEntries, entry)
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

func (v *subprogramChildVisitor) pop(_ *dwarf.Entry, _ visitor) error { return nil }

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
			locations = computeLocations(
				unit, entry.Offset, subprogramPCRanges, typ, locField, loclistReader,
				pointerSize,
			)
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
	name       string
	id         ir.SubprogramID
	// Aggregated ranges from out-of-line and inlined instances.
	outOfLinePCRanges []ir.PCRange
	inlinePCRanges    []ir.InlinePCRanges
	// Variables defined under the abstract DIE keyed by DIE offset.
	variables map[dwarf.Offset]*dwarf.Entry
	// Inlined instances discovered for this abstract subprogram.
	inlined []*inlinedSubprogram
	issue   ir.Issue
}

type abstractSubprogramVisitor struct {
	root               *rootVisitor
	unit               *dwarf.Entry
	abstractSubprogram *abstractSubprogram
}

func (v *abstractSubprogramVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	switch entry.Tag {
	case dwarf.TagFormalParameter:
		fallthrough
	case dwarf.TagVariable:
		v.abstractSubprogram.variables[entry.Offset] = entry
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
	entryOffset dwarf.Offset,
	subprogramRanges []ir.PCRange,
	typ ir.Type,
	locField *dwarf.Field,
	loclistReader *loclist.Reader,
	pointerSize uint8,
) []ir.Location {
	// BUG: We shouldn't pass subprogramRanges below; we should take into
	// consideration the ranges of the current block, not necessarily the ranges
	// of the subprogram.
	locations, err := dwarfutil.ProcessLocations(
		locField, unit, loclistReader, subprogramRanges, typ.GetByteSize(), pointerSize)
	if err != nil {
		if loclistErrorLogLimiter.Allow() {
			log.Warnf(
				"ignoring locations for variable at 0x%x: %v", entryOffset, err,
			)
		}
		return nil
	}
	return locations
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
