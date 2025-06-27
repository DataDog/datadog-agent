// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package actuator

import (
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/codegen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// CompiledProgram is a compiled eBPF program.
type CompiledProgram struct {
	// IR is the IR program that was generated from the probe configuration.
	IR *ir.Program
	// Probes is the list of probes that were compiled.
	Probes []ir.ProbeDefinition
	// CompiledBPF is the compiled eBPF program.
	CompiledBPF compiler.CompiledBPF
}

type loadedProgram struct {
	program      *ir.Program
	collection   *ebpf.Collection
	bpfProgram   *ebpf.Program
	attachpoints []codegen.BPFAttachPoint
}

func (p *loadedProgram) close() {
	if p.collection != nil { // only nil in tests
		p.collection.Close() // should already contain the program
	}
}

type attachedProgram struct {
	program        *ir.Program
	procID         ProcessID
	executableLink *link.Executable
	attachedLinks  []link.Link
}
