// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// tailCallGroupsModifier makes tail calls work between fentry/fexit programs.
//
// Since kernel commit 28ead3eaabc1 ("bpf: Prevent tail call between progs attached to different hooks", v6.12,
// backported to 6.1.135, 6.6.57 and 6.11.4), all the tracing programs that use a program array, either because they
// tail call through it or because they are inserted in it, must be attached to functions sharing the same BTF
// prototype (the kernel compares the attach_func_proto pointers). Since kernel commit 4540aed51b12, they must also
// share the same attach type.
//
// The callers of each program array are thus split into groups sharing the same attach type and prototype. One group
// uses the original program arrays and tail call targets, retargeted to one of the functions of the group. Each other
// group gets its own copy of the program arrays, and its own copy of the tail call targets it can reach, loaded
// against one of the functions of the group. Tail call targets are never attached, so the function they are loaded
// against doesn't matter as long as it shares the prototype of the callers.
//
// Some callers are cloned at runtime and attached to other functions (on-demand hook points). Each of these runtime
// clones uses the original program arrays if its prototype matches the one of the original group, or gets its own
// copy of the program arrays and of the tail call targets it can reach otherwise, see specEditor.
type tailCallGroupsModifier struct {
	sync.Mutex

	// runtimeCallerNames are the callers that can be cloned at runtime and attached to other functions
	runtimeCallerNames []string
	kernelTypes        func(*btf.Cache) (*btf.Spec, error)
	// btfCache is shared by the planning and the loading of the copies of the tail call targets during an init
	btfCache *btf.Cache

	routes       []manager.TailCallRoute
	verifierOpts lib.ProgramOptions
	// clonedGroups are the groups whose tail call targets will be loaded after the init of the manager
	clonedGroups []*tailCallGroup
	// originalKey is the key of the group using the original program arrays and tail call targets
	originalKey tailCallGroupKey
	// runtimeCallers describes the program arrays and tail call targets reachable from each runtime caller
	runtimeCallers map[string]*runtimeTailCallCaller
	// runtimeGroups are the groups created for the runtime clones that can't use the original program arrays
	runtimeGroups map[runtimeTailCallGroupKey]*tailCallGroup

	// clonedMaps and clonedProgs are the copies of the program arrays and of the tail call targets, indexed by a name
	// unique to their group
	clonedMaps  map[string]*lib.Map
	clonedProgs map[string]*lib.Program

	// registered is set once the instruction patcher has been added to the manager, which is reused across inits
	registered bool
	// pending is set until the instruction patcher runs for the current init
	pending bool
}

var (
	_ ddebpf.ModifierBeforeInit = &tailCallGroupsModifier{}
	_ ddebpf.ModifierAfterInit  = &tailCallGroupsModifier{}
	_ ddebpf.ModifierAfterStop  = &tailCallGroupsModifier{}
)

// tailCallGroupKey identifies the programs that can share a program array
type tailCallGroupKey struct {
	attachType lib.AttachType
	// protoID is the BTF ID of the prototype of the hooked function in the kernel BTF, 0 if it couldn't be resolved
	protoID btf.TypeID
	// attachTo is only set when the prototype couldn't be resolved, in which case the group is limited to a function
	attachTo string
}

type tailCallGroup struct {
	key      tailCallGroupKey
	attachTo string
	// runtimeCaller is only set for the groups created for a runtime clone
	runtimeCaller string
	callers       []string
	// arrays are the program arrays used by the programs of the group
	arrays []string
	// routes are the tail call routes reachable from the callers of the group
	routes []manager.TailCallRoute
	// targets are the tail call targets reachable from the callers of the group
	targets []string

	// clonedArrays and clonedTargets are only set for groups that don't use the original arrays and targets
	clonedArrays  map[string]*lib.Map
	clonedTargets map[string]*lib.ProgramSpec
}

func (g *tailCallGroup) String() string {
	if g.runtimeCaller != "" {
		return fmt.Sprintf("%s/%s/%s", attachTypePrefix(g.key.attachType), g.attachTo, g.runtimeCaller)
	}
	return fmt.Sprintf("%s/%s", attachTypePrefix(g.key.attachType), g.attachTo)
}

