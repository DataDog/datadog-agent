// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package features

import (
	"fmt"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/cilium/ebpf/features"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// SupportsTPBTFProbeMem returns nil if a tp_btf program can attach to tracepoint and dereference its
// first argument, which must be a struct pointer. The verifier turns that load into BPF_PROBE_MEM, which
// JIT_ALWAYS_ON kernels reject when the JIT has no exception table support, even if kernel BTF exists.
// Other kernels run such a program in the interpreter, so the trial also requires it to be JIT-compiled.
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

	if err := requireJIT(prog.Info()); err != nil {
		return err
	}

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

type jitedSizer interface {
	JitedSize() (uint32, error)
}

// requireJIT returns nil only if info shows a JIT-compiled program. A zero JIT size means the
// interpreter runs the program, or that the kernel does not report it.
func requireJIT(info jitedSizer, infoErr error) error {
	if infoErr != nil {
		return fmt.Errorf("cannot confirm the program is JIT-compiled: %w", infoErr)
	}
	if _, err := info.JitedSize(); err != nil {
		return fmt.Errorf("cannot confirm the program is JIT-compiled: %w", err)
	}
	return nil
}
