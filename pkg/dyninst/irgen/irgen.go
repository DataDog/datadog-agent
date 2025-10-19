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
	"strconv"
	"strings"
	"time"

	pkgerrors "github.com/pkg/errors"
	"golang.org/x/arch/arm64/arm64asm"
	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

var loclistErrorLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)
var invalidGoRuntimeTypeLogLimiter = rate.NewLimiter(rate.Every(10*time.Minute), 10)

// TODO: This code creates a lot of allocations, but we could greatly reduce
// the number of distinct allocations by using a batched allocation scheme.
// Such an approach makes sense because we know the lifetimes of all the
// objects are going to be the same.

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

type section struct {
	header *safeelf.SectionHeader
	data   object.SectionData
}

func generateIR(
	cfg config,
	programID ir.ProgramID,
	objFile object.FileWithDwarf,
	probeDefs []ir.ProbeDefinition,
) (ret *ir.Program, retErr error) {
	defer func() {
		if retErr != nil {
			return
		}
		if len(ret.Probes) == 0 {
			retErr = &ir.NoSuccessfulProbesError{Issues: ret.Issues}
		}
	}()

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

	// Collect line information about subprograms. It's important for
	// performance to batch analysis of each compilation unit, and do it in
	// incremental pc order for each compilation unit.
	lineSearchRanges := make([]lineSearchRange, 0, len(processed.pendingSubprograms))
	for _, sp := range processed.pendingSubprograms {
		for _, pcRange := range sp.outOfLinePCRanges {
			lineSearchRanges = append(lineSearchRanges, lineSearchRange{
				unit:    sp.unit,
				pcRange: pcRange,
			})
		}
		for _, inlined := range sp.inlinePCRanges {
			for _, pcRange := range inlined.RootRanges {
				lineSearchRanges = append(lineSearchRanges, lineSearchRange{
					unit:    sp.unit,
					pcRange: pcRange,
				})
			}
		}
	}

	lineData, err := collectLineData(d, lineSearchRanges)
	if err != nil {
		return nil, err
	}

	idToSub := make(map[ir.SubprogramID]*ir.Subprogram, len(materializedSubprograms))
	for _, sp := range materializedSubprograms {
		idToSub[sp.ID] = sp
	}

	textSection := section{header: objFile.Section(".text")}
	if textSection.header == nil {
		return nil, fmt.Errorf("failed to find text section")
	}
	textSection.data, err = objFile.SectionData(textSection.header)
	if err != nil {
		return nil, fmt.Errorf("failed to load text section: %w", err)
	}
	defer textSection.data.Close()

	// Instantiate probes and gather any probe-related issues.
	probes, subprograms, probeIssues, err := createProbes(
		arch, processed.pendingSubprograms, lineData, idToSub, &textSection,
		cfg.skipReturnEvents,
	)
	if err != nil {
		return nil, err
	}
	issues = append(issues, probeIssues...)

	// Augment return variable locations with ABI-derived information.
	subprogrProbeMap := make(map[ir.SubprogramID][]*ir.Probe)
	for _, probe := range probes {
		subprogrProbeMap[probe.Subprogram.ID] = append(
			subprogrProbeMap[probe.Subprogram.ID], probe,
		)
	}
	for _, sp := range subprograms {
		probesForSubprogram := subprogrProbeMap[sp.ID]
		if err := augmentReturnLocationsFromABI(
			arch, sp, probesForSubprogram,
		); err != nil {
			return nil, fmt.Errorf(
				"failed to augment return locations for %q: %w", sp.Name, err,
			)
		}
	}

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
		variableByName := make(map[string]*ir.Variable, len(p.variables))
		for _, die := range p.variables {
			parseLocs := !p.abstract
			var ranges []ir.PCRange
			if parseLocs {
				ranges = p.outOfLinePCRanges
			}
			v, err := processVariable(
				p.unit, die,
				parseLocs, ranges,
				loclistReader, pointerSize, tc,
			)
			if err != nil {
				return nil, err
			}
			if v != nil {
				if pv, ok := variableByName[v.Name]; ok {
					// Dwarf sometimes contains same variable repeated, incorrectly,
					// which causes trouble in further probe processing.
					// Ignore repeated entries.
					variableByOffset[die.Offset] = pv
					continue
				}
				sp.Variables = append(sp.Variables, v)
				variableByOffset[die.Offset] = v
				variableByName[v.Name] = v
			}
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
				abstractOrigin, ok, err := maybeGetAttr[dwarf.Offset](
					inlVar, dwarf.AttrAbstractOrigin,
				)
				if err != nil {
					return nil, err
				}
				if ok {
					baseVar, found := variableByOffset[abstractOrigin]
					if !found {
						return nil, fmt.Errorf("abstract variable not found for inlined variable @%#x", inlVar.Offset)
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
						p.unit, inlVar,
						true /* parseLocations */, ranges,
						loclistReader, pointerSize, tc,
					)
					if err != nil {
						return nil, err
					}
					if v != nil {
						if pv, ok := variableByName[v.Name]; ok {
							// We only need to merge locations.
							pv.Locations = append(pv.Locations, v.Locations...)
						} else {
							variableByName[v.Name] = v
							sp.Variables = append(sp.Variables, v)
						}
					}
				}
			}
		}
		for _, v := range sp.Variables {
			slices.SortFunc(v.Locations, func(a, b ir.Location) int {
				return cmp.Compare(a.Range[0], b.Range[0])
			})
		}
		subprograms = append(subprograms, sp)
	}

	return subprograms, nil
}

type lineSearchRange struct {
	unit    *dwarf.Entry
	pcRange ir.PCRange
}

type line struct {
	pc          uint64
	line        uint32
	isStatement bool
}

type lineData struct {
	err error
	// PC of prologue end or 0 if no prologue end statement is found.
	prologueEnd uint64
	lines       []line
}