// objectName returns the name of the copy of a program array or of a tail call target for the group
func (g *tailCallGroup) objectName(name string) string {
	return name + "@" + g.String()
}

// runtimeTailCallCaller describes what a caller cloned at runtime needs to tail call into its targets
type runtimeTailCallCaller struct {
	attachType lib.AttachType
	// reachable holds the routes, targets and program arrays reachable from the caller
	reachable *tailCallGroup
	// refs are the program arrays referenced by the caller and its targets
	refs map[string][]string
	// mapSpecs are the specs of the program arrays reachable from the caller
	mapSpecs map[string]*lib.MapSpec
	// targetSpecs are the specs of the tail call targets reachable from the caller, they are associated to the maps of
	// the manager, except the program arrays, once it is initialized
	targetSpecs map[string]*lib.ProgramSpec
	// initArrays are the copies of the program arrays the caller was associated to at init, indexed by their original
	// name. Once associated to a map, an instruction loses its map reference, the runtime clones of the caller are
	// matched to their program arrays through these maps.
	initArrays map[*lib.Map]string
	// originalArrays are the program arrays of the manager referenced by the caller
	originalArrays map[string]*lib.Map
}

type runtimeTailCallGroupKey struct {
	tailCallGroupKey
	caller string
}

func newTailCallGroupsModifier(runtimeCallerNames ...string) *tailCallGroupsModifier {
	t := &tailCallGroupsModifier{
		runtimeCallerNames: runtimeCallerNames,
		kernelTypes:        (*btf.Cache).Kernel,
	}
	t.reset()
	return t
}

func (t *tailCallGroupsModifier) String() string {
	return "TailCallGroupsModifier"
}

// BeforeInit registers the instruction patcher splitting the program arrays per group of callers. It has to run
// after the constants have been edited, so that the copied programs inherit them.
func (t *tailCallGroupsModifier) BeforeInit(m *manager.Manager, _ names.ModuleName, opts *manager.Options) error {
	t.Lock()
	defer t.Unlock()

	t.reset()
	t.btfCache = btf.NewCache()
	t.routes = opts.TailCallRouter
	t.verifierOpts = opts.VerifierOptions.Programs
	t.pending = true
	if !t.registered {
		m.InstructionPatchers = append(m.InstructionPatchers, t.patch)
		t.registered = true
	}
	return nil
}

// AfterInit loads the copies of the tail call targets and inserts them in the copies of the program arrays
func (t *tailCallGroupsModifier) AfterInit(m *manager.Manager, _ names.ModuleName, _ *manager.Options) error {
	t.Lock()
	defer t.Unlock()

	// the kernel BTF is only kept in memory during the init
	defer func() { t.btfCache = nil }()

	if len(t.clonedGroups) == 0 && len(t.runtimeCallers) == 0 {
		return nil
	}

	collectionMaps, err := m.GetMaps()
	if err != nil {
		t.reset()
		return err
	}

	for name, caller := range t.runtimeCallers {
		// the program arrays are associated when the targets are copied for a group
		maps := make(map[string]*lib.Map, len(collectionMaps))
		for mapName, m := range collectionMaps {
			if _, ok := caller.mapSpecs[mapName]; !ok {
				maps[mapName] = m
			}
		}
		for target, spec := range caller.targetSpecs {
			if err := associateCollectionMaps(spec, maps); err != nil {
				t.reset()
				return fmt.Errorf("failed to associate maps to %s for runtime caller %s: %w", target, name, err)
			}
		}

		for _, ref := range caller.refs[name] {
			arr, ok := collectionMaps[ref]
			if !ok {
				t.reset()
				return fmt.Errorf("program array %s of runtime caller %s not found", ref, name)
			}
			caller.originalArrays[ref] = arr
		}
	}

	if len(t.clonedGroups) == 0 {
		return nil
	}

	start := time.Now()
	if err := t.loadClonedTargets(t.clonedGroups, collectionMaps, t.btfCache); err != nil {
		t.reset()
		return err
	}
	seclog.Infof("loaded %d copied tail call targets in %s", len(t.clonedProgs), time.Since(start))
	t.clonedGroups = nil

	return nil
}

// AfterStop releases the copies of the program arrays and of the tail call targets
func (t *tailCallGroupsModifier) AfterStop(_ *manager.Manager, _ names.ModuleName, _ manager.MapCleanupType) error {
	t.Lock()
	defer t.Unlock()

	t.reset()
	return nil
}

