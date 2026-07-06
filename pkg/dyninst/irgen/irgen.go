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
	"encoding/binary"
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

	"golang.org/x/arch/arm64/arm64asm"
	"golang.org/x/arch/x86/x86asm"
	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/loclist"
	"github.com/DataDog/datadog-agent/pkg/dyninst/exprlang"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gosymname"
	"github.com/DataDog/datadog-agent/pkg/dyninst/gotype"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/redaction"
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
	options ...Option,
) (*ir.Program, error) {
	cfg := g.config
	for _, option := range options {
		option.apply(&cfg)
	}
	elfFile, err := cfg.objectLoader.Load(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load elf file: %w", err)
	}
	defer elfFile.Close()
	return generateIR(cfg, programID, elfFile, probeDefs)
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

	// Splice in the synthetic runtime.recovery probe before sort so it
	// flows through the standard symbol-resolution + injection-point
	// pipeline. The probe attaches at runtime.recovery and lets BPF
	// emit synthetic returns for probed frames being unwound by
	// panic+recover (otherwise their in_progress_calls slot and the
	// userspace bufferedEvent leak). The caller can disable this via
	// WithSkipRuntimeRecoveryProbe — used to honor a circuit-breaker
	// trip on the recovery probe.
	if !cfg.skipRuntimeRecoveryProbe {
		probeDefs = maybeAddRuntimeRecoveryProbe(probeDefs)
	}

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
			retErr = fmt.Errorf("GenerateIR: panic: %w", r)
		default:
			retErr = fmt.Errorf("GenerateIR: panic: %v\n%s", r, debug.Stack())
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
		case "runtime._panic":
			commonTypes.Panic, ok = t.(*ir.StructureType)
		}
		if !ok {
			return nil, fmt.Errorf("expected structure type for %q, got %T", t.GetName(), t)
		}
	}
	if commonTypes.G == nil {
		return nil, errors.New("runtime.g not found")
	}
	if commonTypes.M == nil {
		return nil, errors.New("runtime.m not found")
	}

	// Materialize before creating probes so IR subprograms and vars exist.
	materializedSubprograms, err := materializePending(
		objFile.LoclistReader(), ptrSize, typeCatalog, processed.pendingSubprograms, arch,
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

	// Build a set of additional type names requested via
	// WithAdditionalTypes for efficient lookup during the gotype iteration.
	additionalTypeSet := make(map[string]struct{}, len(cfg.additionalTypes))
	for _, name := range cfg.additionalTypes {
		additionalTypeSet[name] = struct{}{}
	}
	specialAdditionalTypeSet := make(map[string]struct{})
	addDDTraceSpanTypeNames(specialAdditionalTypeSet)
	specialAdditionalTypeSet["context.Context"] = struct{}{}
	specialAdditionalTypeOffsets := make(map[string]dwarf.Offset, len(specialAdditionalTypeSet))

	// Context.Context's gotype TypeID, captured while iterating gotypes
	// below, is used after the method index is built to dynamically
	// enumerate every context.Context implementation in the binary via
	// the implementor iterator.
	var contextInterfaceMethods []gotype.IMethod
	// DWARF offsets of every concrete context.Context implementation
	// discovered dynamically via the implementor iterator.
	var contextImplDwarfOffsets []dwarf.Offset

	var additionalTypeRoots []explorationRoot
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

		// If this type was requested as an additional type, resolve it to
		// a DWARF offset and add it to the type catalog for exploration.
		// We match against the package-qualified full name (constructed
		// from PkgPath + "." + lastSegmentOf(Name)) AND the short name —
		// Go's runtime stores type names in short form
		// ("<pkgLastSegment>.<typeName>"), while specialAdditionalTypeSet
		// uses full paths to disambiguate between e.g. v1 and v2
		// dd-trace-go.
		name := goType.Name().UnsafeName()
		fullName := name
		if pkgPath := goType.PkgPath().UnsafeName(); pkgPath != "" {
			star := ""
			short := name
			if strings.HasPrefix(short, "*") {
				star = "*"
				short = short[1:]
			}
			if dot := strings.IndexByte(short, '.'); dot >= 0 {
				fullName = star + pkgPath + short[dot:]
			}
		}
		var matchName string
		if _, special := specialAdditionalTypeSet[fullName]; special {
			matchName = fullName
		} else if _, special := specialAdditionalTypeSet[name]; special {
			matchName = name
		}
		if matchName != "" {
			if dwarfOffset, ok := typeIndex.resolveDwarfOffset(tid); ok {
				specialAdditionalTypeOffsets[matchName] = dwarfOffset
			}
		}
		if contextInterfaceMethods == nil && name == "context.Context" {
			if iface, ok := goType.Interface(); ok {
				ms, err := iface.Methods(nil)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to get methods for context.Context: %w", err,
					)
				}
				contextInterfaceMethods = ms
			}
		}
		if len(additionalTypeSet) > 0 {
			if _, requested := additionalTypeSet[name]; requested {
				if dwarfOffset, ok := typeIndex.resolveDwarfOffset(tid); ok {
					t, addErr := typeCatalog.addType(dwarfOffset)
					if addErr != nil {
						log.Debugf(
							"failed to add additional type %q at offset %#x: %v",
							name, dwarfOffset, addErr,
						)
					} else {
						budget := uint32(additionalTypeBudget)
						if _, special := specialAdditionalTypeSet[name]; special {
							budget = 1
						}
						additionalTypeRoots = append(additionalTypeRoots, explorationRoot{
							typeID: t.GetID(),
							budget: budget,
						})
					}
				}
				delete(additionalTypeSet, name)
			}
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

	// Discover every concrete context.Context implementation in the binary
	// by enumerating types that satisfy context.Context's method set via
	// the method index. This replaces hardcoding the set of known
	// implementations and lets custom user types participate in
	// context-chain decoding.
	if len(contextInterfaceMethods) > 0 {
		ii := makeImplementorIterator(methodIndex)
		for ii.seek(contextInterfaceMethods); ii.valid(); ii.next() {
			dwarfOffset, ok := typeIndex.resolveDwarfOffset(ii.cur())
			if !ok {
				continue
			}
			contextImplDwarfOffsets = append(contextImplDwarfOffsets, dwarfOffset)
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
		for _, inlined := range sp.inlined {
			for _, pcRange := range inlined.inlinedPCRanges.RootRanges {
				lineSearchRanges = append(lineSearchRanges, lineSearchRange{
					unit:    inlined.unit,
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
		return nil, errors.New("failed to find text section")
	}
	textSection.data, err = objFile.SectionData(textSection.header)
	if err != nil {
		return nil, fmt.Errorf("failed to load text section: %w", err)
	}
	defer textSection.data.Close()

	// Instantiate probes and gather any probe-related issues.
	// This must happen before type expansion so we have Events and
	// InjectionPoints for expression analysis.
	probes, subprograms, probeIssues, err := createProbes(
		arch, processed.pendingSubprograms, lineData, idToSub, &textSection,
		cfg.skipReturnEvents,
	)
	if err != nil {
		return nil, err
	}
	issues = append(issues, probeIssues...)

	// Augment return variable locations with ABI-derived information.
	// Collect all instances targeting each subprogram.
	subprogrInstMap := make(map[ir.SubprogramID][]*ir.ProbeInstance)
	for _, probe := range probes {
		for i := range probe.Instances {
			inst := &probe.Instances[i]
			subprogrInstMap[inst.Subprogram.ID] = append(
				subprogrInstMap[inst.Subprogram.ID], inst,
			)
		}
	}
	for _, sp := range subprograms {
		instsForSubprogram := subprogrInstMap[sp.ID]
		if err := augmentReturnLocationsFromABI(
			arch, sp, instsForSubprogram,
		); err != nil {
			return nil, fmt.Errorf(
				"failed to augment return locations for %q: %w", sp.Name, err,
			)
		}
	}

	// Analyze all probe expressions in one pass. This parses expressions once,
	// matches them to variables, checks availability, and computes exploration
	// roots. Must happen before type expansion. Returns one analyzedProbe per
	// instance.
	budgets := computeDepthBudgets(processed.pendingSubprograms)
	analyzedProbes, explorationRoots := analyzeAllProbes(probes, budgets, typeCatalog, cfg.redaction)
	needsGoContextSupport := analyzedProbesContainGoContext(analyzedProbes)
	// Also enable context support if context.Context appears anywhere in
	// the binary's go runtime types (via the special-additional-types
	// gotype iteration). This catches the common case where a probe
	// captures something whose type tree contains a context.Context
	// field but the static walk above sees a placeholder for the
	// transitively-reachable type.
	if !needsGoContextSupport {
		if _, ok := specialAdditionalTypeOffsets["context.Context"]; ok {
			needsGoContextSupport = true
		}
	}
	// IR type IDs of every concrete context.Context implementation pulled
	// into the type catalog. Passed to annotateSpecialGoTypes so it knows
	// which structures to wrap as GoContextImplementationType.
	contextImplIRTypeIDs := make(map[ir.TypeID]struct{})
	if needsGoContextSupport {
		// Pull the context.Context interface itself into the catalog now,
		// so that the unified expansion below has it as a root.
		// context.Context gets budget 2 (enough to dereference the
		// interface to its impl pointer and then dereference that pointer
		// to the impl struct, materializing the struct as a real
		// StructureType rather than a placeholder).
		if dwarfOffset, ok := specialAdditionalTypeOffsets["context.Context"]; ok {
			t, addErr := typeCatalog.addType(dwarfOffset)
			if addErr != nil {
				log.Debugf(
					"failed to add context.Context at offset %#x: %v",
					dwarfOffset, addErr,
				)
			} else {
				additionalTypeRoots = append(additionalTypeRoots, explorationRoot{
					typeID: t.GetID(),
					budget: 2,
				})
			}
		}
		// Pull every dd-trace-go span type into the catalog. These are
		// not discovered as context.Context implementations (they need
		// DDTraceSpan annotation rather than GoContext annotation) but
		// the chain walk still needs them present so it can attach
		// trace-correlation layout.
		for _, name := range ddTraceSpanTypeNames {
			dwarfOffset, ok := specialAdditionalTypeOffsets[name]
			if !ok {
				continue
			}
			t, addErr := typeCatalog.addType(dwarfOffset)
			if addErr != nil {
				log.Debugf(
					"failed to add dd-trace-go support type %q at offset %#x: %v",
					name, dwarfOffset, addErr,
				)
				continue
			}
			additionalTypeRoots = append(additionalTypeRoots, explorationRoot{
				typeID: t.GetID(),
				budget: 1,
			})
		}
		// Pull every dynamically discovered context.Context implementation
		// into the catalog. The iterator returns both value-form impls
		// (e.g. emptyCtx) and pointer-form impls (e.g. *cancelCtx); the
		// latter is what shows up when the methods have pointer
		// receivers. In both cases what we want as a budget-1 root is
		// the impl *struct*, so that the struct's fields are explored at
		// budget 1 and any interface-typed fields (e.g. cancelCtx.err
		// error, cancelCtx.children map[canceler]struct{}) fire
		// processInterface to pull in their implementors. For pointer
		// forms, root the pointee struct directly rather than the
		// pointer — equivalent to giving the pointer budget 2, but
		// states the intent.
		for _, dwarfOffset := range contextImplDwarfOffsets {
			t, addErr := typeCatalog.addType(dwarfOffset)
			if addErr != nil {
				log.Debugf(
					"failed to add context impl at offset %#x: %v",
					dwarfOffset, addErr,
				)
				continue
			}
			if ptr, ok := t.(*ir.PointerType); ok {
				if placeholder, ok := ptr.Pointee.(*pointeePlaceholderType); ok {
					pointee, addErr := typeCatalog.addType(placeholder.offset)
					if addErr != nil {
						log.Debugf(
							"failed to add pointee for context impl at offset %#x: %v",
							placeholder.offset, addErr,
						)
						continue
					}
					ptr.Pointee = pointee
					t = pointee
				} else {
					t = ptr.Pointee
				}
			}
			additionalTypeRoots = append(additionalTypeRoots, explorationRoot{
				typeID: t.GetID(),
				budget: 1,
			})
		}
	}

	// Resolve placeholder types by a unified, budgeted expansion from
	// exploration roots. Container internals are zero-cost.
	{
		// Include types discovered at runtime via interface decoding.
		explorationRoots = append(explorationRoots, additionalTypeRoots...)

		// Specialize any already-added container types before traversal.
		if err := completeGoTypes(typeCatalog, 1, typeCatalog.idAlloc.alloc); err != nil {
			return nil, err
		}
		if err := expandTypesWithBudgets(
			typeCatalog, typeTab, methodIndex, typeIndex,
			explorationRoots, analyzedProbes,
		); err != nil {
			return nil, err
		}
	}

	// All expansion is complete. annotateSpecialGoTypes runs after this on
	// `needsGoContextSupport`; it was set during the special-types
	// resolution above (where we tried to add context.Context and friends
	// as exploration roots). The chain walk's runtime metadata
	// (GoContext.IsContext, DDTrace span layouts) is then attached to the
	// concrete impls in the catalog.

	// Validate that all expression types were properly explored during
	// expandTypesWithBudgets. This marks invalid segments for expressions
	// that fail to resolve (e.g., type mismatches, missing fields).
	exploreTypesForExpressions(typeCatalog, analyzedProbes)

	// Finalize type information now that we have all referenced types.
	if err := finalizeTypes(typeCatalog, materializedSubprograms); err != nil {
		return nil, err
	}
	// Resolve each dynamically discovered context.Context implementation
	// to an IR struct type ID. We do this after expansion so that
	// pointer-form impls (e.g. *cancelCtx) have had their pointee struct
	// materialized — at the moment addType was called above, the pointee
	// was still a placeholder.
	for _, dwarfOffset := range contextImplDwarfOffsets {
		id, ok := typeCatalog.typesByDwarfType[dwarfOffset]
		if !ok {
			continue
		}
		t := typeCatalog.typesByID[id]
		switch tt := t.(type) {
		case *ir.StructureType:
			contextImplIRTypeIDs[tt.ID] = struct{}{}
		case *ir.PointerType:
			if tt.Pointee != nil && !isPlaceholderIRType(tt.Pointee) {
				if _, isStruct := tt.Pointee.(*ir.StructureType); isStruct {
					contextImplIRTypeIDs[tt.Pointee.GetID()] = struct{}{}
				}
			}
		}
	}
	annotateSpecialGoTypes(typeCatalog, needsGoContextSupport, contextImplIRTypeIDs)

	// Populate event root expressions for every probe.
	probes, eventIssues := populateProbeEventsExpressions(
		probes, analyzedProbes, typeCatalog,
	)
	issues = append(issues, eventIssues...)

	// Synthesise the recovery probe's EventRootType after the standard
	// pipeline runs (it's skipped above because its captures are not
	// user-configurable). The synthesis builds a single @exception capture
	// expression bookended by PanicUnwindPrepareOp / PanicUnwindEvictSlotsOp
	// that drives the standard PROCESS_GO_EMPTY_INTERFACE + CHASE_POINTERS
	// pipeline. Drops the probe (filtering it out of the successful set)
	// if runtime.eface / runtime._panic aren't available in the binary.
	probes = synthesizeRecoveryProbes(probes, commonTypes, typeCatalog)

	// Detect probe definitions that did not match any symbol in the binary.
	unused := findUnusedConfigs(probes, issues, probeDefs)
	for _, probe := range unused {
		if probe.GetKind() == ir.ProbeKindRuntimeRecovery {
			// runtime.recovery is missing from this binary (stripped
			// runtime, exotic toolchain, etc.). Don't surface the
			// internal probe as a user-visible issue; log instead so
			// operators see the protection is disabled.
			log.Warnf(
				"dyninst: runtime.recovery probe could not be attached " +
					"(symbol not found); panic-recover leaks will not be " +
					"cleaned up for this binary",
			)
			continue
		}
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
		GoMapHashInfo:    processed.goMapHashInfo,
		CommonTypes:      commonTypes,
		IsARM64:          arch == "arm64",
		Redaction:        cfg.redaction,
	}, nil
}

// analyzedExpression holds all information about a parsed expression.
// Expressions are parsed once during analysis and reused throughout.
type analyzedExpression struct {
	// The parsed expression AST (parsed once, used many times).
	expr exprlang.Expr

	// The DSL string for error messages.
	dsl string

	// The root variable matched from the subprogram (nil if not found).
	rootVariable *ir.Variable

	// The kind of event at which this expression is evaluated.
	eventKind ir.EventKind

	// The kind of expression for deserialization.
	exprKind ir.RootExpressionKind

	// For template segments, the segment reference and index.
	segment    *ir.JSONSegment
	segmentIdx int // -1 if not a segment

	// For capture expressions, the user-specified name.
	captureExprName string

	// redacted is true when the parsed expression references a redacted
	// identifier. Computed during analysis (where the AST is available) and
	// carried onto ir.RootExpression so the decoder drops the value.
	redacted bool
}

// analyzedCondition represents a parsed and resolved condition tree. The
// tree may be a single leaf (eq / isEmpty) or a compound of and/or/not
// over leaves. Leaves may reference variables at one event kind ("single"
// — eventKind is set, splitCondition is false) or at both entry and
// return ("split" — eventKind is zero, splitCondition is true,
// leafEventKind / entryLeafSlotIndex describe the split).
//
// Only analyzeCondition should construct values of this type: leafRoots
// is keyed by pointer identity of the leaves reachable from expr, and
// mixing pre- and post-@return-rewrite leaves would silently break the
// map. Callers downstream (exploreTypesForExpressions, resolveCondition)
// treat the struct as read-only.
type analyzedCondition struct {
	// expr is the @return-rewritten condition AST.
	expr exprlang.Expr
	// leafRoots maps each condition leaf (EqExpr / IsEmptyExpr) reachable
	// from expr to the variable feeding its LHS.
	leafRoots map[exprlang.Expr]*ir.Variable
	// eventKind is the single event kind shared by every leaf's root in
	// the non-split case. Zero in the split case (use leafEventKind).
	eventKind ir.EventKind
	// splitCondition is true when leaves resolve to both entry and return
	// event kinds. In that case each entry-side leaf is compiled to its
	// own SM sub-function and its outcome is captured as a 2-bit status
	// in a per-call condition_state (uint16); the return-side condition
	// program reads the slots back via ConditionLeafLoadOp. See
	// pkg/dyninst/ir/expression.go for the full set of carry ops.
	splitCondition bool
	// leafEventKind maps each leaf to the event kind of its root variable.
	// Populated for both single and split conditions.
	leafEventKind map[exprlang.Expr]ir.EventKind
	// entryLeafSlotIndex maps each entry-side leaf to its 2-bit slot index
	// in the per-call condition_state. Only populated for split
	// conditions; nil for single-event conditions.
	entryLeafSlotIndex map[exprlang.Expr]uint8
}

// maxConditionEntryLeaves is the maximum number of entry-side leaves
// allowed in a split-event-kind condition. The runtime stores each
// leaf's outcome as a 2-bit status in condition_state (uint16; see
// call_depths_entry_t.condition_state in pkg/dyninst/ebpf/context.h).
// Keep in sync with MAX_CONDITION_ENTRY_LEAVES in
// pkg/dyninst/ebpf/context.h.
const maxConditionEntryLeaves = 8

// analyzedProbe holds all analyzed expressions for a single probe instance.
// There is one analyzedProbe per ProbeInstance (i.e. per (probe, subprogram)
// pair).
type analyzedProbe struct {
	probe       *ir.Probe
	instance    *ir.ProbeInstance
	expressions []analyzedExpression
	template    *ir.Template

	// condition holds the analyzed condition tree, or nil if the probe
	// has no condition (or if analysis failed, in which case
	// conditionIssue is populated).
	condition *analyzedCondition

	// conditionIssue is set when condition analysis fails (parse error,
	// unsupported expression type, variable not found, etc.). Reported as
	// a probe issue by populateProbeExpressions.
	conditionIssue ir.Issue

	// Budget for type exploration.
	budget uint32

	// Whether this is a snapshot probe (affects exploration strategy).
	isSnapshot bool
}

// cleanReturnNames computes display names for return variables used as field
// names under @return in multi-return snapshots. For single returns, it returns
// nil (the caller should use "@return" directly). For multiple returns, it
// strips the leading ~ from compiler-generated names and resolves conflicts
// with underscore prefixing.
func cleanReturnNames(vars []*ir.Variable) map[*ir.Variable]string {
	if len(vars) <= 1 {
		return nil
	}
	// First pass: strip ~ prefix from each name, track which are compiler-generated.
	type entry struct {
		cleaned   string
		generated bool // true if original name had ~ prefix
	}
	entries := make([]entry, len(vars))
	for i, v := range vars {
		if strings.HasPrefix(v.Name, "~") {
			entries[i] = entry{cleaned: v.Name[1:], generated: true}
		} else {
			entries[i] = entry{cleaned: v.Name, generated: false}
		}
	}
	// Collect all names that are taken (user-chosen names always win).
	taken := make(map[string]bool, len(vars))
	for _, e := range entries {
		if !e.generated {
			taken[e.cleaned] = true
		}
	}
	// Second pass: resolve conflicts for generated names.
	result := make(map[*ir.Variable]string, len(vars))
	for i, v := range vars {
		e := entries[i]
		if e.generated {
			name := e.cleaned
			for taken[name] {
				name = "_" + name
			}
			taken[name] = true
			result[v] = name
		} else {
			taken[e.cleaned] = true
			result[v] = e.cleaned
		}
	}
	return result
}

// analyzeCondition parses a when-clause expression and resolves it to an
// analyzedCondition ready for resolveCondition. The returned condition's
// fields are related by pointer identity — leafRoots is keyed by the
// leaves reachable from expr — so this function is the only correct way
// to build an analyzedCondition: callers must never assemble one by
// hand, because it's easy to mismatch pre- vs post-@return-rewrite leaf
// nodes and silently break the pointer-identity map.
//
// Returns a non-none issue when the parse fails, a leaf is malformed, a
// referenced variable is missing/unavailable, or the leaves resolve to
// more than one event kind. addRoot is invoked once per distinct
// resolved variable type so type exploration covers every leaf's root.
func analyzeCondition(
	whenJSON []byte,
	varByName map[string]*ir.Variable,
	returnVars []*ir.Variable,
	returnDisplayNames map[*ir.Variable]string,
	conditionEventKind func(*ir.Variable) (ir.EventKind, bool),
	addRoot func(ir.TypeID, uint32),
	budget uint32,
) (*analyzedCondition, ir.Issue) {
	condExpr, err := exprlang.Parse(whenJSON)
	if err != nil {
		return nil, ir.Issue{
			Kind:    ir.IssueKindUnsupportedFeature,
			Message: fmt.Sprintf("failed to parse condition: %v", err),
		}
	}

	// Collect every leaf of the (possibly compound) condition. A leaf
	// here is an EqExpr or IsEmptyExpr; anything else is rejected.
	leaves := conditionLeafExprs(condExpr)
	if len(leaves) == 0 {
		return nil, ir.Issue{
			Kind:    ir.IssueKindUnsupportedFeature,
			Message: fmt.Sprintf("unsupported condition expression type: %T", condExpr),
		}
	}
	// Validate every leaf is a supported shape first so error messages
	// are deterministic before any rewriting.
	for _, leaf := range leaves {
		sub, ok := conditionLeafSubExpr(leaf)
		if !ok {
			return nil, ir.Issue{
				Kind:    ir.IssueKindUnsupportedFeature,
				Message: fmt.Sprintf("unsupported condition expression type: %T", leaf),
			}
		}
		// A condition leaf's LHS must be a variable-derived path
		// (ref / getmember / index / len / isEmpty). Nesting another
		// boolean operator inside eq's LHS — e.g. `eq(eq(x, 1), true)`
		// — is rejected: it would work by accident because bool is a
		// base type, but it bypasses root-variable analysis and
		// @return rewriting, and it's semantically redundant with
		// writing the inner condition directly.
		if err := checkConditionLHS(sub); err != nil {
			return nil, ir.Issue{
				Kind:    ir.IssueKindUnsupportedFeature,
				Message: err.Error(),
			}
		}
	}

	// If any leaf references @return, rewrite the entire condition tree
	// to replace @return refs with concrete return-variable refs. Each
	// leaf enforces "at most one @return.rN field" via rewriteReturnRefs,
	// but distinct leaves may reference different fields — this is what
	// makes compound conditions useful over multi-return functions.
	//
	// After rewriting we re-collect the leaves so every subsequent step
	// uses the post-rewrite nodes; mixing pre- and post-rewrite leaves
	// would silently break the leafRoots pointer-identity map.
	if len(returnVars) > 0 {
		rewroteAny := false
		for _, leaf := range leaves {
			sub, _ := conditionLeafSubExpr(leaf)
			rootVarName, ok := extractRootVariableName(sub)
			if !ok || rootVarName != "@return" {
				continue
			}
			_, rewrittenLeaf, errMsg := rewriteReturnRefs(
				leaf, returnVars, returnDisplayNames,
			)
			if errMsg != "" {
				return nil, ir.Issue{
					Kind:    ir.IssueKindConditionVariableUnavailable,
					Message: errMsg,
				}
			}
			origLeaf := leaf
			condExpr = exprlang.Rewrite(condExpr, func(e exprlang.Expr) exprlang.Expr {
				if e == origLeaf {
					return rewrittenLeaf
				}
				return nil
			})
			rewroteAny = true
		}
		if rewroteAny {
			leaves = conditionLeafExprs(condExpr)
		}
	}

	// Resolve each leaf's root variable and event kind. Compound conditions
	// that span entry and return are allowed: each entry-side leaf is
	// assigned a 2-bit slot in condition_state and the return-side
	// condition program reads them back via ConditionLeafLoadOp.
	// Conditions whose leaves all share the same event kind take the
	// non-split path.
	leafRoots := make(map[exprlang.Expr]*ir.Variable, len(leaves))
	leafEventKindMap := make(map[exprlang.Expr]ir.EventKind, len(leaves))
	var sawEntry, sawReturn, sawLine bool
	for _, leaf := range leaves {
		sub, _ := conditionLeafSubExpr(leaf)
		rootVarName, ok := extractRootVariableName(sub)
		if !ok {
			return nil, ir.Issue{
				Kind:    ir.IssueKindUnsupportedFeature,
				Message: "failed to extract root variable from condition leaf",
			}
		}
		rootVar := varByName[rootVarName]
		if rootVar == nil {
			if rootVarName == "@duration" {
				return nil, ir.Issue{
					Kind:    ir.IssueKindConditionVariableUnavailable,
					Message: ir.ErrDurationNotOnReturn,
				}
			}
			return nil, ir.Issue{
				Kind:    ir.IssueKindConditionVariableUnavailable,
				Message: fmt.Sprintf("condition variable %q not found", rootVarName),
			}
		}
		leafEvKind, ok := conditionEventKind(rootVar)
		if !ok {
			return nil, ir.Issue{
				Kind:    ir.IssueKindConditionVariableUnavailable,
				Message: fmt.Sprintf("condition variable %q not available at any event", rootVarName),
			}
		}
		switch leafEvKind {
		case ir.EventKindEntry:
			sawEntry = true
		case ir.EventKindReturn:
			sawReturn = true
		case ir.EventKindLine:
			sawLine = true
		}
		leafRoots[leaf] = rootVar
		leafEventKindMap[leaf] = leafEvKind
		addRoot(rootVar.Type.GetID(), budget)
	}
	// Line probes only have a single event so a leaf classified as
	// EventKindLine cannot legitimately mix with anything else; a probe
	// targeting a line cannot also have entry / return events. Reject any
	// such mix as the language does not express it.
	if sawLine && (sawEntry || sawReturn) {
		return nil, ir.Issue{
			Kind:    ir.IssueKindConditionVariableUnavailable,
			Message: "condition references both line and entry/return variables",
		}
	}
	splitCondition := sawEntry && sawReturn
	var evKind ir.EventKind
	var entryLeafSlotIndex map[exprlang.Expr]uint8
	if !splitCondition {
		// Single-event condition. Pick the lone event kind.
		switch {
		case sawEntry:
			evKind = ir.EventKindEntry
		case sawReturn:
			evKind = ir.EventKindReturn
		case sawLine:
			evKind = ir.EventKindLine
		}
	} else {
		// Split condition: assign condition_state slot indices to entry-side
		// leaves in iteration order. Reject if the count exceeds the slot
		// budget (16-bit condition_state / 2 bits per slot = 8 slots).
		entryLeafSlotIndex = make(map[exprlang.Expr]uint8, len(leaves))
		var nextIdx uint8
		for _, leaf := range leaves {
			if leafEventKindMap[leaf] != ir.EventKindEntry {
				continue
			}
			if int(nextIdx) >= maxConditionEntryLeaves {
				return nil, ir.Issue{
					Kind: ir.IssueKindConditionCarryTooLarge,
					Message: fmt.Sprintf(
						"split condition has more than %d entry-side leaves; "+
							"condition_state cannot represent more",
						maxConditionEntryLeaves,
					),
				}
			}
			entryLeafSlotIndex[leaf] = nextIdx
			nextIdx++
		}
	}

	return &analyzedCondition{
		expr:               condExpr,
		leafRoots:          leafRoots,
		eventKind:          evKind,
		splitCondition:     splitCondition,
		leafEventKind:      leafEventKindMap,
		entryLeafSlotIndex: entryLeafSlotIndex,
	}, ir.Issue{}
}

// checkConditionLHS validates that expr is a legal LHS for a condition
// leaf: a variable-derived path built from ref / getmember / index / len
// / isEmpty nodes. Boolean operators (eq, and, or, not) are rejected
// because they produce boolean values that would bypass the root-variable
// and @return analysis paths, and they're semantically redundant with
// writing the inner condition directly.
//
// LiteralExpr is also rejected — literals belong on the RHS of a leaf,
// not the LHS.
func checkConditionLHS(expr exprlang.Expr) error {
	switch e := expr.(type) {
	case *exprlang.RefExpr:
		return nil
	case *exprlang.GetMemberExpr:
		return checkConditionLHS(e.Base)
	case *exprlang.IndexExpr:
		return checkConditionLHS(e.Base)
	case *exprlang.LenExpr:
		return checkConditionLHS(e.Operand)
	case *exprlang.IsEmptyExpr:
		return checkConditionLHS(e.Operand)
	case *exprlang.EqExpr, *exprlang.NeExpr,
		*exprlang.LtExpr, *exprlang.LeExpr,
		*exprlang.GtExpr, *exprlang.GeExpr,
		*exprlang.AndExpr, *exprlang.OrExpr, *exprlang.NotExpr,
		*exprlang.ContainsExpr:
		return fmt.Errorf(
			"condition leaf LHS may not be a boolean expression (%T); "+
				"use the inner expression directly",
			expr,
		)
	case *exprlang.LiteralExpr:
		return errors.New("condition leaf LHS may not be a literal")
	default:
		return fmt.Errorf("unsupported condition leaf LHS type: %T", expr)
	}
}

// conditionLeafExprs yields every leaf of a condition tree (EqExpr /
// IsEmptyExpr). For a non-compound condition this yields exactly one node;
// for a compound, it yields each leaf in source order.
func conditionLeafExprs(expr exprlang.Expr) []exprlang.Expr {
	var leaves []exprlang.Expr
	var walk func(exprlang.Expr)
	walk = func(e exprlang.Expr) {
		switch e := e.(type) {
		case *exprlang.AndExpr:
			walk(e.Left)
			walk(e.Right)
		case *exprlang.OrExpr:
			walk(e.Left)
			walk(e.Right)
		case *exprlang.NotExpr:
			walk(e.Operand)
		default:
			leaves = append(leaves, e)
		}
	}
	walk(expr)
	return leaves
}

// conditionLeafSubExpr returns the sub-expression a leaf's LHS descends
// through (the part passed to resolveExpression in resolveCondition). For
// the comparison nodes (EqExpr / NeExpr / LtExpr / LeExpr / GtExpr /
// GeExpr) it's the Left side; for IsEmptyExpr it's the Operand. Any other
// expression type is a malformed leaf (caller should have rejected
// earlier).
func conditionLeafSubExpr(leaf exprlang.Expr) (exprlang.Expr, bool) {
	switch l := leaf.(type) {
	case *exprlang.EqExpr:
		return l.Left, true
	case *exprlang.NeExpr:
		return l.Left, true
	case *exprlang.LtExpr:
		return l.Left, true
	case *exprlang.LeExpr:
		return l.Left, true
	case *exprlang.GtExpr:
		return l.Left, true
	case *exprlang.GeExpr:
		return l.Left, true
	case *exprlang.IsEmptyExpr:
		return l.Operand, true
	case *exprlang.ContainsExpr:
		return l.Base, true
	case *exprlang.AnyExpr:
		return l.Base, true
	case *exprlang.AllExpr:
		return l.Base, true
	default:
		return nil, false
	}
}

// rewriteReturnRefs rewrites every @return reference in expr to reference
// a concrete return variable.
//
// For single-return functions, ref("@return") is rewritten to ref(varName).
// For multi-return functions, getmember(ref("@return"), "field") is matched
// against displayNames and rewritten to ref(varName); a bare @return is
// rejected because multi-value returns require a field selector.
//
// The expression passed here must resolve to a single return variable —
// if it touches two distinct @return.rN fields, the call fails with
// "expression references multiple @return fields". Callers that want to
// allow multiple distinct @return fields across a larger tree (e.g.
// compound conditions) must split the tree into single-leaf expressions
// and call this function once per leaf, then splice the rewritten leaves
// back together.
func rewriteReturnRefs(
	expr exprlang.Expr,
	returnVars []*ir.Variable,
	displayNames map[*ir.Variable]string,
) (rootVar *ir.Variable, rewritten exprlang.Expr, errMsg string) {
	if len(returnVars) == 1 {
		v := returnVars[0]
		rewritten = exprlang.Rewrite(expr, func(e exprlang.Expr) exprlang.Expr {
			if ref, ok := e.(*exprlang.RefExpr); ok && ref.Ref == "@return" {
				return &exprlang.RefExpr{Ref: v.Name}
			}
			return nil
		})
		return v, rewritten, ""
	}

	// Multi-return case: the expression must identify a single field.
	var matched *ir.Variable
	for node := range exprlang.Children(expr) {
		gme, ok := node.(*exprlang.GetMemberExpr)
		if !ok {
			continue
		}
		ref, ok := gme.Base.(*exprlang.RefExpr)
		if !ok || ref.Ref != "@return" {
			continue
		}
		var v *ir.Variable
		for _, rv := range returnVars {
			if displayNames[rv] == gme.Member {
				v = rv
				break
			}
		}
		if v == nil {
			return nil, nil, fmt.Sprintf(
				"@return field %q does not match any return variable", gme.Member)
		}
		if matched != nil && matched != v {
			return nil, nil, "expression references multiple @return fields"
		}
		matched = v
	}
	if matched == nil {
		return nil, nil, "return value selector (e.g., @return.r0) must be used for multi-value returns"
	}
	rewritten = exprlang.Rewrite(expr, func(e exprlang.Expr) exprlang.Expr {
		gme, ok := e.(*exprlang.GetMemberExpr)
		if !ok {
			return nil
		}
		ref, ok := gme.Base.(*exprlang.RefExpr)
		if !ok || ref.Ref != "@return" {
			return nil
		}
		return &exprlang.RefExpr{Ref: matched.Name}
	})
	return matched, rewritten, ""
}

// extractRootVariableName extracts the root variable name from an expression.
func extractRootVariableName(expr exprlang.Expr) (string, bool) {
	const maxDepth = 30
	for range maxDepth {
		switch e := expr.(type) {
		case *exprlang.RefExpr:
			return e.Ref, true
		case *exprlang.GetMemberExpr:
			expr = e.Base
		case *exprlang.LenExpr:
			expr = e.Operand
		case *exprlang.IsEmptyExpr:
			expr = e.Operand
		case *exprlang.IndexExpr:
			expr = e.Base
		case *exprlang.ContainsExpr:
			expr = e.Base
		case *exprlang.AnyExpr:
			expr = e.Base
		case *exprlang.AllExpr:
			expr = e.Base
		case *exprlang.FilterExpr:
			expr = e.Base
		case *exprlang.EqExpr:
			expr = e.Left
		case *exprlang.NeExpr:
			expr = e.Left
		case *exprlang.LtExpr:
			expr = e.Left
		case *exprlang.LeExpr:
			expr = e.Left
		case *exprlang.GtExpr:
			expr = e.Left
		case *exprlang.GeExpr:
			expr = e.Left
		default:
			return "", false
		}
	}
	return "", false
}

// analyzeAllProbes performs a single pass through all probe instances, parsing
// expressions once and matching them to variables. This must be called after
// probes are created (so we have Events and InjectionPoints) but before type
// expansion. Returns one analyzedProbe per instance and exploration roots for
// type expansion.
func analyzeAllProbes(
	probes []*ir.Probe,
	budgets map[ir.SubprogramID]uint32,
	tc *typeCatalog,
	red *redaction.Config,
) ([]analyzedProbe, []explorationRoot) {
	var analyzed []analyzedProbe

	// Track exploration roots (typeID -> max budget).
	rootBudgets := make(map[ir.TypeID]uint32)
	addRoot := func(typeID ir.TypeID, budget uint32) {
		if existing, ok := rootBudgets[typeID]; !ok || budget > existing {
			rootBudgets[typeID] = budget
		}
	}

	// durationVar is a single synthetic variable reused across all probes
	// that support @duration. Its role signals to the compiler that its
	// LocationOp should emit an ExprLoadDurationOp rather than reading
	// from DWARF locations.
	durationType := tc.typesByID[tc.durationType]
	durationVar := &ir.Variable{
		Name:      "@duration",
		Type:      durationType,
		Role:      ir.VariableRoleDuration,
		DictIndex: -1,
	}

	for _, probe := range probes {
		kind := probe.GetKind()
		isSnapshot := kind == ir.ProbeKindSnapshot
		isCaptureExpression := kind == ir.ProbeKindCaptureExpression

		// Internal runtime.recovery probe has no template, condition,
		// or user-configurable capture expressions. Its @exception capture
		// is built later by synthesizeRecoveryProbeEventRoot, so skip
		// the rcjson-driven analysis pipeline here.
		if kind == ir.ProbeKindRuntimeRecovery {
			for instIdx := range probe.Instances {
				analyzed = append(analyzed, analyzedProbe{
					probe:    probe,
					instance: &probe.Instances[instIdx],
				})
			}
			continue
		}

		for instIdx := range probe.Instances {
			inst := &probe.Instances[instIdx]
			budget := budgets[inst.Subprogram.ID]

			ap := analyzedProbe{
				probe:      probe,
				instance:   inst,
				budget:     budget,
				isSnapshot: isSnapshot || isCaptureExpression,
			}

			// Build variable lookup for this instance's subprogram.
			varByName := make(map[string]*ir.Variable, len(inst.Subprogram.Variables)+1)
			for _, v := range inst.Subprogram.Variables {
				varByName[v.Name] = v
			}

			// Parse template and create template segments (fresh copy per instance).
			if td := probe.ProbeDefinition.GetTemplate(); td != nil {
				ap.template = newTemplate(td)
			}

			// Determine event kinds for this instance.
			isKind := func(kind ir.EventKind) func(*ir.Event) bool {
				return func(ev *ir.Event) bool { return ev.Kind == kind }
			}
			haveEntry := slices.ContainsFunc(inst.Events, isKind(ir.EventKindEntry))
			haveReturn := slices.ContainsFunc(inst.Events, isKind(ir.EventKindReturn))

			// @duration is a synthetic variable available only on probes
			// with a return event. Expose it under varByName so refs in
			// conditions, template segments, and capture expressions
			// resolve through the normal variable-matching paths.
			if haveReturn {
				varByName["@duration"] = durationVar
			}

			// isFloatType returns true if the variable has a float32 or float64 type.
			isFloatType := func(v *ir.Variable) bool {
				k, ok := v.Type.GetGoKind()
				return ok && (k == reflect.Float32 || k == reflect.Float64)
			}

			// Check variable availability at injection points.
			variableIsAvailable := func(ips []ir.InjectionPoint, v *ir.Variable) bool {
				locIdx := 0
				for _, ip := range ips {
					for locIdx < len(v.Locations) &&
						v.Locations[locIdx].Range[1] <= ip.PC {
						locIdx++
					}
					if locIdx < len(v.Locations) &&
						ip.PC >= v.Locations[locIdx].Range[0] {
						return true
					}
				}
				return false
			}

			// floatIsRegisterOnly returns true if a float variable's locations
			// at the given injection points consist exclusively of register pieces.
			// Such variables cannot be read by the eBPF runtime.
			floatIsRegisterOnly := func(ips []ir.InjectionPoint, v *ir.Variable) bool {
				locIdx := 0
				for _, ip := range ips {
					for locIdx < len(v.Locations) && v.Locations[locIdx].Range[1] <= ip.PC {
						locIdx++
					}
					if locIdx < len(v.Locations) && ip.PC >= v.Locations[locIdx].Range[0] {
						for _, piece := range v.Locations[locIdx].Pieces {
							if _, isReg := piece.Op.(ir.Register); !isReg {
								return false
							}
						}
					}
				}
				return true
			}

			// Build segment references for matching.
			type segmentRef struct {
				segment *ir.JSONSegment
				index   int
			}
			segmentRefs := make(map[string][]segmentRef)
			if ap.template != nil {
				for i, s := range ap.template.Segments {
					if seg, ok := s.(*ir.JSONSegment); ok {
						if rootVar, ok := extractRootVariableName(seg.JSON); ok {
							segmentRefs[rootVar] = append(segmentRefs[rootVar],
								segmentRef{segment: seg, index: i})
						}
					}
				}
			}

			// Extract entry injection points for float register checks.
			var entryIPs []ir.InjectionPoint
			for _, ev := range inst.Events {
				if ev.Kind == ir.EventKindEntry {
					entryIPs = ev.InjectionPoints
					break
				}
			}

			// Pre-scan: collect return variables and compute display names for
			// return events. @return is a return-point concept, so these are
			// only used when the probe has a Return event; at line probes,
			// named returns are treated as locals and unnamed returns are
			// filtered out (see the switch below).
			var returnVars []*ir.Variable
			if haveReturn {
				for _, v := range inst.Subprogram.Variables {
					if v.Role == ir.VariableRoleReturn {
						returnVars = append(returnVars, v)
					}
				}
			}
			returnDisplayNames := cleanReturnNames(returnVars) // nil for 0 or 1 returns

			// Process each variable.
			for _, v := range inst.Subprogram.Variables {
				var evKind ir.EventKind
				var exprKind ir.RootExpressionKind

				switch {
				case haveEntry && v.Role == ir.VariableRoleParameter:
					// The entry event for a method probe.
					if isFloatType(v) && floatIsRegisterOnly(entryIPs, v) {
						continue
					}
					evKind = ir.EventKindEntry
					exprKind = ir.RootExpressionKindArgument

				case haveReturn && v.Role == ir.VariableRoleReturn:
					// The return event for a method probe.
					//
					// TODO: We should return available locals from return probes,
					// which would just require extending the next case to also be
					// triggered for return variables.
					evKind = ir.EventKindReturn
					exprKind = ir.RootExpressionKindReturn

				case len(inst.Events) == 1 &&
					inst.Events[0].Kind == ir.EventKindLine:
					// The line-probe case.
					//
					// Return-role vars are either filtered out (unnamed,
					// e.g. ~r0) or treated as plain locals (named). The
					// function hasn't returned yet, so they aren't "return
					// values" here; unnamed slots have no user-facing value
					// and named returns are just pre-declared locals.
					if v.Role == ir.VariableRoleReturn &&
						strings.HasPrefix(v.Name, "~") {
						continue
					}
					ips := inst.Events[0].InjectionPoints
					if !variableIsAvailable(ips, v) {
						continue
					}
					if isFloatType(v) && floatIsRegisterOnly(ips, v) {
						continue
					}
					// TODO: the exprKind should be argument for available arguments.
					evKind = ir.EventKindLine
					exprKind = ir.RootExpressionKindLocal

				default:
					continue
				}

				// For snapshot probes, add variable itself as an expression.
				// Capture expression probes only capture explicitly listed
				// expressions (handled below), not all variables.
				if isSnapshot && !isCaptureExpression {
					// Compute the display name for this expression. The
					// @return wrapping is only for return events; at line
					// probes named returns keep their original name (see
					// switch above).
					name := v.Name
					if exprKind == ir.RootExpressionKindReturn {
						if returnDisplayNames == nil {
							name = "@return"
						} else {
							name = returnDisplayNames[v]
						}
					}
					ap.expressions = append(ap.expressions, analyzedExpression{
						expr:         &exprlang.RefExpr{Ref: v.Name},
						dsl:          name,
						rootVariable: v,
						eventKind:    evKind,
						exprKind:     exprKind,
						segmentIdx:   -1,
					})
					// Snapshot: add all variable types to exploration roots.
					addRoot(v.Type.GetID(), budget)
				}

				// Match template segments to this variable by its original name.
				// Named returns (e.g., "result") remain accessible by name.
				//
				// Note: there's no risk of picking the wrong variable due to
				// shadowing, but there should be! In materializePending we ensure
				// that we only track a single variable with a given name. This is
				// incorrect in cases of shadowing. Instead we could record all
				// shadowed variables and handle ambiguity of template resolution
				// based on the specific return point (as that's all that matters
				// for scoping in the case of shadowing) and come up with a naming
				// scheme to describe the shadowed variables in snapshots.
				segs, ok := segmentRefs[v.Name]
				if !ok {
					continue
				}
				for _, seg := range segs {
					ap.expressions = append(ap.expressions, analyzedExpression{
						expr:         seg.segment.JSON,
						dsl:          seg.segment.DSL,
						rootVariable: v,
						eventKind:    evKind,
						exprKind:     ir.RootExpressionKindTemplateSegment,
						segment:      seg.segment,
						segmentIdx:   seg.index,
					})
					// Log/capture-expression probe: add root variable type to exploration roots.
					// For snapshot probes, the variable was already added above.
					if !isSnapshot || isCaptureExpression {
						addRoot(v.Type.GetID(), budget)
					}
				}
				delete(segmentRefs, v.Name)
			}

			// Handle @duration references in template segments. Only
			// plain {ref: "@duration"} is supported — member access or
			// indexing on @duration produces an InvalidSegment. On
			// probes without a return event we mark the segment
			// invalid; at runtime on return probes the BPF program
			// computes the duration from the entry/return timestamps.
			if segs, ok := segmentRefs["@duration"]; ok {
				for _, seg := range segs {
					if !haveReturn {
						ap.template.Segments[seg.index] = ir.InvalidSegment{
							Error: ir.ErrDurationNotOnReturn,
							DSL:   seg.segment.DSL,
						}
						continue
					}
					if _, plainRef := seg.segment.JSON.(*exprlang.RefExpr); !plainRef {
						ap.template.Segments[seg.index] = ir.InvalidSegment{
							Error: "@duration does not support member access or indexing",
							DSL:   seg.segment.DSL,
						}
						continue
					}
					ap.expressions = append(ap.expressions, analyzedExpression{
						expr:         seg.segment.JSON,
						dsl:          seg.segment.DSL,
						rootVariable: durationVar,
						eventKind:    ir.EventKindReturn,
						exprKind:     ir.RootExpressionKindTemplateSegment,
						segment:      seg.segment,
						segmentIdx:   seg.index,
					})
					addRoot(durationType.GetID(), budget)
				}
				delete(segmentRefs, "@duration")
			}

			// Handle @return references in template segments. @return is
			// a return-point concept and only resolves at probes with a
			// Return event.
			if segs, ok := segmentRefs["@return"]; ok && haveReturn {
				for _, seg := range segs {
					rv, rewritten, errMsg := rewriteReturnRefs(
						seg.segment.JSON, returnVars, returnDisplayNames,
					)
					if rv != nil {
						ap.expressions = append(ap.expressions, analyzedExpression{
							expr:         rewritten,
							dsl:          seg.segment.DSL,
							rootVariable: rv,
							eventKind:    ir.EventKindReturn,
							exprKind:     ir.RootExpressionKindTemplateSegment,
							segment:      seg.segment,
							segmentIdx:   seg.index,
						})
						addRoot(rv.Type.GetID(), budget)
					} else {
						ap.template.Segments[seg.index] = ir.InvalidSegment{
							Error: errMsg,
							DSL:   seg.segment.DSL,
						}
					}
				}
				delete(segmentRefs, "@return")
			}

			// Process capture expressions.
			for _, ce := range probe.ProbeDefinition.GetCaptureExpressions() {
				parsedExpr, err := exprlang.Parse(ce.GetJSON())
				if err != nil {
					continue
				}
				rootVarName, ok := extractRootVariableName(parsedExpr)
				if !ok {
					continue
				}
				// Handle @return references in capture expressions.
				var rootVar *ir.Variable
				if rootVarName == "@return" && haveReturn {
					rootVar, parsedExpr, _ = rewriteReturnRefs(
						parsedExpr, returnVars, returnDisplayNames,
					)
				} else {
					rootVar = varByName[rootVarName]
				}
				// @duration is a capturable expression even on probes
				// without a return event: we bind it to whatever event
				// the probe does have so that at runtime the BPF
				// program writes ExprStatusAbsent and the decoder
				// surfaces a clear "@duration is only available at
				// function return" evaluation error on the snapshot.
				if rootVar == nil && rootVarName == "@duration" {
					rootVar = durationVar
				}
				if rootVar == nil {
					continue
				}
				var evKind ir.EventKind
				switch {
				case rootVar.Role == ir.VariableRoleDuration:
					// Prefer the return event when available. Otherwise
					// fall back to the entry event or the sole line
					// event, so the expression still runs and can
					// report its absent status.
					switch {
					case haveReturn:
						evKind = ir.EventKindReturn
					case haveEntry:
						evKind = ir.EventKindEntry
					case len(inst.Events) == 1 && inst.Events[0].Kind == ir.EventKindLine:
						evKind = ir.EventKindLine
					default:
						continue
					}
				case haveEntry && rootVar.Role == ir.VariableRoleParameter:
					if isFloatType(rootVar) && floatIsRegisterOnly(entryIPs, rootVar) {
						continue
					}
					evKind = ir.EventKindEntry
				case haveReturn && rootVar.Role == ir.VariableRoleReturn:
					evKind = ir.EventKindReturn
				case haveReturn && rootVar.Role == ir.VariableRoleLocal:
					evKind = ir.EventKindReturn
				case len(inst.Events) == 1 && inst.Events[0].Kind == ir.EventKindLine:
					if !variableIsAvailable(inst.Events[0].InjectionPoints, rootVar) {
						continue
					}
					if isFloatType(rootVar) && floatIsRegisterOnly(inst.Events[0].InjectionPoints, rootVar) {
						continue
					}
					evKind = ir.EventKindLine
				default:
					continue
				}
				ap.expressions = append(ap.expressions, analyzedExpression{
					expr:            parsedExpr,
					dsl:             ce.GetDSL(),
					rootVariable:    rootVar,
					eventKind:       evKind,
					exprKind:        ir.RootExpressionKindCaptureExpression,
					segmentIdx:      -1,
					captureExprName: ce.GetName(),
				})
				addRoot(rootVar.Type.GetID(), budget)
			}

			// conditionEventKind determines which event kind a condition variable belongs
			// to, using the same variable-role-to-event mapping as expression analysis.
			conditionEventKind := func(
				rootVar *ir.Variable,
			) (ir.EventKind, bool) {
				events := inst.Events
				switch {
				case rootVar.Role == ir.VariableRoleDuration:
					// durationVar is only registered in varByName when
					// haveReturn is true, so this arm always resolves to
					// the return event.
					return ir.EventKindReturn, true
				case haveEntry && rootVar.Role == ir.VariableRoleParameter:
					if isFloatType(rootVar) && floatIsRegisterOnly(entryIPs, rootVar) {
						return 0, false
					}
					return ir.EventKindEntry, true
				case haveReturn && rootVar.Role == ir.VariableRoleReturn:
					return ir.EventKindReturn, true
				case haveReturn && rootVar.Role == ir.VariableRoleLocal:
					return ir.EventKindReturn, true
				case len(events) == 1 && events[0].Kind == ir.EventKindLine:
					if !variableIsAvailable(events[0].InjectionPoints, rootVar) {
						return 0, false
					}
					if isFloatType(rootVar) && floatIsRegisterOnly(events[0].InjectionPoints, rootVar) {
						return 0, false
					}
					return ir.EventKindLine, true
				default:
					return 0, false
				}
			}

			// Analyze condition expression.
			whenJSON := probe.ProbeDefinition.GetWhen()
			if len(whenJSON) > 0 {
				ap.condition, ap.conditionIssue = analyzeCondition(
					whenJSON,
					varByName,
					returnVars,
					returnDisplayNames,
					conditionEventKind,
					addRoot,
					budget,
				)
				// Reject probes that reference data on the redaction list, to not leak info.
				if ap.condition != nil {
					if name, ok := expressionReferencesRedacted(ap.condition.expr, red); ok {
						ap.condition = nil
						ap.conditionIssue = ir.Issue{
							Kind:    ir.IssueKindInvalidProbeDefinition,
							Message: fmt.Sprintf("condition references redacted identifier %q", name),
						}
					}
				}
			}

			// Mark unmatched segments as invalid.
			for name, segs := range segmentRefs {
				for _, seg := range segs {
					ap.template.Segments[seg.index] = ir.InvalidSegment{
						Error: fmt.Sprintf("failed to resolve reference %q", name),
						DSL:   seg.segment.DSL,
					}
				}
			}

			// Put the template segments first, so we explore their values earlier
			// than snapshot values. If we're going to run out of space, we may as
			// well do it for data that shows up below the fold rather than in the
			// message.
			exprKindToInt := func(kind ir.RootExpressionKind) int {
				if kind == ir.RootExpressionKindTemplateSegment {
					return 0
				}
				return 1
			}
			slices.SortStableFunc(ap.expressions, func(a, b analyzedExpression) int {
				return cmp.Compare(exprKindToInt(a.exprKind), exprKindToInt(b.exprKind))
			})
			// Flag capture/template expressions that read a redacted value so
			// the decoder drops them. The resolved IR keeps only offsets and a
			// display name, so this must be decided from the parsed AST here.
			for i := range ap.expressions {
				if _, ok := expressionReferencesRedacted(ap.expressions[i].expr, red); ok {
					ap.expressions[i].redacted = true
				}
			}
			analyzed = append(analyzed, ap)
		}
	}

	// Convert root budgets to slice.
	roots := make([]explorationRoot, 0, len(rootBudgets))
	for typeID, budget := range rootBudgets {
		roots = append(roots, explorationRoot{typeID: typeID, budget: budget})
	}

	return analyzed, roots
}

// newTemplate creates a new template from a template definition.
//
// Note that the JSONSegment EventKind and EventExpressionIndex will be left
// with zero values to be filled in later.
func newTemplate(td ir.TemplateDefinition) *ir.Template {
	var segments []ir.TemplateSegment
	addSegment := func(s ir.TemplateSegment) {
		segments = append(segments, s)
	}
	addInvalid := func(s ir.TemplateSegmentExpression, msg string) {
		addSegment(ir.InvalidSegment{DSL: s.GetDSL(), Error: msg})
	}
	var i int
	for segment := range td.GetSegments() {
		switch segment := segment.(type) {
		case ir.TemplateSegmentString:
			addSegment(ir.StringSegment(segment.GetString()))
		case ir.TemplateSegmentExpression:
			expr, err := exprlang.Parse(segment.GetJSON())
			if err != nil {
				addInvalid(segment, err.Error())
			} else {
				switch expr := expr.(type) {
				case *exprlang.RefExpr:
				case *exprlang.GetMemberExpr:
				case *exprlang.IndexExpr:
				case *exprlang.LenExpr:
				case *exprlang.IsEmptyExpr:
				case *exprlang.ContainsExpr:
				case *exprlang.EqExpr:
				case *exprlang.NeExpr:
				case *exprlang.LtExpr:
				case *exprlang.LeExpr:
				case *exprlang.GtExpr:
				case *exprlang.GeExpr:
				case *exprlang.AnyExpr:
				case *exprlang.AllExpr:
				case *exprlang.FilterExpr:
				case *exprlang.UnsupportedExpr:
					msg := "unsupported operation: " + expr.Operation
					addInvalid(segment, msg)
					continue
				default:
					msg := fmt.Sprintf("unsupported expression: %T", expr)
					addInvalid(segment, msg)
					continue
				}
				addSegment(&ir.JSONSegment{
					DSL:  segment.GetDSL(),
					JSON: expr,

					// These will be filled in by populateProbeExpressions.
					EventKind:            0,
					EventExpressionIndex: 0,
				})
			}
		}
		i++
	}
	t := &ir.Template{
		TemplateString: td.GetTemplateString(),
		Segments:       segments,
	}
	return t
}

// computeDepthBudgets returns the maximum reference depth per subprogram ID
// across all probes configured for that subprogram.
//
// TODO: Taking the max of all per-expression capture configs as the probe-level
// limit is a short-term solution. This should be updated with logic to set the
// depth per expression underneath eBPF.
func computeDepthBudgets(pending []*pendingSubprogram) map[ir.SubprogramID]uint32 {
	budgets := make(map[ir.SubprogramID]uint32, len(pending))
	for _, p := range pending {
		var maxDepth uint32
		for _, cfg := range p.probesCfgs {
			maxDepth = max(maxDepth, cfg.GetCaptureConfig().GetMaxReferenceDepth())
			for _, ce := range cfg.GetCaptureExpressions() {
				if ceCfg := ce.GetCaptureConfig(); ceCfg != nil {
					maxDepth = max(maxDepth, ceCfg.GetMaxReferenceDepth())
				}
			}
		}
		budgets[p.id] = maxDepth
	}
	return budgets
}

// additionalTypeBudget is the exploration budget assigned to types discovered
// at runtime through interface decoding and fed back via WithAdditionalTypes.
// A budget of 3 is enough to resolve fields and one level of indirection
// without being excessively expensive.
const additionalTypeBudget = 3

// explorationRoot represents a type that should be explored with a budget.
type explorationRoot struct {
	typeID ir.TypeID
	budget uint32
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

// typeQueueProcessor encapsulates the state needed for processing the type
// exploration queue. It handles budget-based type graph traversal.
type typeQueueProcessor struct {
	q               *typeQueue
	processedBest   map[ir.TypeID]uint32
	tc              *typeCatalog
	goTypes         *gotype.Table
	gotypeTypeIndex goTypeToOffsetIndex
	methodIndex     methodToGoTypeIndex

	// Reusable buffers.
	methodBuf   []gotype.IMethod
	ii          implementorIterator
	iiInitiated bool
}

// push enqueues (or improves) a type only if strictly better than any
// already processed or enqueued remaining budget.
func (p *typeQueueProcessor) push(t ir.Type, remaining uint32) {
	id := t.GetID()
	if r, ok := p.processedBest[id]; ok && remaining <= r {
		return
	}
	if idx, ok := p.q.pos[id]; ok {
		if remaining <= p.q.items[idx].remaining {
			return
		}
		p.q.items[idx].remaining = remaining
		heap.Fix(p.q, int(idx))
		return
	}
	p.q.push(typeQueueEntry{id: id, remaining: remaining})
}

// ensureCompleted completes a type by ID.
func (p *typeQueueProcessor) ensureCompleted(id ir.TypeID) error {
	return completeGoTypes(p.tc, id, id)
}

// drainQueue processes all items in the queue until empty.
func (p *typeQueueProcessor) drainQueue() error {
	if !p.iiInitiated {
		p.ii = makeImplementorIterator(p.methodIndex)
		p.iiInitiated = true
	}

	for p.q.Len() > 0 {
		wi := p.q.pop()
		if r, ok := p.processedBest[wi.id]; ok && wi.remaining <= r {
			continue
		}

		// Ensure the current type is specialized before visiting.
		if err := p.ensureCompleted(wi.id); err != nil {
			return err
		}

		t := p.tc.typesByID[wi.id]

		switch tt := t.(type) {

		// Nothing to do for these types.
		case *ir.BaseType,
			*ir.DurationType,
			*ir.TraceContextType,
			*ir.EventRootType,
			*ir.GoChannelType,
			*ir.GoEmptyInterfaceType,
			*ir.GoStringDataType,
			*ir.GoSubroutineType,
			*ir.GoTimeType,
			*ir.UnresolvedPointeeType,
			*ir.VoidPointerType:

		case *ir.GoInterfaceType:
			if wi.remaining <= 0 {
				break
			}
			if err := p.processInterface(tt, wi.remaining); err != nil {
				return err
			}

		// Zero-cost neighbors (do not dereference pointers here).
		case *ir.StructureType:
			for i := range tt.RawFields {
				p.push(tt.RawFields[i].Type, wi.remaining)
			}
		case *ir.GoContextImplementationType:
			for i := range tt.StructureType.RawFields {
				p.push(tt.StructureType.RawFields[i].Type, wi.remaining)
			}
		case *ir.DDTraceSpanType:
			for i := range tt.StructureType.RawFields {
				p.push(tt.StructureType.RawFields[i].Type, wi.remaining)
			}
		case *ir.GoSliceHeaderType:
			p.push(tt.Data, wi.remaining)
		case *ir.GoStringHeaderType:
			p.push(tt.Data, wi.remaining)
		case *ir.ArrayType:
			p.push(tt.Element, wi.remaining)
		case *ir.GoMapType:
			p.push(tt.HeaderType, wi.remaining)
		case *ir.GoHMapHeaderType:
			p.push(tt.BucketsType, wi.remaining)
			p.push(tt.BucketType, wi.remaining)
		case *ir.GoSwissMapHeaderType:
			p.push(tt.TablePtrSliceType, wi.remaining)
			p.push(tt.GroupType, wi.remaining)
		case *ir.GoSliceDataType:
			p.push(tt.Element, wi.remaining)
		case *ir.GoHMapBucketType:
			p.push(tt.KeyType, wi.remaining)
			p.push(tt.ValueType, wi.remaining)
		case *ir.GoSwissMapGroupsType:
			p.push(tt.GroupType, wi.remaining)
			p.push(tt.GroupSliceType, wi.remaining)

		// Depth-cost step: pointer dereference.
		case *ir.PointerType:
			if wi.remaining <= 0 {
				break
			}
			if placeholder, ok := tt.Pointee.(*pointeePlaceholderType); ok {
				newT, err := p.tc.addType(placeholder.offset)
				if err != nil {
					return err
				}
				tt.Pointee = newT
				if err := p.ensureCompleted(newT.GetID()); err != nil {
					return err
				}
			}
			p.push(tt.Pointee, wi.remaining-1)

		default:
			return fmt.Errorf("unexpected ir.Type %[1]T: %#[1]v", tt)
		}

		// Mark processed with this best remaining.
		p.processedBest[wi.id] = wi.remaining
	}
	return nil
}

// processInterface handles interface type exploration by iterating through
// implementations.
func (p *typeQueueProcessor) processInterface(
	tt *ir.GoInterfaceType,
	remaining uint32,
) error {
	grtID, ok := tt.GetGoRuntimeType()
	if !ok {
		return nil
	}
	grt, err := p.goTypes.ParseGoType(gotype.TypeID(grtID))
	if err != nil {
		return fmt.Errorf(
			"failed to parse go type for interface %q: %w", tt.GetName(), err,
		)
	}
	iface, ok := grt.Interface()
	if !ok {
		return fmt.Errorf(
			"go type for interface %q is not an interface: %v",
			tt.GetName(), grt.Kind(),
		)
	}
	clear(p.methodBuf)
	methods, err := iface.Methods(p.methodBuf[:0])
	if err != nil {
		return fmt.Errorf(
			"failed to get methods for interface %q: %w", tt.GetName(), err,
		)
	}
	for p.ii.seek(methods); p.ii.valid(); p.ii.next() {
		impl := p.ii.cur()
		var t ir.Type
		if tid, ok := p.tc.typesByGoRuntimeType[impl]; ok {
			t = p.tc.typesByID[tid]
		} else {
			implOffset, ok := p.gotypeTypeIndex.resolveDwarfOffset(impl)
			if !ok {
				// This is suspicious, but not obviously worth failing out over.
				continue
			}
			if tid, ok := p.tc.typesByDwarfType[implOffset]; ok {
				t = p.tc.typesByID[tid]
			} else {
				var err error
				t, err = p.tc.addType(implOffset)
				if err != nil {
					return fmt.Errorf(
						"failed to add type for implementation of %q: %w",
						tt.GetName(), err,
					)
				}
				if err := p.ensureCompleted(t.GetID()); err != nil {
					return fmt.Errorf(
						"failed to complete type for implementation of %q: %w",
						tt.GetName(), err,
					)
				}
			}
		}
		if ppt, ok := t.(*pointeePlaceholderType); ok {
			var err error
			t, err = p.tc.addType(ppt.offset)
			if err != nil {
				return fmt.Errorf(
					"failed to add type for implementation of %q: %w",
					tt.GetName(), err,
				)
			}
			if err := p.ensureCompleted(t.GetID()); err != nil {
				return fmt.Errorf(
					"failed to complete type for implementation of %q: %w",
					tt.GetName(), err,
				)
			}
		}
		p.push(t, remaining-1)
	}
	return nil
}

// expandTypesWithBudgets performs a unified graph expansion starting from
// pre-computed exploration roots, observing depth budgets. Only pointer
// dereferences consume depth; container internals (strings, slices, maps) are
// zero-cost. Newly materialized types are immediately completed to ensure
// correct container specialization.
//
// After budget-based exploration, expression paths from analyzed probes are
// walked to ensure all types needed for expression evaluation are resolved.
// This happens before placeholders are converted to UnresolvedPointeeType.
func expandTypesWithBudgets(
	tc *typeCatalog,
	goTypes *gotype.Table,
	methodIndex methodToGoTypeIndex,
	gotypeTypeIndex goTypeToOffsetIndex,
	explorationRoots []explorationRoot,
	analyzedProbes []analyzedProbe,
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

	// Update budgets for the pre-computed exploration roots.
	for _, root := range explorationRoots {
		pos := q.pos[root.typeID]
		item := &q.items[pos]
		item.remaining = max(item.remaining, root.budget)
	}

	// Initialize the heap now that everything has been updated.
	heap.Init(q)

	// Create the queue processor with all necessary state.
	proc := &typeQueueProcessor{
		q:               q,
		processedBest:   processedBest,
		tc:              tc,
		goTypes:         goTypes,
		gotypeTypeIndex: gotypeTypeIndex,
		methodIndex:     methodIndex,
	}

	// Process initial exploration roots.
	if err := proc.drainQueue(); err != nil {
		return err
	}

	// Explore types needed by expression paths and push leaf types to the
	// queue with their budgets. This ensures expression results are explored
	// to the full depth.
	if err := exploreExpressionPathTypesFromAnalysis(
		tc, analyzedProbes, proc.push,
	); err != nil {
		return err
	}

	// Process any newly added expression leaf types.
	if err := proc.drainQueue(); err != nil {
		return err
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
			return errors.New("unexpected EOF while reading type")
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

// exploreExpressionPathTypesFromAnalysis walks pre-parsed expression paths
// from analyzed probes, ensures all types along the path are resolved, and
// pushes the leaf types to the exploration queue with the probe's budget.
// This ensures that expression results are explored to the full depth
// configured for the probe.
func exploreExpressionPathTypesFromAnalysis(
	tc *typeCatalog,
	analyzedProbes []analyzedProbe,
	push func(ir.Type, uint32),
) error {
	for _, ap := range analyzedProbes {
		// Build variable lookup for this instance's subprogram.
		varByName := make(map[string]*ir.Variable, len(ap.instance.Subprogram.Variables))
		for _, v := range ap.instance.Subprogram.Variables {
			varByName[v.Name] = v
		}

		// Walk each pre-parsed expression.
		for _, expr := range ap.expressions {
			if expr.rootVariable == nil {
				continue
			}

			// Walk the expression path, resolving types.
			leafType, err := walkExpressionPathTypes(tc, expr.expr, varByName)
			if err != nil {
				// Error will be reported later during expression resolution.
				continue
			}

			// Push the leaf type to the queue with the probe's budget.
			// This ensures the expression result is explored to full depth.
			if leafType != nil {
				push(leafType, ap.budget)
			}
		}
	}
	return nil
}

// walkExpressionPathTypes walks an expression and resolves any placeholder
// types encountered along the path. Returns the leaf type of the expression.
func walkExpressionPathTypes(
	tc *typeCatalog,
	expr exprlang.Expr,
	varByName map[string]*ir.Variable,
) (ir.Type, error) {
	switch e := expr.(type) {
	case *exprlang.RefExpr:
		v, ok := varByName[e.Ref]
		if !ok {
			return nil, nil // Variable not found, will error later.
		}
		if err := ensureTypeExplored(tc, v.Type); err != nil {
			return nil, err
		}
		return v.Type, nil

	case *exprlang.GetMemberExpr:
		// First walk the base expression.
		if _, err := walkExpressionPathTypes(tc, e.Base, varByName); err != nil {
			return nil, err
		}

		// Get the base type.
		baseType, err := getExprType(tc, e.Base, varByName)
		if err != nil {
			return nil, err
		}

		// Walk to the member and get the field type.
		return walkMemberPathTypes(tc, baseType, e.Member)

	case *exprlang.IndexExpr:
		// Walk the base expression first.
		if _, err := walkExpressionPathTypes(tc, e.Base, varByName); err != nil {
			return nil, err
		}

		// Get the base type and resolve to element type.
		baseType, err := getExprType(tc, e.Base, varByName)
		if err != nil {
			return nil, err
		}
		elemType, err := indexElementType(tc, baseType)
		if err != nil {
			return nil, err
		}
		if err := ensureTypeExplored(tc, elemType); err != nil {
			return nil, err
		}
		return elemType, nil

	default:
		return nil, nil
	}
}

// getExprType returns the type of an expression.
func getExprType(
	tc *typeCatalog,
	expr exprlang.Expr,
	varByName map[string]*ir.Variable,
) (ir.Type, error) {
	switch e := expr.(type) {
	case *exprlang.RefExpr:
		v, ok := varByName[e.Ref]
		if !ok {
			return nil, fmt.Errorf("variable %q not found", e.Ref)
		}
		return v.Type, nil

	case *exprlang.GetMemberExpr:
		baseType, err := getExprType(tc, e.Base, varByName)
		if err != nil {
			return nil, err
		}

		// Dereference pointer if needed.
		curType := baseType
		if ptrType, ok := curType.(*ir.PointerType); ok {
			pointee := tc.typesByID[ptrType.Pointee.GetID()]
			curType = pointee
		}

		// Get struct field.
		structType, ok := curType.(*ir.StructureType)
		if !ok {
			return nil, fmt.Errorf("cannot access member on non-struct type %T", curType)
		}

		f, err := field(tc, structType, e.Member)
		if err != nil {
			return nil, err
		}
		return f.Type, nil

	case *exprlang.IndexExpr:
		baseType, err := getExprType(tc, e.Base, varByName)
		if err != nil {
			return nil, err
		}
		return indexElementType(tc, baseType)

	default:
		return nil, fmt.Errorf("unsupported expression type %T", expr)
	}
}

// walkMemberPathTypes walks from a type to a member, resolving placeholders.
// Returns the field's type.
func walkMemberPathTypes(
	tc *typeCatalog,
	t ir.Type,
	memberName string,
) (ir.Type, error) {
	curType := t

	// Dereference pointer if needed.
	if ptrType, ok := curType.(*ir.PointerType); ok {
		if err := resolvePointerPlaceholder(tc, ptrType); err != nil {
			return nil, err
		}
		pointee := tc.typesByID[ptrType.Pointee.GetID()]
		curType = pointee
	}

	// Access struct field.
	structType, ok := curType.(*ir.StructureType)
	if !ok {
		return nil, fmt.Errorf(
			"cannot access member %q on non-struct type %T", memberName, curType,
		)
	}

	// Ensure struct is completed.
	if err := completeGoTypes(tc, structType.GetID(), structType.GetID()); err != nil {
		return nil, err
	}

	// Find and explore the field type.
	f, err := field(tc, structType, memberName)
	if err != nil {
		return nil, err
	}

	if err := ensureTypeExplored(tc, f.Type); err != nil {
		return nil, err
	}

	return f.Type, nil
}

// ensureTypeExplored ensures a type and its immediate dependencies are
// explored, resolving placeholders as needed. For pointers, this also explores
// the pointee since expressions dereference their final pointer result.
func ensureTypeExplored(tc *typeCatalog, t ir.Type) error {
	switch tt := t.(type) {
	case *ir.PointerType:
		if err := resolvePointerPlaceholder(tc, tt); err != nil {
			return err
		}
		// Also explore pointee for final dereference.
		pointee := tc.typesByID[tt.Pointee.GetID()]
		return ensureTypeExplored(tc, pointee)
	case *ir.StructureType:
		return completeGoTypes(tc, tt.GetID(), tt.GetID())
	default:
		return nil
	}
}

// resolvePointerPlaceholder resolves a pointer's placeholder pointee if needed.
func resolvePointerPlaceholder(tc *typeCatalog, ptrType *ir.PointerType) error {
	pointee := tc.typesByID[ptrType.Pointee.GetID()]
	ppt, ok := pointee.(*pointeePlaceholderType)
	if !ok {
		return nil // Not a placeholder.
	}

	// Resolve the placeholder.
	newT, err := tc.addType(ppt.offset)
	if err != nil {
		return fmt.Errorf("failed to resolve pointee placeholder: %w", err)
	}
	ptrType.Pointee = newT

	// Complete the new type.
	if err := completeGoTypes(tc, newT.GetID(), newT.GetID()); err != nil {
		return fmt.Errorf("failed to complete pointee type: %w", err)
	}

	return nil
}

func materializePending(
	loclistReader *loclist.Reader,
	pointerSize uint8,
	tc *typeCatalog,
	pending []*pendingSubprogram,
	arch object.Architecture,
) ([]*ir.Subprogram, error) {
	subprograms := make([]*ir.Subprogram, 0, len(pending))
	for _, p := range pending {
		// Build IR subprogram from discovery state.
		sp := &ir.Subprogram{
			ID:                p.id,
			Name:              p.name,
			OutOfLinePCRanges: p.outOfLinePCRanges,
		}
		for _, inlined := range p.inlined {
			if len(inlined.inlinedPCRanges.Ranges) == 0 {
				continue
			}
			sp.InlinePCRanges = append(sp.InlinePCRanges, inlined.inlinedPCRanges)
		}
		// Build a map from typedef DWARF offset → dict index for annotating
		// variables whose DWARF type is a shape typedef.
		typedefOffsetToDictIdx := make(map[dwarf.Offset]int, len(p.dictTypedefs))
		for _, td := range p.dictTypedefs {
			typedefOffsetToDictIdx[td.offset] = td.dictIdx
		}

		// First, create variables defined directly under the subprogram/abstract DIEs.
		variableByOffset := make(map[dwarf.Offset]*ir.Variable, len(p.variables))
		// TODO: In the future we should track variables by lexical block scope
		// to be able to differentiate shadowing from malformed DWARF.
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
				// Annotate dict index if the variable's DWARF type directly
				// references a shape typedef (both parameters and return values).
				if typeOff, err := getAttr[dwarf.Offset](die, dwarf.AttrType); err == nil {
					if dictIdx, ok := typedefOffsetToDictIdx[typeOff]; ok {
						v.DictIndex = dictIdx
					}
				}
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

		// Set DictRegister if any variables got dict indices.
		if len(p.dictTypedefs) > 0 {
			sp.DictRegister = findDictRegister(sp, abiForArch(arch))
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
// probe-specific issues encountered in the process. For generic functions,
// multiple subprograms (shape instantiations) matching the same probe config
// are grouped into a single Probe with multiple ProbeInstances.
func createProbes(
	arch object.Architecture,
	pending []*pendingSubprogram,
	lineData map[ir.PCRange]lineData,
	idToSubprogram map[ir.SubprogramID]*ir.Subprogram,
	textSection *section,
	skipReturnEvents bool,
) ([]*ir.Probe, []*ir.Subprogram, []ir.ProbeIssue, error) {
	var (
		issues      []ir.ProbeIssue
		subprograms []*ir.Subprogram
	)

	// Collect instances grouped by probe config ID. We use a map to group
	// and a slice to preserve insertion order for deterministic output.
	type probeEntry struct {
		cfg       ir.ProbeDefinition
		instances []ir.ProbeInstance
	}
	probesByID := make(map[string]*probeEntry)
	var probeOrder []string
	subprogramSeen := make(map[ir.SubprogramID]bool)

	for _, p := range pending {
		if !p.issue.IsNone() {
			for _, cfg := range p.probesCfgs {
				issues = append(issues, ir.ProbeIssue{ProbeDefinition: cfg, Issue: p.issue})
			}
			continue
		}

		sp := idToSubprogram[p.id]
		for _, cfg := range p.probesCfgs {
			inst, iss, err := newProbeInstance(
				arch, cfg, sp, lineData, textSection, skipReturnEvents,
			)
			if err != nil {
				return nil, nil, nil, err
			}
			if !iss.IsNone() {
				issues = append(issues, ir.ProbeIssue{ProbeDefinition: cfg, Issue: iss})
				continue
			}
			id := cfg.GetID()
			entry, ok := probesByID[id]
			if !ok {
				entry = &probeEntry{cfg: cfg}
				probesByID[id] = entry
				probeOrder = append(probeOrder, id)
			}
			entry.instances = append(entry.instances, *inst)
			if !subprogramSeen[sp.ID] {
				subprogramSeen[sp.ID] = true
				subprograms = append(subprograms, sp)
			}
		}
	}

	// Build probe list in deterministic order.
	probes := make([]*ir.Probe, 0, len(probeOrder))
	for _, id := range probeOrder {
		entry := probesByID[id]
		probes = append(probes, &ir.Probe{
			ProbeDefinition: entry.cfg,
			Instances:       entry.instances,
		})
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
	goMapHashInfo      ir.GoMapHashInfo
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
		return processedDwarf{}, errors.New("runtime.firstmoduledata not found")
	}
	return processedDwarf{
		pendingSubprograms: append(v.subprograms, inlinedSubprograms...),
		goModuledataInfo:   v.goRuntimeInformation,
		goMapHashInfo:      v.goMapHashInfo,
		interestingTypes:   v.interestingTypes,
	}, nil
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
	goMapHashInfo        ir.GoMapHashInfo

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

func mapContains[K comparable, V any](m map[K]V, key K) bool {
	_, ok := m[key]
	return ok
}

// pendingSubprogram collects DWARF discovery for a subprogram without building
// IR. It holds the DIEs and ranges needed for materialization.
type pendingSubprogram struct {
	unit              *dwarf.Entry
	subprogramEntry   *dwarf.Entry
	variables         []*dwarf.Entry
	name              string
	outOfLinePCRanges []ir.PCRange

	// Inlined instances associated with this (abstract) subprogram.
	inlined    []*inlinedSubprogram
	abstract   bool
	id         ir.SubprogramID
	probesCfgs []ir.ProbeDefinition
	issue      ir.Issue
	// dictTypedefs records generic shape type parameter metadata. Empty
	// for non-generic functions.
	dictTypedefs []dictTypedef
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
		// Skip compiler-generated trampolines as probe targets. These are
		// wrapper functions for concrete generic instantiations used by
		// indirect calls (e.g., function pointers, interface dispatch).
		// However, we must still visit their children because trampolines
		// can contain inlined subroutines that are the only concrete
		// instances of an abstract (inlined) function.
		if entry.AttrField(dwarf.AttrTrampoline) != nil {
			return &inlinedSubroutineChildVisitor{
				root:      v.root,
				outOfLine: true,
			}, nil
		}

		probesCfgs := v.root.interests.subprograms[name]
		if len(probesCfgs) == 0 {
			probesCfgs = v.root.interests.matchGenericPatterns(name)
		}
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
		if entry.Tag != dwarf.TagTypedef && (name == "runtime.g" || name == "runtime.m" || name == "runtime._panic") {
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
		if !ok {
			return nil, nil
		}

		switch name {
		case "runtime.firstmoduledata":
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

		case "runtime.useAeshash", "runtime.aeskeysched":
			// Extract addresses of hash-related globals needed for swiss
			// table map lookups. See go_swiss_maps.md for details.
			addr, err := extractGlobalVarAddr(entry, name, v.root.pointerSize)
			if err != nil {
				// Non-fatal: map index expressions will be unsupported.
				return nil, nil
			}
			switch name {
			case "runtime.useAeshash":
				v.root.goMapHashInfo.UseAeshashAddr = addr
			case "runtime.aeskeysched":
				v.root.goMapHashInfo.AeskeyschedAddr = addr
			}
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

// extractGlobalVarAddr extracts the DW_OP_addr from a DW_TAG_variable entry.
// This is used for simple global variables where we only need the address.
func extractGlobalVarAddr(
	entry *dwarf.Entry,
	name string,
	pointerSize uint8,
) (uint64, error) {
	location, err := getAttr[[]byte](entry, dwarf.AttrLocation)
	if err != nil {
		return 0, fmt.Errorf("failed to get location for %s: %w", name, err)
	}
	// Use pointer size as the byte size — we only need the address, not the
	// data. The loclist parser requires a size but it doesn't affect DW_OP_addr.
	instructions, err := loclist.ParseInstructions(location, pointerSize, uint32(pointerSize))
	if err != nil {
		return 0, fmt.Errorf("failed to parse location for %s: %w", name, err)
	}
	if len(instructions) != 1 {
		return 0, fmt.Errorf("%s has %d location instructions, expected 1", name, len(instructions))
	}
	addr, ok := instructions[0].Op.(ir.Addr)
	if !ok {
		return 0, fmt.Errorf("%s location is not an address, got %T", name, instructions[0].Op)
	}
	return addr.Addr, nil
}

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
		return 0, 0, errors.New("struct type has no children")
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
			return 0, 0, errors.New("unexpected EOF while reading struct type")
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
				dictTypedefs:      t.dictTypedefs,
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

// dictTypedef records a .paramN typedef from a generic shape function,
// mapping a typedef name to its dictionary index.
type dictTypedef struct {
	name    string       // e.g. ".param0"
	dictIdx int          // DW_AT_go_dict_index value
	offset  dwarf.Offset // the typedef's own DIE offset
	typeOff dwarf.Offset // what the typedef points to (the shape type)
}

type subprogramChildVisitor struct {
	root            *rootVisitor
	unit            *dwarf.Entry
	subprogramEntry *dwarf.Entry
	probesCfgs      []ir.ProbeDefinition
	// Discovery: collect variable DIEs for later materialization.
	variableEntries       []*dwarf.Entry
	hasInlinedSubprograms bool
	// dictTypedefs records generic shape type parameter metadata from
	// DW_TAG_typedef entries with DW_AT_go_dict_index.
	dictTypedefs []dictTypedef
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
		// Typedefs in generic shape functions carry dict index metadata.
		// Record them so we can annotate variables with DictIndex later.
		if len(v.probesCfgs) > 0 {
			name, _, _ := maybeGetAttr[string](entry, dwarf.AttrName)
			dictIdx, hasDictIdx, _ := maybeGetAttr[int64](entry, dwarf.Attr(dwAtGoDictIndex))
			if hasDictIdx {
				typeOff, _, _ := maybeGetAttr[dwarf.Offset](entry, dwarf.AttrType)
				v.dictTypedefs = append(v.dictTypedefs, dictTypedef{
					name:    name,
					dictIdx: int(dictIdx),
					offset:  entry.Offset,
					typeOff: typeOff,
				})
			}
		}
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

type abstractSubprogram struct {
	unit       *dwarf.Entry
	probesCfgs []ir.ProbeDefinition
	name       string
	// Aggregated ranges from out-of-line and inlined instances.
	outOfLinePCRanges []ir.PCRange
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
	unit           *dwarf.Entry
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
			unit:            ctx.unitEntry,
			abstractOrigin:  ctx.abstractOrigin,
			inlinedPCRanges: ranges,
			variables:       ctx.variables,
		})
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
			unit:              ctx.unitEntry,
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
	unitEntry      *dwarf.Entry
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

// unsupportedFeatureError is a typed error indicating that a probe uses a
// feature that we recognise but have not implemented. It propagates from
// the resolver/emitter up to the Issue-emission sites, where errors.As
// surfaces it as ir.IssueKindUnsupportedFeature instead of the default
// ir.IssueKindConditionExpressionUnresolvable.
type unsupportedFeatureError struct {
	message string
}

func (e *unsupportedFeatureError) Error() string { return e.message }

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
		unitEntry             *dwarf.Entry
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
		unitEntry = nil
		found, _ := slices.BinarySearch(units[unitIdx:], refOffset)
		if found == 0 {
			return fmt.Errorf("ref %#x precedes first unit", refOffset)
		}
		unitIdx += found - 1
		reader = d.Reader()
		reader.Seek(units[unitIdx])
		var err error
		if unitEntry, err = reader.Next(); err != nil {
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
				return errors.New("no ranges for concrete subprogram entry")
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
				unitEntry:      unitEntry,
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
				if t.Name == "time.Time" {
					if err := completeGoTimeType(tc, t); err != nil {
						return err
					}
				}
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

// completeGoTimeType converts a time.Time StructureType into a GoTimeType
// and, when possible, resolves the time.Location cache fields so the BPF
// program can write the captured instant's UTC offset in place of the loc
// pointer. The function is best-effort: if any expected field is missing
// (e.g. a future Go version reshuffles internals) the type is left as a
// plain StructureType and the decoder falls back to UTC-only rendering.
func completeGoTimeType(tc *typeCatalog, st *ir.StructureType) error {
	wall, err := field(tc, st, "wall")
	if err != nil {
		return nil
	}
	ext, err := field(tc, st, "ext")
	if err != nil {
		return nil
	}
	loc, err := field(tc, st, "loc")
	if err != nil {
		return nil
	}

	timeType := &ir.GoTimeType{
		StructureType:   st,
		WallFieldOffset: wall.Offset,
		ExtFieldOffset:  ext.Offset,
		LocFieldOffset:  loc.Offset,
	}

	// Try to resolve the time.Location pointee so the BPF runtime can
	// chase the cache fast path. Failure is non-fatal: the decoder
	// renders in UTC when CacheResolved is false.
	if loc, ok := tryResolveTimeLocation(tc, loc.Type); ok {
		timeType.CacheResolved = true
		timeType.CacheStartOffset = loc.cacheStartOffset
		timeType.CacheEndOffset = loc.cacheEndOffset
		timeType.CacheZoneOffset = loc.cacheZoneOffset
		timeType.ZoneOffsetFieldOffset = loc.zoneOffsetFieldOffset
		timeType.ZoneOffsetFieldSize = loc.zoneOffsetFieldSize
	}

	tc.typesByID[st.ID] = timeType
	return nil
}

type resolvedTimeLocation struct {
	cacheStartOffset      uint32
	cacheEndOffset        uint32
	cacheZoneOffset       uint32
	zoneOffsetFieldOffset uint32
	zoneOffsetFieldSize   uint32
}

func tryResolveTimeLocation(
	tc *typeCatalog, locFieldType ir.Type,
) (resolvedTimeLocation, bool) {
	loc, err := resolvePointeeType[*ir.StructureType](tc, locFieldType)
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	cacheStart, err := field(tc, loc, "cacheStart")
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	cacheEnd, err := field(tc, loc, "cacheEnd")
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	cacheZone, err := field(tc, loc, "cacheZone")
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	zone, err := resolvePointeeType[*ir.StructureType](tc, cacheZone.Type)
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	zoneOffset, err := field(tc, zone, "offset")
	if err != nil {
		return resolvedTimeLocation{}, false
	}
	// The BPF opcode encoding packs cache_end/cache_zone/zone_offset
	// offsets into 16-bit fields to fit within BPF's argument-register
	// budget. time.Location and time.zone are tiny so this is generous,
	// but bail out (degrading to UTC rendering) if a future Go layout
	// outgrows it.
	if cacheEnd.Offset > 0xFFFF ||
		cacheZone.Offset > 0xFFFF ||
		zoneOffset.Offset > 0xFFFF {
		return resolvedTimeLocation{}, false
	}
	return resolvedTimeLocation{
		cacheStartOffset:      cacheStart.Offset,
		cacheEndOffset:        cacheEnd.Offset,
		cacheZoneOffset:       cacheZone.Offset,
		zoneOffsetFieldOffset: zoneOffset.Offset,
		zoneOffsetFieldSize:   zoneOffset.Type.GetByteSize(),
	}, true
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
			Name:             st.Name + ".str",
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
			Name:             st.Name + ".array",
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

// exploreTypesForExpressions validates that all types needed for expressions
// are available and marks invalid segments for expressions that fail to resolve
// (e.g., type mismatches, missing fields). Expressions that fail are cleared
// so they won't be processed by populateProbeEventsExpressions.
func exploreTypesForExpressions(
	tc *typeCatalog,
	analyzedProbes []analyzedProbe,
) {
	for i := range analyzedProbes {
		ap := &analyzedProbes[i]
		for j := range ap.expressions {
			expr := &ap.expressions[j]
			if expr.rootVariable == nil {
				continue
			}
			exprPath := ""
			if ref, ok := expr.expr.(*exprlang.RefExpr); ok {
				exprPath = ref.Ref
			}
			if _, err := exploreExpressionTypes(
				expr.expr, expr.rootVariable.Type, tc, exprPath,
			); err != nil {
				// Mark segment as invalid instead of failing.
				if expr.segment != nil && ap.template != nil {
					ap.template.Segments[expr.segmentIdx] = ir.InvalidSegment{
						Error: err.Error(),
						DSL:   expr.dsl,
					}
				} else {
					// Capture (non-segment) expression failure. Surface
					// typed unsupported-feature errors as a probe-level
					// IssueKindUnsupportedFeature so test infrastructure
					// can match them via `issue:UnsupportedFeature` tags.
					// Other capture-expression errors continue to be
					// silently skipped (matches pre-existing behavior).
					var unsup *unsupportedFeatureError
					if errors.As(err, &unsup) {
						ap.conditionIssue = ir.Issue{
							Kind:    ir.IssueKindUnsupportedFeature,
							Message: unsup.message,
						}
					}
				}
				// Clear the expression so it won't be processed later.
				expr.rootVariable = nil
			}
		}

		// Validate condition types. For compound conditions this walks
		// every leaf; the first leaf that fails exploration fails the
		// whole probe — matches today's single-leaf semantics.
		if cond := ap.condition; cond != nil {
			for _, leaf := range conditionLeafExprs(cond.expr) {
				sub, ok := conditionLeafSubExpr(leaf)
				if !ok {
					ap.conditionIssue = ir.Issue{
						Kind:    ir.IssueKindUnsupportedFeature,
						Message: fmt.Sprintf("unsupported condition expression type: %T", leaf),
					}
					ap.condition = nil
					break
				}
				rootVar, ok := cond.leafRoots[leaf]
				if !ok || rootVar == nil {
					ap.conditionIssue = ir.Issue{
						Kind:    ir.IssueKindConditionExpressionUnresolvable,
						Message: "condition leaf has no resolved root variable",
					}
					ap.condition = nil
					break
				}
				if _, err := exploreExpressionTypes(
					sub, rootVar.Type, tc, "",
				); err != nil {
					var unsup *unsupportedFeatureError
					if errors.As(err, &unsup) {
						ap.conditionIssue = ir.Issue{
							Kind:    ir.IssueKindUnsupportedFeature,
							Message: unsup.message,
						}
					} else {
						ap.conditionIssue = ir.Issue{
							Kind:    ir.IssueKindConditionExpressionUnresolvable,
							Message: fmt.Sprintf("condition type exploration failed: %v", err),
						}
					}
					ap.condition = nil
					break
				}
			}
		}
	}
}

// exploreExpressionTypes walks an expression tree and eagerly resolves types
// when encountering placeholders or unresolved pointees. This reuses the same
// traversal logic as resolveExpression but focuses on type resolution.
// Returns the resolved type after exploration.
func exploreExpressionTypes(
	expr exprlang.Expr,
	currentType ir.Type,
	tc *typeCatalog,
	exprPath string, // Path for error messages (e.g., "a.b.c")
) (ir.Type, error) {
	switch e := expr.(type) {
	case *exprlang.RefExpr:
		// Base case: nothing to do for simple refs.
		return currentType, nil

	case *exprlang.GetMemberExpr:
		// Collect all members in the chain (e.g., a.b.c becomes [c, b, a]).
		var members []string
		var base exprlang.Expr = e
		for {
			if gm, ok := base.(*exprlang.GetMemberExpr); ok {
				members = append(members, gm.Member)
				base = gm.Base
			} else {
				break
			}
		}
		// Reverse to get correct order (a.b.c).
		slices.Reverse(members)

		// Build expression path for error messages.
		basePath := exprPath
		if basePath == "" {
			if ref, ok := base.(*exprlang.RefExpr); ok {
				basePath = ref.Ref
			}
		}
		memberPath := basePath
		for _, m := range members {
			if memberPath != "" {
				memberPath += "." + m
			} else {
				memberPath = m
			}
		}

		// Explore base expression first and get its resolved type.
		baseType, err := exploreExpressionTypes(base, currentType, tc, basePath)
		if err != nil {
			return nil, err
		}

		// Now traverse the member chain, resolving types as we go.
		curType := baseType
		for i, memberName := range members {
			// Build current expression path for error messages.
			currentPath := basePath
			for j := 0; j <= i; j++ {
				if currentPath != "" {
					currentPath += "." + members[j]
				} else {
					currentPath = members[j]
				}
			}

			// Handle pointer dereference if needed.
			if ptrType, ok := curType.(*ir.PointerType); ok {
				// Check for void pointer.
				if _, isVoid := ptrType.Pointee.(*ir.VoidPointerType); isVoid {
					return nil, fmt.Errorf(
						"cannot dereference void pointer in expression %q",
						currentPath,
					)
				}

				// Check for unresolved pointee or placeholder.
				pointee := ptrType.Pointee
				pointee = tc.typesByID[pointee.GetID()]
				if ppt, ok := pointee.(*pointeePlaceholderType); ok {
					// Eagerly resolve the placeholder.
					newT, err := tc.addType(ppt.offset)
					if err != nil {
						return nil, fmt.Errorf(
							"failed to resolve pointee placeholder in expression %q: %w",
							currentPath, err,
						)
					}
					if err := completeGoTypes(tc, newT.GetID(), newT.GetID()); err != nil {
						return nil, fmt.Errorf(
							"failed to complete pointee type in expression %q: %w",
							currentPath, err,
						)
					}
					ptrType.Pointee = newT
					curType = newT
				} else if _, isUnresolved := pointee.(*ir.UnresolvedPointeeType); isUnresolved {
					// Unresolved pointee means it wasn't explored. We can't resolve it here
					// since we don't have the DWARF offset. This should have been handled
					// during initial type exploration, but if it wasn't, we'll fail during
					// expression resolution.
					return nil, fmt.Errorf(
						"cannot resolve expression %q: pointee type %q not explored",
						currentPath, pointee.GetName(),
					)
				} else {
					curType = pointee
				}
			}

			// Handle structure field access.
			structType, ok := curType.(*ir.StructureType)
			if !ok {
				return nil, fmt.Errorf(
					"cannot access member %q on type %T (%q) in expression %q",
					memberName, curType, curType.GetName(), currentPath,
				)
			}

			// Ensure struct type is completed (fields are populated).
			if err := completeGoTypes(tc, structType.GetID(), structType.GetID()); err != nil {
				return nil, fmt.Errorf(
					"failed to complete struct type in expression %q: %w",
					currentPath, err,
				)
			}

			// Find field.
			field, err := field(tc, structType, memberName)
			if err != nil {
				return nil, fmt.Errorf(
					"failed to resolve field %q in expression %q: %w",
					memberName, currentPath, err,
				)
			}

			curType = field.Type
			// Recursively explore field type if it's a pointer or struct.
			if _, err := exploreExpressionTypes(
				&exprlang.RefExpr{}, curType, tc, currentPath,
			); err != nil {
				return nil, err
			}
		}

		// Final dereference if result is a pointer.
		if ptrType, ok := curType.(*ir.PointerType); ok {
			// Check for void pointer.
			if _, isVoid := ptrType.Pointee.(*ir.VoidPointerType); isVoid {
				return nil, fmt.Errorf(
					"cannot dereference void pointer in expression %q",
					memberPath,
				)
			}

			pointee := ptrType.Pointee
			pointee = tc.typesByID[pointee.GetID()]
			if ppt, ok := pointee.(*pointeePlaceholderType); ok {
				// Eagerly resolve the placeholder.
				newT, err := tc.addType(ppt.offset)
				if err != nil {
					return nil, fmt.Errorf(
						"failed to resolve final pointee placeholder in expression %q: %w",
						memberPath, err,
					)
				}
				if err := completeGoTypes(tc, newT.GetID(), newT.GetID()); err != nil {
					return nil, fmt.Errorf(
						"failed to complete final pointee type in expression %q: %w",
						memberPath, err,
					)
				}
				ptrType.Pointee = newT
				curType = newT
			} else if _, isUnresolved := pointee.(*ir.UnresolvedPointeeType); isUnresolved {
				return nil, fmt.Errorf(
					"cannot resolve expression %q: final pointee type %q not explored",
					memberPath, pointee.GetName(),
				)
			} else {
				curType = pointee
			}
		}

		return curType, nil
	case *exprlang.LenExpr:
		return exploreLenExprTypes(e.Operand, currentType, tc, exprPath)

	case *exprlang.IsEmptyExpr:
		return exploreLenExprTypes(e.Operand, currentType, tc, exprPath)

	case *exprlang.EqExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)
	case *exprlang.NeExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)
	case *exprlang.LtExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)
	case *exprlang.LeExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)
	case *exprlang.GtExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)
	case *exprlang.GeExpr:
		return exploreComparisonExprTypes(e.Left, e.Right, currentType, tc, exprPath)

	case *exprlang.IndexExpr:
		return exploreIndexExprTypes(e, currentType, tc, exprPath)

	case *exprlang.AnyExpr:
		return exploreAnyAllTypes(e.Base, e.Pred, currentType, tc, exprPath)

	case *exprlang.AllExpr:
		return exploreAnyAllTypes(e.Base, e.Pred, currentType, tc, exprPath)

	case *exprlang.FilterExpr:
		// filter shares the predicate-validation semantics of any/all
		// (the predicate body must reference only @it / @value, the
		// base must be a slice or map). exploreAnyAllTypes is reused
		// for that validation; the static type returned to callers is
		// a per-call-site synthetic handle that's not known until
		// resolution time, so we return currentType as a placeholder.
		// Nested-position use (e.g. len(filter(...))) is rejected later
		// by resolveExpression's FilterExpr case as an
		// unsupportedFeatureError.
		if _, err := exploreAnyAllTypes(e.Base, e.Pred, currentType, tc, exprPath); err != nil {
			return nil, err
		}
		return currentType, nil

	case *exprlang.ContainsExpr:
		// Resolve the base. For map bases we keep the existing key-presence
		// validation. For slice / array bases we delegate to the any/all
		// type-explore path with a synthesized `@it == key` predicate — the
		// emit layer will desugar the same way, so we want the same type
		// check here. The result type is always bool.
		resolvedType, err := exploreExpressionTypes(e.Base, currentType, tc, exprPath)
		if err != nil {
			return nil, err
		}
		litExpr, ok := e.Key.(*exprlang.LiteralExpr)
		if !ok {
			return nil, fmt.Errorf(
				"contains: key must be a literal, got %T", e.Key,
			)
		}
		canonicalBase := tc.typesByID[resolvedType.GetID()]
		switch canonicalBase.(type) {
		case *ir.GoSliceHeaderType, *ir.ArrayType:
			eq := &exprlang.EqExpr{
				Left:  &exprlang.RefExpr{Ref: "@it"},
				Right: litExpr,
			}
			if _, err := exploreAnyAllTypes(e.Base, eq, currentType, tc, exprPath); err != nil {
				return nil, err
			}
			if tc.boolType == 0 {
				return nil, errors.New("bool type not found")
			}
			return tc.typesByID[tc.boolType], nil
		case *ir.GoStringHeaderType:
			if err := checkContainsStringBase(litExpr); err != nil {
				return nil, err
			}
			if tc.boolType == 0 {
				return nil, errors.New("bool type not found")
			}
			return tc.typesByID[tc.boolType], nil
		case *ir.GoMapType:
			// fall through to map handling below
		default:
			return nil, fmt.Errorf(
				"contains: operand must be a map, slice, or array, got %s",
				resolvedType.GetName(),
			)
		}
		mapType := canonicalBase.(*ir.GoMapType)
		headerType := tc.typesByID[mapType.HeaderType.GetID()]
		swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
		if !ok {
			if _, isHMap := headerType.(*ir.GoHMapHeaderType); isHMap {
				return nil, errors.New("contains not supported on old-style hmap; only swiss maps (Go 1.24+) are supported")
			}
			return nil, fmt.Errorf("contains not supported on map header type %T", headerType)
		}
		keyType, _, err := swissMapKeyValueTypes(swissHeader, tc)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve map key type: %w", err)
		}
		keyType = tc.typesByID[keyType.GetID()]
		if err := validateSwissMapKeyLiteral(litExpr, keyType); err != nil {
			return nil, err
		}
		if tc.boolType == 0 {
			return nil, errors.New("bool type not found")
		}
		return tc.typesByID[tc.boolType], nil

	default:
		// Unknown expression type - nothing to explore.
		return currentType, nil
	}
}

// exploreAnyAllTypes validates an any/all expression's base resolves to a
// supported collection type (slice, array, or swiss-table map) and explores
// the predicate body's types under the iteration variable's type. Returns
// bool — every any/all expression produces a bool regardless of element type.
func exploreAnyAllTypes(
	base, pred exprlang.Expr,
	currentType ir.Type,
	tc *typeCatalog,
	exprPath string,
) (ir.Type, error) {
	baseType, err := exploreExpressionTypes(base, currentType, tc, exprPath)
	if err != nil {
		return nil, err
	}
	canonical := tc.typesByID[baseType.GetID()]
	// Map each in-scope ref name (`@it`, `@key`, `@value`) to the type
	// its bytes carry. For slice/array there's only `@it`; for maps,
	// `@it` and `@key` are the key type and `@value` is the value type.
	refTypes := map[string]ir.Type{}
	switch t := canonical.(type) {
	case *ir.GoSliceHeaderType:
		refTypes["@it"] = t.Data.Element
	case *ir.ArrayType:
		refTypes["@it"] = t.Element
	case *ir.GoMapType:
		headerType := tc.typesByID[t.HeaderType.GetID()]
		swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
		if !ok {
			if _, isHMap := headerType.(*ir.GoHMapHeaderType); isHMap {
				return nil, &unsupportedFeatureError{
					message: "any/all/filter over old-style hmap not supported; only swiss maps (Go 1.24+) are supported",
				}
			}
			return nil, fmt.Errorf(
				"any/all over map: unsupported header type %T", headerType,
			)
		}
		keyType, valType, err := swissMapKeyValueTypes(swissHeader, tc)
		if err != nil {
			return nil, err
		}
		refTypes["@it"] = keyType
		refTypes["@key"] = keyType
		refTypes["@value"] = valType
	default:
		return nil, fmt.Errorf(
			"any/all base must be a slice, array, or map; got %s (%T)",
			canonical.GetName(), canonical,
		)
	}
	// Explore each leaf of the predicate body using the type of its
	// rooted ref. This catches mismatches like `any(intSlice, @it == "x")`
	// or `any(map[K]V, @value == bogus)` early.
	for _, leaf := range conditionLeafExprs(pred) {
		sub, ok := conditionLeafSubExpr(leaf)
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf: cannot derive sub-expression from %T", leaf,
			)
		}
		rootName, ok := extractRootVariableName(sub)
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf: cannot derive root variable from %T", sub,
			)
		}
		rootType, ok := refTypes[rootName]
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf references %q, which is not in scope", rootName,
			)
		}
		if _, err := exploreExpressionTypes(sub, rootType, tc, rootName); err != nil {
			return nil, err
		}
	}
	if tc.boolType == 0 {
		return nil, errors.New("bool type not found")
	}
	return tc.typesByID[tc.boolType], nil
}

// exploreComparisonExprTypes resolves the LHS of a comparison node
// (Eq / Ne / Lt / Le / Gt / Ge), validates the RHS is a literal, and
// returns the bool result type. Shared across all six comparison nodes
// because they have identical type-exploration semantics.
func exploreComparisonExprTypes(
	left, right exprlang.Expr,
	currentType ir.Type,
	tc *typeCatalog,
	exprPath string,
) (ir.Type, error) {
	if _, err := exploreExpressionTypes(left, currentType, tc, exprPath); err != nil {
		return nil, err
	}
	if _, ok := right.(*exprlang.LiteralExpr); !ok {
		return nil, fmt.Errorf("right expression is not a literal: %T", right)
	}
	if tc.boolType == 0 {
		return nil, errors.New("bool type not found")
	}
	return tc.typesByID[tc.boolType], nil
}

// exploreIndexExprTypes explores types for an index expression, resolving
// the base expression and validating the element type.
func exploreIndexExprTypes(
	e *exprlang.IndexExpr,
	currentType ir.Type,
	tc *typeCatalog,
	exprPath string,
) (ir.Type, error) {
	// Explore the base expression to resolve its type.
	resolvedType, err := exploreExpressionTypes(e.Base, currentType, tc, exprPath)
	if err != nil {
		return nil, err
	}

	// The index must always be a literal.
	litExpr, ok := e.Index.(*exprlang.LiteralExpr)
	if !ok {
		return nil, fmt.Errorf("index must be a literal, got %T", e.Index)
	}

	// Resolve the collection type to get the element type.
	canonical := tc.typesByID[resolvedType.GetID()]
	switch t := canonical.(type) {
	case *ir.ArrayType:
		idx, ok := litExpr.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("array index must be an integer literal, got %T", litExpr.Value)
		}
		if idx < 0 {
			return nil, fmt.Errorf("array index must be non-negative, got %d", idx)
		}
		if uint32(idx) >= t.Count {
			return nil, fmt.Errorf("index %d out of bounds for array of length %d", idx, t.Count)
		}
		elemType := t.Element
		if err := ensureTypeExplored(tc, elemType); err != nil {
			return nil, err
		}
		return elemType, nil
	case *ir.GoSliceHeaderType:
		idx, ok := litExpr.Value.(int64)
		if !ok {
			return nil, fmt.Errorf("slice index must be an integer literal, got %T", litExpr.Value)
		}
		if idx < 0 {
			return nil, fmt.Errorf("slice index must be non-negative, got %d", idx)
		}
		elemType := t.Data.Element
		if err := ensureTypeExplored(tc, elemType); err != nil {
			return nil, err
		}
		return elemType, nil
	case *ir.GoMapType:
		return exploreSwissMapIndexTypes(t, litExpr, tc)
	default:
		return nil, fmt.Errorf("index not supported on type %T (%q)", canonical, canonical.GetName())
	}
}

// exploreSwissMapIndexTypes validates the key literal and returns the map's
// value element type for a swiss map index expression.
func exploreSwissMapIndexTypes(
	mapType *ir.GoMapType,
	litExpr *exprlang.LiteralExpr,
	tc *typeCatalog,
) (ir.Type, error) {
	// Unwrap GoMapType → header type.
	headerType := tc.typesByID[mapType.HeaderType.GetID()]
	swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
	if !ok {
		if _, isHMap := headerType.(*ir.GoHMapHeaderType); isHMap {
			return nil, errors.New("index not supported on old-style hmap; only swiss maps (Go 1.24+) are supported")
		}
		return nil, fmt.Errorf("index not supported on map header type %T", headerType)
	}

	// Navigate to key/value types via the group structure:
	// GroupType → "slots" field → ArrayType → Element (slot struct) → "key"/"elem" fields
	keyType, valType, err := swissMapKeyValueTypes(swissHeader, tc)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve map key/value types: %w", err)
	}

	// Validate the literal key matches the map's key type.
	keyType = tc.typesByID[keyType.GetID()]
	if err := validateSwissMapKeyLiteral(litExpr, keyType); err != nil {
		return nil, err
	}

	// Ensure the value type is explored.
	valType = tc.typesByID[valType.GetID()]
	if err := ensureTypeExplored(tc, valType); err != nil {
		return nil, err
	}
	return valType, nil
}

// swissMapKeyValueTypes extracts the key and value types from a swiss map's
// group structure.
func swissMapKeyValueTypes(
	swissHeader *ir.GoSwissMapHeaderType,
	tc *typeCatalog,
) (keyType, valType ir.Type, err error) {
	slotsField, err := field(tc, swissHeader.GroupType, "slots")
	if err != nil {
		return nil, nil, fmt.Errorf("group has no slots field: %w", err)
	}
	slotsFieldType := tc.typesByID[slotsField.Type.GetID()]
	entryArray, ok := slotsFieldType.(*ir.ArrayType)
	if !ok {
		return nil, nil, fmt.Errorf("slots field is not an array type: %T", slotsFieldType)
	}
	slotStruct, ok := entryArray.Element.(*ir.StructureType)
	if !ok {
		return nil, nil, fmt.Errorf("slot array element is not a struct: %T", entryArray.Element)
	}
	keyField, err := field(tc, slotStruct, "key")
	if err != nil {
		return nil, nil, fmt.Errorf("slot struct has no key field: %w", err)
	}
	elemField, err := field(tc, slotStruct, "elem")
	if err != nil {
		return nil, nil, fmt.Errorf("slot struct has no elem field: %w", err)
	}
	return keyField.Type, elemField.Type, nil
}

// validateSwissMapKeyLiteral checks that the literal value is compatible with
// the map's key type.
func validateSwissMapKeyLiteral(
	litExpr *exprlang.LiteralExpr,
	keyType ir.Type,
) error {
	switch keyType.(type) {
	case *ir.BaseType:
		// Base type keys accept integer, float, bool, and string literals
		// (validated further during coerceLiteral in resolveSwissMapIndex).
		switch litExpr.Value.(type) {
		case int64, float64, bool, string:
			// int64 and float64 from JSON; bool for true/false keys;
			// string for hex/octal notation.
			// coerceLiteral handles the full validation and range checking.
		default:
			return fmt.Errorf(
				"map index: base type key requires integer or bool literal, got %T",
				litExpr.Value,
			)
		}
	case *ir.GoStringHeaderType:
		litStr, ok := litExpr.Value.(string)
		if !ok {
			return fmt.Errorf(
				"map index: string key requires string literal, got %T",
				litExpr.Value,
			)
		}
		if len(litStr) > ir.MaxMapStringKeyLength {
			return fmt.Errorf(
				"map index: string key too long (%d bytes, max %d)",
				len(litStr), ir.MaxMapStringKeyLength,
			)
		}
	default:
		return fmt.Errorf(
			"map index: unsupported key type %T (%q); only base types and strings are supported",
			keyType, keyType.GetName(),
		)
	}
	return nil
}

// exploreLenExprTypes explores types for a len/isEmpty operand, resolving
// through pointers and validating the collection type has a length field.
func exploreLenExprTypes(
	operand exprlang.Expr,
	currentType ir.Type,
	tc *typeCatalog,
	exprPath string,
) (ir.Type, error) {
	// Explore the operand to resolve its type.
	resolvedType, err := exploreExpressionTypes(operand, currentType, tc, exprPath)
	if err != nil {
		return nil, err
	}

	// Look up the canonical type from the catalog (may have been promoted
	// from StructureType to GoStringHeaderType etc. by completeGoTypes).
	resolvedType = tc.typesByID[resolvedType.GetID()]

	// Unwrap GoMapType to its header type (maps are pointers to headers).
	_, isMap := resolvedType.(*ir.GoMapType)
	resolvedType = unwrapMapType(resolvedType, tc)

	// If result is a pointer, dereference to get the collection type.
	if ptrType, ok := resolvedType.(*ir.PointerType); ok {
		pointee := ptrType.Pointee
		pointee = tc.typesByID[pointee.GetID()]
		if ppt, ok := pointee.(*pointeePlaceholderType); ok {
			newT, err := tc.addType(ppt.offset)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve pointee for len: %w", err)
			}
			resolvedType = newT
		} else {
			resolvedType = pointee
		}
	}

	// Get the struct and field for the length.
	structType, fieldName, err := lenFieldInfo(resolvedType)
	if err != nil {
		return nil, err
	}

	f, err := field(tc, structType, fieldName)
	if err != nil {
		return nil, fmt.Errorf("len field lookup failed: %w", err)
	}

	// Normalize map count types to uint64 for consistency across
	// Go versions and architectures.
	if isMap && tc.uint64Type != 0 {
		return tc.typesByID[tc.uint64Type], nil
	}
	return f.Type, nil
}

// lenFieldInfo maps a collection type to the struct and field name containing
// its length.
func lenFieldInfo(typ ir.Type) (*ir.StructureType, string, error) {
	switch t := typ.(type) {
	case *ir.GoStringHeaderType:
		return t.StructureType, "len", nil
	case *ir.GoSliceHeaderType:
		return t.StructureType, "len", nil
	case *ir.GoHMapHeaderType:
		return t.StructureType, "count", nil
	case *ir.GoSwissMapHeaderType:
		return t.StructureType, "used", nil
	default:
		var kindString string
		if kind, ok := typ.GetGoKind(); ok {
			kindString = kind.String()
		} else {
			// It's not clear how one could get here, but just in case
			// we'll state the name of internal ir type. It might help
			// debug the situation.
			kindString = reflect.TypeOf(typ).Name()
		}
		return nil, "", fmt.Errorf(
			"len/isEmpty not supported on type %q (%s)",
			typ.GetName(), kindString,
		)
	}
}

// unwrapMapType returns the map's header type if typ is a GoMapType, otherwise
// returns typ unchanged. GoMapType is an IR-level wrapper that hides the
// pointer-to-header representation of Go maps; callers that need the header
// (e.g. len/isEmpty) must see through it.
func unwrapMapType(typ ir.Type, tc *typeCatalog) ir.Type {
	if m, ok := typ.(*ir.GoMapType); ok {
		return tc.typesByID[m.HeaderType.GetID()]
	}
	return typ
}

// resolveGetMemberChain resolves a GetMemberExpr chain (e.g. a.b.c) to an IR
// expression *without* applying the final auto-deref. If the chain ends at a
// pointer-typed field, the returned Expression's Type is the pointer; the
// caller chooses whether to deref via applyFinalPointerDeref.
//
// trailingBias is an accumulated field offset that the caller must apply to
// any subsequent op it appends (typically 0; non-zero only in defensive
// fallback paths where field-access didn't fold into an existing op).
func resolveGetMemberChain(
	e *exprlang.GetMemberExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (expr ir.Expression, trailingBias uint32, _ error) {
	// Collect all members in the chain (e.g., a.b.c becomes [c, b, a]).
	var members []string
	var base exprlang.Expr = e
	for {
		if gm, ok := base.(*exprlang.GetMemberExpr); ok {
			members = append(members, gm.Member)
			base = gm.Base
		} else {
			break
		}
	}
	// Reverse to get correct order (a.b.c).
	slices.Reverse(members)

	// Resolve base expression (RefExpr or other).
	baseExpr, err := resolveExpression(base, rootVar, tc)
	if err != nil {
		return ir.Expression{}, 0, fmt.Errorf(
			"failed to resolve base expression: %w", err,
		)
	}

	currentType := baseExpr.Type
	operations := baseExpr.Operations
	bias := uint32(0)
	hasDereferenced := false
	// Track the index of the last DereferenceOp we added, so we can update
	// the correct one when we encounter field accesses after dereferences.
	lastDerefOpIdx := -1

	// Detect if the base expression already ends with a DereferenceOp or
	// SwissMapLookupOp (e.g., from slice index or map index resolution).
	// If so, initialize state so the member loop updates the correct op.
	if len(operations) > 0 {
		switch operations[len(operations)-1].(type) {
		case *ir.DereferenceOp, *ir.SwissMapLookupOp:
			hasDereferenced = true
			lastDerefOpIdx = len(operations) - 1
		}
	}

	for _, memberName := range members {
		// Handle pointer dereference if needed.
		if ptrType, ok := currentType.(*ir.PointerType); ok {
			if _, isVoid := ptrType.Pointee.(*ir.VoidPointerType); isVoid {
				return ir.Expression{}, 0, errors.New("cannot dereference void pointer")
			}
			if _, isUnresolved := ptrType.Pointee.(*ir.UnresolvedPointeeType); isUnresolved {
				return ir.Expression{}, 0, fmt.Errorf(
					"cannot resolve expression: pointee type %q not explored",
					ptrType.Pointee.GetName(),
				)
			}
			pointee, err := resolvePointeeType[ir.Type](tc, currentType)
			if err != nil {
				return ir.Expression{}, 0, fmt.Errorf(
					"failed to resolve pointee type: %w", err,
				)
			}
			operations = append(operations, &ir.DereferenceOp{
				Bias:     bias,
				ByteSize: pointee.GetByteSize(),
			})
			lastDerefOpIdx = len(operations) - 1
			currentType = pointee
			bias = 0
			hasDereferenced = true
		}

		structType, ok := currentType.(*ir.StructureType)
		if !ok {
			return ir.Expression{}, 0, fmt.Errorf(
				"cannot access member %q on type %T (%q)",
				memberName, currentType, currentType.GetName(),
			)
		}
		field, err := field(tc, structType, memberName)
		if err != nil {
			return ir.Expression{}, 0, fmt.Errorf(
				"field %q not found in type %q",
				memberName, structType.Name,
			)
		}

		if !hasDereferenced {
			// Direct struct access: update LocationOp offset directly.
			if len(operations) == 1 {
				if locOp, ok := operations[0].(*ir.LocationOp); ok {
					locOp.Offset += field.Offset
					locOp.ByteSize = field.Type.GetByteSize()
				}
			}
		} else if lastDerefOpIdx >= 0 && lastDerefOpIdx < len(operations) {
			// After dereference or map lookup: update the operation that
			// corresponds to the current data read (tracked by lastDerefOpIdx).
			switch op := operations[lastDerefOpIdx].(type) {
			case *ir.DereferenceOp:
				op.Bias += field.Offset
				op.ByteSize = field.Type.GetByteSize()
			case *ir.SwissMapLookupOp:
				op.ValInSlotOffset += uint16(field.Offset)
				op.ValByteSize = field.Type.GetByteSize()
			default:
				bias += field.Offset
			}
		} else {
			bias += field.Offset
		}

		currentType = field.Type
	}

	return ir.Expression{Type: currentType, Operations: operations}, bias, nil
}

// applyFinalPointerDeref appends a DereferenceOp that follows the pointer at
// the end of expr through to its pointee. expr.Type must be *ir.PointerType.
// trailingBias is applied as the deref bias (typically 0; non-zero only when
// the GetMemberExpr chain accumulated an offset that didn't fold into an
// existing op).
//
// Used by resolveExpression's GetMemberExpr branch to give capture semantics
// (the pointed-to value, not the raw pointer).
func applyFinalPointerDeref(
	expr ir.Expression,
	trailingBias uint32,
	tc *typeCatalog,
) (ir.Expression, error) {
	ptrType, ok := expr.Type.(*ir.PointerType)
	if !ok {
		return ir.Expression{}, fmt.Errorf(
			"applyFinalPointerDeref: type is not a pointer: %T", expr.Type,
		)
	}
	if _, isVoid := ptrType.Pointee.(*ir.VoidPointerType); isVoid {
		return ir.Expression{}, errors.New("cannot dereference void pointer")
	}
	if _, isUnresolved := ptrType.Pointee.(*ir.UnresolvedPointeeType); isUnresolved {
		return ir.Expression{}, fmt.Errorf(
			"cannot resolve expression: pointee type %q not explored",
			ptrType.Pointee.GetName(),
		)
	}
	pointee, err := resolvePointeeType[ir.Type](tc, expr.Type)
	if err != nil {
		return ir.Expression{}, fmt.Errorf(
			"failed to resolve final pointee type: %w", err,
		)
	}
	ops := append(expr.Operations, &ir.DereferenceOp{
		Bias:     trailingBias,
		ByteSize: pointee.GetByteSize(),
	})
	return ir.Expression{Type: pointee, Operations: ops}, nil
}

// resolveExpression resolves an expression AST to an IR Expression.
func resolveExpression(
	expr exprlang.Expr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	switch e := expr.(type) {
	case *exprlang.RefExpr:
		variableSize := rootVar.Type.GetByteSize()
		return ir.Expression{
			Type: rootVar.Type,
			Operations: []ir.ExpressionOp{
				&ir.LocationOp{
					Variable: rootVar,
					Offset:   0,
					ByteSize: uint32(variableSize),
				},
			},
		}, nil

	case *exprlang.GetMemberExpr:
		expr, bias, err := resolveGetMemberChain(e, rootVar, tc)
		if err != nil {
			return ir.Expression{}, err
		}
		// Final dereference if result is a pointer. This is the "auto-deref"
		// affordance: a member-access chain ending at a pointer-typed field
		// resolves to the *pointed-to* value, matching capture semantics.
		// Callers that need the pointer itself (e.g. null comparison) call
		// resolveGetMemberChain directly.
		if _, ok := expr.Type.(*ir.PointerType); ok {
			return applyFinalPointerDeref(expr, bias, tc)
		}
		return expr, nil

	case *exprlang.IndexExpr:
		return resolveIndexExpression(e, rootVar, tc)

	case *exprlang.LenExpr:
		return resolveLenExpression(e.Operand, rootVar, tc)

	case *exprlang.IsEmptyExpr:
		return resolveIsEmptyComparison(e, rootVar, tc)

	case *exprlang.ContainsExpr:
		return resolveContainsExpression(e, rootVar, tc)

	case *exprlang.EqExpr:
		return resolveComparisonExpression(ir.CmpEq, e.Left, e.Right, rootVar, tc)
	case *exprlang.NeExpr:
		return resolveComparisonExpression(ir.CmpNe, e.Left, e.Right, rootVar, tc)
	case *exprlang.LtExpr:
		return resolveComparisonExpression(ir.CmpLt, e.Left, e.Right, rootVar, tc)
	case *exprlang.LeExpr:
		return resolveComparisonExpression(ir.CmpLe, e.Left, e.Right, rootVar, tc)
	case *exprlang.GtExpr:
		return resolveComparisonExpression(ir.CmpGt, e.Left, e.Right, rootVar, tc)
	case *exprlang.GeExpr:
		return resolveComparisonExpression(ir.CmpGe, e.Left, e.Right, rootVar, tc)

	case *exprlang.AnyExpr:
		return resolveAnyAllExpression(e.Base, e.Pred, ir.QuantifierAny, rootVar, tc)
	case *exprlang.AllExpr:
		return resolveAnyAllExpression(e.Base, e.Pred, ir.QuantifierAll, rootVar, tc)

	case *exprlang.FilterExpr:
		// filter() is only legal as the top-level expression of a
		// capture or template segment; reaching here means a filter()
		// appeared nested inside another operator (e.g. len(filter(...))
		// or any(filter(...))). Surface as a typed unsupported-feature
		// error so the probe fails with IssueKindUnsupportedFeature
		// rather than being silently dropped by the generic
		// "skip failed snapshot expressions" path.
		return ir.Expression{}, &unsupportedFeatureError{
			message: "filter is not composable: filter(...) is only allowed as the top-level expression of a capture or message template segment, not nested inside another operator",
		}

	default:
		return ir.Expression{}, fmt.Errorf(
			"unsupported expression type: %T", expr,
		)
	}
}

// resolveFilterExpression lowers a top-level filter() expression in
// capture or template-segment position. Dispatches by source-collection
// type to the slice or map filter resolver, each of which synthesizes
// the per-call-site filter type pair and builds the inline marker op
// sequence.
func resolveFilterExpression(
	e *exprlang.FilterExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	baseExpr, err := resolveExpression(e.Base, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve filter base: %w", err)
	}
	canonical := tc.typesByID[baseExpr.Type.GetID()]
	switch t := canonical.(type) {
	case *ir.GoSliceHeaderType:
		return emitSliceFilterMarker(baseExpr, t, e.Pred, rootVar, tc)
	case *ir.GoMapType:
		return emitMapFilterMarker(baseExpr, t, e.Pred, tc)
	case *ir.ArrayType:
		return ir.Expression{}, errors.New(
			"filter over an array is not supported; only slices and maps are supported",
		)
	default:
		return ir.Expression{}, fmt.Errorf(
			"filter base must be a slice or map; got %s (%T)",
			canonical.GetName(), canonical,
		)
	}
}

// resolveAnyAllExpression lowers an AnyExpr or AllExpr in expression
// (template-segment / capture-expression) position. It reuses the same
// emitAnyAllLoop machinery as the condition path, so the resulting opcode
// sequence is byte-identical to a condition-position any/all. The result
// type is always bool: a single byte at sm->offset that ExprSaveOp records
// as the expression's value.
func resolveAnyAllExpression(
	base, pred exprlang.Expr,
	quantifier ir.Quantifier,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	if tc.boolType == 0 {
		return ir.Expression{}, errors.New("bool type not found")
	}
	var la labelAllocator
	ops, err := emitAnyAllLoop(base, pred, quantifier, rootVar, tc, &la)
	if err != nil {
		return ir.Expression{}, err
	}
	return ir.Expression{
		Type:       tc.typesByID[tc.boolType],
		Operations: ops,
	}, nil
}

// checkContainsStringBase validates the key literal for a `contains` call
// whose base is a Go string. Substring containment is a recognised feature
// we have not implemented yet — when the key is a string literal we return
// an *unsupportedFeatureError so the surrounding probe surfaces an
// IssueKindUnsupportedFeature. Other literal kinds (null, int, float, bool)
// are reported as a type mismatch via the default
// IssueKindConditionExpressionUnresolvable path.
func checkContainsStringBase(litExpr *exprlang.LiteralExpr) error {
	if litExpr.Value == nil {
		return errors.New(
			"contains: substring search expects a string literal; got null",
		)
	}
	if _, ok := litExpr.Value.(string); ok {
		return &unsupportedFeatureError{
			message: "contains: substring containment on a string base is not yet supported",
		}
	}
	return fmt.Errorf(
		"contains: substring search expects a string literal; got %T (value %v)",
		litExpr.Value, litExpr.Value,
	)
}

// resolveContainsExpression resolves contains(coll, literalKey) into IR ops
// that produce a single bool. For a map base this uses
// ir.SwissMapLookupOp.ExistenceOnly. For a slice or array base it desugars
// to any(coll, {@it == key}) and reuses emitAnyAllLoop — the generated
// opcodes are byte-identical to a user-written any(...) form. For a string
// base substring containment is recognised but not yet implemented; it
// returns an *unsupportedFeatureError so the surrounding probe can surface
// the right Issue kind in condition position (in template / capture
// position the failure becomes an InvalidSegment / silent skip).
func resolveContainsExpression(
	e *exprlang.ContainsExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	baseExpr, err := resolveExpression(e.Base, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve contains base: %w", err)
	}
	canonical := tc.typesByID[baseExpr.Type.GetID()]
	switch canonical.(type) {
	case *ir.GoSliceHeaderType, *ir.ArrayType:
		litExpr, ok := e.Key.(*exprlang.LiteralExpr)
		if !ok {
			return ir.Expression{}, fmt.Errorf(
				"contains: key must be a literal, got %T", e.Key,
			)
		}
		pred := &exprlang.EqExpr{
			Left:  &exprlang.RefExpr{Ref: "@it"},
			Right: litExpr,
		}
		return resolveAnyAllExpression(e.Base, pred, ir.QuantifierAny, rootVar, tc)
	case *ir.GoStringHeaderType:
		litExpr, ok := e.Key.(*exprlang.LiteralExpr)
		if !ok {
			return ir.Expression{}, fmt.Errorf(
				"contains: key must be a literal, got %T", e.Key,
			)
		}
		return ir.Expression{}, checkContainsStringBase(litExpr)
	}
	mapType, ok := canonical.(*ir.GoMapType)
	if !ok {
		return ir.Expression{}, fmt.Errorf(
			"contains: operand must be a map, slice, or array, got %s",
			baseExpr.Type.GetName(),
		)
	}
	litExpr, ok := e.Key.(*exprlang.LiteralExpr)
	if !ok {
		return ir.Expression{}, fmt.Errorf(
			"contains: key must be a literal, got %T", e.Key,
		)
	}
	ops, resultType, err := buildSwissMapLookup(
		baseExpr.Operations, mapType, litExpr, true, tc,
	)
	if err != nil {
		return ir.Expression{}, err
	}
	return ir.Expression{Type: resultType, Operations: ops}, nil
}

// resolveComparisonExpression lowers a comparison node (Eq / Ne / Lt / Le
// / Gt / Ge) by resolving the LHS expression and dispatching to
// resolveComparison with a literal RHS. Shared across all six comparison
// shapes — only the CmpOp differs.
func resolveComparisonExpression(
	op ir.CmpOp,
	left, right exprlang.Expr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	litExpr, ok := right.(*exprlang.LiteralExpr)
	if !ok {
		return ir.Expression{}, fmt.Errorf(
			"unsupported %s RHS type: %T (only literals are supported)",
			op, right,
		)
	}
	lhsExpr, err := resolveComparisonLHS(left, litExpr, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve %s LHS: %w", op, err)
	}
	return resolveComparison(op, lhsExpr, litExpr, tc)
}

// resolveComparisonLHS resolves the LHS of a comparison. For comparisons
// against null on a member-access chain (e.g. `m.ptrField != null`), it
// skips the auto-deref so the LHS keeps its pointer type. The general
// resolveExpression path always auto-derefs trailing pointers to provide
// capture semantics, which would strip exactly the byte the null comparison
// needs.
func resolveComparisonLHS(
	left exprlang.Expr,
	rhsLit *exprlang.LiteralExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	if rhsLit.Value == nil {
		if gmExpr, ok := left.(*exprlang.GetMemberExpr); ok {
			expr, bias, err := resolveGetMemberChain(gmExpr, rootVar, tc)
			if err != nil {
				return ir.Expression{}, err
			}
			if _, isPtr := expr.Type.(*ir.PointerType); isPtr {
				// Defensive: a non-zero trailing bias here means the chain
				// hit one of the fallback paths in resolveGetMemberChain.
				// Apply it as an offset on the trailing op so the pointer
				// bytes land at sm->offset.
				if bias != 0 {
					if locOp, ok := expr.Operations[len(expr.Operations)-1].(*ir.LocationOp); ok {
						locOp.Offset += bias
					}
				}
				return expr, nil
			}
		}
	}
	return resolveExpression(left, rootVar, tc)
}

// indexElementType returns the element type for an indexable collection type
// (array, slice, or map).
func indexElementType(tc *typeCatalog, baseType ir.Type) (ir.Type, error) {
	canonical := tc.typesByID[baseType.GetID()]
	// Dereference pointer if needed (e.g., *[N]T or *[]T).
	if ptrType, ok := canonical.(*ir.PointerType); ok {
		canonical = tc.typesByID[ptrType.Pointee.GetID()]
	}
	switch t := canonical.(type) {
	case *ir.ArrayType:
		return t.Element, nil
	case *ir.GoSliceHeaderType:
		return t.Data.Element, nil
	case *ir.GoMapType:
		headerType := tc.typesByID[t.HeaderType.GetID()]
		swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
		if !ok {
			return nil, fmt.Errorf("index not supported on non-swiss map type %T", headerType)
		}
		_, valType, err := swissMapKeyValueTypes(swissHeader, tc)
		if err != nil {
			return nil, err
		}
		return valType, nil
	default:
		return nil, fmt.Errorf("index not supported on type %T (%q)", canonical, canonical.GetName())
	}
}

// resolveIndexExpression resolves an index expression to IR operations that
// read only the single element at the given index. For arrays, this adjusts
// the offset on the existing LocationOp/DereferenceOp. For slices, this
// narrows to the data pointer and adds a DereferenceOp with element offset.
// For maps, this emits a SwissMapLookupOp that performs an O(1) hash lookup.
func resolveIndexExpression(
	e *exprlang.IndexExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	// Validate the index is a non-negative integer literal.
	litExpr, ok := e.Index.(*exprlang.LiteralExpr)
	if !ok {
		return ir.Expression{}, fmt.Errorf("index must be a literal, got %T", e.Index)
	}
	// Resolve the base expression.
	baseExpr, err := resolveExpression(e.Base, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve index base: %w", err)
	}

	// Determine the collection type and element type.
	canonical := tc.typesByID[baseExpr.Type.GetID()]
	switch t := canonical.(type) {
	case *ir.ArrayType:
		idx, ok := litExpr.Value.(int64)
		if !ok || idx < 0 {
			return ir.Expression{}, errors.New("array index must be a non-negative integer literal")
		}
		if idx > math.MaxUint32 {
			return ir.Expression{}, fmt.Errorf("index %d exceeds maximum (%d)", idx, math.MaxUint32)
		}
		return resolveArrayIndex(baseExpr, t, idx)
	case *ir.GoSliceHeaderType:
		idx, ok := litExpr.Value.(int64)
		if !ok || idx < 0 {
			return ir.Expression{}, errors.New("slice index must be a non-negative integer literal")
		}
		if idx > math.MaxUint32 {
			return ir.Expression{}, fmt.Errorf("index %d exceeds maximum (%d)", idx, math.MaxUint32)
		}
		return resolveSliceIndex(baseExpr, t, idx, tc)
	case *ir.GoMapType:
		return resolveSwissMapIndex(baseExpr, t, litExpr, tc)
	default:
		return ir.Expression{}, fmt.Errorf(
			"index not supported on type %T (%q)", canonical, canonical.GetName(),
		)
	}
}

// resolveArrayIndex resolves indexing into an array. Since arrays are stored
// inline (on the stack or within a struct), we adjust the last operation's
// offset to point directly at the element, reading only elemSize bytes.
func resolveArrayIndex(
	baseExpr ir.Expression,
	arrType *ir.ArrayType,
	idx int64,
) (ir.Expression, error) {
	if uint32(idx) >= arrType.Count {
		return ir.Expression{}, fmt.Errorf(
			"index %d out of bounds for array of length %d", idx, arrType.Count,
		)
	}

	elemType := arrType.Element
	elemSize := elemType.GetByteSize()
	elementOffset := uint32(idx) * elemSize

	operations := baseExpr.Operations
	if len(operations) == 0 {
		return ir.Expression{}, errors.New("no operations to adjust for array index")
	}

	// Adjust the last operation to read only the element.
	lastOp := operations[len(operations)-1]
	switch op := lastOp.(type) {
	case *ir.LocationOp:
		op.Offset += elementOffset
		op.ByteSize = elemSize
	case *ir.DereferenceOp:
		op.Bias += elementOffset
		op.ByteSize = elemSize
	default:
		return ir.Expression{}, fmt.Errorf(
			"unexpected last operation type %T for array index", lastOp,
		)
	}

	return ir.Expression{
		Type:       elemType,
		Operations: operations,
	}, nil
}

// resolveSliceIndex resolves indexing into a slice. The slice header has a data
// pointer at offset 0. We narrow the base operations to read only the pointer,
// then append a DereferenceOp to read the element at the computed offset.
func resolveSliceIndex(
	baseExpr ir.Expression,
	sliceType *ir.GoSliceHeaderType,
	idx int64,
	tc *typeCatalog,
) (ir.Expression, error) {
	elemType := sliceType.Data.Element
	elemSize := elemType.GetByteSize()
	elementOffset := uint32(idx) * elemSize
	ptrSize := uint32(tc.ptrSize)

	operations := baseExpr.Operations
	if len(operations) == 0 {
		return ir.Expression{}, errors.New("no operations to adjust for slice index")
	}

	// Read the data pointer and length from the slice header. We need both
	// fields (2 * ptrSize bytes) so the bounds check can validate the index
	// against the runtime length.
	lastOp := operations[len(operations)-1]
	switch op := lastOp.(type) {
	case *ir.LocationOp:
		op.ByteSize = 2 * ptrSize
	case *ir.DereferenceOp:
		op.ByteSize = 2 * ptrSize
	default:
		return ir.Expression{}, fmt.Errorf(
			"unexpected last operation type %T for slice index", lastOp,
		)
	}

	// Validate index < len at runtime. On success this is a no-op; the data
	// pointer remains at byte 0 of the scratch region for the following
	// dereference. On failure it writes ExprStatusOOB and aborts.
	operations = append(operations, &ir.SliceBoundsCheckOp{
		Index: uint32(idx),
	})

	// Dereference the data pointer and read the element.
	operations = append(operations, &ir.DereferenceOp{
		Bias:     elementOffset,
		ByteSize: elemSize,
	})

	return ir.Expression{
		Type:       elemType,
		Operations: operations,
	}, nil
}

// resolveSwissMapIndex resolves a map index expression (`m[literalKey]`)
// into IR operations that perform an O(1) hash lookup. The base expression
// evaluates to a map pointer. We dereference it to read the map header,
// then emit a SwissMapLookupOp that encodes all structural offsets and the
// literal key.
func resolveSwissMapIndex(
	baseExpr ir.Expression,
	mapType *ir.GoMapType,
	litExpr *exprlang.LiteralExpr,
	tc *typeCatalog,
) (ir.Expression, error) {
	ops, resultType, err := buildSwissMapLookup(
		baseExpr.Operations, mapType, litExpr, false, tc,
	)
	if err != nil {
		return ir.Expression{}, err
	}
	return ir.Expression{Type: resultType, Operations: ops}, nil
}

// buildSwissMapLookup emits the IR ops for a swiss-map lookup. When
// existenceOnly is true the op is the contains(map, key) variant: the
// returned result type is bool and the emitted SwissMapLookupOp carries
// ExistenceOnly=true with ValByteSize=1. When false the result type is the
// map's value element type and SwissMapLookupOp mirrors the Go `m[k]`
// semantics.
//
// `ops` is the input operation list (typically from a resolved base
// expression that produces the map pointer). In value-lookup mode it is
// appended with a DereferenceOp for the map header and then the
// SwissMapLookupOp. In existence-only mode the DereferenceOp is omitted:
// the setup opcode does the deref itself so it can convert a nil map
// pointer into a bool-false result without aborting the stack machine.
func buildSwissMapLookup(
	ops []ir.ExpressionOp,
	mapType *ir.GoMapType,
	litExpr *exprlang.LiteralExpr,
	existenceOnly bool,
	tc *typeCatalog,
) ([]ir.ExpressionOp, ir.Type, error) {
	// Unwrap GoMapType → header type.
	headerType := tc.typesByID[mapType.HeaderType.GetID()]
	swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
	if !ok {
		return nil, nil, fmt.Errorf(
			"swiss map lookup: expected GoSwissMapHeaderType, got %T", headerType,
		)
	}

	// The base expression gives us a map pointer (*Map). Dereference to read
	// the full map header. In existence-only mode the deref is null-tolerant
	// so contains(nil_map, k) → bool-false rather than aborting with
	// condition_nil_deref: the zeroed header falls through to the
	// SwissMapLookupOp handler, which sees dir_ptr == 0 and writes bool 0.
	headerSize := swissHeader.StructureType.GetByteSize()
	ops = append(ops, &ir.DereferenceOp{
		Bias:       0,
		ByteSize:   headerSize,
		NullAsZero: existenceOnly,
	})

	// Extract map header field offsets from DWARF.
	seedField, err := field(tc, swissHeader.StructureType, "seed")
	if err != nil {
		return nil, nil, fmt.Errorf("map header missing seed field: %w", err)
	}
	dirPtrField, err := field(tc, swissHeader.StructureType, "dirPtr")
	if err != nil {
		return nil, nil, fmt.Errorf("map header missing dirPtr field: %w", err)
	}
	dirLenField, err := field(tc, swissHeader.StructureType, "dirLen")
	if err != nil {
		return nil, nil, fmt.Errorf("map header missing dirLen field: %w", err)
	}
	globalShiftField, err := field(tc, swissHeader.StructureType, "globalShift")
	if err != nil {
		return nil, nil, fmt.Errorf("map header missing globalShift field: %w", err)
	}

	// Navigate to the group structure for slot layout:
	// GroupType → "ctrl" field, "slots" field → ArrayType → Element (slot struct)
	ctrlField, err := field(tc, swissHeader.GroupType, "ctrl")
	if err != nil {
		return nil, nil, fmt.Errorf("group type missing ctrl field: %w", err)
	}
	slotsField, err := field(tc, swissHeader.GroupType, "slots")
	if err != nil {
		return nil, nil, fmt.Errorf("group type missing slots field: %w", err)
	}
	slotsFieldType := tc.typesByID[slotsField.Type.GetID()]
	entryArray, ok := slotsFieldType.(*ir.ArrayType)
	if !ok {
		return nil, nil, fmt.Errorf("slots field is not an array: %T", slotsFieldType)
	}
	slotStruct, ok := entryArray.Element.(*ir.StructureType)
	if !ok {
		return nil, nil, fmt.Errorf("slot element is not a struct: %T", entryArray.Element)
	}
	keyField, err := field(tc, slotStruct, "key")
	if err != nil {
		return nil, nil, fmt.Errorf("slot struct missing key field: %w", err)
	}
	elemField, err := field(tc, slotStruct, "elem")
	if err != nil {
		return nil, nil, fmt.Errorf("slot struct missing elem field: %w", err)
	}

	// Navigate to table struct → groupsReference for data/lengthMask offsets.
	tablePtrType, ok := swissHeader.TablePtrSliceType.Element.(*ir.PointerType)
	if !ok {
		return nil, nil, fmt.Errorf("table ptr slice element is not a pointer: %T", swissHeader.TablePtrSliceType.Element)
	}
	tableType, ok := tc.typesByID[tablePtrType.Pointee.GetID()].(*ir.StructureType)
	if !ok {
		return nil, nil, fmt.Errorf("table pointee is not a struct: %T", tc.typesByID[tablePtrType.Pointee.GetID()])
	}
	groupsField, err := field(tc, tableType, "groups")
	if err != nil {
		return nil, nil, fmt.Errorf("table type missing groups field: %w", err)
	}
	groupsType, ok := groupsField.Type.(*ir.GoSwissMapGroupsType)
	if !ok {
		return nil, nil, fmt.Errorf("groups field is not GoSwissMapGroupsType: %T", groupsField.Type)
	}
	dataField, err := field(tc, groupsType.StructureType, "data")
	if err != nil {
		return nil, nil, fmt.Errorf("groupsReference missing data field: %w", err)
	}
	lengthMaskField, err := field(tc, groupsType.StructureType, "lengthMask")
	if err != nil {
		return nil, nil, fmt.Errorf("groupsReference missing lengthMask field: %w", err)
	}

	// Encode the literal key.
	keyType := tc.typesByID[keyField.Type.GetID()]
	valType := tc.typesByID[elemField.Type.GetID()]

	var keyData []byte
	var isStringKey bool
	var keyByteSize uint8

	switch kt := keyType.(type) {
	case *ir.BaseType:
		keyByteSize = uint8(kt.GetByteSize())
		goKind, _ := kt.GetGoKind()
		keyData, err = coerceLiteral(litExpr.Value, goKind, uint32(keyByteSize))
		if err != nil {
			return nil, nil, fmt.Errorf("swiss map lookup: %w", err)
		}
	case *ir.GoStringHeaderType:
		isStringKey = true
		keyByteSize = uint8(kt.StructureType.GetByteSize()) // 16 on amd64
		litStr, ok := litExpr.Value.(string)
		if !ok {
			return nil, nil, fmt.Errorf(
				"swiss map lookup: string-keyed map requires string literal, got %T",
				litExpr.Value,
			)
		}
		keyData = make([]byte, 4+len(litStr))
		binary.LittleEndian.PutUint32(keyData[:4], uint32(len(litStr)))
		copy(keyData[4:], litStr)
	default:
		return nil, nil, fmt.Errorf("swiss map lookup: unsupported key type %T", keyType)
	}

	// In existence-only mode the op writes a 1-byte bool at sm->offset.
	// Keep val_byte_size honest at 1 so the bytecode doesn't carry a
	// misleading value size (the BPF handler skips the value dereference
	// anyway, but this documents intent).
	valByteSize := valType.GetByteSize()
	if existenceOnly {
		valByteSize = 1
	}

	ops = append(ops, &ir.SwissMapLookupOp{
		KeyData:       keyData,
		IsStringKey:   isStringKey,
		ExistenceOnly: existenceOnly,
		KeyByteSize:   keyByteSize,
		ValByteSize:   valByteSize,

		SeedOffset:        uint8(seedField.Offset),
		DirPtrOffset:      uint8(dirPtrField.Offset),
		DirLenOffset:      uint8(dirLenField.Offset),
		GlobalShiftOffset: uint8(globalShiftField.Offset),

		CtrlOffset:      uint8(ctrlField.Offset),
		SlotsOffset:     uint8(slotsField.Offset),
		SlotSize:        uint16(slotStruct.GetByteSize()),
		KeyInSlotOffset: uint8(keyField.Offset),
		ValInSlotOffset: uint16(elemField.Offset),

		TableGroupsFieldOffset:   uint8(groupsField.Offset),
		GroupsDataFieldOffset:    uint8(dataField.Offset),
		GroupsLenMaskFieldOffset: uint8(lengthMaskField.Offset),

		GroupByteSize: uint16(swissHeader.GroupType.GetByteSize()),

		HeaderByteSize: headerSize,
	})

	var resultType ir.Type = valType
	if existenceOnly {
		resultType = tc.typesByID[tc.boolType]
	}
	return ops, resultType, nil
}

// resolveLenExpression resolves a len/isEmpty operand to an IR expression
// that reads the length field from the collection header.
func resolveLenExpression(
	operand exprlang.Expr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	// Resolve the operand expression.
	baseExpr, err := resolveExpression(operand, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve len operand: %w", err)
	}

	currentType := tc.typesByID[baseExpr.Type.GetID()]
	operations := baseExpr.Operations

	// GoMapType is a pointer to a map header; unwrap and dereference.
	isMap := false
	if m, ok := currentType.(*ir.GoMapType); ok {
		isMap = true
		headerType := tc.typesByID[m.HeaderType.GetID()]
		operations = append(operations, &ir.DereferenceOp{
			Bias:     0,
			ByteSize: headerType.GetByteSize(),
		})
		currentType = headerType
	}

	// If the resolved type is a pointer to a collection, dereference it.
	if _, ok := currentType.(*ir.PointerType); ok {
		pointee, err := resolvePointeeType[ir.Type](tc, currentType)
		if err != nil {
			return ir.Expression{}, fmt.Errorf("failed to resolve pointee for len: %w", err)
		}

		pointeeSize := pointee.GetByteSize()
		operations = append(operations, &ir.DereferenceOp{
			Bias:     0,
			ByteSize: pointeeSize,
		})
		currentType = tc.typesByID[pointee.GetID()]
	}

	// Look up the length field info.
	structType, fieldName, err := lenFieldInfo(currentType)
	if err != nil {
		return ir.Expression{}, err
	}

	f, err := field(tc, structType, fieldName)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("len field %q not found: %w", fieldName, err)
	}

	// Adjust the operations to read just the length field.
	// This follows the same pattern as GetMemberExpr field access.
	if len(operations) >= 1 {
		lastOp := operations[len(operations)-1]
		switch op := lastOp.(type) {
		case *ir.LocationOp:
			op.Offset += f.Offset
			op.ByteSize = f.Type.GetByteSize()
		case *ir.DereferenceOp:
			op.Bias += f.Offset
			op.ByteSize = f.Type.GetByteSize()
		}
	}

	// Map count fields have varying types across Go versions and architectures
	// (int vs uint64 etc.). Normalize to uint64 so that snapshots and
	// downstream consumers see a consistent type. Note that it's only for the
	// old hmap's that its an int and not a uint64.
	exprType := f.Type
	if isMap && tc.uint64Type != 0 {
		exprType = tc.typesByID[tc.uint64Type]
	}

	return ir.Expression{
		Type:       exprType,
		Operations: operations,
	}, nil
}

// coerceLiteral encodes a literal value into bytes matching the target type's
// kind and size. It handles coercion between JSON literal types (int64, float64,
// string, bool) and Go variable types, including string-to-numeric parsing for
// values that exceed JSON's float64 precision (>2^53) or need hex/octal/binary
// notation.
func coerceLiteral(value any, targetKind reflect.Kind, byteSize uint32) ([]byte, error) {
	litData := make([]byte, byteSize)

	// Helper to determine if a kind is unsigned integer.
	isUnsigned := func(k reflect.Kind) bool {
		switch k {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return true
		default:
			return false
		}
	}

	// Helper to determine if a kind is any integer (signed or unsigned).
	isInteger := func(k reflect.Kind) bool {
		switch k {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			return true
		default:
			return false
		}
	}

	// Helper to determine if a kind is a float.
	isFloat := func(k reflect.Kind) bool {
		return k == reflect.Float32 || k == reflect.Float64
	}

	// encodeInt encodes an int64 into litData at the given byte size.
	encodeInt := func(v int64) {
		switch byteSize {
		case 1:
			litData[0] = byte(v)
		case 2:
			binary.LittleEndian.PutUint16(litData, uint16(v))
		case 4:
			binary.LittleEndian.PutUint32(litData, uint32(v))
		case 8:
			binary.LittleEndian.PutUint64(litData, uint64(v))
		}
	}

	// encodeUint encodes a uint64 into litData at the given byte size.
	encodeUint := func(v uint64) {
		switch byteSize {
		case 1:
			litData[0] = byte(v)
		case 2:
			binary.LittleEndian.PutUint16(litData, uint16(v))
		case 4:
			binary.LittleEndian.PutUint32(litData, uint32(v))
		case 8:
			binary.LittleEndian.PutUint64(litData, v)
		}
	}

	// encodeFloat encodes a float64 into litData at the given byte size.
	encodeFloat := func(v float64) error {
		switch byteSize {
		case 4:
			binary.LittleEndian.PutUint32(litData, math.Float32bits(float32(v)))
		case 8:
			binary.LittleEndian.PutUint64(litData, math.Float64bits(v))
		default:
			return fmt.Errorf("condition: float with unsupported size %d", byteSize)
		}
		return nil
	}

	bitSize := int(byteSize * 8)

	// checkInt validates that v fits in the target integer and encodes it.
	checkInt := func(v int64) error {
		if isUnsigned(targetKind) {
			if v < 0 || (bitSize < 64 && uint64(v) >= 1<<bitSize) {
				return fmt.Errorf(
					"condition: literal %d out of range for %v (%d-bit)",
					v, targetKind, bitSize,
				)
			}
			encodeUint(uint64(v))
		} else {
			minVal := -(int64(1) << (bitSize - 1))
			maxVal := int64(1)<<(bitSize-1) - 1
			if v < minVal || v > maxVal {
				return fmt.Errorf(
					"condition: literal %d out of range for %v (%d-bit)",
					v, targetKind, bitSize,
				)
			}
			encodeInt(v)
		}
		return nil
	}

	switch v := value.(type) {
	case int64:
		switch {
		case isInteger(targetKind):
			if err := checkInt(v); err != nil {
				return nil, err
			}
		case isFloat(targetKind):
			return litData, encodeFloat(float64(v))
		default:
			return nil, fmt.Errorf(
				"condition: int64 literal incompatible with target kind %v",
				targetKind,
			)
		}

	case float64:
		switch {
		case isFloat(targetKind):
			return litData, encodeFloat(v)
		case isInteger(targetKind):
			if v != math.Trunc(v) {
				return nil, fmt.Errorf(
					"condition: non-integer float64 %v cannot be coerced to %v",
					v, targetKind,
				)
			}
			if err := checkInt(int64(v)); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf(
				"condition: float64 literal incompatible with target kind %v",
				targetKind,
			)
		}

	case bool:
		if v {
			litData[0] = 1
		}

	case string:
		// String-to-numeric coercion: parse the string as a number.
		// Base 0 gives hex (0xff), octal (0o77), binary (0b1010) for free.
		switch {
		case isUnsigned(targetKind):
			u, err := strconv.ParseUint(v, 0, bitSize)
			if err != nil {
				return nil, fmt.Errorf(
					"condition: string %q cannot be parsed as %v: %w",
					v, targetKind, err,
				)
			}
			encodeUint(u)
		case isInteger(targetKind):
			i, err := strconv.ParseInt(v, 0, bitSize)
			if err != nil {
				return nil, fmt.Errorf(
					"condition: string %q cannot be parsed as %v: %w",
					v, targetKind, err,
				)
			}
			encodeInt(i)
		case isFloat(targetKind):
			f, err := strconv.ParseFloat(v, bitSize)
			if err != nil {
				return nil, fmt.Errorf(
					"condition: string %q cannot be parsed as %v: %w",
					v, targetKind, err,
				)
			}
			return litData, encodeFloat(f)
		default:
			return nil, fmt.Errorf(
				"condition: string literal incompatible with target kind %v", targetKind,
			)
		}

	default:
		return nil, fmt.Errorf(
			"condition: unsupported literal type %T for base type comparison", value,
		)
	}

	return litData, nil
}

// coerceDurationLiteral encodes a user-provided millisecond literal
// (float64 or integer) into an 8-byte little-endian int64 nanoseconds
// value, matching what the BPF program writes for @duration.
func coerceDurationLiteral(value any) ([]byte, error) {
	var ns int64
	switch v := value.(type) {
	case float64:
		ns = int64(math.Round(v * 1e6))
	case int64:
		// Treat an integer literal as whole milliseconds.
		ns = v * 1_000_000
	default:
		return nil, fmt.Errorf(
			"@duration can only be compared to a numeric millisecond "+
				"literal (got %T)", value,
		)
	}
	out := make([]byte, 8)
	binary.LittleEndian.PutUint64(out, uint64(ns))
	return out, nil
}

// isNilComparable reports whether t can be compared against a null literal.
// All supported types have a pointer at the start of their in-memory
// representation whose zero value coincides with Go's `== nil` semantics:
//
//   - *T (PointerType): the pointer itself (8 bytes).
//   - unsafe.Pointer (VoidPointerType): 8-byte pointer.
//   - map (GoMapType): pointer to the map header (8 bytes).
//   - slice (GoSliceHeaderType): 24-byte header; the leading 8 bytes are
//     the data pointer, which matches Go's `s == nil` check.
//   - interface (GoInterfaceType / GoEmptyInterfaceType): 16-byte header;
//     the leading 8 bytes are the itab/type-descriptor pointer, which
//     matches Go's `i == nil` rule (a typed nil pointer in an interface is
//     not equal to nil because the type slot is populated).
func isNilComparable(t ir.Type) bool {
	switch t.(type) {
	case *ir.PointerType,
		*ir.VoidPointerType,
		*ir.GoMapType,
		*ir.GoSliceHeaderType,
		*ir.GoEmptyInterfaceType,
		*ir.GoInterfaceType:
		return true
	}
	return false
}

// isOrderingOp reports whether op is a strict ordering (lt/le/gt/ge) as
// opposed to equality (eq/ne). Ordering ops are rejected for null,
// boolean, and floating-point comparisons.
func isOrderingOp(op ir.CmpOp) bool {
	switch op {
	case ir.CmpLt, ir.CmpLe, ir.CmpGt, ir.CmpGe:
		return true
	}
	return false
}

// resolveComparison builds comparison ops for op (eq / ne / lt / le / gt
// / ge) between an already-resolved LHS expression and a literal RHS.
// Returns a bool-typed Expression without ConditionCheckOp.
func resolveComparison(
	op ir.CmpOp,
	lhsExpr ir.Expression,
	litExpr *exprlang.LiteralExpr,
	tc *typeCatalog,
) (ir.Expression, error) {
	lhsType := lhsExpr.Type
	ops := lhsExpr.Operations

	// Null-literal comparison: only eq/ne are meaningful. Supported for
	// pointers, maps, slices, and interfaces — for all four we compare
	// the first 8 bytes of the value against zero (see isNilComparable
	// for the layout details).
	if litExpr.Value == nil {
		if isOrderingOp(op) {
			return ir.Expression{}, fmt.Errorf(
				"%s: ordering against null is not supported",
				op,
			)
		}
		if !isNilComparable(lhsType) {
			return ir.Expression{}, fmt.Errorf(
				"%s: type %s cannot be compared to null",
				op, lhsType.GetName(),
			)
		}
		ops = append(ops, &ir.ExprPushOffsetOp{ByteSize: 8})
		ops = append(ops, &ir.ExprLoadLiteralOp{Data: make([]byte, 8)})
		ops = append(ops, &ir.ExprCmpBaseOp{
			Op:       op,
			Kind:     ir.CmpKindUint,
			ByteSize: 8,
		})
		boolType := tc.typesByID[tc.boolType]
		return ir.Expression{Type: boolType, Operations: ops}, nil
	}

	// Non-null literal against a nullable-only type: reject up front.
	if isNilComparable(lhsType) {
		return ir.Expression{}, fmt.Errorf(
			"%s: type %s can only be compared to null",
			op, lhsType.GetName(),
		)
	}

	// Check if LHS is a string type.
	if _, isString := lhsType.(*ir.GoStringHeaderType); isString {
		// String comparison (eq/ne use byte equality; ordering uses
		// lexicographic byte order — see ir.ExprCmpStringOp).
		litStr, ok := litExpr.Value.(string)
		if !ok {
			return ir.Expression{}, fmt.Errorf(
				"%s: string variable compared with non-string literal %T",
				op, litExpr.Value,
			)
		}
		if len(litStr) > ir.MaxStringLiteralLength {
			return ir.Expression{}, fmt.Errorf(
				"%s: string literal too long (%d bytes, max %d)",
				op, len(litStr), ir.MaxStringLiteralLength,
			)
		}

		// Read the string content from userspace.
		ops = append(ops, &ir.ExprReadStringOp{MaxLen: ir.MaxStringLiteralLength})

		// Load the literal as [u32 len][bytes...].
		litData := make([]byte, 4+len(litStr))
		binary.LittleEndian.PutUint32(litData[:4], uint32(len(litStr)))
		copy(litData[4:], litStr)
		ops = append(ops, &ir.ExprLoadLiteralOp{Data: litData})

		// Compare strings.
		ops = append(ops, &ir.ExprCmpStringOp{Op: op})
	} else if baseType, isBase := lhsType.(*ir.BaseType); isBase {
		// Base type comparison.
		byteSize := baseType.GetByteSize()
		if byteSize > 8 {
			return ir.Expression{}, fmt.Errorf(
				"%s: base type too large for comparison (%d bytes)",
				op, byteSize,
			)
		}

		// Determine the target Go kind for literal coercion and to
		// pick the ir.CmpKind that ExprCmpBaseOp uses for ordering.
		targetKind := reflect.Invalid
		if goKind, ok := baseType.GetGoKind(); ok {
			targetKind = goKind
		}
		cmpKind, err := cmpKindForGoKind(targetKind, op)
		if err != nil {
			return ir.Expression{}, err
		}

		// Push LHS offset and advance.
		ops = append(ops, &ir.ExprPushOffsetOp{ByteSize: uint32(byteSize)})

		// Encode the literal value with type coercion.
		litData, err := coerceLiteral(litExpr.Value, targetKind, byteSize)
		if err != nil {
			return ir.Expression{}, err
		}
		ops = append(ops, &ir.ExprLoadLiteralOp{Data: litData})

		// Compare base values.
		ops = append(ops, &ir.ExprCmpBaseOp{
			Op:       op,
			Kind:     cmpKind,
			ByteSize: uint8(byteSize),
		})
	} else if _, isDuration := lhsType.(*ir.DurationType); isDuration {
		// @duration has user-facing millisecond semantics but is stored
		// as int64 nanoseconds at BPF eval time. Convert the literal
		// from milliseconds into nanoseconds and emit an 8-byte signed
		// comparison against the value the BPF program wrote for
		// @duration.
		litData, err := coerceDurationLiteral(litExpr.Value)
		if err != nil {
			return ir.Expression{}, err
		}
		ops = append(ops, &ir.ExprPushOffsetOp{ByteSize: 8})
		ops = append(ops, &ir.ExprLoadLiteralOp{Data: litData})
		ops = append(ops, &ir.ExprCmpBaseOp{
			Op:       op,
			Kind:     ir.CmpKindInt,
			ByteSize: 8,
		})
	} else {
		return ir.Expression{}, fmt.Errorf(
			"%s: unsupported LHS type %T for comparison",
			op, lhsType,
		)
	}

	boolType := tc.typesByID[tc.boolType]
	return ir.Expression{Type: boolType, Operations: ops}, nil
}

// cmpKindForGoKind picks the ir.CmpKind that ExprCmpBaseOp will use for
// the given Go base-type kind, and rejects ordering ops on bool/float
// where signed-integer ordering doesn't apply. Floats are restricted to
// eq/ne (bitwise) for now — IEEE-754 ordering with NaN/signed-zero
// semantics is deferred. Bool ordering is similarly nonsensical.
func cmpKindForGoKind(k reflect.Kind, op ir.CmpOp) (ir.CmpKind, error) {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return ir.CmpKindInt, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return ir.CmpKindUint, nil
	case reflect.Bool:
		if isOrderingOp(op) {
			return 0, fmt.Errorf(
				"%s: ordering on bool is not supported",
				op,
			)
		}
		return ir.CmpKindUint, nil
	case reflect.Float32, reflect.Float64:
		if isOrderingOp(op) {
			return 0, fmt.Errorf(
				"%s: ordering on float types is not supported",
				op,
			)
		}
		return ir.CmpKindUint, nil
	default:
		// Unknown / unspecified kind. eq/ne fall back to bitwise unsigned
		// compare (matches the previous Eq behaviour); reject ordering
		// since the byte-by-byte ordering would not map to anything
		// meaningful at the source language level.
		if isOrderingOp(op) {
			return 0, fmt.Errorf(
				"%s: ordering not supported for base type kind %v",
				op, k,
			)
		}
		return ir.CmpKindUint, nil
	}
}

// resolveIsEmptyComparison resolves isEmpty(x) as len(x) == 0, returning a
// bool-typed Expression without ConditionCheckOp.
func resolveIsEmptyComparison(
	ie *exprlang.IsEmptyExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	lenExpr, err := resolveLenExpression(ie.Operand, rootVar, tc)
	if err != nil {
		return ir.Expression{}, fmt.Errorf("failed to resolve isEmpty operand: %w", err)
	}
	zeroLit := &exprlang.LiteralExpr{Value: int64(0)}
	return resolveComparison(ir.CmpEq, lenExpr, zeroLit, tc)
}

// resolveCondition lowers an analyzed condition tree into an IR Expression
// with a single trailing ConditionCheckOp. Compound conditions (and/or/not)
// are emitted with short-circuit jumps — see pkg/dyninst/ir/expression.go for
// the CondJumpOp/CondLabelOp/CondNotOp semantics.
func resolveCondition(
	condExpr exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	tc *typeCatalog,
) (*ir.Expression, error) {
	var la labelAllocator
	ops, err := emitCondition(condExpr, leafRoots, tc, &la)
	if err != nil {
		return nil, err
	}
	ops = append(ops, &ir.ConditionCheckOp{})
	boolType := tc.typesByID[tc.boolType]
	return &ir.Expression{Type: boolType, Operations: ops}, nil
}

// resolveSplitConditionEntry builds the entry-side IR Expression for a
// split-event-kind condition. The returned program is the entry-side
// driver:
//
//  1. ConditionStateInit clears condition_state.
//  2. For each entry leaf in iteration order, ConditionLeafEval triggers
//     a Call to the per-leaf SM sub-function followed by a record op
//     that captures the leaf's outcome (false / true / eval-error /
//     nil-deref) into condition_state[i]. Leaf-internal aborts return
//     to the driver via sm_return — they do not abort the driver.
//  3. ExprPrepare resets the scratch frame for the AST replay.
//  4. emitGate walks the AST: entry leaves lower to ConditionLeafLoad
//     (reads condition_state[i] and short-circuits surrounding ops on
//     error); pure-return subtrees prune to a polarity-correct
//     constant via ExprLoadLiteral.
//  5. Tail label, then ConditionCheckPreserveError. On false →
//     condition_failed = true, event.c skips the entry event AND
//     bypasses in_progress_calls insertion (the return event then
//     sees CALL_DEPTHS_ABSENT and is suppressed). On true with the
//     eval_error flag set by a ConditionLeafLoad → the entry event
//     fires with condition_eval_error surfaced on the header.
//
// The function also populates condition.LeafBodies — one *ir.Expression
// per entry leaf — so the compiler can emit each leaf as its own
// SM sub-function.
func resolveSplitConditionEntry(
	condExpr exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	leafEventKind map[exprlang.Expr]ir.EventKind,
	entryLeafSlotIndex map[exprlang.Expr]uint8,
	tc *typeCatalog,
) (*ir.Expression, error) {
	var la labelAllocator
	tail := la.newLabel()
	leafBodies, err := buildLeafBodies(condExpr, leafRoots, leafEventKind, entryLeafSlotIndex, tc)
	if err != nil {
		return nil, err
	}
	ops := []ir.ExpressionOp{&ir.ConditionStateInitOp{}}
	// Issue per-leaf evaluation calls in iteration order matching the
	// indices assigned by entryLeafSlotIndex. Each ConditionLeafEvalOp
	// lowers to (CallOp leafFn, ConditionLeafRecordOp) at compile time.
	leaves := conditionLeafExprs(condExpr)
	for _, leaf := range leaves {
		if leafEventKind[leaf] != ir.EventKindEntry {
			continue
		}
		ops = append(ops, &ir.ConditionLeafEvalOp{
			LeafIdx: entryLeafSlotIndex[leaf],
		})
	}
	// Reset scratch for the AST replay: per-leaf eval can leave the
	// scratch frame in arbitrary state on abort, and even on success
	// we want a clean slate before the gate writes its boolean.
	ops = append(ops, &ir.ExprPrepareOp{})
	gateOps, err := emitGate(condExpr, leafEventKind, entryLeafSlotIndex, &la, false, tail)
	if err != nil {
		return nil, err
	}
	ops = append(ops, gateOps...)
	ops = append(ops, &ir.CondLabelOp{ID: tail})
	ops = append(ops, &ir.ConditionCheckPreserveErrorOp{})
	boolType := tc.typesByID[tc.boolType]
	return &ir.Expression{
		Type:       boolType,
		Operations: ops,
		LeafBodies: leafBodies,
		IsSplit:    true,
	}, nil
}

// resolveSplitConditionReturn builds the return-side IR Expression for a
// split-event-kind condition. The returned program runs entirely as an
// AST replay over condition_state (populated at entry time and propagated
// through call_depths_delete onto the return-side SM):
//
//  1. emitReturnSplit walks the AST: entry leaves lower to
//     ConditionLeafLoad (which surfaces eval errors only when the
//     surrounding short-circuit actually reaches the leaf); return
//     leaves use the existing emitCondition machinery. The compiler
//     prepends an implicit ExprPrepareOp before this body.
//  2. Tail label, then ConditionCheckPreserveError. The check sets
//     condition_failed when the AST evaluates to false; preserves
//     condition_eval_error if any leaf surfaced an error.
//
// LeafBodies is left nil here — the entry-side driver owns leaf-body
// generation; the return-side only consumes condition_state.
func resolveSplitConditionReturn(
	condExpr exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	leafEventKind map[exprlang.Expr]ir.EventKind,
	entryLeafSlotIndex map[exprlang.Expr]uint8,
	tc *typeCatalog,
) (*ir.Expression, error) {
	var la labelAllocator
	tail := la.newLabel()
	ops, err := emitReturnSplit(condExpr, leafRoots, leafEventKind, entryLeafSlotIndex, tc, &la, tail)
	if err != nil {
		return nil, err
	}
	ops = append(ops, &ir.CondLabelOp{ID: tail})
	ops = append(ops, &ir.ConditionCheckPreserveErrorOp{})
	boolType := tc.typesByID[tc.boolType]
	return &ir.Expression{Type: boolType, Operations: ops, IsSplit: true}, nil
}

// buildLeafBodies compiles each entry-side leaf of a split-event-kind
// condition into its own *ir.Expression. The compiler turns each into a
// ProcessConditionLeaf SM sub-function. Each body's Operations leaves a
// boolean byte at sm->offset on success; on abort (nil deref / OOB) the
// existing condition error paths handle the abort and propagate
// condition_eval_error / condition_nil_deref to the driver via
// sm_return.
//
// The returned slice is indexed by leaf index (matching
// entryLeafSlotIndex values), with nil entries left for leaves that are
// not entry-side.
func buildLeafBodies(
	condExpr exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	leafEventKind map[exprlang.Expr]ir.EventKind,
	entryLeafSlotIndex map[exprlang.Expr]uint8,
	tc *typeCatalog,
) ([]*ir.Expression, error) {
	leaves := conditionLeafExprs(condExpr)
	// Determine the highest assigned leaf index so the slice is sized
	// right. Indexes are dense (0..N-1) but we don't rely on that here.
	maxIdx := -1
	for _, leaf := range leaves {
		if leafEventKind[leaf] != ir.EventKindEntry {
			continue
		}
		if int(entryLeafSlotIndex[leaf]) > maxIdx {
			maxIdx = int(entryLeafSlotIndex[leaf])
		}
	}
	if maxIdx < 0 {
		return nil, nil
	}
	bodies := make([]*ir.Expression, maxIdx+1)
	for _, leaf := range leaves {
		if leafEventKind[leaf] != ir.EventKindEntry {
			continue
		}
		var la labelAllocator
		leafOps, err := emitCondition(leaf, leafRoots, tc, &la)
		if err != nil {
			return nil, err
		}
		boolType := tc.typesByID[tc.boolType]
		bodies[entryLeafSlotIndex[leaf]] = &ir.Expression{
			Type:       boolType,
			Operations: leafOps,
		}
	}
	return bodies, nil
}

// emitGate lowers a condition tree to an entry-side gate program. Used
// by the entry-side driver after per-leaf evaluation has populated
// condition_state; the gate decides whether to short-circuit the entire
// probe firing (return event included) when the entry-only slice of the
// tree is already false.
//
// An entry-side leaf compiles to ConditionLeafLoadOp{i, ErrorTarget=tail}.
// On boolean status (false/true) it writes the bit at sm->offset and
// continues; on error status it sets condition_eval_error, writes 1 at
// sm->offset, and jumps to tail — bypassing surrounding short-circuit
// and Not ops so the eval-error flag survives to event.c surfacing.
//
// A pure-return subtree is "pruned" to a constant ExprLoadLiteralOp.
// `negated` tracks polarity through NotOps so the pruned constant flips
// back to `true` (the safe value: it cannot prove the gate false) after
// any wrapping NotOps. Without polarity tracking, !(@return == X) under
// an AND with an entry leaf would prune to true → NotOp-flip to false,
// falsely proving the gate false and incorrectly suppressing the return
// event.
func emitGate(
	e exprlang.Expr,
	leafEventKind map[exprlang.Expr]ir.EventKind,
	entryLeafSlotIndex map[exprlang.Expr]uint8,
	la *labelAllocator,
	negated bool,
	tail ir.LabelID,
) ([]ir.ExpressionOp, error) {
	if !subtreeHasEntryLeaf(e, leafEventKind) {
		// Conservative answer for an unknown-at-entry subtree.
		var data byte = 1
		if negated {
			data = 0
		}
		return []ir.ExpressionOp{
			&ir.ExprLoadLiteralOp{Data: []byte{data}},
		}, nil
	}
	switch n := e.(type) {
	case *exprlang.NotExpr:
		inner, err := emitGate(
			n.Operand, leafEventKind, entryLeafSlotIndex, la, !negated, tail,
		)
		if err != nil {
			return nil, err
		}
		return append(inner, &ir.CondNotOp{}), nil
	case *exprlang.AndExpr:
		end := la.newLabel()
		left, err := emitGate(
			n.Left, leafEventKind, entryLeafSlotIndex, la, negated, tail,
		)
		if err != nil {
			return nil, err
		}
		right, err := emitGate(
			n.Right, leafEventKind, entryLeafSlotIndex, la, negated, tail,
		)
		if err != nil {
			return nil, err
		}
		out := left
		out = append(out, &ir.CondJumpOp{Cond: false, Target: end})
		out = append(out, right...)
		out = append(out, &ir.CondLabelOp{ID: end})
		return out, nil
	case *exprlang.OrExpr:
		end := la.newLabel()
		left, err := emitGate(
			n.Left, leafEventKind, entryLeafSlotIndex, la, negated, tail,
		)
		if err != nil {
			return nil, err
		}
		right, err := emitGate(
			n.Right, leafEventKind, entryLeafSlotIndex, la, negated, tail,
		)
		if err != nil {
			return nil, err
		}
		out := left
		out = append(out, &ir.CondJumpOp{Cond: true, Target: end})
		out = append(out, right...)
		out = append(out, &ir.CondLabelOp{ID: end})
		return out, nil
	default:
		// Entry-side leaf (subtreeHasEntryLeaf is true and this node is
		// a leaf). Read condition_state[i] and either write the bit or
		// short-circuit to tail on error.
		return []ir.ExpressionOp{
			&ir.ConditionLeafLoadOp{
				LeafIdx:     entryLeafSlotIndex[e],
				ErrorTarget: tail,
			},
		}, nil
	}
}

// subtreeHasEntryLeaf returns true when at least one leaf reachable from e
// is an entry-side leaf in leafEventKind.
func subtreeHasEntryLeaf(
	e exprlang.Expr,
	leafEventKind map[exprlang.Expr]ir.EventKind,
) bool {
	switch n := e.(type) {
	case *exprlang.NotExpr:
		return subtreeHasEntryLeaf(n.Operand, leafEventKind)
	case *exprlang.AndExpr:
		return subtreeHasEntryLeaf(n.Left, leafEventKind) ||
			subtreeHasEntryLeaf(n.Right, leafEventKind)
	case *exprlang.OrExpr:
		return subtreeHasEntryLeaf(n.Left, leafEventKind) ||
			subtreeHasEntryLeaf(n.Right, leafEventKind)
	default:
		return leafEventKind[e] == ir.EventKindEntry
	}
}

// emitReturnSplit emits the body of a split-event-kind return-side
// condition program. Entry-side leaves lower to ConditionLeafLoadOp
// (reads the leaf's status from condition_state and short-circuits to
// `tail` on error); return-side leaves fall through to the existing
// emitCondition machinery (evaluated at runtime as today).
func emitReturnSplit(
	e exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	leafEventKind map[exprlang.Expr]ir.EventKind,
	entryLeafSlotIndex map[exprlang.Expr]uint8,
	tc *typeCatalog,
	la *labelAllocator,
	tail ir.LabelID,
) ([]ir.ExpressionOp, error) {
	switch n := e.(type) {
	case *exprlang.NotExpr:
		inner, err := emitReturnSplit(n.Operand, leafRoots, leafEventKind, entryLeafSlotIndex, tc, la, tail)
		if err != nil {
			return nil, err
		}
		return append(inner, &ir.CondNotOp{}), nil
	case *exprlang.AndExpr:
		end := la.newLabel()
		left, err := emitReturnSplit(n.Left, leafRoots, leafEventKind, entryLeafSlotIndex, tc, la, tail)
		if err != nil {
			return nil, err
		}
		right, err := emitReturnSplit(n.Right, leafRoots, leafEventKind, entryLeafSlotIndex, tc, la, tail)
		if err != nil {
			return nil, err
		}
		out := left
		out = append(out, &ir.CondJumpOp{Cond: false, Target: end})
		out = append(out, right...)
		out = append(out, &ir.CondLabelOp{ID: end})
		return out, nil
	case *exprlang.OrExpr:
		end := la.newLabel()
		left, err := emitReturnSplit(n.Left, leafRoots, leafEventKind, entryLeafSlotIndex, tc, la, tail)
		if err != nil {
			return nil, err
		}
		right, err := emitReturnSplit(n.Right, leafRoots, leafEventKind, entryLeafSlotIndex, tc, la, tail)
		if err != nil {
			return nil, err
		}
		out := left
		out = append(out, &ir.CondJumpOp{Cond: true, Target: end})
		out = append(out, right...)
		out = append(out, &ir.CondLabelOp{ID: end})
		return out, nil
	default:
		// Leaf. Entry-side → ConditionLeafLoadOp; return-side → existing
		// emitCondition path.
		if kind, ok := leafEventKind[e]; ok && kind == ir.EventKindEntry {
			return []ir.ExpressionOp{
				&ir.ConditionLeafLoadOp{
					LeafIdx:     entryLeafSlotIndex[e],
					ErrorTarget: tail,
				},
			}, nil
		}
		return emitCondition(e, leafRoots, tc, la)
	}
}

// labelAllocator hands out LabelIDs unique within a single condition handler.
type labelAllocator struct {
	next ir.LabelID
}

func (la *labelAllocator) newLabel() ir.LabelID {
	la.next++
	return la.next
}

// emitCondition recursively emits IR ops for a condition expression, leaving
// the resulting boolean byte at sm->offset on execution.
func emitCondition(
	e exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	switch e := e.(type) {
	case *exprlang.EqExpr:
		return emitComparisonLeaf(ir.CmpEq, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.NeExpr:
		return emitComparisonLeaf(ir.CmpNe, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.LtExpr:
		return emitComparisonLeaf(ir.CmpLt, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.LeExpr:
		return emitComparisonLeaf(ir.CmpLe, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.GtExpr:
		return emitComparisonLeaf(ir.CmpGt, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.GeExpr:
		return emitComparisonLeaf(ir.CmpGe, e.Left, e.Right, leafRoots[e], tc)
	case *exprlang.IsEmptyExpr:
		return emitIsEmptyLeaf(e, leafRoots[e], tc)
	case *exprlang.ContainsExpr:
		return emitContainsLeaf(e, leafRoots[e], tc, la)
	case *exprlang.AnyExpr:
		return emitAnyAllLoop(e.Base, e.Pred, ir.QuantifierAny, leafRoots[e], tc, la)
	case *exprlang.AllExpr:
		return emitAnyAllLoop(e.Base, e.Pred, ir.QuantifierAll, leafRoots[e], tc, la)
	case *exprlang.NotExpr:
		inner, err := emitCondition(e.Operand, leafRoots, tc, la)
		if err != nil {
			return nil, err
		}
		return append(inner, &ir.CondNotOp{}), nil
	case *exprlang.AndExpr:
		return emitShortCircuit(e.Left, e.Right, leafRoots, tc, la, false)
	case *exprlang.OrExpr:
		return emitShortCircuit(e.Left, e.Right, leafRoots, tc, la, true)
	default:
		return nil, fmt.Errorf("unsupported condition expression type: %T", e)
	}
}

// emitShortCircuit emits a binary short-circuit chain. jumpOn == false is
// AND (jump past Right when Left is false); jumpOn == true is OR.
func emitShortCircuit(
	left, right exprlang.Expr,
	leafRoots map[exprlang.Expr]*ir.Variable,
	tc *typeCatalog,
	la *labelAllocator,
	jumpOn bool,
) ([]ir.ExpressionOp, error) {
	end := la.newLabel()
	leftOps, err := emitCondition(left, leafRoots, tc, la)
	if err != nil {
		return nil, err
	}
	rightOps, err := emitCondition(right, leafRoots, tc, la)
	if err != nil {
		return nil, err
	}
	out := leftOps
	out = append(out, &ir.CondJumpOp{Cond: jumpOn, Target: end})
	out = append(out, rightOps...)
	out = append(out, &ir.CondLabelOp{ID: end})
	return out, nil
}

// emitComparisonLeaf lowers a comparison condition leaf (Eq / Ne / Lt /
// Le / Gt / Ge) into IR ops that write a single boolean byte at
// sm->offset.
func emitComparisonLeaf(
	op ir.CmpOp,
	left, right exprlang.Expr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) ([]ir.ExpressionOp, error) {
	if rootVar == nil {
		return nil, errors.New("condition leaf has no resolved root variable")
	}
	litExpr, ok := right.(*exprlang.LiteralExpr)
	if !ok {
		return nil, fmt.Errorf(
			"unsupported condition RHS type: %T (only literals are supported)",
			right,
		)
	}
	lhsExpr, err := resolveComparisonLHS(left, litExpr, rootVar, tc)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve condition LHS: %w", err)
	}
	expr, err := resolveComparison(op, lhsExpr, litExpr, tc)
	if err != nil {
		return nil, err
	}
	return expr.Operations, nil
}

// emitIsEmptyLeaf lowers an IsEmptyExpr leaf into ops that write a single
// boolean byte at sm->offset.
func emitIsEmptyLeaf(
	ie *exprlang.IsEmptyExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) ([]ir.ExpressionOp, error) {
	if rootVar == nil {
		return nil, errors.New("condition leaf has no resolved root variable")
	}
	expr, err := resolveIsEmptyComparison(ie, rootVar, tc)
	if err != nil {
		return nil, err
	}
	return expr.Operations, nil
}

// emitContainsLeaf lowers a ContainsExpr leaf into ops that leave a single
// boolean byte at sm->offset. For a map base, this is a key-presence check
// via SwissMapLookupOp.ExistenceOnly. For a slice or array base whose element
// is a comparable base type (int / uint / float / bool / string), this
// desugars to `any(coll, {@it == key})` and emits the same opcode sequence
// as the user-written any/all form.
func emitContainsLeaf(
	ce *exprlang.ContainsExpr,
	rootVar *ir.Variable,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	if rootVar == nil {
		return nil, errors.New("condition leaf has no resolved root variable")
	}
	baseExpr, err := resolveExpression(ce.Base, rootVar, tc)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve contains base: %w", err)
	}
	canonical := tc.typesByID[baseExpr.Type.GetID()]
	switch canonical.(type) {
	case *ir.GoSliceHeaderType, *ir.ArrayType:
		litExpr, ok := ce.Key.(*exprlang.LiteralExpr)
		if !ok {
			return nil, fmt.Errorf(
				"contains: key must be a literal, got %T", ce.Key,
			)
		}
		pred := &exprlang.EqExpr{
			Left:  &exprlang.RefExpr{Ref: "@it"},
			Right: litExpr,
		}
		return emitAnyAllLoop(ce.Base, pred, ir.QuantifierAny, rootVar, tc, la)
	case *ir.GoStringHeaderType:
		litExpr, ok := ce.Key.(*exprlang.LiteralExpr)
		if !ok {
			return nil, fmt.Errorf(
				"contains: key must be a literal, got %T", ce.Key,
			)
		}
		return nil, checkContainsStringBase(litExpr)
	}
	expr, err := resolveContainsExpression(ce, rootVar, tc)
	if err != nil {
		return nil, err
	}
	return expr.Operations, nil
}

// emitAnyAllLoop lowers an AnyExpr or AllExpr leaf into ops that leave a
// single boolean byte at sm->offset. Supported collection kinds are slices,
// arrays, and Go swiss-table maps; others surface an Issue. The predicate
// body may only reference @it (and @key / @value for maps) and literals.
func emitAnyAllLoop(
	base, pred exprlang.Expr,
	quantifier ir.Quantifier,
	rootVar *ir.Variable,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	if rootVar == nil {
		return nil, errors.New("any/all leaf has no resolved root variable")
	}

	// Resolve the base expression. Its trailing op produces the collection
	// descriptor at sm->offset (slice header / map header / array region).
	baseExpr, err := resolveExpression(base, rootVar, tc)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve any/all base: %w", err)
	}

	// Canonicalize the resolved base type and dispatch by collection kind.
	canonical := tc.typesByID[baseExpr.Type.GetID()]
	switch t := canonical.(type) {
	case *ir.GoSliceHeaderType:
		return emitSlicePredicateLoop(baseExpr, t, pred, quantifier, tc, la)
	case *ir.ArrayType:
		return emitArrayPredicateLoop(baseExpr, t, pred, quantifier, tc, la)
	case *ir.GoMapType:
		return emitSwissMapPredicateLoop(baseExpr, t, pred, quantifier, tc, la)
	default:
		return nil, fmt.Errorf(
			"any/all base must be a slice, array, or map; got %T (%q)",
			canonical, canonical.GetName(),
		)
	}
}

// emitSlicePredicateLoop emits the IR ops for any/all over a slice.
//
// The base expression's final op already produces the 24-byte slice header
// at sm->offset (this is the same contract resolveSliceIndex etc. use).
// We then synthesize an @it variable whose type is the slice's element
// type, route the predicate body through emitCondition with @it standing in
// as the "root variable" (via a leafRoots remap), and wrap with the
// SliceLoopBeginOp/SliceLoopEndOp pair.
func emitSlicePredicateLoop(
	baseExpr ir.Expression,
	sliceType *ir.GoSliceHeaderType,
	pred exprlang.Expr,
	quantifier ir.Quantifier,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	elemType := tc.typesByID[sliceType.Data.Element.GetID()]
	elemSize := elemType.GetByteSize()
	if elemSize == 0 {
		return nil, errors.New("any/all over a slice with zero-sized elements is not supported")
	}
	if elemSize > ir.CollectionPredicateMaxElemBytes {
		return nil, fmt.Errorf(
			"any/all over a slice with element size %d exceeds the %d-byte per-iteration scratch budget",
			elemSize, ir.CollectionPredicateMaxElemBytes,
		)
	}

	// Validate the predicate body only references @it and literals.
	// @key / @value are not valid over a slice — only over maps.
	if err := checkPredicateBodyScope(pred, false); err != nil {
		return nil, err
	}

	// Synthesize an @it variable whose Type is the element type. The
	// SliceLoopBeginOp reads the current element into a scratch slot
	// before each body invocation; the synthetic variable has
	// VariableRoleLoopIt which the compiler's EncodeLocationOp recognises
	// as "bytes are at sm->offset; emit an ExprAdvanceOffsetOp if Offset>0
	// and let the rest of the body proceed normally."
	itVar := &ir.Variable{
		Name: "@it",
		Type: elemType,
		Role: ir.VariableRoleLoopIt,
	}
	bodyOps, err := emitPredicateBody(pred, map[string]*ir.Variable{"@it": itVar}, tc, la)
	if err != nil {
		return nil, err
	}

	endLabel := la.newLabel()
	bodyLabel := la.newLabel()

	ops := make([]ir.ExpressionOp, 0, len(baseExpr.Operations)+5+len(bodyOps))
	ops = append(ops, baseExpr.Operations...)
	ops = append(ops, &ir.SliceLoopBeginOp{
		Quantifier:   quantifier,
		ElemByteSize: elemSize,
		EndLabel:     endLabel,
	})
	ops = append(ops, &ir.CondLabelOp{ID: bodyLabel})
	ops = append(ops, bodyOps...)
	ops = append(ops, &ir.SliceLoopEndOp{
		BodyLabel: bodyLabel,
	})
	ops = append(ops, &ir.CondLabelOp{ID: endLabel})
	return ops, nil
}

// checkPredicateBodyScope walks an any/all predicate body and rejects:
//   - References to anything other than @it. @key / @value are additionally
//     allowed when allowKeyValue is true (i.e. the collection is a map);
//     they are rejected for slice / array bases.
//   - Outer-scope references like `self.x` or function arguments.
//   - Nested any/all. Each loop reuses fixed scratch slots (accumulator,
//     @it), so a nested loop would overwrite the outer's state.
//   - contains() over any collection. The slice/array form desugars to
//     any(coll, {@it == key}) inside emitContainsLeaf, so allowing it here
//     would create the same nested-loop hazard as a literal any/all; the
//     map form is rejected for symmetry — it's never useful in a predicate
//     body where the only legal RefExprs are @it / @key / @value.
func checkPredicateBodyScope(pred exprlang.Expr, allowKeyValue bool) error {
	for e := range exprlang.Children(pred) {
		switch e := e.(type) {
		case *exprlang.RefExpr:
			switch e.Ref {
			case "@it":
				// always allowed
			case "@key", "@value":
				if !allowKeyValue {
					return fmt.Errorf(
						"any/all predicate body: %s is only valid for map collections",
						e.Ref,
					)
				}
			default:
				return fmt.Errorf(
					"any/all predicate body may only reference @it (and @key/@value for maps), got %q",
					e.Ref,
				)
			}
		case *exprlang.AnyExpr, *exprlang.AllExpr:
			return errors.New("nested any/all is not supported")
		case *exprlang.ContainsExpr:
			return errors.New("contains() is not supported inside an any/all predicate body")
		case *exprlang.FilterExpr:
			return errors.New("nested filter inside an any/all/filter predicate is not supported")
		}
	}
	return nil
}

// canonicalizeMapPredRefs rewrites `@key` to `@it` everywhere in pred.
// `@key` is an accepted synonym for `@it` over map collections; the rest of
// irgen routes on the canonical name.
func canonicalizeMapPredRefs(pred exprlang.Expr) exprlang.Expr {
	return exprlang.Rewrite(pred, func(e exprlang.Expr) exprlang.Expr {
		ref, ok := e.(*exprlang.RefExpr)
		if !ok {
			return nil
		}
		if ref.Ref == "@key" {
			return &exprlang.RefExpr{Ref: "@it"}
		}
		return nil
	})
}

// emitSwissMapPredicateLoop emits the IR ops for any/all over a swiss-table
// map. The base expression yields a *Map (8-byte pointer). We deref it to
// load the map header into scratch; the SwissMapLoopBeginOp then walks
// dir → table → group → slot, materialising each (key, value) entry into
// the loop's per-iteration scratch slot before evaluating the predicate body.
//
// Inside the body, `@it` refers to the current key (`@key` is an accepted
// synonym), and `@value` refers to the current value. The two share the
// same scratch slot: the key lives at offset 0, the value at the next
// 8-byte-aligned offset after the key.
func emitSwissMapPredicateLoop(
	baseExpr ir.Expression,
	mapType *ir.GoMapType,
	pred exprlang.Expr,
	quantifier ir.Quantifier,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	headerType := tc.typesByID[mapType.HeaderType.GetID()]
	swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
	if !ok {
		if _, isHMap := headerType.(*ir.GoHMapHeaderType); isHMap {
			return nil, errors.New(
				"any/all over old-style hmap not supported; only swiss maps (Go 1.24+)",
			)
		}
		return nil, fmt.Errorf(
			"any/all over map: unsupported header type %T", headerType,
		)
	}

	if err := checkPredicateBodyScope(pred, true); err != nil {
		return nil, err
	}
	// `@key` is a synonym for `@it` (the current key). Normalise to `@it`
	// before passing to emitCondition so leaf-root extraction sees a single
	// canonical name.
	pred = canonicalizeMapPredRefs(pred)

	keyType, valType, err := swissMapKeyValueTypes(swissHeader, tc)
	if err != nil {
		return nil, err
	}
	keyType = tc.typesByID[keyType.GetID()]
	valType = tc.typesByID[valType.GetID()]
	keySize := keyType.GetByteSize()
	valSize := valType.GetByteSize()
	if keySize == 0 || valSize == 0 {
		return nil, errors.New("any/all over map: zero-sized key or value not supported")
	}
	// The scratch slot holds key + 8-byte-aligned value. Reject if the
	// combined layout would exceed the per-iteration scratch budget.
	// Note: large keys/values are stored out-of-line by Go's runtime
	// (>128 bytes), so the slot's key/value field type is *K / *V (8
	// bytes). The size check fires only for in-slot data that's still
	// inexplicably large.
	valOffsetInSlot := (keySize + 7) &^ 7
	itTotal := valOffsetInSlot + valSize
	if itTotal > ir.CollectionPredicateMaxElemBytes {
		return nil, fmt.Errorf(
			"any/all over map[%s]%s: per-iteration scratch size %d exceeds the %d-byte budget",
			keyType.GetName(), valType.GetName(),
			itTotal, ir.CollectionPredicateMaxElemBytes,
		)
	}

	// Lay out the base ops: read the map header into scratch via
	// DereferenceOp (same pattern resolveSwissMapIndex uses). The
	// SwissMapLoopBeginOp then reads the header at sm->offset and walks
	// the dir.
	headerSize := swissHeader.StructureType.GetByteSize()
	ops := make([]ir.ExpressionOp, 0, len(baseExpr.Operations)+5)
	ops = append(ops, baseExpr.Operations...)
	ops = append(ops, &ir.DereferenceOp{
		Bias:       0,
		ByteSize:   headerSize,
		NullAsZero: true, // nil map → header is all zeros → loop short-circuits to empty.
	})

	// Field offsets — same as resolveSwissMapIndex.
	dirPtrField, err := field(tc, swissHeader.StructureType, "dirPtr")
	if err != nil {
		return nil, fmt.Errorf("map header missing dirPtr field: %w", err)
	}
	dirLenField, err := field(tc, swissHeader.StructureType, "dirLen")
	if err != nil {
		return nil, fmt.Errorf("map header missing dirLen field: %w", err)
	}
	ctrlField, err := field(tc, swissHeader.GroupType, "ctrl")
	if err != nil {
		return nil, fmt.Errorf("group type missing ctrl field: %w", err)
	}
	slotsField, err := field(tc, swissHeader.GroupType, "slots")
	if err != nil {
		return nil, fmt.Errorf("group type missing slots field: %w", err)
	}
	slotsFieldType := tc.typesByID[slotsField.Type.GetID()]
	entryArray, ok := slotsFieldType.(*ir.ArrayType)
	if !ok {
		return nil, fmt.Errorf("slots field is not an array: %T", slotsFieldType)
	}
	slotStruct, ok := entryArray.Element.(*ir.StructureType)
	if !ok {
		return nil, fmt.Errorf("slot element is not a struct: %T", entryArray.Element)
	}
	keyField, err := field(tc, slotStruct, "key")
	if err != nil {
		return nil, fmt.Errorf("slot struct missing key field: %w", err)
	}
	elemField, err := field(tc, slotStruct, "elem")
	if err != nil {
		return nil, fmt.Errorf("slot struct missing elem field: %w", err)
	}
	tablePtrType, ok := swissHeader.TablePtrSliceType.Element.(*ir.PointerType)
	if !ok {
		return nil, fmt.Errorf("table ptr slice element is not a pointer: %T", swissHeader.TablePtrSliceType.Element)
	}
	tableType, ok := tc.typesByID[tablePtrType.Pointee.GetID()].(*ir.StructureType)
	if !ok {
		return nil, fmt.Errorf("table pointee is not a struct: %T", tc.typesByID[tablePtrType.Pointee.GetID()])
	}
	groupsField, err := field(tc, tableType, "groups")
	if err != nil {
		return nil, fmt.Errorf("table type missing groups field: %w", err)
	}
	groupsType, ok := groupsField.Type.(*ir.GoSwissMapGroupsType)
	if !ok {
		return nil, fmt.Errorf("groups field is not GoSwissMapGroupsType: %T", groupsField.Type)
	}
	dataField, err := field(tc, groupsType.StructureType, "data")
	if err != nil {
		return nil, fmt.Errorf("groupsReference missing data field: %w", err)
	}
	lengthMaskField, err := field(tc, groupsType.StructureType, "lengthMask")
	if err != nil {
		return nil, fmt.Errorf("groupsReference missing lengthMask field: %w", err)
	}

	itVar := &ir.Variable{
		Name:           "@it",
		Type:           keyType,
		Role:           ir.VariableRoleLoopIt,
		LoopBaseOffset: 0,
	}
	valueVar := &ir.Variable{
		Name:           "@value",
		Type:           valType,
		Role:           ir.VariableRoleLoopIt,
		LoopBaseOffset: valOffsetInSlot,
	}
	bodyOps, err := emitPredicateBody(pred, map[string]*ir.Variable{
		"@it":    itVar,
		"@value": valueVar,
	}, tc, la)
	if err != nil {
		return nil, err
	}

	endLabel := la.newLabel()
	bodyLabel := la.newLabel()

	ops = append(ops, &ir.SwissMapLoopBeginOp{
		Quantifier:  quantifier,
		KeyByteSize: keySize,
		ValByteSize: valSize,
		EndLabel:    endLabel,

		DirPtrOffset:             uint8(dirPtrField.Offset),
		DirLenOffset:             uint8(dirLenField.Offset),
		CtrlOffset:               uint8(ctrlField.Offset),
		SlotsOffset:              uint8(slotsField.Offset),
		KeyInSlotOffset:          uint8(keyField.Offset),
		ValInSlotOffset:          uint16(elemField.Offset),
		SlotSize:                 uint16(slotStruct.GetByteSize()),
		GroupByteSize:            uint16(swissHeader.GroupType.GetByteSize()),
		TableGroupsFieldOffset:   uint8(groupsField.Offset),
		GroupsDataFieldOffset:    uint8(dataField.Offset),
		GroupsLenMaskFieldOffset: uint8(lengthMaskField.Offset),
	})
	ops = append(ops, &ir.CondLabelOp{ID: bodyLabel})
	ops = append(ops, bodyOps...)
	ops = append(ops, &ir.SwissMapLoopEndOp{
		BodyLabel: bodyLabel,
	})
	ops = append(ops, &ir.CondLabelOp{ID: endLabel})
	return ops, nil
}

// emitArrayPredicateLoop emits the IR ops for any/all over an array.
//
// The base expression's chain normally reads the full array contents into
// scratch; we need just the array's base pointer so the BPF loop can
// stream elements one at a time. We achieve this by replacing the trailing
// LocationOp/DereferenceOp's "read contents" with an ExprLoadAddressOp that
// produces an 8-byte pointer at sm->offset. The ArrayLoopBeginOp then
// reads that pointer and iterates via bpf_probe_read_user.
func emitArrayPredicateLoop(
	baseExpr ir.Expression,
	arrType *ir.ArrayType,
	pred exprlang.Expr,
	quantifier ir.Quantifier,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	elemType := tc.typesByID[arrType.Element.GetID()]
	elemSize := elemType.GetByteSize()
	if elemSize == 0 {
		return nil, errors.New("any/all over an array with zero-sized elements is not supported")
	}
	if elemSize > ir.CollectionPredicateMaxElemBytes {
		return nil, fmt.Errorf(
			"any/all over an array with element size %d exceeds the %d-byte per-iteration scratch budget",
			elemSize, ir.CollectionPredicateMaxElemBytes,
		)
	}
	if arrType.Count > ir.CollectionPredicateMaxIterations {
		return nil, fmt.Errorf(
			"any/all over array of length %d not supported (max %d)",
			arrType.Count, ir.CollectionPredicateMaxIterations,
		)
	}

	// @key / @value are not valid over an array — only over maps.
	if err := checkPredicateBodyScope(pred, false); err != nil {
		return nil, err
	}

	// Replace the trailing op of the base chain with an ExprLoadAddressOp
	// so the array's *address* (8 bytes) lands at sm->offset, not its
	// contents.
	if len(baseExpr.Operations) == 0 {
		return nil, errors.New("any/all over array: base expression produced no operations")
	}
	ops := make([]ir.ExpressionOp, 0, len(baseExpr.Operations)+5)
	tail := baseExpr.Operations[len(baseExpr.Operations)-1]
	ops = append(ops, baseExpr.Operations[:len(baseExpr.Operations)-1]...)
	switch op := tail.(type) {
	case *ir.LocationOp:
		ops = append(ops, &ir.ExprLoadAddressOp{
			Variable: op.Variable,
			Offset:   op.Offset,
		})
	case *ir.DereferenceOp:
		// The base produced a pointer to the array via DereferenceOp's
		// inner pointer; we need that pointer (or rather its target
		// address, which is the array's base) at sm->offset. We model
		// this as an in-place pointer load (the preceding op left the
		// pointer in scratch; we add op.Bias to it).
		//
		// In-place mode is load-bearing: ExprLoadAddressOp{Variable:nil}
		// requires an 8-byte pointer already at sm->offset. Verify the
		// upstream op produces exactly 8 bytes there so a future chain
		// shape we haven't thought of can't silently corrupt scratch.
		if len(ops) == 0 {
			return nil, errors.New(
				"any/all over array: DereferenceOp tail with no preceding op",
			)
		}
		prev := ops[len(ops)-1]
		var prevSize uint32
		switch p := prev.(type) {
		case *ir.LocationOp:
			prevSize = p.ByteSize
		case *ir.DereferenceOp:
			prevSize = p.ByteSize
		default:
			return nil, fmt.Errorf(
				"any/all over array: DereferenceOp tail preceded by unsupported op %T",
				prev,
			)
		}
		if prevSize != 8 {
			return nil, fmt.Errorf(
				"any/all over array: DereferenceOp tail preceded by op producing %d bytes (need 8)",
				prevSize,
			)
		}
		ops = append(ops, &ir.ExprLoadAddressOp{
			Variable:    nil,
			PointerBias: op.Bias,
		})
	default:
		return nil, fmt.Errorf(
			"any/all over array: unsupported trailing op %T", tail,
		)
	}

	itVar := &ir.Variable{
		Name: "@it",
		Type: elemType,
		Role: ir.VariableRoleLoopIt,
	}
	bodyOps, err := emitPredicateBody(pred, map[string]*ir.Variable{"@it": itVar}, tc, la)
	if err != nil {
		return nil, err
	}

	endLabel := la.newLabel()
	bodyLabel := la.newLabel()

	ops = append(ops, &ir.ArrayLoopBeginOp{
		Quantifier:     quantifier,
		ElemByteSize:   elemSize,
		CompileTimeLen: arrType.Count,
		EndLabel:       endLabel,
	})
	ops = append(ops, &ir.CondLabelOp{ID: bodyLabel})
	ops = append(ops, bodyOps...)
	ops = append(ops, &ir.ArrayLoopEndOp{
		BodyLabel: bodyLabel,
	})
	ops = append(ops, &ir.CondLabelOp{ID: endLabel})
	return ops, nil
}

// predicateBodyScratchBudget computes a conservative upper bound on the
// number of scratch buffer bytes a single iteration of a filter loop may
// consume. The bound is the per-iteration storage for @it plus the sum
// of worst-case writes performed by every op in the predicate body, plus
// the emit-element overhead (data-item header + element payload + 8-byte
// padding).
//
// The sum is conservative: in practice ExprPushOffsetOp / data-stack
// usage means consecutive ops can overlap, so this overestimates. For
// correctness we just need an upper bound; tighter accounting can be
// added later.
//
// Used by InitFilterSliceLoopOp / InitFilterMapLoopOp so the BPF runtime
// can scratch_buf_bounds_check the per-iteration headroom before
// reading @it.
func predicateBodyScratchBudget(bodyOps []ir.ExpressionOp, elemSize, emitPayloadSize uint32) uint32 {
	// Per-iteration starting budget: room for @it.
	budget := elemSize
	for _, op := range bodyOps {
		switch o := op.(type) {
		case *ir.ExprPushOffsetOp:
			budget += o.ByteSize
		case *ir.ExprLoadLiteralOp:
			budget += uint32(len(o.Data))
		case *ir.ExprReadStringOp:
			budget += 4 + uint32(o.MaxLen)
		case *ir.ExprCmpBaseOp:
			budget += uint32(o.ByteSize)
		case *ir.ExprCmpStringOp:
			budget++
		default:
			// Conservative fallback: assume the op writes up to 32 bytes
			// past the current offset. Most expression ops write nothing
			// or just a comparison result byte; this is a slack term that
			// covers small base-type ops we haven't enumerated.
			budget += 32
		}
	}
	// Emit overhead: 16-byte data-item header + element/pair payload + up
	// to 7 bytes of trailing alignment padding.
	budget += 16 + emitPayloadSize + 7
	return budget
}

// emitSliceFilterMarker emits the inline-pass op sequence for a
// top-level filter() expression whose source is a slice. The base
// expression's operations leave the 24-byte slice header at sm->offset;
// EmitFilterSliceMarkerOp then captures (data_ptr, len) from the
// header, leaves data_ptr at sm->offset as the wire handle, and
// enqueues a FILTER_DEFERRED chase item under a freshly-synthesized
// GoFilteredSliceDataType_N. The data type's EnqueueOps is the
// deferred filter loop body (InitFilterSliceLoopOp + predicate body +
// FilterSliceLoopStepOp); the compiler lowers it to the type's
// enqueue_pc.
func emitSliceFilterMarker(
	baseExpr ir.Expression,
	sliceType *ir.GoSliceHeaderType,
	pred exprlang.Expr,
	rootVar *ir.Variable,
	tc *typeCatalog,
) (ir.Expression, error) {
	_ = rootVar // currently unused; signature keeps the rootVar contract
	elemType := tc.typesByID[sliceType.Data.Element.GetID()]
	elemSize := elemType.GetByteSize()
	if elemSize == 0 {
		return ir.Expression{}, errors.New(
			"filter over a slice with zero-sized elements is not supported",
		)
	}
	if elemSize > ir.CollectionPredicateMaxElemBytes {
		return ir.Expression{}, fmt.Errorf(
			"filter over a slice with element size %d exceeds the %d-byte per-iteration scratch budget",
			elemSize, ir.CollectionPredicateMaxElemBytes,
		)
	}

	// Validate the predicate body only references @it / literals. The
	// same rule as any/all over slices applies — @key / @value are
	// map-only.
	if err := checkPredicateBodyScope(pred, false); err != nil {
		return ir.Expression{}, err
	}

	itVar := &ir.Variable{
		Name: "@it",
		Type: elemType,
		Role: ir.VariableRoleLoopIt,
	}
	var la labelAllocator
	bodyOps, err := emitPredicateBody(pred, map[string]*ir.Variable{"@it": itVar}, tc, &la)
	if err != nil {
		return ir.Expression{}, err
	}

	// Synthesize the per-call-site type pair. The user-visible "type"
	// field on the filter result mirrors the source collection's name
	// (e.g. `[]int`) so the wire output looks just like a regular slice
	// capture. The internal data type's name carries a "@filter" prefix
	// purely for IR-dump readability.
	sliceName := sliceType.GetName()
	dataTypeID := tc.idAlloc.next()
	handleTypeID := tc.idAlloc.next()
	dataType := &ir.GoFilteredSliceDataType{
		TypeCommon: ir.TypeCommon{
			ID:               dataTypeID,
			Name:             "@filter " + sliceName + ".data",
			DynamicSizeClass: ir.DynamicSizeFilterDeferred,
			ByteSize:         elemSize,
		},
		Element:      elemType,
		ElemByteSize: elemSize,
	}
	handleType := &ir.GoFilteredSliceType{
		TypeCommon: ir.TypeCommon{
			ID:       handleTypeID,
			Name:     sliceName,
			ByteSize: 8,
		},
		Data: dataType,
	}
	tc.typesByID[dataTypeID] = dataType
	tc.typesByID[handleTypeID] = handleType

	// Build EnqueueOps (the deferred loop body, run from the data type's
	// enqueue_pc). Labels are local to the enqueue_pc, allocated from a
	// fresh allocator below; the predicate body's labels were allocated
	// from `la` above and are already part of bodyOps.
	endLabel := la.newLabel()
	bodyLabel := la.newLabel()
	iterBudget := predicateBodyScratchBudget(bodyOps, elemSize, elemSize)
	enqueueOps := make([]ir.ExpressionOp, 0, 4+len(bodyOps))
	enqueueOps = append(enqueueOps, &ir.InitFilterSliceLoopOp{
		ElemByteSize:      elemSize,
		IterScratchBudget: iterBudget,
		EndLabel:          endLabel,
	})
	enqueueOps = append(enqueueOps, &ir.CondLabelOp{ID: bodyLabel})
	enqueueOps = append(enqueueOps, bodyOps...)
	enqueueOps = append(enqueueOps, &ir.FilterSliceLoopStepOp{
		ElemByteSize:  elemSize,
		ElementTypeID: elemType.GetID(),
		BodyLabel:     bodyLabel,
	})
	enqueueOps = append(enqueueOps, &ir.CondLabelOp{ID: endLabel})
	dataType.EnqueueOps = enqueueOps

	// Build the inline-pass op sequence: base ops (which leave the
	// slice header at sm->offset) + the marker op. The compiler's
	// trailing ExprSaveOp will copy the 8-byte handle the marker
	// leaves at sm->offset into the event-root expression slot.
	ops := make([]ir.ExpressionOp, 0, len(baseExpr.Operations)+1)
	ops = append(ops, baseExpr.Operations...)
	ops = append(ops, &ir.EmitFilterSliceMarkerOp{
		FilterDataTypeID: dataTypeID,
		ElemByteSize:     elemSize,
	})
	return ir.Expression{
		Type:       handleType,
		Operations: ops,
	}, nil
}

// emitMapFilterMarker emits the inline-pass op sequence for a top-level
// filter() expression whose source is a map. Unlike the any/all map
// path, no DereferenceOp is emitted: the marker op runs on the raw
// map[K]V pointer at sm->offset and defers the header read to the
// chase phase's enqueue_pc init. The marker does perform one small
// inline bpf_probe_read_user of the `used` field to set
// ExprStatusTruncated upfront when used > MAX_ITERATIONS.
func emitMapFilterMarker(
	baseExpr ir.Expression,
	mapType *ir.GoMapType,
	pred exprlang.Expr,
	tc *typeCatalog,
) (ir.Expression, error) {
	headerType := tc.typesByID[mapType.HeaderType.GetID()]
	swissHeader, ok := headerType.(*ir.GoSwissMapHeaderType)
	if !ok {
		if _, isHMap := headerType.(*ir.GoHMapHeaderType); isHMap {
			// Old-style hmap (pre-Go 1.24) — surface as
			// UnsupportedFeature so the probe is rejected with a
			// typed issue rather than a generic resolution error.
			// The user-facing message mirrors the equivalent
			// any/all and contains rejections elsewhere in irgen.
			return ir.Expression{}, &unsupportedFeatureError{
				message: "filter over old-style hmap not supported; only swiss maps (Go 1.24+) are supported",
			}
		}
		return ir.Expression{}, fmt.Errorf(
			"filter over map: unsupported header type %T", headerType,
		)
	}

	if err := checkPredicateBodyScope(pred, true); err != nil {
		return ir.Expression{}, err
	}
	pred = canonicalizeMapPredRefs(pred)

	keyType, valType, err := swissMapKeyValueTypes(swissHeader, tc)
	if err != nil {
		return ir.Expression{}, err
	}
	keyType = tc.typesByID[keyType.GetID()]
	valType = tc.typesByID[valType.GetID()]
	keySize := keyType.GetByteSize()
	valSize := valType.GetByteSize()
	if keySize == 0 || valSize == 0 {
		return ir.Expression{}, errors.New("filter over map: zero-sized key or value not supported")
	}
	valOffsetInPair := (keySize + 7) &^ 7
	itTotal := valOffsetInPair + valSize
	if itTotal > ir.CollectionPredicateMaxElemBytes {
		return ir.Expression{}, fmt.Errorf(
			"filter over map[%s]%s: per-iteration scratch size %d exceeds the %d-byte budget",
			keyType.GetName(), valType.GetName(),
			itTotal, ir.CollectionPredicateMaxElemBytes,
		)
	}

	// Resolve swiss-map layout offsets — same path emitSwissMapPredicateLoop walks.
	dirPtrField, err := field(tc, swissHeader.StructureType, "dirPtr")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("map header missing dirPtr field: %w", err)
	}
	dirLenField, err := field(tc, swissHeader.StructureType, "dirLen")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("map header missing dirLen field: %w", err)
	}
	usedField, err := field(tc, swissHeader.StructureType, "used")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("map header missing used field: %w", err)
	}
	ctrlField, err := field(tc, swissHeader.GroupType, "ctrl")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("group type missing ctrl field: %w", err)
	}
	slotsField, err := field(tc, swissHeader.GroupType, "slots")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("group type missing slots field: %w", err)
	}
	slotsFieldType := tc.typesByID[slotsField.Type.GetID()]
	entryArray, ok := slotsFieldType.(*ir.ArrayType)
	if !ok {
		return ir.Expression{}, fmt.Errorf("slots field is not an array: %T", slotsFieldType)
	}
	slotStruct, ok := entryArray.Element.(*ir.StructureType)
	if !ok {
		return ir.Expression{}, fmt.Errorf("slot element is not a struct: %T", entryArray.Element)
	}
	keyField, err := field(tc, slotStruct, "key")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("slot struct missing key field: %w", err)
	}
	elemField, err := field(tc, slotStruct, "elem")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("slot struct missing elem field: %w", err)
	}
	tablePtrType, ok := swissHeader.TablePtrSliceType.Element.(*ir.PointerType)
	if !ok {
		return ir.Expression{}, fmt.Errorf("table ptr slice element is not a pointer: %T", swissHeader.TablePtrSliceType.Element)
	}
	tableType, ok := tc.typesByID[tablePtrType.Pointee.GetID()].(*ir.StructureType)
	if !ok {
		return ir.Expression{}, fmt.Errorf("table pointee is not a struct: %T", tc.typesByID[tablePtrType.Pointee.GetID()])
	}
	groupsField, err := field(tc, tableType, "groups")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("table type missing groups field: %w", err)
	}
	groupsType, ok := groupsField.Type.(*ir.GoSwissMapGroupsType)
	if !ok {
		return ir.Expression{}, fmt.Errorf("groups field is not GoSwissMapGroupsType: %T", groupsField.Type)
	}
	dataField, err := field(tc, groupsType.StructureType, "data")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("groupsReference missing data field: %w", err)
	}
	lengthMaskField, err := field(tc, groupsType.StructureType, "lengthMask")
	if err != nil {
		return ir.Expression{}, fmt.Errorf("groupsReference missing lengthMask field: %w", err)
	}

	itVar := &ir.Variable{
		Name:           "@it",
		Type:           keyType,
		Role:           ir.VariableRoleLoopIt,
		LoopBaseOffset: 0,
	}
	valueVar := &ir.Variable{
		Name:           "@value",
		Type:           valType,
		Role:           ir.VariableRoleLoopIt,
		LoopBaseOffset: valOffsetInPair,
	}
	var la labelAllocator
	bodyOps, err := emitPredicateBody(pred, map[string]*ir.Variable{
		"@it":    itVar,
		"@value": valueVar,
	}, tc, &la)
	if err != nil {
		return ir.Expression{}, err
	}

	// Synthesize the per-call-site type pair. See the matching comment
	// in emitSliceFilterMarker — the user-visible "type" field mirrors
	// the source map's name (e.g. `map[string]int`).
	mapName := mapType.GetName()
	dataTypeID := tc.idAlloc.next()
	handleTypeID := tc.idAlloc.next()
	dataType := &ir.GoFilteredMapDataType{
		TypeCommon: ir.TypeCommon{
			ID:               dataTypeID,
			Name:             "@filter " + mapName + ".data",
			DynamicSizeClass: ir.DynamicSizeFilterDeferred,
			ByteSize:         itTotal,
		},
		KeyType:         keyType,
		ValueType:       valType,
		ValOffsetInPair: valOffsetInPair,
	}
	handleType := &ir.GoFilteredMapType{
		TypeCommon: ir.TypeCommon{
			ID:       handleTypeID,
			Name:     mapName,
			ByteSize: 8,
		},
		Data: dataType,
	}
	tc.typesByID[dataTypeID] = dataType
	tc.typesByID[handleTypeID] = handleType

	endLabel := la.newLabel()
	bodyLabel := la.newLabel()
	iterBudget := predicateBodyScratchBudget(bodyOps, itTotal, itTotal)
	enqueueOps := make([]ir.ExpressionOp, 0, 4+len(bodyOps))
	enqueueOps = append(enqueueOps, &ir.InitFilterMapLoopOp{
		KeyByteSize:       keySize,
		ValByteSize:       valSize,
		ValOffsetInPair:   valOffsetInPair,
		IterScratchBudget: iterBudget,
		EndLabel:          endLabel,

		DirPtrOffset:             uint8(dirPtrField.Offset),
		DirLenOffset:             uint8(dirLenField.Offset),
		CtrlOffset:               uint8(ctrlField.Offset),
		SlotsOffset:              uint8(slotsField.Offset),
		KeyInSlotOffset:          uint8(keyField.Offset),
		ValInSlotOffset:          uint16(elemField.Offset),
		SlotSize:                 uint16(slotStruct.GetByteSize()),
		GroupByteSize:            uint16(swissHeader.GroupType.GetByteSize()),
		TableGroupsFieldOffset:   uint8(groupsField.Offset),
		GroupsDataFieldOffset:    uint8(dataField.Offset),
		GroupsLenMaskFieldOffset: uint8(lengthMaskField.Offset),
	})
	enqueueOps = append(enqueueOps, &ir.CondLabelOp{ID: bodyLabel})
	enqueueOps = append(enqueueOps, bodyOps...)
	enqueueOps = append(enqueueOps, &ir.FilterMapLoopStepOp{
		KeyTypeID:   keyType.GetID(),
		ValueTypeID: valType.GetID(),
		BodyLabel:   bodyLabel,
	})
	enqueueOps = append(enqueueOps, &ir.CondLabelOp{ID: endLabel})
	dataType.EnqueueOps = enqueueOps

	// Inline ops: the base expression's resolveExpression chain finished
	// with the raw map[K]V pointer at sm->offset (no DereferenceOp). The
	// marker op then captures the pointer and enqueues the chase.
	ops := make([]ir.ExpressionOp, 0, len(baseExpr.Operations)+1)
	ops = append(ops, baseExpr.Operations...)
	ops = append(ops, &ir.EmitFilterMapMarkerOp{
		FilterDataTypeID: dataType.ID,
		SwissHeaderSize:  swissHeader.StructureType.GetByteSize(),
		UsedFieldOffset:  uint32(usedField.Offset),
	})
	return ir.Expression{
		Type:       handleType,
		Operations: ops,
	}, nil
}

