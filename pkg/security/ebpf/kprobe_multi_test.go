// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"errors"
	"os"
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
			backend := &kprobeMultiBackend{checkSupport: func() error {
				t.Fatal("disabled backend must not check support")
				return nil
			}}
			require.NoError(t, backend.BeforeInit(nil, names.ModuleName{}, nil))
		})
	}
}

func TestKprobeMultiBackendFallsBackBeforeRetargeting(t *testing.T) {
	for name, supportErr := range map[string]error{
		"unsupported_kernel": lib.ErrNotSupported,
		"missing_dependency": errors.New("companion ebpf-manager patch unavailable"),
		"permission_denied":  os.ErrPermission,
		"probe_failure":      errors.New("unexpected capability probe failure"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", "true")
			checked := false
			backend := &kprobeMultiBackend{checkSupport: func() error {
				checked = true
				return supportErr
			}}
			// A nil manager proves fallback happens before accessing or changing
			// any program specs, keeping callers and tail-call targets on legacy.
			require.NoError(t, backend.BeforeInit(nil, names.ModuleName{}, nil))
			require.True(t, checked)
		})
	}
}

func TestKprobeMultiBackendPreservesManagerErrors(t *testing.T) {
	t.Setenv("DD_CWS_KPROBE_MULTI_EXPERIMENT", "true")
	backend := &kprobeMultiBackend{checkSupport: func() error { return nil }}
	m := &manager.Manager{}
	_, expectedErr := m.GetProgramSpecs()
	require.Error(t, expectedErr)
	// Capability fallback must not hide unrelated initialization failures.
	require.ErrorIs(t, backend.BeforeInit(m, names.ModuleName{}, nil), expectedErr)
}