func (t *tailCallGroupsModifier) reset() {
	ddebpf.RemoveNameMappingsCollection(t.clonedCollection())
	for _, prog := range t.clonedProgs {
		_ = prog.Close()
	}
	for _, m := range t.clonedMaps {
		_ = m.Close()
	}
	t.clonedProgs = make(map[string]*lib.Program)
	t.clonedMaps = make(map[string]*lib.Map)
	t.clonedGroups = nil
	t.btfCache = nil
	t.originalKey = tailCallGroupKey{}
	t.runtimeCallers = nil
	t.runtimeGroups = nil
}

// clonedCollection returns the copies of the program arrays and of the tail call targets
func (t *tailCallGroupsModifier) clonedCollection() *lib.Collection {
	return &lib.Collection{Programs: t.clonedProgs, Maps: t.clonedMaps}
}

// addNameMappings registers the copies of the program arrays and of the tail call targets in the eBPF telemetry
func (t *tailCallGroupsModifier) addNameMappings(module string) {
	t.Lock()
	defer t.Unlock()

	ddebpf.AddNameMappingsCollection(t.clonedCollection(), module)
}

func attachTypePrefix(attachType lib.AttachType) string {
	switch attachType {
	case lib.AttachTraceFEntry:
		return "fentry"
	case lib.AttachTraceFExit:
		return "fexit"
	case lib.AttachTraceRawTp:
		return "tp_btf"
	default:
		return attachType.String()
	}
}

// retarget makes a tail call target loadable in the program arrays of the provided group
func retarget(spec *lib.ProgramSpec, group *tailCallGroup) {
	spec.AttachType = group.key.attachType
	spec.AttachTo = group.attachTo
	spec.SectionName = attachTypePrefix(group.key.attachType) + "/" + group.attachTo
}

// programArrayRefs returns the program arrays referenced by the instructions of a program
func programArrayRefs(spec *lib.ProgramSpec, programArrays map[string]bool) []string {
	var refs []string
	for _, ins := range spec.Instructions {
		if !ins.IsLoadFromMap() {
			continue
		}
		if ref := ins.Reference(); programArrays[ref] && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
	}
	return refs
}

func (t *tailCallGroupsModifier) patch(m *manager.Manager) error {
	if !t.pending {
		return nil
	}
	t.pending = false

	start := time.Now()

	progSpecs, err := m.GetProgramSpecs()
	if err != nil {
		return err
	}
	mapSpecs, err := m.GetMapSpecs()
	if err != nil {
		return err
	}

	hookFuncs := make(map[string]bool)
	for _, probe := range m.GetProbes() {
		hookFuncs[probe.EBPFFuncName] = true
	}

	plan := planTailCallGroups(progSpecs, mapSpecs, t.routes, probes.AllFentryTailCallRouteCallers(), hookFuncs, t.runtimeCallerNames, func() (*btf.Spec, error) {
		return t.kernelTypes(t.btfCache)
	})
	if plan == nil {
		return nil
	}

	for name := range plan.targets {
		retarget(progSpecs[name], plan.original)
	}

	t.Lock()
	defer t.Unlock()

	t.originalKey = plan.original.key

	var clonedTargetsCount int
	initArrays := make(map[string]map[*lib.Map]string)
	for _, group := range plan.groups {
		if group == plan.original {
			continue
		}

		if err := t.cloneGroup(group, progSpecs, mapSpecs, plan.refs); err != nil {
			t.reset()
			return err
		}
		for _, name := range group.callers {
			if err := associateProgramArrays(progSpecs[name], plan.refs[name], group.clonedArrays); err != nil {
				t.reset()
				return fmt.Errorf("failed to associate program arrays to %s for tail call group %s: %w", name, group, err)
			}
		}
		t.clonedGroups = append(t.clonedGroups, group)
		clonedTargetsCount += len(group.clonedTargets)

		for _, name := range group.callers {
			if _, ok := plan.runtimeCallers[name]; ok {
				initArrays[name] = make(map[*lib.Map]string, len(group.clonedArrays))
				for array, arr := range group.clonedArrays {
					initArrays[name][arr] = array
				}
			}
		}
	}

	// keep a copy of what the runtime callers can reach, so that their clones can get their own program arrays and
	// tail call targets if they are attached to a function with another prototype
	t.runtimeCallers = make(map[string]*runtimeTailCallCaller, len(plan.runtimeCallers))
	for name, reachable := range plan.runtimeCallers {
		caller := &runtimeTailCallCaller{
			attachType:     progSpecs[name].AttachType,
			reachable:      reachable,
			refs:           map[string][]string{name: plan.refs[name]},
			mapSpecs:       make(map[string]*lib.MapSpec, len(reachable.arrays)),
			targetSpecs:    make(map[string]*lib.ProgramSpec, len(reachable.targets)),
			initArrays:     initArrays[name],
			originalArrays: make(map[string]*lib.Map, len(plan.refs[name])),
		}
		for _, array := range reachable.arrays {
			caller.mapSpecs[array] = mapSpecs[array].Copy()
		}
		for _, target := range reachable.targets {
			caller.refs[target] = plan.refs[target]
			caller.targetSpecs[target] = progSpecs[target].Copy()
		}
		t.runtimeCallers[name] = caller
	}

	seclog.Infof("tail calls between tracing programs split in %d groups (%d copied tail call targets) in %s", len(plan.groups), clonedTargetsCount, time.Since(start))
	for _, group := range plan.groups {
		seclog.Debugf("tail call group %s: callers=%v targets=%v", group, group.callers, group.targets)
	}

	return nil
}