// emitPredicateBody lowers an any/all predicate body to ops that leave a
// bool byte at sm->offset.
//
// The body is routed through emitCondition with each in-scope loop variable
// stood up as a real ir.Variable of role VariableRoleLoopIt. The compiler
// recognizes that role and treats LocationOps against it as no-ops (the
// bytes are at sm->offset already) or as an ExprAdvanceOffsetOp shift when
// Offset>0 (for @it.field) — also adding the variable's LoopBaseOffset so
// the map @value variable resolves to its slot within the scratch.
//
// vars maps a reference name (`@it`, `@value`) to the variable carrying
// that role for this loop. Slices/arrays pass a single-entry map keyed
// `@it`; maps pass two entries.
func emitPredicateBody(
	pred exprlang.Expr,
	vars map[string]*ir.Variable,
	tc *typeCatalog,
	la *labelAllocator,
) ([]ir.ExpressionOp, error) {
	leaves := conditionLeafExprs(pred)
	leafRoots := make(map[exprlang.Expr]*ir.Variable, len(leaves))
	for _, leaf := range leaves {
		sub, ok := conditionLeafSubExpr(leaf)
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf: cannot derive sub-expression from %T", leaf,
			)
		}
		rootName, ok := extractRootVariableName(sub)
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf: cannot derive root variable from %T", sub,
			)
		}
		root, ok := vars[rootName]
		if !ok {
			return nil, fmt.Errorf(
				"any/all predicate leaf references %q, which is not in scope", rootName,
			)
		}
		leafRoots[leaf] = root
	}
	return emitCondition(pred, leafRoots, tc, la)
}

