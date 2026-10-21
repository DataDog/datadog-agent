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
	"time"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
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
type tailCallGroupsModifier struct {
	routes       []manager.TailCallRoute
	verifierOpts lib.ProgramOptions
	clonedGroups []*tailCallGroup
	clonedMaps   []*lib.Map
	clonedProgs  []*lib.Program

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
	callers  []string
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
	return fmt.Sprintf("%s/%s", attachTypePrefix(g.key.attachType), g.attachTo)
}

func newTailCallGroupsModifier() *tailCallGroupsModifier {
	return &tailCallGroupsModifier{}
}

func (t *tailCallGroupsModifier) String() string {
	return "TailCallGroupsModifier"
}

// BeforeInit registers the instruction patcher splitting the program arrays per group of callers. It has to run
// after the constants have been edited, so that the copied programs inherit them.
func (t *tailCallGroupsModifier) BeforeInit(m *manager.Manager, _ names.ModuleName, opts *manager.Options) error {
	t.reset()
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
	if len(t.clonedGroups) == 0 {
		return nil
	}

	if err := t.loadClonedTargets(m); err != nil {
		t.reset()
		return err
	}
	return nil
}

// AfterStop releases the copies of the program arrays and of the tail call targets
func (t *tailCallGroupsModifier) AfterStop(_ *manager.Manager, _ names.ModuleName, _ manager.MapCleanupType) error {
	t.reset()
	return nil
}

func (t *tailCallGroupsModifier) reset() {
	for _, prog := range t.clonedProgs {
		_ = prog.Close()
	}
	for _, m := range t.clonedMaps {
		_ = m.Close()
	}
	t.clonedProgs = nil
	t.clonedMaps = nil
	t.clonedGroups = nil
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

	plan := planTailCallGroups(progSpecs, mapSpecs, t.routes, hookFuncs, btf.LoadKernelSpec)
	if plan == nil {
		return nil
	}

	for name := range plan.targets {
		retarget(progSpecs[name], plan.original)
	}

	var clonedTargetsCount int
	for _, group := range plan.groups {
		if group == plan.original {
			continue
		}

		if err := t.cloneGroup(group, progSpecs, mapSpecs, plan.refs); err != nil {
			t.reset()
			return err
		}
		t.clonedGroups = append(t.clonedGroups, group)
		clonedTargetsCount += len(group.clonedTargets)
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
}

// planTailCallGroups splits the hook points using program arrays in groups of programs that can share them. It
// returns nil if no tracing program uses a program array.
func planTailCallGroups(progSpecs map[string]*lib.ProgramSpec, mapSpecs map[string]*lib.MapSpec, allRoutes []manager.TailCallRoute, hookFuncs map[string]bool, loadKernelSpec func() (*btf.Spec, error)) *tailCallPlan {
	programArrays := make(map[string]bool)
	for name, spec := range mapSpecs {
		if spec.Type == lib.ProgramArray {
			programArrays[name] = true
		}
	}

	plan := &tailCallPlan{
		refs:    make(map[string][]string),
		targets: make(map[string]bool),
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

	for _, group := range plan.groups {
		group.resolveRoutes(routes, plan.refs)
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

		key := tailCallGroupKey{attachType: spec.AttachType}
		var fn *btf.Func
		if kernelSpec != nil && spec.AttachType != lib.AttachTraceRawTp && kernelSpec.TypeByName(spec.AttachTo, &fn) == nil {
			if id, err := kernelSpec.TypeID(fn.Type); err == nil {
				key.protoID = id
			}
		}
		if key.protoID == 0 {
			key.attachTo = spec.AttachTo
		}

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

// resolveRoutes computes the routes, targets and program arrays reachable from the callers of the group
func (g *tailCallGroup) resolveRoutes(routes []manager.TailCallRoute, refs map[string][]string) {
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

// cloneGroup creates the program arrays of the group, points the callers of the group to them, and prepares the
// copies of the tail call targets of the group
func (t *tailCallGroupsModifier) cloneGroup(group *tailCallGroup, progSpecs map[string]*lib.ProgramSpec, mapSpecs map[string]*lib.MapSpec, refs map[string][]string) error {
	group.clonedArrays = make(map[string]*lib.Map, len(group.arrays))
	for _, name := range group.arrays {
		arr, err := lib.NewMap(mapSpecs[name].Copy())
		if err != nil {
			return fmt.Errorf("failed to create program array %s for tail call group %s: %w", name, group, err)
		}
		t.clonedMaps = append(t.clonedMaps, arr)
		group.clonedArrays[name] = arr
	}

	group.clonedTargets = make(map[string]*lib.ProgramSpec, len(group.targets))
	for _, name := range group.targets {
		spec := progSpecs[name].Copy()
		retarget(spec, group)
		group.clonedTargets[name] = spec
	}

	for name, spec := range group.clonedTargets {
		if err := associateProgramArrays(spec, refs[name], group.clonedArrays); err != nil {
			return fmt.Errorf("failed to associate program arrays to %s for tail call group %s: %w", name, group, err)
		}
	}
	for _, name := range group.callers {
		if err := associateProgramArrays(progSpecs[name], refs[name], group.clonedArrays); err != nil {
			return fmt.Errorf("failed to associate program arrays to %s for tail call group %s: %w", name, group, err)
		}
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

func (t *tailCallGroupsModifier) loadClonedTargets(m *manager.Manager) error {
	start := time.Now()

	collectionMaps, err := m.GetMaps()
	if err != nil {
		return err
	}

	for _, group := range t.clonedGroups {
		loaded := make(map[string]*lib.Program, len(group.clonedTargets))
		for name, spec := range group.clonedTargets {
			if err := associateCollectionMaps(spec, collectionMaps); err != nil {
				return fmt.Errorf("failed to associate maps to %s for tail call group %s: %w", name, group, err)
			}

			prog, err := lib.NewProgramWithOptions(spec, t.verifierOpts)
			if err != nil {
				var ve *lib.VerifierError
				if errors.As(err, &ve) {
					return fmt.Errorf("verifier error loading %s for tail call group %s: %w\n%+v", name, group, err, ve)
				}
				return fmt.Errorf("failed to load %s for tail call group %s: %w", name, group, err)
			}
			t.clonedProgs = append(t.clonedProgs, prog)
			loaded[name] = prog
		}

		for _, route := range group.routes {
			prog := loaded[route.ProbeIdentificationPair.EBPFFuncName]
			if err := group.clonedArrays[route.ProgArrayName].Put(route.Key, uint32(prog.FD())); err != nil {
				return fmt.Errorf("failed to insert %s in %s for tail call group %s: %w", route.ProbeIdentificationPair.EBPFFuncName, route.ProgArrayName, group, err)
			}
		}

		// the copies are only referenced by the program arrays and the cloned programs from now on
		group.clonedTargets = nil
	}

	seclog.Infof("loaded %d copied tail call targets in %s", len(t.clonedProgs), time.Since(start))
	return nil
}
