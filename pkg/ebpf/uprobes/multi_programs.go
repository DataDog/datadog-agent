// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && bpf

package uprobes

import (
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
)

// MultiProgramSpec holds a multi-uprobe variant of a program graph. Data maps
// are shared with the original graph. Program arrays must be private because
// the kernel requires matching expected_attach_type throughout a tail call.
type MultiProgramSpec struct {
	spec   *ebpf.CollectionSpec
	routes []manager.TailCallRoute
}

// PrepareUprobeMultiPrograms must run as the last manager instruction patcher:
// copy constants, telemetry patches and CO-RE metadata before the manager can
// discard unmapped program specs. It never modifies the original graph.
func PrepareUprobeMultiPrograms(mgr *manager.Manager, roots []string, routes []manager.TailCallRoute) (*MultiProgramSpec, error) {
	programs, err := mgr.GetProgramSpecs()
	if err != nil {
		return nil, err
	}
	maps, err := mgr.GetMapSpecs()
	if err != nil {
		return nil, err
	}
	return prepareMultiPrograms(programs, maps, roots, routes)
}

func prepareMultiPrograms(programs map[string]*ebpf.ProgramSpec, maps map[string]*ebpf.MapSpec, roots []string, routes []manager.TailCallRoute) (*MultiProgramSpec, error) {
	result := &MultiProgramSpec{spec: &ebpf.CollectionSpec{
		Programs: make(map[string]*ebpf.ProgramSpec), Maps: make(map[string]*ebpf.MapSpec),
	}}
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if result.spec.Programs[name] != nil {
			continue
		}
		program := programs[name]
		if program == nil || program.Type != ebpf.Kprobe {
			return nil, fmt.Errorf("missing or incompatible multi-uprobe program %s", name)
		}
		program = program.Copy()
		program.AttachType = ebpf.AttachTraceUprobeMulti
		result.spec.Programs[name] = program
		for _, ins := range program.Instructions {
			ref := ins.Reference()
			mapSpec := maps[ref]
			if mapSpec == nil || result.spec.Maps[ref] != nil {
				continue
			}
			result.spec.Maps[ref] = mapSpec.Copy()
			if mapSpec.Type != ebpf.ProgramArray {
				continue
			}
			result.spec.Maps[ref].Contents = nil
			result.spec.Maps[ref].Pinning = ebpf.PinNone
			for _, route := range routes {
				if route.ProgArrayName != ref {
					continue
				}
				if route.Program != nil {
					return nil, fmt.Errorf("cannot clone external tail-call program for %s", ref)
				}
				result.routes = append(result.routes, route)
				queue = append(queue, route.ProbeIdentificationPair.EBPFFuncName)
			}
		}
	}
	return result, nil
}

// MultiPrograms owns loaded multi-uprobe programs and private tail-call arrays.
// Close only after all attachers using Programs have stopped.
type MultiPrograms struct {
	collection *ebpf.Collection
	mappings   *ebpf.Collection
}

// Load loads the cloned graph using the original program options (including
// kernel BTF), and replacements for every non-program-array map.
func (spec *MultiProgramSpec) Load(mgr *manager.Manager, opts ebpf.CollectionOptions, module string) (*MultiPrograms, error) {
	opts.MapReplacements = make(map[string]*ebpf.Map)
	for name, mapSpec := range spec.spec.Maps {
		if mapSpec.Type == ebpf.ProgramArray {
			continue
		}
		m, found, err := mgr.GetMap(name)
		if err != nil {
			return nil, err
		}
		if !found || m == nil {
			return nil, fmt.Errorf("missing shared multi-uprobe map %s", name)
		}
		opts.MapReplacements[name] = m
	}
	collection, err := ebpf.NewCollectionWithOptions(spec.spec, opts)
	if err != nil {
		return nil, fmt.Errorf("load multi-uprobe graph: %w", err)
	}
	for _, route := range spec.routes {
		if err := collection.Maps[route.ProgArrayName].Update(route.Key, collection.Programs[route.ProbeIdentificationPair.EBPFFuncName], ebpf.UpdateAny); err != nil {
			collection.Close()
			return nil, fmt.Errorf("populate multi-uprobe tail call %s: %w", route.ProbeIdentificationPair.EBPFFuncName, err)
		}
	}
	// Register only new objects: removing mappings for shared maps would also
	// remove the original manager's mappings.
	mappings := &ebpf.Collection{Programs: collection.Programs, Maps: make(map[string]*ebpf.Map)}
	for name, m := range collection.Maps {
		if spec.spec.Maps[name].Type == ebpf.ProgramArray {
			mappings.Maps[name] = m
		}
	}
	ddebpf.AddNameMappingsCollection(mappings, module)
	return &MultiPrograms{collection: collection, mappings: mappings}, nil
}

// Programs returns the programs available for attachment, indexed by ELF name.
func (programs *MultiPrograms) Programs() map[string]*ebpf.Program {
	return programs.collection.Programs
}

// Close releases the owned references, leaving shared maps alive in the manager.
func (programs *MultiPrograms) Close() {
	if programs == nil || programs.collection == nil {
		return
	}
	ddebpf.RemoveNameMappingsCollection(programs.mappings)
	programs.collection.Close()
	programs.collection = nil
}