// collectLineData evaluates DWARF line programs to aggregate line data for given ranges.
func collectLineData(
	dwarfData *dwarf.Data,
	searchRanges []lineSearchRange,
) (map[ir.PCRange]lineData, error) {
	if len(searchRanges) == 0 {
		return make(map[ir.PCRange]lineData), nil
	}
	slices.SortFunc(searchRanges, func(a, b lineSearchRange) int {
		return cmp.Or(
			cmp.Compare(a.pcRange[0], b.pcRange[0]),
			cmp.Compare(a.pcRange[1], b.pcRange[1]),
		)
	})
	// Remove duplicates.
	i := 1
	for j := 1; j < len(searchRanges); j++ {
		if searchRanges[i-1].pcRange != searchRanges[j].pcRange {
			if searchRanges[i-1].unit == searchRanges[j].unit &&
				searchRanges[i-1].pcRange[1] > searchRanges[j].pcRange[0] {
				return nil, fmt.Errorf("overlapping line search ranges in unit %#x: %#x and %#x",
					searchRanges[i-1].unit.Offset,
					searchRanges[i-1].pcRange,
					searchRanges[j].pcRange)
			}
			searchRanges[i] = searchRanges[j]
			i++
		}
	}
	searchRanges = searchRanges[:i]

	res := make(map[ir.PCRange]lineData, len(searchRanges))
	var prevUnit *dwarf.Entry
	var lineReader *dwarf.LineReader
	for _, sp := range searchRanges {
		if prevUnit != sp.unit {
			prevUnit = sp.unit
			lr, err := dwarfData.LineReader(prevUnit)
			if err != nil {
				return nil, fmt.Errorf("failed to get line reader: %w", err)
			}
			lineReader = lr
		}
		res[sp.pcRange] = collectLineDataForRange(lineReader, sp.pcRange)
	}
	return res, nil
}

// createProbes instantiates probes for each pending sub-program and gathers any
// probe-specific issues encountered in the process.
func createProbes(
	arch object.Architecture,
	pending []*pendingSubprogram,
	lineData map[ir.PCRange]lineData,
	idToSubprogram map[ir.SubprogramID]*ir.Subprogram,
	textSection *section,
	skipReturnEvents bool,
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
			probe, iss, err := newProbe(
				arch, cfg, sp, &eventIDAlloc, lineData, textSection,
				skipReturnEvents,
			)
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
		pointerSize:         uint8(arch.PointerSize()),
		typeIndexBuilder:    typeIndexBuilder,
	}

	// Visit the entire DWARF tree.
	if err := visitDwarf(d.Reader(), v); err != nil {
		return processedDwarf{}, err
	}

	inlinedSubprograms, err := convertAbstractSubprogramsToPending(
		v.dwarf,
		v.abstractSubprograms,
		v.unitOffsets,
		v.outOfLineSubprogramOffsets,
		v.inlineInstances,
		v.outOfLineInstances,
		&v.subprogramIDAlloc,
	)
	if err != nil {
		return processedDwarf{}, err
	}

	if v.goRuntimeInformation == (ir.GoModuledataInfo{}) {
		return processedDwarf{}, fmt.Errorf("runtime.firstmoduledata not found")
	}
	return processedDwarf{
		pendingSubprograms: append(v.subprograms, inlinedSubprograms...),
		goModuledataInfo:   v.goRuntimeInformation,
		interestingTypes:   v.interestingTypes,
	}, nil
}

// convertAbstractSubprogramsToPending constructs the pending subprograms for the inlined
// subprograms stored in the abstractSubprograms map.
func convertAbstractSubprogramsToPending(
	d *dwarf.Data,
	abstractSubprograms map[dwarf.Offset]*abstractSubprogram,
	unitOffsets []dwarf.Offset,
	outOfLineSubprogramOffsets []dwarf.Offset,
	inlineInstances []concreteSubprogramRef,
	outOfLineInstances []concreteSubprogramRef,
	idAllocator *idAllocator[ir.SubprogramID],
) ([]*pendingSubprogram, error) {
	// Prepare data for enrichment: filter refs without abstract origins and
	// sort all data by offset to enable incremental advancement through the
	// DWARF tree.
	slices.Sort(unitOffsets)
	slices.Sort(outOfLineSubprogramOffsets)
	noAbstractOrigin := func(a concreteSubprogramRef) bool {
		_, ok := abstractSubprograms[a.abstractOrigin]
		return !ok
	}
	inlineInstances = slices.DeleteFunc(inlineInstances, noAbstractOrigin)
	outOfLineInstances = slices.DeleteFunc(outOfLineInstances, noAbstractOrigin)
	slices.SortFunc(inlineInstances, concreteSubprogramRef.cmpByOffset)
	slices.SortFunc(outOfLineInstances, concreteSubprogramRef.cmpByOffset)

	var inlinedInstanceError *inlinedInstanceError
	for ctx, err := range iterConcreteSubprograms(
		d, inlineInstances, unitOffsets, outOfLineSubprogramOffsets,
	) {
		switch {
		case errors.As(err, &inlinedInstanceError):
			abs := abstractSubprograms[inlinedInstanceError.abstractOrigin]
			if abs.issue.IsNone() {
				abs.issue = ir.Issue{
					Kind:    ir.IssueKindInvalidDWARF,
					Message: inlinedInstanceError.err.Error(),
				}
			}
			continue
		case err != nil:
			return nil, err
		}
		abs := abstractSubprograms[ctx.abstractOrigin]
		if !abs.issue.IsNone() {
			continue
		}
		ranges := ir.InlinePCRanges{
			Ranges:     ctx.entryRanges,
			RootRanges: ctx.rootRanges,
		}
		abs.inlined = append(abs.inlined, &inlinedSubprogram{
			abstractOrigin:  ctx.abstractOrigin,
			inlinedPCRanges: ranges,
			variables:       ctx.variables,
		})
		abs.inlinePCRanges = append(abs.inlinePCRanges, ranges)
	}

	for ctx, err := range iterConcreteSubprograms(
		d, outOfLineInstances, unitOffsets, nil, /* no ranges needed */
	) {
		switch {
		case errors.As(err, &inlinedInstanceError):
			abs := abstractSubprograms[inlinedInstanceError.abstractOrigin]
			if abs.issue.IsNone() {
				abs.issue = ir.Issue{
					Kind:    ir.IssueKindInvalidDWARF,
					Message: inlinedInstanceError.err.Error(),
				}
			}
			continue
		case err != nil:
			return nil, err
		}
		abs := abstractSubprograms[ctx.abstractOrigin]
		if !abs.issue.IsNone() {
			continue
		}
		if len(abs.outOfLinePCRanges) != 0 {
			abs.issue = ir.Issue{
				Kind:    ir.IssueKindMalformedExecutable,
				Message: "multiple out-of-line instances of abstract subprogram",
			}
			continue
		}
		outOfLine := &inlinedSubprogram{
			abstractOrigin:    ctx.abstractOrigin,
			outOfLinePCRanges: ctx.entryRanges,
			variables:         ctx.variables,
		}
		abs.outOfLinePCRanges = append(abs.outOfLinePCRanges, ctx.entryRanges...)
		abs.inlined = append(abs.inlined, outOfLine)
	}

	// Append the abstract sub-programs in deterministic order.
	var ret []*pendingSubprogram
	abstractSubs := iterMapSorted(abstractSubprograms, cmp.Compare)
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
		ret = append(ret, &pendingSubprogram{
			unit:              abs.unit,
			subprogramEntry:   nil,
			name:              abs.name,
			outOfLinePCRanges: abs.outOfLinePCRanges,
			inlinePCRanges:    abs.inlinePCRanges,
			inlined:           abs.inlined,
			variables:         varVars,
			probesCfgs:        abs.probesCfgs,
			issue:             abs.issue,
			id:                idAllocator.next(),
			abstract:          true,
		})
	}

	return ret, nil
}