// tailCallPlan describes how the program arrays used by tracing programs are split
type tailCallPlan struct {
	groups []*tailCallGroup
	// original is the group using the original program arrays and tail call targets
	original *tailCallGroup
	// refs are the program arrays referenced by each program
	refs map[string][]string
	// targets are the tracing programs using program arrays that aren't hook points, they are either inserted in
	// program arrays or not used in the current configuration
	targets map[string]bool
	// runtimeCallers holds the routes, targets and program arrays reachable from each caller cloned at runtime
	runtimeCallers map[string]*tailCallGroup
}

// planTailCallGroups splits the hook points using program arrays in groups of programs that can share them. It
// returns nil if no tracing program uses a program array.
func planTailCallGroups(progSpecs map[string]*lib.ProgramSpec, mapSpecs map[string]*lib.MapSpec, allRoutes []manager.TailCallRoute, routeCallers []probes.TailCallRouteCallers, hookFuncs map[string]bool, runtimeCallerNames []string, loadKernelSpec func() (*btf.Spec, error)) *tailCallPlan {
	programArrays := make(map[string]bool)
	for name, spec := range mapSpecs {
		if spec.Type == lib.ProgramArray {
			programArrays[name] = true
		}
	}

	plan := &tailCallPlan{
		refs:           make(map[string][]string),
		targets:        make(map[string]bool),
		runtimeCallers: make(map[string]*tailCallGroup),
	}

	tracingArrays := make(map[string]bool)
	for name, spec := range progSpecs {
		plan.refs[name] = programArrayRefs(spec, programArrays)
		if spec.Type == lib.Tracing {
			for _, ref := range plan.refs[name] {
				tracingArrays[ref] = true
			}
		}
	}
	if len(tracingArrays) == 0 {
		return nil
	}

	// select the routes towards tracing programs, the other ones are not affected by the attach prototype check
	var routes []manager.TailCallRoute
	for _, route := range allRoutes {
		if !tracingArrays[route.ProgArrayName] || route.Program != nil {
			continue
		}
		spec, ok := progSpecs[route.ProbeIdentificationPair.EBPFFuncName]
		if !ok || spec.Type != lib.Tracing {
			continue
		}
		routes = append(routes, route)
		plan.targets[route.ProbeIdentificationPair.EBPFFuncName] = true
	}

	// the tracing programs that aren't hook points can't start a chain of tail calls. They still have to be loaded
	// against the prototype of the original program arrays, even when they have no route in the current configuration.
	for name, spec := range progSpecs {
		if spec.Type == lib.Tracing && !hookFuncs[name] && slices.ContainsFunc(plan.refs[name], func(ref string) bool { return tracingArrays[ref] }) {
			plan.targets[name] = true
		}
	}

	plan.groups = groupTailCallCallers(progSpecs, plan.refs, tracingArrays, plan.targets, loadKernelSpec)
	if len(plan.groups) == 0 {
		return nil
	}

	callers := make(map[manager.TailCallRoute][]string)
	for _, rc := range routeCallers {
		for _, route := range routes {
			if route.ProgArrayName == rc.ProgArrayName && route.Key == rc.Key {
				callers[route] = rc.Callers
			}
		}
	}
	for _, group := range plan.groups {
		group.resolveRoutes(routes, plan.refs, callers)
	}

	for _, name := range runtimeCallerNames {
		if spec, ok := progSpecs[name]; !ok || spec.Type != lib.Tracing || !slices.ContainsFunc(plan.refs[name], func(ref string) bool { return tracingArrays[ref] }) {
			continue
		}
		reachable := &tailCallGroup{callers: []string{name}}
		reachable.resolveRoutes(routes, plan.refs, callers)
		plan.runtimeCallers[name] = reachable
	}

	// the group reaching the most targets uses the original program arrays and targets, which avoids loading copies
	// for it. The original targets that this group can't reach are retargeted too, so that they can still be inserted
	// in the original program arrays.
	plan.original = slices.MaxFunc(plan.groups, func(a, b *tailCallGroup) int {
		return len(a.targets) - len(b.targets)
	})

	return plan
}

