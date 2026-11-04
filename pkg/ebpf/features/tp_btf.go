// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package features

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// SupportsTPBTFProbeMem returns nil if a tp_btf program can attach to tracepoint and dereference its
// first argument, which must be a struct pointer. The verifier turns that load into BPF_PROBE_MEM, which
// JIT_ALWAYS_ON kernels reject when the JIT has no exception table support, even if kernel BTF exists.
var SupportsTPBTFProbeMem = funcs.MemoizeArgNoError(func(tracepoint string) error {
	if err := features.HaveProgramType(ebpf.Tracing); err != nil {
		return err
	}

	spec := &ebpf.ProgramSpec{
		Type:       ebpf.Tracing,
		AttachType: ebpf.AttachTraceRawTp,
		AttachTo:   tracepoint,
		Instructions: asm.Instructions{
			// r1 = first tracepoint argument, then an 8-byte load through it
			asm.LoadMem(asm.R1, asm.R1, 0, asm.DWord),
			asm.LoadMem(asm.R0, asm.R1, 0, asm.DWord),
			asm.Mov.Imm(asm.R0, 0),
			asm.Return(),
		},
		License: "GPL",
	}
	prog, err := ebpf.NewProgramWithOptions(spec, ebpf.ProgramOptions{
		LogDisabled: true,
	})
	if err != nil {
		return err
	}
	defer prog.Close()

	l, err := link.AttachTracing(link.TracingOptions{
		Program:    prog,
		AttachType: ebpf.AttachTraceRawTp,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	return nil
})