// concreteSubprogramContext is a dwarf entry corresponding to a concrete
// instance of an inlined subprogram, along with a reader positioned at that
// entry, the corresponding abstractOrigin, and, if the entry corresponds to
// an inlined instance, the root pc ranges of the out-of-line subprogram that
// contains it.
type concreteSubprogramContext struct {
	abstractOrigin dwarf.Offset
	entry          *dwarf.Entry
	entryRanges    []ir.PCRange
	reader         *dwarf.Reader
	rootRanges     [][2]uint64 // nil if entry is an out-of-line instance
	variables      []*dwarf.Entry
}

func cmpRange(a, b [2]uint64) int { return cmp.Compare(a[0], b[0]) }

// validateNonOverlappingPCRanges checks that sorted PC ranges do not overlap.
// Returns an error if any ranges overlap.
func validateNonOverlappingPCRanges(
	ranges []ir.PCRange, offset dwarf.Offset, context string,
) error {
	for i := 1; i < len(ranges); i++ {
		if ranges[i-1][1] > ranges[i][0] {
			return fmt.Errorf(
				"overlapping pc ranges for %s at %#x: [%#x, %#x) and [%#x, %#x)",
				context, offset,
				ranges[i-1][0], ranges[i-1][1], ranges[i][0], ranges[i][1],
			)
		}
	}
	return nil
}

type inlinedInstanceError struct {
	abstractOrigin dwarf.Offset
	concreteOffset dwarf.Offset
	err            error
}

func (e *inlinedInstanceError) Error() string {
	return fmt.Sprintf(
		"inlined instance error for abstract origin %#x at offset %#x: %v",
		e.abstractOrigin, e.concreteOffset, e.err,
	)
}

// iterConcreteSubprograms yields concrete subprogram contexts for each
// element of refs.
//
// The function incrementally advances through sorted compile units and concrete
// subprograms (if any), positioning DWARF readers and loading ranges as needed.
//
// Parameters must be sorted by offset:
//   - refs: sorted by offset, then abstractOrigin
//   - units: sorted by offset
//   - concreteSubprograms: sorted by offset (or nil to skip range tracking)
func iterConcreteSubprograms(
	d *dwarf.Data,
	refs []concreteSubprogramRef,
	units []dwarf.Offset,
	concreteSubprograms []dwarf.Offset,
) iter.Seq2[concreteSubprogramContext, error] {
	var (
		unitIdx               int
		concreteSubprogramIdx int
		currentSubprogram     struct {
			offset dwarf.Offset
			entry  *dwarf.Entry
			ranges [][2]uint64
		}
		reader          *dwarf.Reader
		variableVisitor concreteSubprogramVariableCollector
	)

	trackConcreteSubprograms := len(concreteSubprograms) > 0
	if trackConcreteSubprograms {
		currentSubprogram.offset = concreteSubprograms[0]
	}

	maybeAdvanceUnitAndReader := func(refOffset dwarf.Offset) error {
		if reader != nil &&
			(unitIdx+1 >= len(units) || units[unitIdx+1] > refOffset) {
			return nil // no advancement needed
		}
		found, _ := slices.BinarySearch(units[unitIdx:], refOffset)
		if found == 0 {
			return fmt.Errorf("ref %#x precedes first unit", refOffset)
		}
		unitIdx += found - 1
		reader = d.Reader()
		reader.Seek(units[unitIdx])
		if _, err := reader.Next(); err != nil {
			return fmt.Errorf("failed to get next entry: %w", err)
		}
		return nil
	}

	maybeAdvanceRootRanges := func(refOffset dwarf.Offset) error {
		if !trackConcreteSubprograms {
			return nil
		}

		if concreteSubprogramIdx+1 < len(concreteSubprograms) &&
			concreteSubprograms[concreteSubprogramIdx+1] <= refOffset {
			found, _ := slices.BinarySearch(
				concreteSubprograms[concreteSubprogramIdx:], refOffset,
			)
			if found == 0 {
				return fmt.Errorf(
					"ref %#x precedes first concrete subprogram",
					refOffset,
				)
			}
			concreteSubprogramIdx += found - 1
			currentSubprogram.offset = concreteSubprograms[concreteSubprogramIdx]
			currentSubprogram.entry = nil
			currentSubprogram.ranges = nil
		}

		if currentSubprogram.entry == nil {
			reader.Seek(currentSubprogram.offset)
			var err error
			currentSubprogram.entry, err = reader.Next()
			if err != nil {
				return fmt.Errorf(
					"failed to get next concrete subprogram entry: %w",
					err,
				)
			}
			if currentSubprogram.ranges, err = d.Ranges(
				currentSubprogram.entry,
			); err != nil {
				return fmt.Errorf(
					"failed to get ranges for concrete subprogram entry: %w",
					err,
				)
			}
			if len(currentSubprogram.ranges) == 0 {
				return fmt.Errorf("no ranges for concrete subprogram entry")
			}

			slices.SortFunc(currentSubprogram.ranges, cmpRange)
			if err := validateNonOverlappingPCRanges(
				currentSubprogram.ranges,
				currentSubprogram.offset,
				"concrete subprogram",
			); err != nil {
				return err
			}
		}
		return nil
	}

	return func(yield func(concreteSubprogramContext, error) bool) {
		for _, ref := range refs {
			if err := maybeAdvanceUnitAndReader(ref.offset); err != nil {
				yield(concreteSubprogramContext{}, err)
				return
			}
			if err := maybeAdvanceRootRanges(ref.offset); err != nil {
				yield(concreteSubprogramContext{}, err)
				return
			}
			reader.Seek(ref.offset)
			entry, err := reader.Next()
			if err != nil {
				yield(concreteSubprogramContext{}, err)
				return
			}

			inlinedPCRanges, err := d.Ranges(entry)
			if err != nil {
				if !yield(concreteSubprogramContext{}, &inlinedInstanceError{
					abstractOrigin: ref.abstractOrigin,
					concreteOffset: ref.offset,
					err:            err,
				}) {
					return
				}
				continue
			}
			slices.SortFunc(inlinedPCRanges, func(a, b ir.PCRange) int {
				return cmp.Compare(a[0], b[0])
			})
			if err := validateNonOverlappingPCRanges(
				inlinedPCRanges, entry.Offset, "inlined subroutine",
			); err != nil {
				if !yield(concreteSubprogramContext{}, &inlinedInstanceError{
					abstractOrigin: ref.abstractOrigin,
					concreteOffset: ref.offset,
					err:            err,
				}) {
					return
				}
				continue
			}

			variableVisitor = concreteSubprogramVariableCollector{me: entry}
			if err := visitReader(entry, reader, &variableVisitor); err != nil {
				if !yield(concreteSubprogramContext{}, &inlinedInstanceError{
					abstractOrigin: ref.abstractOrigin,
					concreteOffset: ref.offset,
					err:            err,
				}) {
					return
				}
				continue
			}
			if !yield(concreteSubprogramContext{
				reader:         reader,
				rootRanges:     currentSubprogram.ranges,
				abstractOrigin: ref.abstractOrigin,
				entry:          entry,
				entryRanges:    inlinedPCRanges,
				variables:      variableVisitor.variableEntries,
			}, nil) {
				return
			}
		}
	}
}

