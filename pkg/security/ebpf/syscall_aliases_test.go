// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package ebpf

import (
	"bytes"
	"testing"

	lib "github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetargetSyscallAliases(t *testing.T) {
	long := &btf.Int{Name: "long", Size: 8, Encoding: btf.Signed}
	ptRegs := &btf.Pointer{Target: &btf.Const{Type: &btf.Struct{Name: "pt_regs"}}}
	wrapperProto := &btf.FuncProto{Return: long, Params: []btf.FuncParam{{Name: "regs", Type: ptRegs}}}
	argsProto := &btf.FuncProto{Return: long, Params: []btf.FuncParam{{Name: "fd", Type: long}}}

	builder, err := btf.NewBuilder([]btf.Type{
		// __x64_sys_setsid is an alias without BTF
		&btf.Func{Name: "__do_sys_setsid", Type: wrapperProto, Linkage: btf.StaticFunc},
		&btf.Func{Name: "__x64_sys_openat", Type: wrapperProto, Linkage: btf.GlobalFunc},
		&btf.Func{Name: "__do_sys_openat", Type: wrapperProto, Linkage: btf.StaticFunc},
		// __do_sys_close takes the syscall arguments, it can't replace the wrapper
		&btf.Func{Name: "__do_sys_close", Type: argsProto, Linkage: btf.StaticFunc},
	}, nil)
	require.NoError(t, err)
	raw, err := builder.Marshal(nil, nil)
	require.NoError(t, err)
	kernelSpec, err := btf.LoadSpecFromReader(bytes.NewReader(raw))
	require.NoError(t, err)

	progSpecs := map[string]*lib.ProgramSpec{
		"fentry__64_sys_setsid":  tracingProgSpec(lib.AttachTraceFEntry, "__x64_sys_setsid"),
		"fexit__64_sys_setsid":   tracingProgSpec(lib.AttachTraceFExit, "__x64_sys_setsid"),
		"fentry__32_sys_setsid":  tracingProgSpec(lib.AttachTraceFEntry, "__ia32_sys_setsid"),
		"fentry__64_sys_openat":  tracingProgSpec(lib.AttachTraceFEntry, "__x64_sys_openat"),
		"fentry__64_sys_close":   tracingProgSpec(lib.AttachTraceFEntry, "__x64_sys_close"),
		"fentry__64_sys_unknown": tracingProgSpec(lib.AttachTraceFEntry, "__x64_sys_unknown"),
		"kprobe__64_sys_setsid":  {Type: lib.Kprobe, AttachTo: "__x64_sys_setsid"},
	}

	retargetSyscallAliases(progSpecs, "__x64_sys_", func() (*btf.Spec, error) { return kernelSpec, nil })

	assert.Equal(t, "__do_sys_setsid", progSpecs["fentry__64_sys_setsid"].AttachTo)
	assert.Equal(t, "__do_sys_setsid", progSpecs["fexit__64_sys_setsid"].AttachTo)
	// only the native wrapper is retargeted, the 32 bit one would hook the same function twice
	assert.Equal(t, "__ia32_sys_setsid", progSpecs["fentry__32_sys_setsid"].AttachTo)
	assert.Equal(t, "__x64_sys_openat", progSpecs["fentry__64_sys_openat"].AttachTo)
	assert.Equal(t, "__x64_sys_close", progSpecs["fentry__64_sys_close"].AttachTo)
	assert.Equal(t, "__x64_sys_unknown", progSpecs["fentry__64_sys_unknown"].AttachTo)
	assert.Equal(t, "__x64_sys_setsid", progSpecs["kprobe__64_sys_setsid"].AttachTo)
}