// populateProbeEventsExpressions resolves expressions for every analyzed
// instance. analyzedProbes has one entry per instance, flattened across all
// probes. On failure, the entire probe (all instances) is removed.
func populateProbeEventsExpressions(
	probes []*ir.Probe,
	analyzedProbes []analyzedProbe,
	typeCatalog *typeCatalog,
) (successful []*ir.Probe, failed []ir.ProbeIssue) {
	// analyzedProbes is one per instance, ordered by (probe, instance).
	// Walk through, resolving expressions for each instance.
	failedProbes := make(map[*ir.Probe]ir.Issue)
	for i := range analyzedProbes {
		ap := &analyzedProbes[i]
		if ap.probe.GetKind() == ir.ProbeKindRuntimeRecovery {
			// Recovery probe's Event.Type and capture expression are
			// built by synthesizeRecoveryProbes (called by the caller
			// immediately after this function returns). Skip the
			// rcjson-driven expression resolution.
			continue
		}
		if _, alreadyFailed := failedProbes[ap.probe]; alreadyFailed {
			continue
		}
		if issue := populateInstanceExpressions(ap.instance, ap, typeCatalog); !issue.IsNone() {
			failedProbes[ap.probe] = issue
		}
	}

	for _, probe := range probes {
		if issue, failed2 := failedProbes[probe]; failed2 {
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

func populateInstanceExpressions(
	inst *ir.ProbeInstance,
	ap *analyzedProbe,
	typeCatalog *typeCatalog,
) ir.Issue {
	// Report condition analysis failures recorded during analyzeAllProbes
	// or exploreTypesForExpressions.
	if !ap.conditionIssue.IsNone() {
		return ap.conditionIssue
	}

	for _, event := range inst.Events {
		// Resolve condition. Single-event conditions go on the matching
		// event only. Split-event conditions emit two distinct programs:
		// the entry-side gate on the entry event and the return-side
		// combination on the return event.
		if cond := ap.condition; cond != nil {
			if cond.splitCondition {
				switch event.Kind {
				case ir.EventKindEntry:
					resolved, err := resolveSplitConditionEntry(
						cond.expr, cond.leafRoots, cond.leafEventKind,
						cond.entryLeafSlotIndex, typeCatalog,
					)
					if err != nil {
						var unsup *unsupportedFeatureError
						if errors.As(err, &unsup) {
							return ir.Issue{
								Kind:    ir.IssueKindUnsupportedFeature,
								Message: unsup.message,
							}
						}
						return ir.Issue{
							Kind:    ir.IssueKindConditionExpressionUnresolvable,
							Message: fmt.Sprintf("failed to resolve entry-side condition: %v", err),
						}
					}
					event.Condition = resolved
				case ir.EventKindReturn:
					resolved, err := resolveSplitConditionReturn(
						cond.expr, cond.leafRoots, cond.leafEventKind,
						cond.entryLeafSlotIndex, typeCatalog,
					)
					if err != nil {
						var unsup *unsupportedFeatureError
						if errors.As(err, &unsup) {
							return ir.Issue{
								Kind:    ir.IssueKindUnsupportedFeature,
								Message: unsup.message,
							}
						}
						return ir.Issue{
							Kind:    ir.IssueKindConditionExpressionUnresolvable,
							Message: fmt.Sprintf("failed to resolve return-side condition: %v", err),
						}
					}
					event.Condition = resolved
				}
			} else if cond.eventKind == event.Kind {
				resolved, err := resolveCondition(cond.expr, cond.leafRoots, typeCatalog)
				if err != nil {
					var unsup *unsupportedFeatureError
					if errors.As(err, &unsup) {
						return ir.Issue{
							Kind:    ir.IssueKindUnsupportedFeature,
							Message: unsup.message,
						}
					}
					return ir.Issue{
						Kind:    ir.IssueKindConditionExpressionUnresolvable,
						Message: fmt.Sprintf("failed to resolve condition: %v", err),
					}
				}
				event.Condition = resolved
			}
		}

		issue := populateEventExpressions(inst, event, ap, typeCatalog)
		if !issue.IsNone() {
			return issue
		}
	}

	// Set the instance's template from the analysis.
	inst.Template = ap.template
	return ir.Issue{}
}

func populateEventExpressions(
	inst *ir.ProbeInstance,
	event *ir.Event,
	ap *analyzedProbe,
	typeCatalog *typeCatalog,
) ir.Issue {
	id := typeCatalog.idAlloc.next()
	var expressions []*ir.RootExpression
	for _, expr := range ap.expressions {
		if expr.eventKind != event.Kind {
			continue
		}
		v := expr.rootVariable
		if v == nil {
			// Expression was already marked invalid by exploreTypesForExpressions.
			continue
		}

		// Resolve expression to IR. Filter is leaf-only and dispatched
		// to its own resolver; all other expressions go through the
		// generic resolveExpression (which itself rejects FilterExpr
		// when reached via nested recursion).
		var resolvedExpr ir.Expression
		var err error
		if filterExpr, ok := expr.expr.(*exprlang.FilterExpr); ok {
			resolvedExpr, err = resolveFilterExpression(filterExpr, v, typeCatalog)
		} else {
			resolvedExpr, err = resolveExpression(expr.expr, v, typeCatalog)
		}
		if err != nil {
			// Typed unsupported-feature errors fail the whole probe with
			// IssueKindUnsupportedFeature so test infrastructure can match
			// them via `issue:UnsupportedFeature` tags rather than the
			// probe silently loading with empty captures.
			var unsup *unsupportedFeatureError
			if errors.As(err, &unsup) {
				return ir.Issue{
					Kind:    ir.IssueKindUnsupportedFeature,
					Message: unsup.message,
				}
			}
			// For template segments, mark as invalid instead of failing probe.
			if expr.segment != nil && ap.template != nil {
				ap.template.Segments[expr.segmentIdx] = ir.InvalidSegment{
					Error: fmt.Sprintf("failed to resolve expression: %v", err),
					DSL:   expr.dsl,
				}
				continue
			}
			// For snapshot expressions (no segment), skip silently.
			continue
		}

		// Update segment with expression index after successful resolution.
		if seg := expr.segment; seg != nil {
			seg.EventKind = expr.eventKind
			seg.EventExpressionIndex = len(expressions)
		}

		name := expr.dsl
		if expr.captureExprName != "" {
			name = expr.captureExprName
		}
		expressions = append(expressions, &ir.RootExpression{
			Name:       name,
			Offset:     uint32(0),
			Kind:       expr.exprKind,
			Expression: resolvedExpr,
			DictIndex:  v.DictIndex,
			Redacted:   expr.redacted,
		})
	}
	exprStatusArraySize := uint32((ir.ExprStatusBits*len(expressions) + 7) / 8)
	byteSize := uint64(exprStatusArraySize)

	// Build dict entries for generic shape functions. Each dict entry
	// occupies 8 bytes in the event output (after expression status array,
	// before expressions). The eBPF resolves the runtime type at probe time.
	// Only emit entries for dict indices actually referenced by expressions
	// in this event.
	var dictEntries []ir.DictEntry
	if inst.Subprogram.DictRegister != nil {
		dictReg := *inst.Subprogram.DictRegister
		seenIdx := make(map[int]struct{})
		for _, e := range expressions {
			if _, seen := seenIdx[e.DictIndex]; e.DictIndex >= 0 && !seen {
				seenIdx[e.DictIndex] = struct{}{}
				dictEntries = append(dictEntries, ir.DictEntry{
					DictIndex:    e.DictIndex,
					DictRegister: dictReg,
					Offset:       uint32(byteSize),
				})
				byteSize += 8 // uint64 for resolved runtime type
			}
		}
	}

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
			Name:     fmt.Sprintf("Probe[%s]%s", inst.Subprogram.Name, eventKind),
			ByteSize: uint32(byteSize),
		},
		ExprStatusArraySize: exprStatusArraySize,
		DictEntries:         dictEntries,
		Expressions:         expressions,
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

func newProbeInstance(
	arch object.Architecture,
	probeCfg ir.ProbeDefinition,
	subprogram *ir.Subprogram,
	lineData map[ir.PCRange]lineData,
	textSection *section,
	skipReturnEvents bool,
) (*ir.ProbeInstance, ir.Issue, error) {
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

	// runtime.recovery probe: skip the normal entry/return resolution
	// and disassembly. runtime.recovery never returns normally (it
	// tail-calls into gogo), so a single entry injection point at the
	// subprogram's start is sufficient.
	if kind == ir.ProbeKindRuntimeRecovery {
		if subprogram.OutOfLinePCRanges == nil {
			return nil, ir.Issue{
				Kind:    ir.IssueKindMalformedExecutable,
				Message: "runtime.recovery has no out-of-line PC range",
			}, nil
		}
		entryPC := subprogram.OutOfLinePCRanges[0][0]
		return &ir.ProbeInstance{
			Subprogram: subprogram,
			Events: []*ir.Event{{
				Kind: ir.EventKindEntry,
				InjectionPoints: []ir.InjectionPoint{{
					PC:                  entryPC,
					Frameless:           false,
					HasAssociatedReturn: false,
					NoReturnReason:      ir.NoReturnReasonReturnsDisabled,
				}},
			}},
		}, ir.Issue{}, nil
	}
	var injectionPoints []ir.InjectionPoint
	var returnEvent *ir.Event
	if subprogram.OutOfLinePCRanges != nil {
		var issue ir.Issue
		var err error
		injectionPoints, returnEvent, issue, err = pickInjectionPoint(
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
		if !issue.IsNone() || err != nil {
			return nil, issue, err
		}
	}
	for _, inlined := range subprogram.InlinePCRanges {
		ips, _, issue, err := pickInjectionPoint(
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
		if !issue.IsNone() || err != nil {
			return nil, issue, err
		}
		injectionPoints = ips
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
			InjectionPoints: injectionPoints,
			Condition:       nil,
			Kind:            eventKind,
			SourceLine:      sourceLine,
			// Will be populated after all the types have been resolved
			// and placeholders have been filled in.
			Type: nil,
		},
	}
	if returnEvent != nil {
		events = append(events, returnEvent)
	}

	inst := &ir.ProbeInstance{
		Subprogram: subprogram,
		Events:     events,
	}
	return inst, ir.Issue{}, nil
}

// Returns a list of injection points for a given probe, as well as optional
// return event, if required.
func pickInjectionPoint(
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
	hasLineInfo := len(lines.lines) > 0
	addr := rootRanges[0][0]
	funcByteLen := rootRanges[0][1] - addr
	frameless := lines.prologueEnd == 0
	data := textSection.data.Data()
	offset := (addr - textSection.header.Addr)
	if offset+funcByteLen > uint64(len(data)) {
		return buf, nil, ir.Issue{
			Kind:    ir.IssueKindInvalidDWARF,
			Message: fmt.Sprintf("function body is too large: %d > %d", offset+funcByteLen, len(data)),
		}, nil
	}
	body := data[offset : offset+funcByteLen]
	var returnEvent *ir.Event
	switch where := where.(type) {
	case ir.FunctionWhere:
		if inlined {
			injectionPC := ranges[0][0]
			// Disassemble the function to validate injection pc, for resiliance
			// against corrupted DWARF and line tables.
			_, issue := disassembleFunction(
				arch,
				addr,
				injectionPC,
				frameless,
				body,
				false, /* collectReturnLocations */
			)
			if !issue.IsNone() {
				return buf, nil, issue, nil
			}
			buf = append(buf, ir.InjectionPoint{
				PC:                  ranges[0][0],
				Frameless:           frameless,
				HasAssociatedReturn: false,
				NoReturnReason:      ir.NoReturnReasonInlined,
			})
		} else {
			call, err := pickCallInjectionPoint(arch, addr, frameless, lines, body)
			if err != nil {
				return nil, nil, ir.Issue{}, err
			}

			// Functions without line info shouldn't have return probes.
			// Don't collect return locations if we lack line info.
			collectReturnLocations := !skipReturnEvents && hasLineInfo

			// Disassemble the function to find return locations and validate the
			// injection PC.
			returnLocations, issue := disassembleFunction(
				arch,
				addr,
				call.pc,
				call.frameless,
				body,
				collectReturnLocations,
			)
			if !issue.IsNone() {
				return buf, nil, issue, nil
			}

			var hasAssociatedReturn bool
			var noReturnReason ir.NoReturnReason
			if skipReturnEvents {
				hasAssociatedReturn = false
				noReturnReason = ir.NoReturnReasonReturnsDisabled
			} else if !hasLineInfo {
				hasAssociatedReturn = false
				noReturnReason = ir.NoReturnReasonNoBody
			} else if len(returnLocations) == 1 && returnLocations[0].PC == call.pc {
				// Add a workaround for the fact that single-instruction
				// functions would have the same entry and exit probes, but the
				// ordering between them would not be well-defined, so in this
				// extremely uncommon case the user doesn't get to see the
				// return probe. It's okay because there literally cannot be a
				// return value.
				hasAssociatedReturn = false
				noReturnReason = ir.NoReturnReasonNoBody
				returnLocations = returnLocations[:0]
			} else if len(returnLocations) > 0 {
				hasAssociatedReturn = true
			} else {
				// Disassembly didn't find any return locations (e.g. the
				// epilogue pattern wasn't recognized). Treat the same as
				// no-body: emit the entry event immediately without waiting
				// for a return that will never arrive.
				hasAssociatedReturn = false
				noReturnReason = ir.NoReturnReasonNoBody
			}

			buf = append(buf, ir.InjectionPoint{
				PC:                  call.pc,
				Frameless:           call.frameless,
				HasAssociatedReturn: hasAssociatedReturn,
				NoReturnReason:      noReturnReason,
				TopPCOffset:         call.topPCOffset,
			})
			if hasAssociatedReturn {
				returnEvent = &ir.Event{
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
		if !issue.IsNone() || err != nil {
			return buf, nil, issue, err
		}
		frameless := lines.prologueEnd == 0
		// Disassemble the function to validate injection PC.
		_, issue = disassembleFunction(
			arch,
			addr,
			injectionPC,
			frameless,
			body,
			false, /* collectReturnLocations */
		)
		if !issue.IsNone() {
			return buf, nil, issue, nil
		}
		buf = append(buf, ir.InjectionPoint{
			PC:                  injectionPC,
			Frameless:           frameless,
			HasAssociatedReturn: false,
			NoReturnReason:      ir.NoReturnReasonLineProbe,
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

type injectionPoint struct {
	frameless   bool
	pc          uint64
	topPCOffset int8
}

func pickCallInjectionPoint(arch object.Architecture, addr uint64, frameless bool, loc lineData, body []byte) (injectionPoint, error) {
	switch arch {
	case "amd64":
		pc := loc.prologueEnd
		if pc == 0 {
			pc = addr
		}
		// For non-frameless functions, the PrologueEnd marker may land on MOV
		// RSP,RBP rather than after it. This happens when the function sets up
		// a frame pointer but doesn't allocate local stack space (no SUB RSP
		// follows). Probing before MOV RSP,RBP executes means RBP still holds
		// the caller's frame pointer, causing a stack-depth mismatch between
		// entry and return probes. Advance past it so RBP is established when
		// the probe fires.
		var topPCOffset int8
		if !frameless {
			off := pc - addr
			if off < uint64(len(body)) {
				inst, err := x86asm.Decode(body[off:], 64)
				if err == nil && inst.Op == x86asm.MOV &&
					inst.Args[0] == x86asm.RBP && inst.Args[1] == x86asm.RSP {
					topPCOffset = -int8(inst.Len)
					pc += uint64(inst.Len)
				}
			}
		}
		return injectionPoint{
			frameless:   frameless,
			pc:          pc,
			topPCOffset: topPCOffset,
		}, nil
	case "arm64":
		// This is a heuristics to work around the fact that the prologue end
		// marker is not placed after the stack frame has been setup.
		//
		// Instead we recognize that the line table entry following the entry
		// marked as prologue end actually represents the end of the prologue.
		// We also track the topPCOffset to adjust the pc we report in the
		// stack trace because the line we are actually probing may correspond
		// to a different source line than the entrypoint.
		pc := loc.prologueEnd
		if pc == 0 {
			pc = addr
		}
		var topPCOffset int8
		if !frameless {
			idx := slices.IndexFunc(loc.lines, func(line line) bool {
				return line.pc == loc.prologueEnd
			})
			if idx != -1 && idx+1 < len(loc.lines) {
				nextLine := loc.lines[idx+1]
				topPCOffset = int8(pc - nextLine.pc)
				pc = nextLine.pc
			}
		}
		return injectionPoint{
			frameless:   frameless,
			pc:          pc,
			topPCOffset: topPCOffset,
		}, nil
	default:
		return injectionPoint{}, fmt.Errorf("unsupported architecture: %s", arch)
	}
}

// disassembleFunction analyzes a function body to find return locations and
// determine the correct injection point, dispatching to architecture-specific
// implementations.
func disassembleFunction(
	arch object.Architecture,
	addr uint64,
	injectionPC uint64,
	frameless bool,
	body []byte,
	collectReturnLocations bool,
) ([]ir.InjectionPoint, ir.Issue) {
	switch arch {
	case "amd64":
		return disassembleAmd64Function(addr, injectionPC, frameless, body, collectReturnLocations)
	case "arm64":
		return disassembleArm64Function(addr, injectionPC, frameless, body, collectReturnLocations)
	default:
		return nil, ir.Issue{
			Kind:    ir.IssueKindDisassemblyFailed,
			Message: fmt.Sprintf("unsupported architecture: %s", arch),
		}
	}
}

// disassembleAmd64Function implemented disassembleFunction for amd64.
func disassembleAmd64Function(
	addr uint64,
	injectionPC uint64,
	frameless bool,
	body []byte,
	collectReturnLocations bool,
) ([]ir.InjectionPoint, ir.Issue) {
	var returnLocations []ir.InjectionPoint
	var prevInst x86asm.Inst
	validInjectionPC := false
	for offset := 0; offset < len(body); {
		instruction, err := x86asm.Decode(body[offset:], 64)
		if err != nil {
			return nil, ir.Issue{
				Kind: ir.IssueKindDisassemblyFailed,
				Message: fmt.Sprintf(
					"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
					offset, addr+uint64(offset), body[offset:min(offset+15, len(body))], err,
				),
			}
		}
		if offset == int(injectionPC)-int(addr) {
			validInjectionPC = true
		}
		if !frameless &&
			instruction.Op == x86asm.POP && instruction.Args[0] == x86asm.RBP {

			// The epilogue starts at the stack adjustment if present,
			// otherwise at the POP RBP itself (functions that set up a
			// frame pointer but don't allocate local stack space).
			epilogueStart := addr + uint64(offset)
			if (prevInst.Op == x86asm.ADD || prevInst.Op == x86asm.SUB) &&
				prevInst.Args[0] == x86asm.RSP {
				epilogueStart -= uint64(prevInst.Len)
			}

			maybeRet, err := x86asm.Decode(body[offset+instruction.Len:], 64)
			if err != nil {
				offset := offset + instruction.Len
				return nil, ir.Issue{
					Kind: ir.IssueKindDisassemblyFailed,
					Message: fmt.Sprintf(
						"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
						offset, addr+uint64(offset), body[offset:min(offset+15, len(body))], err,
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
					return nil, ir.Issue{
						Kind: ir.IssueKindDisassemblyFailed,
						Message: fmt.Sprintf(
							"failed to decode x86-64 instruction: at offset %d of %#x %#x: %v",
							offset, addr+uint64(offset), body[offset:min(offset+15, len(body))], err,
						),
					}
				}
			}
			instruction = maybeRet
			if instruction.Op == x86asm.RET && collectReturnLocations {
				offset += nopLen + instruction.Len
				returnLocations = append(returnLocations, ir.InjectionPoint{
					PC:                  epilogueStart,
					Frameless:           frameless,
					HasAssociatedReturn: false,
					NoReturnReason:      ir.NoReturnReasonIsReturn,
				})
			}
		}
		if frameless && instruction.Op == x86asm.RET && collectReturnLocations {
			returnLocations = append(returnLocations, ir.InjectionPoint{
				PC:                  addr + uint64(offset),
				Frameless:           frameless,
				HasAssociatedReturn: false,
				NoReturnReason:      ir.NoReturnReasonIsReturn,
			})
		}
		offset += instruction.Len
		prevInst = instruction
	}
	if !validInjectionPC {
		return nil, ir.Issue{
			Kind:    ir.IssueKindDisassemblyFailed,
			Message: fmt.Sprintf("injection PC not on any instruction boundary: %#x", injectionPC),
		}
	}
	return returnLocations, ir.Issue{}
}

const Arm64InstructionByteLength = 4

// disassembleArm64Function implements disassembleFunction for arm64.
func disassembleArm64Function(
	addr uint64,
	injectionPC uint64,
	frameless bool,
	body []byte,
	collectReturnLocations bool,
) ([]ir.InjectionPoint, ir.Issue) {
	var returnLocations []ir.InjectionPoint
	const instLen = 4
	validInjectionPC := false
	if len(body)%4 != 0 {
		return nil, ir.Issue{
			Kind:    ir.IssueKindDisassemblyFailed,
			Message: fmt.Sprintf("function body %d is not aligned to 4 bytes at %#x", len(body), addr),
		}
	}
	for offset := 0; offset < len(body); {
		instBytes := body[offset : offset+4]
		instruction, err := arm64asm.Decode(instBytes)
		if offset == int(injectionPC)-int(addr) {
			validInjectionPC = true
		}
		if err != nil {
			// Skip instructions we can't decode. The arm64asm package doesn't
			// support all instructions (e.g. arm LSE atomics)
			// Since we only care about the epilouge, unknown instructions
			// or padding are skipped. Every instruction is exactly 4 bytes
			// so we can do this safely, unlike x86.
			offset += Arm64InstructionByteLength
			continue
		}
		if instruction.Op == arm64asm.RET {
			retPC := addr + uint64(offset)
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
			if collectReturnLocations {
				returnLocations = append(returnLocations, ir.InjectionPoint{
					PC:                  retPC,
					Frameless:           frameless,
					HasAssociatedReturn: false,
				})
			}
		}
		offset += 4 // Each instruction is 4 bytes long
	}
	if !validInjectionPC {
		return nil, ir.Issue{
			Kind:    ir.IssueKindDisassemblyFailed,
			Message: fmt.Sprintf("injection PC not on any instruction boundary: %#x", injectionPC),
		}
	}
	return returnLocations, ir.Issue{}
}

func collectLineDataForRange(
	lineReader *dwarf.LineReader, r ir.PCRange,
) lineData {
	var lineEntry dwarf.LineEntry
	// Save position before seeking. Line tables are state-machine encoded and
	// only support efficient forward iteration; seeking backward requires
	// restarting from the beginning. By saving our position (which is already
	// in the correct compile unit), we can restore it cheaply if SeekPC fails.
	prevPos := lineReader.Tell()
	// SeekPC finds the line table entry covering a given PC, but we need the
	// first entry whose address is >= r[0], which may be different. SeekPC
	// also fails for PCs in "holes" - addresses not covered by any line table
	// sequence.
	//
	// DWARF line tables consist of sequences, each covering a contiguous PC
	// range. A sequence is a state-machine-encoded log mapping PCs to source
	// locations (file, line, column). Holes exist between sequences or at
	// their boundaries.
	//
	// TODO: Find a way to seek to the first entry in a range rather than just
	// the entry that covers this PC. See https://github.com/golang/go/issues/73996.
	err := lineReader.SeekPC(r[0], &lineEntry)
	// Workaround for holes: SeekPC fails with ErrUnknownPC when the function's
	// start PC falls in a gap between line table sequences (common for functions
	// at sequence boundaries). After failure, SeekPC leaves the reader past the
	// first entry of the next sequence (see Go's debug/dwarf SeekPC impl). We
	// try two recovery strategies:
	//
	// 1. Read the next entry (which SeekPC left us near) and use
	//    SeekPC(addr-1) to land on the first entry of that sequence.
	// 2. If that fails, do a full scan from the beginning of the CU's line
	//    table. Sequences may not be in PC order, so we must scan all entries
	//    without breaking early on out-of-range addresses.
	if errors.Is(err, dwarf.ErrUnknownPC) {
		nextErr := lineReader.Next(&lineEntry)
		if nextErr == nil && lineEntry.Address > r[0] && lineEntry.Address < r[1] {
			lineReader.Seek(prevPos)
			nextErr = lineReader.SeekPC(lineEntry.Address-1, &lineEntry)
			if nextErr == nil && lineEntry.Address >= r[0] {
				err = nil
			}
		}
		if err != nil {
			// Full scan: sequences in the line table may not be in PC order,
			// so we scan all entries to find one in [r[0], r[1]).
			lineReader.Reset()
			for {
				nextErr := lineReader.Next(&lineEntry)
				if nextErr != nil {
					break
				}
				if lineEntry.EndSequence {
					continue
				}
				if lineEntry.Address >= r[0] && lineEntry.Address < r[1] {
					err = nil
					break
				}
			}
		}
	}
	if err != nil {
		// Functions without DWARF line information (compiler-generated stubs,
		// assembly wrappers, functions at sequence boundaries with no coverage)
		// are acceptable - they just won't have line info or return probes.
		// Restore the reader to prevPos for efficient forward seeking.
		lineReader.Seek(prevPos)
		if errors.Is(err, dwarf.ErrUnknownPC) {
			return lineData{}
		}
		// Other errors are genuine problems
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
		DictIndex: -1, // no dict resolution by default
	}, nil
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
// genericPattern represents a probe target with [...] wildcards that should
// match any generic type parameter instantiation in DWARF.
type genericPattern struct {
	// segments are the literal parts of the pattern split on "[...]".
	// For "A[...]B[...]C" the segments are ["A", "B", "C"].
	segments []string
	probes   []ir.ProbeDefinition
}

type interests struct {
	compileUnits    map[string]struct{}
	subprograms     map[string][]ir.ProbeDefinition
	genericPatterns []genericPattern // probes targeting pkg.Type[...].Method
}

func makeInterests(cfg []ir.ProbeDefinition) (interests, []ir.ProbeIssue) {
	i := interests{
		compileUnits: make(map[string]struct{}),
		subprograms:  make(map[string][]ir.ProbeDefinition),
	}
	var issues []ir.ProbeIssue
	for _, probe := range cfg {
		switch probe.GetKind() {
		case ir.ProbeKindSnapshot, ir.ProbeKindCaptureExpression:
		case ir.ProbeKindLog:
		case ir.ProbeKindRuntimeRecovery:
		default:
			issues = append(issues, ir.ProbeIssue{
				ProbeDefinition: probe,
				Issue: ir.Issue{
					Kind:    ir.IssueKindUnsupportedFeature,
					Message: fmt.Sprintf("probe kind %v is not supported", probe.GetKind()),
				},
			})
			continue
		}
		switch where := probe.GetWhere().(type) {
		case ir.FunctionWhere:
			methodName := where.Location()
			i.compileUnits[compileUnitFromName(methodName)] = struct{}{}
			i.addProbe(methodName, probe)
		case ir.LineWhere:
			methodName, _, _ := where.Line()
			i.compileUnits[compileUnitFromName(methodName)] = struct{}{}
			i.addProbe(methodName, probe)
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

// addProbe routes a probe to either the exact-match subprograms map or the
// generic patterns list, depending on whether the name contains "[...]".
func (i *interests) addProbe(methodName string, probe ir.ProbeDefinition) {
	if !strings.Contains(methodName, "[...]") {
		i.subprograms[methodName] = append(i.subprograms[methodName], probe)
		return
	}
	// Split on every "[...]" to get the literal segments.
	segments := strings.Split(methodName, "[...]")

	// Check if this pattern already exists.
	for j := range i.genericPatterns {
		if slices.Equal(i.genericPatterns[j].segments, segments) {
			i.genericPatterns[j].probes = append(i.genericPatterns[j].probes, probe)
			return
		}
	}
	i.genericPatterns = append(i.genericPatterns, genericPattern{
		segments: segments,
		probes:   []ir.ProbeDefinition{probe},
	})
}

// matchGenericPatterns checks if a DWARF subprogram name matches any generic
// pattern. Each "[...]" in the pattern matches a balanced bracket group
// "[<anything>]" in the name. Multiple "[...]" segments are supported.
// Returns the matching probes, or nil.
func (i *interests) matchGenericPatterns(name string) []ir.ProbeDefinition {
	for _, pat := range i.genericPatterns {
		if matchGenericSegments(name, pat.segments) {
			return pat.probes
		}
	}
	return nil
}

// matchGenericSegments checks whether name matches the pattern defined by
// segments. segments[0] must be a literal prefix; each subsequent segment
// must appear after consuming one balanced bracket group "[…]".
func matchGenericSegments(name string, segments []string) bool {
	if !strings.HasPrefix(name, segments[0]) {
		return false
	}
	rest := name[len(segments[0]):]
	for _, seg := range segments[1:] {
		// Expect a '[' at the current position.
		if len(rest) == 0 || rest[0] != '[' {
			return false
		}
		bracketEnd := gosymname.MatchBracket(rest, 0)
		if bracketEnd < 2 { // must contain at least one char between brackets
			return false
		}
		rest = rest[bracketEnd+1:]
		if !strings.HasPrefix(rest, seg) {
			return false
		}
		rest = rest[len(seg):]
	}
	return len(rest) == 0
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