// concreteSubprogramVariableCollector is a visitor that collects variable DIEs
// for a concrete instance of an inlined subprogram.
type concreteSubprogramVariableCollector struct {
	me              *dwarf.Entry
	variableEntries []*dwarf.Entry
}

func (v *concreteSubprogramVariableCollector) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	if entry == v.me {
		return v, nil
	}
	switch entry.Tag {
	case dwarf.TagFormalParameter, dwarf.TagVariable:
		v.variableEntries = append(v.variableEntries, entry)
		return nil, nil
	case dwarf.TagLexDwarfBlock:
		return v, nil
	case dwarf.TagInlinedSubroutine:
		return nil, nil
	case dwarf.TagTypedef:
		return nil, nil
	default:
		return v, nil
	}
}

func (v *concreteSubprogramVariableCollector) pop(_ *dwarf.Entry, _ visitor) error {
	return nil
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
		var variableKind ir.RootExpressionKind
		switch event.Kind {
		case ir.EventKindEntry:
			if variable.Role != ir.VariableRoleParameter {
				continue
			}
			variableKind = ir.RootExpressionKindArgument
		case ir.EventKindReturn:
			if variable.Role != ir.VariableRoleReturn {
				continue
			}
			variableKind = ir.RootExpressionKindLocal
		case ir.EventKindLine:
			// We capture any variable that is available at any of the injection points.
			// We rely on both locations and injection points being sorted by PC to make
			// this check linear. The location ranges might overlap, but this sweep is
			// still correct.
			available := false
			locIdx := 0
			for _, injectionPoint := range event.InjectionPoints {
				for locIdx < len(variable.Locations) && variable.Locations[locIdx].Range[1] <= injectionPoint.PC {
					locIdx++
				}
				if locIdx < len(variable.Locations) && injectionPoint.PC >= variable.Locations[locIdx].Range[0] {
					available = true
					break
				}
			}
			if !available {
				continue
			}
			variableKind = ir.RootExpressionKindLocal
		default:
			panic(fmt.Sprintf("unexpected event kind: %v", event.Kind))
		}
		expr := newRootExpressionForVariable(variable, variableKind)
		expressions = append(expressions, expr)
	}

	// Process template segments if template exists
	var variableExpressionSet map[string]int
	if probe.Template != nil {
		// Build a set of existing variable expressions for quick lookup
		variableExpressionSet = make(map[string]int, len(expressions))
		for i, expr := range expressions {
			variableExpressionSet[expr.Name] = i
		}

		for i, segment := range probe.Template.Segments {
			jsonSeg, ok := segment.(ir.JSONSegment)
			if !ok {
				continue // Skip string segments
			}

			// Parse the DSL expression
			expr, err := exprlang.Parse(jsonSeg.JSON)
			if err != nil {
				return ir.Issue{
					Kind:    ir.IssueKindUnsupportedFeature,
					Message: fmt.Sprintf("failed to parse template segment: %v", err),
				}
			}

			var refExpr *exprlang.RefExpr
			switch e := expr.(type) {
			case *exprlang.RefExpr:
				refExpr = e
			default:
				return ir.Issue{
					Kind:    ir.IssueKindUnsupportedFeature,
					Message: fmt.Sprintf("unsupported template operation: %T, only 'ref' operation is supported", expr),
				}
			}

			// Find the variable in the subprogram
			var targetVariableRole ir.VariableRole
			switch event.Kind {
			case ir.EventKindEntry:
				targetVariableRole = ir.VariableRoleParameter
			case ir.EventKindReturn:
				targetVariableRole = ir.VariableRoleReturn
			case ir.EventKindLine:
				targetVariableRole = ir.VariableRoleLocal
			default:
				panic(fmt.Sprintf("unexpected event kind: %v", event.Kind))
			}

			var targetVar *ir.Variable
			for _, v := range probe.Subprogram.Variables {
				if v.Role != targetVariableRole {
					continue
				}
				if v.Name == refExpr.Ref {
					targetVar = v
					break
				}
			}
			if targetVar == nil {
				// Variable not found - could be unavailable at this event kind
				continue
			}

			jsonSeg.EventKind = event.Kind
			// Check if we already have an expression for this variable
			if existingIdx, ok := variableExpressionSet[targetVar.Name]; ok {
				// Reuse existing expression index
				jsonSeg.EventExpressionIndex = existingIdx
				probe.Template.Segments[i] = jsonSeg
				continue
			}

			// Create new template segment expression
			templateExpr := newRootExpressionForVariable(targetVar, ir.RootExpressionKindTemplateSegment)
			expressions = append(expressions, templateExpr)
			exprIdx := len(expressions) - 1
			variableExpressionSet[targetVar.Name] = exprIdx

			// Map this segment to the expression
			jsonSeg.EventExpressionIndex = exprIdx
			probe.Template.Segments[i] = jsonSeg
		}
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
	var eventKind string
	if event.Kind != ir.EventKindEntry {
		eventKind = event.Kind.String()
	}
	event.Type = &ir.EventRootType{
		TypeCommon: ir.TypeCommon{
			ID:       id,
			Name:     fmt.Sprintf("Probe[%s]%s", probe.Subprogram.Name, eventKind),
			ByteSize: uint32(byteSize),
		},
		PresenceBitsetSize: presenceBitsetSize,
		Expressions:        expressions,
	}
	typeCatalog.typesByID[event.Type.ID] = event.Type
	return ir.Issue{}
}