// groupTailCallCallers splits the tracing programs that tail call through a program array, but aren't tail call
// targets themselves, in groups of programs sharing the same attach type and prototype
func groupTailCallCallers(progSpecs map[string]*lib.ProgramSpec, refs map[string][]string, tracingArrays, targets map[string]bool, loadKernelSpec func() (*btf.Spec, error)) []*tailCallGroup {
	var (
		kernelSpec       *btf.Spec
		kernelSpecLoaded bool
	)

	groups := make(map[tailCallGroupKey]*tailCallGroup)
	for name, spec := range progSpecs {
		if spec.Type != lib.Tracing || targets[name] || !slices.ContainsFunc(refs[name], func(ref string) bool { return tracingArrays[ref] }) {
			continue
		}

		if !kernelSpecLoaded {
			var err error
			if kernelSpec, err = loadKernelSpec(); err != nil {
				// a group per hooked function is always valid, it only requires more copies
				seclog.Warnf("failed to load kernel BTF, tail call targets will be copied for each hooked function: %v", err)
			}
			kernelSpecLoaded = true
		}

		key := newTailCallGroupKey(kernelSpec, spec.AttachType, spec.AttachTo)
		group, ok := groups[key]
		if !ok {
			group = &tailCallGroup{key: key}
			groups[key] = group
		}
		group.callers = append(group.callers, name)
	}

	result := make([]*tailCallGroup, 0, len(groups))
	for _, group := range groups {
		// sort the callers to get a stable representative function for each group
		slices.Sort(group.callers)
		group.attachTo = progSpecs[group.callers[0]].AttachTo
		result = append(result, group)
	}
	slices.SortFunc(result, func(a, b *tailCallGroup) int {
		return strings.Compare(a.String(), b.String())
	})
	return result
}

// newTailCallGroupKey returns the key of the group of a program of the provided attach type, attached to the
// provided function
func newTailCallGroupKey(kernelSpec *btf.Spec, attachType lib.AttachType, attachTo string) tailCallGroupKey {
	key := tailCallGroupKey{attachType: attachType}
	var fn *btf.Func
	if kernelSpec != nil && attachType != lib.AttachTraceRawTp && kernelSpec.TypeByName(attachTo, &fn) == nil {
		if id, err := kernelSpec.TypeID(fn.Type); err == nil {
			key.protoID = id
		}
	}
	if key.protoID == 0 {
		key.attachTo = attachTo
	}
	return key
}

