// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"testing"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/names"
)

func TestKprobeMultiBackendPreservesPrograms(t *testing.T) {
	specs := map[string]*lib.ProgramSpec{
		"entry":      {Type: lib.Kprobe, SectionName: "kprobe/func", AttachTo: "func"},
		"return":     {Type: lib.Kprobe, SectionName: "kretprobe/func", AttachTo: "func"},
		"tail_call":  {Type: lib.Kprobe, SectionName: "kprobe/tail_call_target", AttachTo: "tail_call_target"},
		"uprobe":     {Type: lib.Kprobe, SectionName: "uprobe/func", AttachTo: "func"},
		"uretprobe":  {Type: lib.Kprobe, SectionName: "uretprobe/func", AttachTo: "func"},
		"tracepoint": {Type: lib.TracePoint, SectionName: "tracepoint/syscalls/sys_enter_open"},
		"fentry":     {Type: lib.Tracing, AttachType: lib.AttachTraceFEntry, SectionName: "fentry/func", AttachTo: "func"},
		"fexit":      {Type: lib.Tracing, AttachType: lib.AttachTraceFExit, SectionName: "fexit/func", AttachTo: "func"},
	}
	expected := make(map[string]*lib.ProgramSpec, len(specs))
	for name, spec := range specs {
		spec.Name = name
		spec.Instructions = asm.Instructions{asm.Mov.Imm(asm.R0, 0), asm.Return()}
		expected[name] = spec.Copy()
	}
	for _, name := range []string{"entry", "return", "tail_call"} {
		expected[name].AttachType = lib.AttachTraceKprobeMulti
	}
	retargetKprobePrograms(specs)
	require.Equal(t, expected, specs, "only the expected attach type of kprobe callers and targets may change")
}

func TestKprobeMultiBackendDisabled(t *testing.T) {
	for _, value := range []string{"", "false", "grouped", "grouped-return"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", value)
			// A disabled backend must neither query the kernel nor access the manager.
			require.NoError(t, (&kprobeMultiBackend{}).BeforeInit(nil, names.ModuleName{}, nil))
		})
	}
}

func TestKprobeMultiBackendRequiresDependencySupport(t *testing.T) {
	if _, ok := any(&manager.Probe{}).(interface{ SupportsKprobeMulti() bool }); ok {
		t.Skip("this check exercises the unpatched dependency")
	}
	t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", "true")
	require.ErrorContains(t, (&kprobeMultiBackend{}).BeforeInit(nil, names.ModuleName{}, nil), "companion ebpf-manager patch")
}