// concreteSubprogramRef is a reference to a concrete instance of an inlined
// subprogram. It can reference either an inlined instance or an out-of-line
// instance of the inlined subprogram referenced by the abstract origin.
type concreteSubprogramRef struct {
	offset         dwarf.Offset
	abstractOrigin dwarf.Offset
}

func (c concreteSubprogramRef) cmpByOffset(b concreteSubprogramRef) int {
	return cmp.Or(
		cmp.Compare(c.offset, b.offset),
		cmp.Compare(c.abstractOrigin, b.abstractOrigin),
	)
}

type rootVisitor struct {
	pointerSize         uint8
	interests           interests
	dwarf               *dwarf.Data
	subprogramIDAlloc   idAllocator[ir.SubprogramID]
	subprograms         []*pendingSubprogram
	abstractSubprograms map[dwarf.Offset]*abstractSubprogram

	// Unit offsets are used to track the offsets of the top-level compile
	// units nodes in dwarf.
	//
	// This is needed to properly construct DWARF readers in a way that avoids
	// https://github.com/golang/go/issues/72778 in go 1.24 (where it is still
	// present).
	unitOffsets []dwarf.Offset

	// Dwarf offsets of all out-of-line subprograms that contain inlined
	// subprograms. These may either be non-inlined subprograms or out-of-line
	// instances of inlined subprograms.
	//
	// This is used to find the root PC ranges for inlined instances of inlined
	// subprograms.
	outOfLineSubprogramOffsets []dwarf.Offset

	// All concrete inlined subprogram instances. Note that there can be quite
	// a large number of these, so we intentionally store just the offsets and
	// the abstract origin.
	//
	// We need to store these because we may not yet have visited the abstract
	// origin by the time we visit the concrete instance, and without visiting
	// the abstract origin, we do not know the name, or whether we're interested
	// in the instance.
	inlineInstances []concreteSubprogramRef
	// Similar to inlineInstances, but for out-of-line instances of inlined
	// subprograms.
	outOfLineInstances []concreteSubprogramRef

	interestingTypes []dwarf.Offset
	typeIndexBuilder goTypeToOffsetIndexBuilder

	goRuntimeInformation ir.GoModuledataInfo

	// This is used to avoid allocations of unitChildVisitor for each
	// compile unit.
	freeUnitChildVisitor *unitChildVisitor
}