// resolveRoutes computes the routes, targets and program arrays reachable from the callers of the group
func (g *tailCallGroup) resolveRoutes(routes []manager.TailCallRoute, refs map[string][]string, callers map[manager.TailCallRoute][]string) {
	programs := slices.Clone(g.callers)
	included := make([]bool, len(routes))

	for changed := true; changed; {
		changed = false

		for i, route := range routes {
			if included[i] {
				continue
			}

			reachable := slices.ContainsFunc(programs, func(prog string) bool {
				return slices.Contains(refs[prog], route.ProgArrayName)
			})
			if !reachable {
				continue
			}

			if routeCallers, ok := callers[route]; ok && !slices.ContainsFunc(routeCallers, func(caller string) bool {
				return slices.Contains(programs, caller)
			}) {
				continue
			}

			included[i] = true
			changed = true
			g.routes = append(g.routes, route)

			if target := route.ProbeIdentificationPair.EBPFFuncName; !slices.Contains(programs, target) {
				programs = append(programs, target)
				g.targets = append(g.targets, target)
			}
		}
	}

	for _, prog := range programs {
		for _, ref := range refs[prog] {
			if !slices.Contains(g.arrays, ref) {
				g.arrays = append(g.arrays, ref)
			}
		}
	}
}

// cloneGroup creates the program arrays of the group and prepares the copies of the tail call targets of the group
func (t *tailCallGroupsModifier) cloneGroup(group *tailCallGroup, progSpecs map[string]*lib.ProgramSpec, mapSpecs map[string]*lib.MapSpec, refs map[string][]string) error {
	group.clonedArrays = make(map[string]*lib.Map, len(group.arrays))
	for _, name := range group.arrays {
		arr, err := lib.NewMap(mapSpecs[name].Copy())
		if err != nil {
			return fmt.Errorf("failed to create program array %s for tail call group %s: %w", name, group, err)
		}
		t.clonedMaps[group.objectName(name)] = arr
		group.clonedArrays[name] = arr
	}

	group.clonedTargets = make(map[string]*lib.ProgramSpec, len(group.targets))
	for _, name := range group.targets {
		spec := progSpecs[name].Copy()
		retarget(spec, group)
		if err := associateProgramArrays(spec, refs[name], group.clonedArrays); err != nil {
			return fmt.Errorf("failed to associate program arrays to %s for tail call group %s: %w", name, group, err)
		}
		group.clonedTargets[name] = spec
	}

	return nil
}

func associateProgramArrays(spec *lib.ProgramSpec, refs []string, arrays map[string]*lib.Map) error {
	for _, ref := range refs {
		if err := spec.Instructions.AssociateMap(ref, arrays[ref]); err != nil {
			return err
		}
	}
	return nil
}

// associateCollectionMaps points the map references of a program to the maps loaded by the manager
func associateCollectionMaps(spec *lib.ProgramSpec, collectionMaps map[string]*lib.Map) error {
	for i := range spec.Instructions {
		ins := &spec.Instructions[i]
		if !ins.IsLoadFromMap() {
			continue
		}
		if m, ok := collectionMaps[ins.Reference()]; ok {
			if err := ins.AssociateMap(m); err != nil {
				return err
			}
		}
	}
	return nil
}

// loadClonedTargets loads the copies of the tail call targets of the provided groups and inserts them in the copies
// of the program arrays. The copies are loaded as a single collection sharing the provided BTF cache, so that the
// kernel BTF is only loaded once.
func (t *tailCallGroupsModifier) loadClonedTargets(groups []*tailCallGroup, collectionMaps map[string]*lib.Map, btfCache *btf.Cache) error {
	collSpec := &lib.CollectionSpec{Programs: make(map[string]*lib.ProgramSpec)}
	for _, group := range groups {
		for name, spec := range group.clonedTargets {
			if collectionMaps != nil {
				if err := associateCollectionMaps(spec, collectionMaps); err != nil {
					return fmt.Errorf("failed to associate maps to %s for tail call group %s: %w", name, group, err)
				}
			}
			collSpec.Programs[group.objectName(name)] = spec
		}
	}

	coll, err := lib.NewCollectionWithOptions(collSpec, lib.CollectionOptions{
		Programs: t.verifierOpts,
		Cache:    btfCache,
	})
	if err != nil {
		var ve *lib.VerifierError
		if errors.As(err, &ve) {
			return fmt.Errorf("verifier error loading copied tail call targets: %w\n%+v", err, ve)
		}
		return fmt.Errorf("failed to load copied tail call targets: %w", err)
	}
	for name, prog := range coll.Programs {
		t.clonedProgs[name] = prog
	}

	for _, group := range groups {
		for _, route := range group.routes {
			prog := coll.Programs[group.objectName(route.ProbeIdentificationPair.EBPFFuncName)]
			if err := group.clonedArrays[route.ProgArrayName].Put(route.Key, uint32(prog.FD())); err != nil {
				return fmt.Errorf("failed to insert %s in %s for tail call group %s: %w", route.ProbeIdentificationPair.EBPFFuncName, route.ProgArrayName, group, err)
			}
		}

		// the copies are only referenced by the program arrays and the cloned programs from now on
		group.clonedTargets = nil
	}

	return nil
}

