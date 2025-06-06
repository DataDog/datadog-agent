// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"testing"

	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/network/go/dwarfutils/locexpr"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var MinimumKernelVersion = kernel.VersionCode(5, 17, 0)

func skipIfKernelNotSupported(t *testing.T) {
	curKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if curKernelVersion < MinimumKernelVersion {
		t.Skipf("Kernel version %v is not supported", curKernelVersion)
	}
}

func TestCompileBPFProgram(t *testing.T) {
	skipIfKernelNotSupported(t)

	pointee := &ir.BaseType{
		TypeCommon: ir.TypeCommon{
			ID:       4,
			Name:     "int",
			ByteSize: 4,
		},
	}
	pointer := &ir.PointerType{
		TypeCommon: ir.TypeCommon{
			ID:       3,
			Name:     "*int",
			ByteSize: 8,
		},
		Pointee: pointee,
	}
	sp := &ir.Subprogram{
		Name:              "CoolButBuggyFunction",
		OutOfLinePCRanges: []ir.PCRange{},
		InlinePCRanges:    [][]ir.PCRange{},
		Variables: []*ir.Variable{
			{
				Name: "BadVar",
				Type: pointer,
				Locations: []ir.Location{
					{
						Range: ir.PCRange{1000, 1200},
						Pieces: []locexpr.LocationPiece{
							{
								Size:        8,
								InReg:       false,
								StackOffset: 12,
								Register:    -1,
							},
						},
					},
				},
				IsParameter: false,
			},
		},
		Lines: []ir.SubprogramLine{},
	}
	p := &ir.Program{
		ID: 123,
		Probes: []*ir.Probe{
			{
				ID:         "UUID1",
				Kind:       ir.ProbeKindLog,
				Version:    7,
				Tags:       nil,
				Subprogram: sp,
				Events: []*ir.Event{
					{
						ID: 456,
						Type: &ir.EventRootType{
							TypeCommon: ir.TypeCommon{
								ID:       10,
								Name:     "CoolButBuggyFunction",
								ByteSize: 0,
							},
							PresenseBitsetSize: 0,
							Expressions: []*ir.RootExpression{
								{
									Name:   "BadVar",
									Offset: 0,
									Expression: ir.Expression{
										Type: pointee,
										Operations: []ir.Op{
											&ir.LocationOp{
												Variable: sp.Variables[0],
												Offset:   0,
												ByteSize: 4,
											},
										},
									},
								},
							},
						},
						InjectionPCs: []uint64{1100},
						Condition:    nil,
					},
				},
			},
		},
		Subprograms: []*ir.Subprogram{sp},
		Types:       map[ir.TypeID]ir.Type{pointer.ID: pointer, pointee.ID: pointee},
		MaxTypeID:   132,
	}
	obj, err := CompileBPFProgram(p, nil)
	if err != nil {
		t.Fatalf("Failed to compile BPF program: %v", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(obj.Obj)
	if err != nil {
		t.Fatalf("Failed to load ebpf spec: %v", err)
	}
	t.Log(spec)

	t.Log("Loading...")
	opts := ebpf.CollectionOptions{
		Programs: ebpf.ProgramOptions{
			LogLevel:    (ebpf.LogLevelBranch | ebpf.LogLevelInstruction | ebpf.LogLevelStats),
			LogDisabled: false,
		},
	}
	compiledBPF, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		t.Fatalf("Failed to load ebpf obj: %v", err)
	}
	t.Log(compiledBPF)

	prog, ok := compiledBPF.Programs["probe_run_with_cookie"]
	if !ok {
		t.Fatalf("Failed to find ebpf program: %v", compiledBPF)
	}
	t.Log(prog)
}