// couldBeInteresting could possibly be interesting. If we've already visited
// the abstract origin and we didn't put it in our map of abstract subprograms,
// then we know this is not interesting and we don't need to index it.
func (v *rootVisitor) couldBeInteresting(ref concreteSubprogramRef) bool {
	return ref.abstractOrigin > ref.offset ||
		mapContains(v.abstractSubprograms, ref.abstractOrigin)
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
	v.unitOffsets = append(v.unitOffsets, entry.Offset)
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

func mapContains[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
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
			abstractOrigin, err := getAttr[dwarf.Offset](entry, dwarf.AttrAbstractOrigin)
			if err != nil {
				return nil, fmt.Errorf("failed to get abstract origin for out-of-line instance: %w", err)
			}
			if ref := (concreteSubprogramRef{
				offset:         entry.Offset,
				abstractOrigin: abstractOrigin,
			}); v.root.couldBeInteresting(ref) {
				v.root.outOfLineInstances = append(v.root.outOfLineInstances, ref)
			}

			return &inlinedSubroutineChildVisitor{
				root:      v.root,
				outOfLine: true,
			}, nil
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

		return &subprogramChildVisitor{
			root:            v.root,
			subprogramEntry: entry,
			unit:            v.unit,
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

func (v *unitChildVisitor) pop(entry *dwarf.Entry, childVisitor visitor) error {
	switch t := childVisitor.(type) {
	case nil:
		return nil
	case *subprogramChildVisitor:
		if t.hasInlinedSubprograms {
			v.root.outOfLineSubprogramOffsets = append(
				v.root.outOfLineSubprogramOffsets, t.subprogramEntry.Offset)
		}
		if len(t.probesCfgs) > 0 {
			var spName string
			if n, ok, _ := maybeGetAttr[string](t.subprogramEntry, dwarf.AttrName); ok {
				spName = n
			}
			ranges, err := v.root.dwarf.Ranges(t.subprogramEntry)
			var issue ir.Issue
			if err != nil {
				issue = ir.Issue{
					Kind:    ir.IssueKindInvalidDWARF,
					Message: err.Error(),
				}
			} else {
				slices.SortFunc(ranges, cmpRange)
				if err := validateNonOverlappingPCRanges(
					ranges, t.subprogramEntry.Offset, "subprogram",
				); err != nil {
					issue = ir.Issue{
						Kind:    ir.IssueKindInvalidDWARF,
						Message: err.Error(),
					}
				}
			}
			spID := v.root.subprogramIDAlloc.next()
			v.root.subprograms = append(v.root.subprograms, &pendingSubprogram{
				unit:              t.unit,
				subprogramEntry:   t.subprogramEntry,
				name:              spName,
				outOfLinePCRanges: ranges,
				variables:         t.variableEntries,
				probesCfgs:        t.probesCfgs,
				id:                spID,
				issue:             issue,
			})
		}
		return nil
	case *inlinedSubroutineChildVisitor:
		if t.outOfLine && t.hasInlinedSubprograms {
			v.root.outOfLineSubprogramOffsets = append(
				v.root.outOfLineSubprogramOffsets, entry.Offset)
		}
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
	lineData map[ir.PCRange]lineData,
	textSection *section,
	skipReturnEvents bool,
) (*ir.Probe, ir.Issue, error) {
	kind := probeCfg.GetKind()
	if !kind.IsValid() {
		return nil, ir.Issue{
			Kind:    ir.IssueKindInvalidProbeDefinition,
			Message: fmt.Sprintf("invalid probe kind: %v", kind),
		}, nil
	}

	if subprogram.OutOfLinePCRanges == nil && len(subprogram.InlinePCRanges) == 0 {
		return nil, ir.Issue{
			Kind:    ir.IssueKindMalformedExecutable,
			Message: fmt.Sprintf("subprogram %s has no pc ranges", subprogram.Name),
		}, nil
	}
	var injectionPoints []ir.InjectionPoint
	var returnEvent *ir.Event
	if subprogram.OutOfLinePCRanges != nil {
		var issue ir.Issue
		var err error
		injectionPoints, returnEvent, issue, err = pickInjectionPoint(
			eventIDAlloc,
			subprogram.Name,
			subprogram.OutOfLinePCRanges,
			subprogram.OutOfLinePCRanges,
			false, /* inlined */
			probeCfg.GetWhere(),
			arch,
			lineData,
			textSection,
			injectionPoints,
			skipReturnEvents,
		)
		if issue != (ir.Issue{}) || err != nil {
			return nil, issue, err
		}
	}
	for _, inlined := range subprogram.InlinePCRanges {
		var issue ir.Issue
		var err error
		injectionPoints, _, issue, err = pickInjectionPoint(
			eventIDAlloc,
			subprogram.Name,
			inlined.Ranges,
			inlined.RootRanges,
			true, /* inlined */
			probeCfg.GetWhere(),
			arch,
			lineData,
			textSection,
			injectionPoints,
			skipReturnEvents,
		)
		if issue != (ir.Issue{}) || err != nil {
			return nil, issue, err
		}
	}
	slices.SortFunc(injectionPoints, func(a, b ir.InjectionPoint) int {
		return cmp.Compare(a.PC, b.PC)
	})
	var eventKind ir.EventKind
	var sourceLine string
	switch where := probeCfg.GetWhere().(type) {
	case ir.FunctionWhere:
		eventKind = ir.EventKindEntry
	case ir.LineWhere:
		eventKind = ir.EventKindLine
		_, _, sourceLine = where.Line()
	}
	events := []*ir.Event{
		{
			ID:              eventIDAlloc.next(),
			InjectionPoints: injectionPoints,
			Condition:       nil,
			Kind:            eventKind,
			SourceLine:      sourceLine,
			Type:            nil,
		},
	}
	if returnEvent != nil {
		events = append(events, returnEvent)
	}

	segments := []ir.TemplateSegment{}
	probeTemplate := probeCfg.GetTemplate()
	if probeTemplate != nil {
		for seg := range probeTemplate.GetSegments() {
			// See what variables are used in the segment
			// Find the relevant event that this segment should correspond with
			// For example, if the segment is relevant to the entry event, then get the entry event, find the index of the expression within that event, and set the EventExpressionIndex to that index.
			// Similarly, if the segment is relevant to the return event, then get the return event, find the index of the expression within that event, and set the EventExpressionIndex to that index.
			switch s := seg.(type) {
			case rcjson.StringSegment:
				segments = append(segments, ir.StringSegment(s))
			case rcjson.JSONSegment:
				segments = append(segments, ir.JSONSegment{
					JSON: s.GetJSON(),
					DSL:  s.GetDSL(),
				})
			default:
				return nil, ir.Issue{
					Kind:    ir.IssueKindInvalidProbeDefinition,
					Message: fmt.Sprintf("invalid template segment: %T", seg),
				}, nil
			}
		}
	}
	template := &ir.Template{
		TemplateString: probeTemplate.GetTemplateString(),
		Segments:       segments,
	}
	probe := &ir.Probe{
		ProbeDefinition: probeCfg,
		Subprogram:      subprogram,
		Events:          events,
		Template:        template,
	}
	return probe, ir.Issue{}, nil
}

// Returns a list of injection points for a given probe, as well as optional
// return event, if required.
func pickInjectionPoint(
	eventIDAlloc *idAllocator[ir.EventID],
	subprogramName string,
	ranges []ir.PCRange,
	rootRanges []ir.PCRange,
	inlined bool,
	where ir.Where,
	arch object.Architecture,
	lineData map[ir.PCRange]lineData,
	textSection *section,
	buf []ir.InjectionPoint,
	skipReturnEvents bool,
) ([]ir.InjectionPoint, *ir.Event, ir.Issue, error) {
	lines, ok := lineData[rootRanges[0]]
	if !ok {
		return buf, nil, ir.Issue{}, fmt.Errorf("missing line data for range: [0x%x, 0x%x)",
			rootRanges[0][0], rootRanges[0][1])
	}
	if lines.err != nil {
		return buf, nil, ir.Issue{
			Kind:    ir.IssueKindInvalidDWARF,
			Message: lines.err.Error(),
		}, nil
	}
	frameless := lines.prologueEnd == 0
	var returnEvent *ir.Event
	switch where := where.(type) {
	case ir.FunctionWhere:
		if inlined {
			buf = append(buf, ir.InjectionPoint{
				PC:                  ranges[0][0],
				Frameless:           frameless,
				HasAssociatedReturn: false,
			})
		} else {
			pc := ranges[0][0]
			funcByteLen := ranges[0][1] - pc
			data := textSection.data.Data()
			offset := (pc - textSection.header.Addr)
			if offset+funcByteLen > uint64(len(data)) {
				return buf, nil, ir.Issue{
					Kind:    ir.IssueKindInvalidDWARF,
					Message: fmt.Sprintf("function body is too large: %d > %d", offset+funcByteLen, len(data)),
				}, nil
			}
			body := data[offset : offset+funcByteLen]

			// Disassemble the function to find return locations and determine
			// the correct injection point.
			result, issue := disassembleFunction(
				arch, subprogramName, pc, funcByteLen, body, lines,
			)
			if !issue.IsNone() {
				return buf, nil, issue, nil
			}

			returnLocations := result.returnLocations
			injectionPC := result.injectionPC
			topPCOffset := result.topPCOffset

			// Add a workaround for the fact that single-instruction functions
			// would have the same entry and exit probes, but the ordering between
			// them would not be well-defined, so in this extremely uncommon case
			// the user doesn't get to see the return probe. It's okay because
			// there literally cannot be a return value.
			hasAssociatedReturn := !skipReturnEvents
			if len(returnLocations) == 1 && returnLocations[0].PC == pc {
				hasAssociatedReturn = false
				returnLocations = returnLocations[:0]
			}

			buf = append(buf, ir.InjectionPoint{
				PC:                  injectionPC,
				Frameless:           frameless,
				HasAssociatedReturn: hasAssociatedReturn,
				TopPCOffset:         topPCOffset,
			})
			if hasAssociatedReturn {
				returnEvent = &ir.Event{
					ID:              eventIDAlloc.next(),
					InjectionPoints: returnLocations,
					Kind:            ir.EventKindReturn,
					Type:            nil,
				}
			}
		}
	case ir.LineWhere:
		_, _, lineStr := where.Line()
		line, err := strconv.Atoi(lineStr)
		if err != nil {
			return buf, nil, ir.Issue{
				Kind:    ir.IssueKindInvalidProbeDefinition,
				Message: fmt.Sprintf("invalid line number: %v", lineStr),
			}, nil
		}
		injectionPC, issue, err := pickLineInjectionPC(line, ranges, rootRanges, lineData)
		if issue != (ir.Issue{}) || err != nil {
			return buf, nil, issue, err
		}
		buf = append(buf, ir.InjectionPoint{
			PC:                  injectionPC,
			Frameless:           frameless,
			HasAssociatedReturn: false,
		})
	}
	return buf, returnEvent, ir.Issue{}, nil
}

func pickLineInjectionPC(
	line int, ranges []ir.PCRange, rootRanges []ir.PCRange, lineData map[ir.PCRange]lineData,
) (uint64, ir.Issue, error) {
	nonStmtPc := uint64(0)
	rootIdx := 0
	for _, r := range ranges {
		for rootIdx < len(rootRanges) && rootRanges[rootIdx][1] < r[1] {
			rootIdx++
		}
		if rootIdx >= len(rootRanges) || rootRanges[rootIdx][0] > r[0] {
			return 0, ir.Issue{}, fmt.Errorf("no root range found for range: [0x%x, 0x%x)",
				r[0], r[1])
		}
		lines, ok := lineData[rootRanges[rootIdx]]
		if !ok {
			return 0, ir.Issue{}, fmt.Errorf("missing line data for range: [0x%x, 0x%x)",
				r[0], r[1])
		}
		for _, l := range lines.lines {
			if l.pc < r[0] {
				continue
			}
			if l.pc >= r[1] {
				break
			}
			if l.line == uint32(line) {
				if l.isStatement {
					// Statements are preferred as injection points.
					return l.pc, ir.Issue{}, nil
				}
				if nonStmtPc == 0 {
					nonStmtPc = l.pc
				}
			}
		}
	}
	if nonStmtPc == 0 {
		return 0, ir.Issue{
			Kind:    ir.IssueKindInvalidProbeDefinition,
			Message: fmt.Sprintf("no suitable injection point found for line: %v", line),
		}, nil
	}
	return nonStmtPc, ir.Issue{}, nil
}

// disassemblyResult holds the results of architecture-specific function
// disassembly including return locations and prologue adjustments.
type disassemblyResult struct {
	returnLocations []ir.InjectionPoint
	injectionPC     uint64
	topPCOffset     int8
}

// disassembleFunction analyzes a function body to find return locations and
// determine the correct injection point, dispatching to architecture-specific
// implementations.
func disassembleFunction(
	arch object.Architecture,
	subprogramName string,
	pc uint64,
	funcByteLen uint64,
	body []byte,
	loc lineData,
) (disassemblyResult, ir.Issue) {
	switch arch {
	case "amd64":
		return disassembleAmd64Function(pc, body, loc)
	case "arm64":
		return disassembleArm64Function(subprogramName, pc, funcByteLen, body, loc)
	default:
		return disassemblyResult{}, ir.Issue{
			Kind:    ir.IssueKindDisassemblyFailed,
			Message: fmt.Sprintf("unsupported architecture: %s", arch),
		}
	}
}

// disassembleAmd64Function analyzes an amd64 function body to find return
// locations (epilogues).
func disassembleAmd64Function(
	pc uint64, body []byte, loc lineData,
) (disassemblyResult, ir.Issue) {
	injectionPC := loc.prologueEnd
	if injectionPC == 0 {
		injectionPC = pc
	}
	frameless := loc.prologueEnd == 0
	var returnLocations []ir.InjectionPoint
	var prevInst x86asm.Inst
	for offset := 0; offset < len(body); {
		instruction, err := x86asm.Decode(body[offset:], 64)
		if err != nil {
			return disassemblyResult{}, ir.Issue{
				Kind: ir.IssueKindDisassemblyFailed,
				Message: fmt.Sprintf(
					"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
					offset, pc+uint64(offset), body[offset:min(offset+15, len(body))], err,
				),
			}
		}
		if !frameless &&
			instruction.Op == x86asm.POP && instruction.Args[0] == x86asm.RBP &&
			prevInst.Op == x86asm.ADD && prevInst.Args[0] == x86asm.RSP {

			epilogueStart := pc + uint64(offset) - uint64(prevInst.Len)
			maybeRet, err := x86asm.Decode(body[offset+instruction.Len:], 64)
			if err != nil {
				offset := offset + instruction.Len
				return disassemblyResult{}, ir.Issue{
					Kind: ir.IssueKindDisassemblyFailed,
					Message: fmt.Sprintf(
						"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
						offset, pc+uint64(offset), body[offset:min(offset+15, len(body))], err,
					),
				}
			}

			// Sometimes there's nops for inline markers, consume them.
			var nopLen int
			for maybeRet.Op == x86asm.NOP {
				nopLen += maybeRet.Len
				maybeRet, err = x86asm.Decode(body[offset+instruction.Len+nopLen:], 64)
				if err != nil {
					offset := offset + instruction.Len + nopLen
					return disassemblyResult{}, ir.Issue{
						Kind: ir.IssueKindDisassemblyFailed,
						Message: fmt.Sprintf(
							"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
							offset, pc+uint64(offset), body[offset:min(offset+15, len(body))], err,
						),
					}
				}
			}
			if maybeRet.Op == x86asm.RET {
				returnLocations = append(returnLocations, ir.InjectionPoint{
					PC:                  epilogueStart,
					Frameless:           frameless,
					HasAssociatedReturn: false,
				})
				offset += instruction.Len + nopLen
				instruction = maybeRet
			}

		}
		if frameless && instruction.Op == x86asm.RET {
			returnLocations = append(returnLocations, ir.InjectionPoint{
				PC:                  pc + uint64(offset),
				Frameless:           frameless,
				HasAssociatedReturn: false,
			})
		}
		offset += instruction.Len
		prevInst = instruction
	}
	return disassemblyResult{
		returnLocations: returnLocations,
		injectionPC:     injectionPC,
		topPCOffset:     0,
	}, ir.Issue{}
}

// disassembleArm64Function analyzes an ARM64 function body to find return
// locations and adjust the prologue injection point if needed.
func disassembleArm64Function(
	subprogramName string,
	pc uint64,
	funcByteLen uint64,
	body []byte,
	loc lineData,
) (disassemblyResult, ir.Issue) {
	frameless := loc.prologueEnd == 0
	traceEnabled := log.ShouldLog(log.TraceLvl)
	if traceEnabled {
		log.Tracef(
			"decoding arm64 function: %s %#x-%#x: %#x %v",
			subprogramName, pc, pc+funcByteLen, pc, frameless,
		)
	}

	var returnLocations []ir.InjectionPoint
	const instLen = 4
	for offset := 0; offset < len(body); {
		instruction, err := arm64asm.Decode(body[offset:])
		if err != nil {
			offset += instLen
			if traceEnabled {
				log.Tracef(
					"failed to decode arm64 instruction: %v at offset %d of %#x %#x",
					err, offset, pc+uint64(offset), body[offset:min(offset+4, len(body))],
				)
			}
			continue
		}
		if instruction.Op == arm64asm.RET {
			retPC := pc + uint64(offset)
			// NB: it's crude to hard-code that the epilogue is two
			// instructions long but that's what it has been for as long
			// as I've cared to look, and the change coming down the pipe
			// to do something about it also intends to keep it that way.
			//
			// See https://go-review.googlesource.com/c/go/+/674615
			const epilogueByteLen = 2 * instLen
			if !frameless && offset > epilogueByteLen {
				retPC -= epilogueByteLen
			}
			returnLocations = append(returnLocations, ir.InjectionPoint{
				PC:                  retPC,
				Frameless:           frameless,
				HasAssociatedReturn: false,
			})
		}
		offset += 4 // Each instruction is 4 bytes long
	}

	// This is a heuristics to work around the fact that the prologue end
	// marker is not placed after the stack frame has been setup.
	//
	// Instead we recognize that the line table entry following the entry
	// marked as prologue end actually represents the end of the prologue.
	// We also track the topPCOffset to adjust the pc we report in the
	// stack trace because the line we are actually probing may correspond
	// to a different source line than the entrypoint.
	injectionPC := loc.prologueEnd
	if injectionPC == 0 {
		injectionPC = pc
	}
	var topPCOffset int8
	if !frameless {
		idx := slices.IndexFunc(loc.lines, func(line line) bool {
			return line.pc == loc.prologueEnd
		})
		if idx != -1 && idx+1 < len(loc.lines) {
			nextLine := loc.lines[idx+1]
			topPCOffset = int8(pc - nextLine.pc)
			injectionPC = nextLine.pc
		}
	}

	return disassemblyResult{
		returnLocations: returnLocations,
		injectionPC:     injectionPC,
		topPCOffset:     topPCOffset,
	}, ir.Issue{}
}

func collectLineDataForRange(
	lineReader *dwarf.LineReader, r ir.PCRange,
) lineData {
	var lineEntry dwarf.LineEntry
	prevPos := lineReader.Tell()
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
		return lineData{err: err}
	}
	prologueEnd := uint64(0)
	lines := []line{}
	for lineEntry.Address < r[1] {
		if lineEntry.PrologueEnd {
			prologueEnd = lineEntry.Address
		}
		lines = append(lines, line{
			pc:          lineEntry.Address,
			line:        uint32(lineEntry.Line),
			isStatement: lineEntry.IsStmt,
		})
		if err := lineReader.Next(&lineEntry); err != nil {
			return lineData{err: err}
		}
	}
	return lineData{
		prologueEnd: prologueEnd,
		lines:       lines,
	}
}

type subprogramChildVisitor struct {
	root            *rootVisitor
	unit            *dwarf.Entry
	subprogramEntry *dwarf.Entry
	probesCfgs      []ir.ProbeDefinition
	// Discovery: collect variable DIEs for later materialization.
	variableEntries       []*dwarf.Entry
	hasInlinedSubprograms bool
}

func (v *subprogramChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		v.hasInlinedSubprograms = true
		v := &inlinedSubroutineChildVisitor{root: v.root}
		return v.push(entry)
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

func processVariable(
	unit, entry *dwarf.Entry,
	parseLocations bool,
	subprogramPCRanges []ir.PCRange,
	loclistReader *loclist.Reader,
	pointerSize uint8,
	typeCatalog *typeCatalog,
) (*ir.Variable, error) {
	name, err := getAttr[string](entry, dwarf.AttrName)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(name, ".") {
		return nil, nil
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
		slices.SortFunc(locations, func(a, b ir.Location) int {
			return cmp.Compare(a.Range[0], b.Range[0])
		})
	}
	isParameter := entry.Tag == dwarf.TagFormalParameter
	isVariable := entry.Tag == dwarf.TagVariable
	isReturn, _, err := maybeGetAttr[bool](entry, dwarf.AttrVarParam)
	if err != nil {
		return nil, err
	}
	var role ir.VariableRole
	if isVariable {
		role = ir.VariableRoleLocal
	} else if isReturn {
		role = ir.VariableRoleReturn
	} else if isParameter {
		role = ir.VariableRoleParameter
	}
	return &ir.Variable{
		Name:      name,
		Type:      typ,
		Locations: locations,
		Role:      role,
	}, nil
}

type abstractSubprogram struct {
	unit       *dwarf.Entry
	probesCfgs []ir.ProbeDefinition
	name       string
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
	variables         []*dwarf.Entry
}

type inlinedSubroutineChildVisitor struct {
	root                  *rootVisitor
	outOfLine             bool
	hasInlinedSubprograms bool
}

func (v *inlinedSubroutineChildVisitor) push(
	entry *dwarf.Entry,
) (childVisitor visitor, err error) {
	switch entry.Tag {
	case dwarf.TagInlinedSubroutine:
		v.hasInlinedSubprograms = true
		abstractOrigin, err := getAttr[dwarf.Offset](entry, dwarf.AttrAbstractOrigin)
		if err != nil {
			return nil, err
		}
		if ref := (concreteSubprogramRef{
			offset:         entry.Offset,
			abstractOrigin: abstractOrigin,
		}); v.root.couldBeInteresting(ref) {
			v.root.inlineInstances = append(v.root.inlineInstances, ref)
		}
		fallthrough
	case dwarf.TagLexDwarfBlock:
		return v, nil
	case dwarf.TagFormalParameter, dwarf.TagVariable, dwarf.TagTypedef, dwarf.TagLabel:
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected tag for inlined subroutine child: %s", entry.Tag)
	}
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
	locations, err := loclist.ProcessLocations(
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
		case ir.LineWhere:
			methodName, _, _ := where.Line()
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

func newRootExpressionForVariable(variable *ir.Variable, variableKind ir.RootExpressionKind) *ir.RootExpression {
	variableSize := variable.Type.GetByteSize()
	return &ir.RootExpression{
		Name:   variable.Name,
		Offset: uint32(0),
		Kind:   variableKind,
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
}
