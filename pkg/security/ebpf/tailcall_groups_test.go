// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"bytes"
	"errors"
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tracingProgSpec(attachType lib.AttachType, attachTo string, arrays ...string) *lib.ProgramSpec {
	var insns asm.Instructions
	for _, array := range arrays {
		insns = append(insns, asm.LoadMapPtr(asm.R2, 0).WithReference(array))
	}
	insns = append(insns, asm.Mov.Imm(asm.R0, 0), asm.Return())

	return &lib.ProgramSpec{
		Type:         lib.Tracing,
		AttachType:   attachType,
		AttachTo:     attachTo,
		Instructions: insns,
	}
}

func route(array string, key uint32, target string) manager.TailCallRoute {
	return manager.TailCallRoute{
		ProgArrayName:           array,
		Key:                     key,
		ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: target},
	}
}

// testTailCallSetup mimics the dentry resolver: hooks start a resolution, the resolver tail calls into a callback,
// a callback can start another resolution with another callback, and callbacks send their event through a shared
// program
func testTailCallSetup() (map[string]*lib.ProgramSpec, map[string]*lib.MapSpec, []manager.TailCallRoute) {
	progSpecs := map[string]*lib.ProgramSpec{
		"hook_func_a":       tracingProgSpec(lib.AttachTraceFEntry, "func_a", "progs"),
		"hook_func_b":       tracingProgSpec(lib.AttachTraceFEntry, "func_b", "progs"),
		"rethook_func_a":    tracingProgSpec(lib.AttachTraceFExit, "func_a", "send"),
		"tail_call_resolve": tracingProgSpec(lib.AttachTraceFEntry, "start_kernel", "progs", "callbacks"),
		"tail_call_cb_a":    tracingProgSpec(lib.AttachTraceFEntry, "start_kernel", "send"),
		"tail_call_cb_b":    tracingProgSpec(lib.AttachTraceFEntry, "start_kernel"),
		"tail_call_stage_1": tracingProgSpec(lib.AttachTraceFEntry, "start_kernel", "progs"),
		"tail_call_stage_2": tracingProgSpec(lib.AttachTraceFEntry, "start_kernel"),
		"tail_call_send":    tracingProgSpec(lib.AttachTraceFEntry, "start_kernel"),
		"tail_call_unused":  tracingProgSpec(lib.AttachTraceFEntry, "start_kernel", "progs"),
		"kprobe_unrelated":  {Type: lib.Kprobe, Instructions: asm.Instructions{asm.LoadMapPtr(asm.R2, 0).WithReference("kprobe_progs"), asm.Return()}},
	}

	mapSpecs := map[string]*lib.MapSpec{
		"progs":        {Type: lib.ProgramArray},
		"callbacks":    {Type: lib.ProgramArray},
		"send":         {Type: lib.ProgramArray},
		"kprobe_progs": {Type: lib.ProgramArray},
	}

	routes := []manager.TailCallRoute{
		route("progs", 0, "tail_call_resolve"),
		route("callbacks", 1, "tail_call_cb_a"),
		route("callbacks", 2, "tail_call_cb_b"),
		route("callbacks", 3, "tail_call_stage_1"),
		route("callbacks", 4, "tail_call_stage_2"),
		route("send", 0, "tail_call_send"),
	}

	return progSpecs, mapSpecs, routes
}

var testHookFuncs = map[string]bool{
	"hook_func_a":      true,
	"hook_func_b":      true,
	"rethook_func_a":   true,
	"kprobe_unrelated": true,
}

func noKernelSpec() (*btf.Spec, error) {
	return nil, errors.New("no kernel BTF")
}

func groupByName(t *testing.T, plan *tailCallPlan, name string) *tailCallGroup {
	for _, group := range plan.groups {
		if group.String() == name {
			return group
		}
	}
	require.Failf(t, "group not found", "%s", name)
	return nil
}