// specEditor returns the function to apply to the spec of a clone of a runtime caller attached to the provided
// function, to point it to the program arrays matching the prototype of the function. The original program arrays are
// used when the function shares the prototype of the original group. Otherwise, the program arrays and tail call
// targets of a group dedicated to this prototype are created on first use. A nil function is returned for the
// programs that aren't runtime callers using program arrays.
func (t *tailCallGroupsModifier) specEditor(funcName string, attachTo string) (func(*lib.ProgramSpec), error) {
	t.Lock()
	defer t.Unlock()

	caller, ok := t.runtimeCallers[funcName]
	if !ok {
		return nil, nil
	}

	btfCache := btf.NewCache()
	kernelSpec, err := t.kernelTypes(btfCache)
	if err != nil {
		seclog.Warnf("failed to load kernel BTF, tail call targets will be copied for %s: %v", attachTo, err)
	}

	arrays := caller.originalArrays
	if key := newTailCallGroupKey(kernelSpec, caller.attachType, attachTo); key != t.originalKey {
		runtimeKey := runtimeTailCallGroupKey{tailCallGroupKey: key, caller: funcName}
		group, ok := t.runtimeGroups[runtimeKey]
		if !ok {
			if group, err = t.newRuntimeGroup(key, funcName, attachTo, caller, btfCache); err != nil {
				return nil, err
			}
			if t.runtimeGroups == nil {
				t.runtimeGroups = make(map[runtimeTailCallGroupKey]*tailCallGroup)
			}
			t.runtimeGroups[runtimeKey] = group
		}
		arrays = group.clonedArrays
	}

	return func(spec *lib.ProgramSpec) {
		for i := range spec.Instructions {
			ins := &spec.Instructions[i]
			if !ins.IsLoadFromMap() {
				continue
			}

			name := ins.Reference()
			if m, ok := ins.Map().(*lib.Map); ok && name == "" {
				name = caller.initArrays[m]
			}
			if arr, ok := arrays[name]; ok && slices.Contains(caller.refs[funcName], name) {
				_ = ins.AssociateMap(arr)
			}
		}
	}, nil
}

// newRuntimeGroup creates the program arrays and loads the tail call targets reachable from a runtime caller attached
// to the provided function
func (t *tailCallGroupsModifier) newRuntimeGroup(key tailCallGroupKey, funcName string, attachTo string, caller *runtimeTailCallCaller, btfCache *btf.Cache) (*tailCallGroup, error) {
	start := time.Now()

	group := &tailCallGroup{
		key:           key,
		attachTo:      attachTo,
		runtimeCaller: funcName,
		callers:       []string{funcName},
		arrays:        caller.reachable.arrays,
		routes:        caller.reachable.routes,
		targets:       caller.reachable.targets,
	}

	// the copies are added to the modifier as they are created, release them if the group can't be fully loaded
	release := func() {
		for _, name := range group.arrays {
			if arr, ok := t.clonedMaps[group.objectName(name)]; ok {
				_ = arr.Close()
				delete(t.clonedMaps, group.objectName(name))
			}
		}
		for _, name := range group.targets {
			if prog, ok := t.clonedProgs[group.objectName(name)]; ok {
				_ = prog.Close()
				delete(t.clonedProgs, group.objectName(name))
			}
		}
	}

	if err := t.cloneGroup(group, caller.targetSpecs, caller.mapSpecs, caller.refs); err != nil {
		release()
		return nil, err
	}
	if err := t.loadClonedTargets([]*tailCallGroup{group}, nil, btfCache); err != nil {
		release()
		return nil, err
	}

	seclog.Infof("tail call group %s created for a runtime clone of %s in %s (%d copied tail call targets)", group, funcName, time.Since(start), len(group.targets))
	return group, nil
}