func TestPlanTailCallGroups(t *testing.T) {
	progSpecs, mapSpecs, routes := testTailCallSetup()

	plan := planTailCallGroups(progSpecs, mapSpecs, routes, testHookFuncs, noKernelSpec)
	require.NotNil(t, plan)

	// without kernel BTF, the callers are grouped per hooked function
	require.Len(t, plan.groups, 3)

	// every target reachable through the program arrays of a group is part of the group
	allTargets := []string{"tail_call_resolve", "tail_call_cb_a", "tail_call_cb_b", "tail_call_stage_1", "tail_call_stage_2", "tail_call_send"}

	funcA := groupByName(t, plan, "fentry/func_a")
	assert.Equal(t, []string{"hook_func_a"}, funcA.callers)
	assert.ElementsMatch(t, allTargets, funcA.targets)
	assert.ElementsMatch(t, []string{"progs", "callbacks", "send"}, funcA.arrays)
	assert.ElementsMatch(t, routes, funcA.routes)

	funcB := groupByName(t, plan, "fentry/func_b")
	assert.ElementsMatch(t, allTargets, funcB.targets)
	assert.ElementsMatch(t, []string{"progs", "callbacks", "send"}, funcB.arrays)

	// the attach type is part of the group
	retFuncA := groupByName(t, plan, "fexit/func_a")
	assert.ElementsMatch(t, []string{"tail_call_send"}, retFuncA.targets)
	assert.ElementsMatch(t, []string{"send"}, retFuncA.arrays)

	assert.Same(t, funcA, plan.original)

	// a target without route is retargeted with the original targets, it isn't a caller
	assert.Len(t, plan.targets, 7)
	assert.True(t, plan.targets["tail_call_unused"])
}

func TestPlanTailCallGroupsSharedPrototype(t *testing.T) {
	progSpecs, mapSpecs, routes := testTailCallSetup()

	proto := &btf.FuncProto{Return: &btf.Int{Name: "int", Size: 4, Encoding: btf.Signed}}
	builder, err := btf.NewBuilder([]btf.Type{
		&btf.Func{Name: "func_a", Type: proto, Linkage: btf.GlobalFunc},
		&btf.Func{Name: "func_b", Type: proto, Linkage: btf.GlobalFunc},
	}, nil)
	require.NoError(t, err)
	raw, err := builder.Marshal(nil, nil)
	require.NoError(t, err)
	kernelSpec, err := btf.LoadSpecFromReader(bytes.NewReader(raw))
	require.NoError(t, err)

	plan := planTailCallGroups(progSpecs, mapSpecs, routes, testHookFuncs, func() (*btf.Spec, error) { return kernelSpec, nil })
	require.NotNil(t, plan)

	// func_a and func_b share the same prototype, so their fentry hooks can share program arrays
	require.Len(t, plan.groups, 2)
	shared := plan.original
	assert.Equal(t, lib.AttachTraceFEntry, shared.key.attachType)
	assert.Equal(t, []string{"hook_func_a", "hook_func_b"}, shared.callers)
	assert.Equal(t, "func_a", shared.attachTo)
	assert.Len(t, shared.targets, 6)
}

func TestPlanTailCallGroupsKprobeOnly(t *testing.T) {
	progSpecs := map[string]*lib.ProgramSpec{
		"kprobe_hook":   {Type: lib.Kprobe, Instructions: asm.Instructions{asm.LoadMapPtr(asm.R2, 0).WithReference("progs"), asm.Return()}},
		"kprobe_target": {Type: lib.Kprobe, Instructions: asm.Instructions{asm.Return()}},
	}
	mapSpecs := map[string]*lib.MapSpec{"progs": {Type: lib.ProgramArray}}
	routes := []manager.TailCallRoute{route("progs", 0, "kprobe_target")}

	assert.Nil(t, planTailCallGroups(progSpecs, mapSpecs, routes, map[string]bool{"kprobe_hook": true}, noKernelSpec))
}
