// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && ebpf_bindata && pcap && cgo

// Package tests holds tests related files
package tests

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/asm"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes/rawpacket"
)

// r0Uninitialized stands for the program entry among the r0 definitions
const r0Uninitialized = -1

// checkTCActUnspec hardens probes.CheckUnspecReturnCode, whose runtime use on the raw packet filters
// can't afford false positives: every path to an exit must set r0 to TC_ACT_UNSPEC with an immediate.
func checkTCActUnspec(progSpecs map[string]*ebpf.ProgramSpec) error {
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(progSpecs)) {
		progSpec := progSpecs[name]
		if progSpec.Type != ebpf.SchedCLS || probes.IsProgAllowedToTCActShot(progSpec.Name) {
			continue
		}
		if err := checkProgTCActUnspec(progSpec.Instructions); err != nil {
			errs = append(errs, fmt.Errorf("program %s: %w", progSpec.Name, err))
		}
	}
	return errors.Join(errs...)
}

func checkProgTCActUnspec(insns asm.Instructions) error {
	flow, err := newProgFlow(insns)
	if err != nil {
		return err
	}
	defs, err := flow.r0Definitions()
	if err != nil {
		return err
	}

	var errs []error
	for i, ins := range insns {
		if defs[i] == nil || ins.OpCode.JumpOp() != asm.Exit {
			continue
		}
		for _, def := range slices.Sorted(maps.Keys(defs[i])) {
			if def == r0Uninitialized {
				errs = append(errs, fmt.Errorf("exit at insn %d: r0 may be uninitialized", flow.offsets[i]))
			} else if value, ok := r0Immediate(insns[def]); !ok {
				errs = append(errs, fmt.Errorf("exit at insn %d: r0 may be set by insn %d `%v`", flow.offsets[i], flow.offsets[def], insns[def]))
			} else if value != probes.TCActUnspec {
				errs = append(errs, fmt.Errorf("exit at insn %d: may return %d, set by insn %d", flow.offsets[i], value, flow.offsets[def]))
			}
		}
	}
	return errors.Join(errs...)
}

type progFlow struct {
	insns   asm.Instructions
	offsets []asm.RawInstructionOffset
	indexes map[asm.RawInstructionOffset]int
	symbols map[string]int
}

func newProgFlow(insns asm.Instructions) (*progFlow, error) {
	if len(insns) == 0 {
		return nil, errors.New("no instructions")
	}

	symbols, err := insns.SymbolOffsets()
	if err != nil {
		return nil, err
	}

	flow := &progFlow{
		insns:   insns,
		offsets: make([]asm.RawInstructionOffset, len(insns)),
		indexes: make(map[asm.RawInstructionOffset]int, len(insns)),
		symbols: symbols,
	}
	for iter := insns.Iterate(); iter.Next(); {
		flow.offsets[iter.Index] = iter.Offset
		flow.indexes[iter.Offset] = iter.Index
	}
	return flow, nil
}

// successors returns the instructions that can follow the i-th one. Calls fall through: subprograms
// return to their caller, and a failed tail call continues with the next instruction.
func (f *progFlow) successors(i int) ([]int, error) {
	ins := f.insns[i]

	var succs []int
	switch op := ins.OpCode.JumpOp(); op {
	case asm.Exit:
		return nil, nil
	case asm.InvalidJumpOp, asm.Call:
	default:
		target, ok := f.symbols[ins.Reference()]
		if ins.Reference() == "" {
			delta := int64(ins.Offset)
			if ins.OpCode.Class() == asm.Jump32Class && op == asm.Ja {
				delta = ins.Constant
			}
			target, ok = f.indexes[asm.RawInstructionOffset(int64(f.offsets[i])+1+delta)]
		}
		if !ok {
			return nil, fmt.Errorf("insn %d: invalid jump target", f.offsets[i])
		}
		if op == asm.Ja {
			return []int{target}, nil
		}
		succs = append(succs, target)
	}

	if i+1 == len(f.insns) {
		return nil, fmt.Errorf("insn %d: falls off the end of the program", f.offsets[i])
	}
	return append(succs, i+1), nil
}

// r0Definitions returns, for each instruction reachable from the entry, the instructions that may have
// last written r0
func (f *progFlow) r0Definitions() ([]map[int]bool, error) {
	defs := make([]map[int]bool, len(f.insns))
	defs[0] = map[int]bool{r0Uninitialized: true}

	queue := []int{0}
	for len(queue) > 0 {
		i := queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		out := defs[i]
		if writesR0(f.insns[i]) {
			out = map[int]bool{i: true}
		}

		succs, err := f.successors(i)
		if err != nil {
			return nil, err
		}
		for _, succ := range succs {
			if defs[succ] == nil {
				defs[succ] = make(map[int]bool)
			}
			changed := false
			for def := range out {
				if !defs[succ][def] {
					defs[succ][def] = true
					changed = true
				}
			}
			if changed {
				queue = append(queue, succ)
			}
		}
	}
	return defs, nil
}

func writesR0(ins asm.Instruction) bool {
	switch class := ins.OpCode.Class(); {
	case class.IsALU(), class == asm.LdXClass:
		return ins.Dst == asm.R0
	case class == asm.LdClass:
		// legacy packet loads implicitly write r0
		return ins.Dst == asm.R0 || ins.OpCode.Mode() == asm.AbsMode || ins.OpCode.Mode() == asm.IndMode
	case class == asm.StXClass:
		switch ins.OpCode.AtomicOp() {
		case asm.InvalidAtomic, asm.AddAtomic, asm.AndAtomic, asm.OrAtomic, asm.XorAtomic:
			return false
		case asm.CmpXchg:
			return true
		default:
			// fetch and load-acquire variants write one of their operands
			return ins.Src == asm.R0 || ins.Dst == asm.R0
		}
	default:
		return ins.OpCode.JumpOp() == asm.Call
	}
}

// r0Immediate returns the TC action set by an instruction writing r0 with an immediate
func r0Immediate(ins asm.Instruction) (int32, bool) {
	switch {
	case ins.OpCode == asm.Mov.Op(asm.ImmSource), ins.OpCode == asm.Mov.Op32(asm.ImmSource):
	// a referenced constant is patched at load time, by LOAD_CONSTANT for instance
	case ins.IsConstantLoad(asm.DWord) && ins.Reference() == "":
	default:
		return 0, false
	}
	// the kernel only reads the low 32 bits of r0
	return int32(ins.Constant), true
}

func TestTCActUnspec(t *testing.T) {
	t.Run("static", func(t *testing.T) {
		if err := checkTCActUnspec(loadColSpec(t).Programs); err != nil {
			t.Error(err)
		}
	})

	t.Run("ko", func(t *testing.T) {
		insts := asm.Instructions{
			asm.Mov.Imm(asm.R0, probes.TCActOk),
			asm.Return(),
		}
		progSpecs := make(map[string]*ebpf.ProgramSpec)
		progSpecs["test"] = &ebpf.ProgramSpec{
			Name:         "test",
			Type:         ebpf.SchedCLS,
			Instructions: insts,
		}

		err := probes.CheckUnspecReturnCode(progSpecs)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("hardened", func(t *testing.T) {
		tests := []struct {
			name  string
			insts asm.Instructions
			ok    bool
		}{
			{
				name:  "unspec",
				insts: asm.Instructions{asm.Mov.Imm(asm.R0, probes.TCActUnspec), asm.Return()},
				ok:    true,
			},
			{
				name:  "unspec-32-bits",
				insts: asm.Instructions{asm.Mov.Imm32(asm.R0, probes.TCActUnspec), asm.Return()},
				ok:    true,
			},
			{
				name:  "unspec-dword",
				insts: asm.Instructions{asm.LoadImm(asm.R0, 0xffffffff, asm.DWord), asm.Return()},
				ok:    true,
			},
			{
				name: "unspec-on-every-path",
				insts: asm.Instructions{
					asm.FnKtimeGetNs.Call(),
					asm.JEq.Imm(asm.R0, 0, "zero"),
					asm.Mov.Imm(asm.R0, probes.TCActUnspec),
					asm.Ja.Label("exit"),
					asm.Mov.Imm(asm.R0, probes.TCActUnspec).WithSymbol("zero"),
					asm.Return().WithSymbol("exit"),
				},
				ok: true,
			},
			{
				name:  "ok",
				insts: asm.Instructions{asm.Mov.Imm(asm.R0, probes.TCActOk), asm.Return()},
			},
			{
				name:  "uninitialized",
				insts: asm.Instructions{asm.Return()},
			},
			{
				name: "helper-result",
				insts: asm.Instructions{
					asm.Mov.Imm(asm.R0, probes.TCActUnspec),
					asm.FnKtimeGetNs.Call(),
					asm.Return(),
				},
			},
			{
				name: "subprogram-result",
				insts: asm.Instructions{
					asm.Mov.Imm(asm.R0, probes.TCActUnspec),
					asm.Call.Label("subprogram"),
					asm.Return(),
					asm.Mov.Imm(asm.R0, probes.TCActUnspec).WithSymbol("subprogram"),
					asm.Return(),
				},
			},
			{
				name: "register-move",
				insts: asm.Instructions{
					asm.Mov.Imm(asm.R0, probes.TCActUnspec),
					asm.Mov.Imm(asm.R6, probes.TCActOk),
					asm.Mov.Reg(asm.R0, asm.R6),
					asm.Return(),
				},
			},
			{
				name:  "load-time-constant",
				insts: asm.Instructions{asm.LoadImm(asm.R0, 0xffffffff, asm.DWord).WithReference("tc_act"), asm.Return()},
			},
			{
				name: "jump-to-exit",
				insts: asm.Instructions{
					asm.Mov.Imm(asm.R0, probes.TCActOk),
					asm.JEq.Imm(asm.R1, 0, "exit"),
					asm.Mov.Imm(asm.R0, probes.TCActUnspec),
					asm.Return(),
					asm.Return().WithSymbol("exit"),
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				err := checkTCActUnspec(map[string]*ebpf.ProgramSpec{
					"test": {Name: "test", Type: ebpf.SchedCLS, Instructions: test.insts},
				})
				if test.ok {
					assert.NoError(t, err)
				} else {
					assert.Error(t, err)
				}
			})
		}
	})

	t.Run("rawpacket", func(t *testing.T) {
		filters := []rawpacket.Filter{
			{
				RuleID:    "ok",
				BPFFilter: "tcp dst port 5555 and tcp[tcpflags] == tcp-syn",
			},
		}
		progSpecs, err := rawpacket.FiltersToProgramSpecs(1, 2, filters, rawpacket.DefaultProgOpts())
		if err != nil {
			t.Error(err)
		}

		dropFilters := []rawpacket.Filter{
			{
				RuleID:    "drop",
				BPFFilter: "tcp dst port 5555",
				Policy:    rawpacket.PolicyDrop,
				Pid:       123,
			},
		}
		dropOpts := rawpacket.DefaultProgOpts()
		dropOpts.WithProgPrefix("raw_packet_drop_action_")
		dropOpts.WithDropStatsMapFd(3)
		dropSpecs, err := rawpacket.DropActionsToProgramSpecs(1, 2, dropFilters, dropOpts)
		if err != nil {
			t.Error(err)
		}

		progSpecsMap := make(map[string]*ebpf.ProgramSpec)
		for _, progSpec := range append(progSpecs, dropSpecs...) {
			progSpecsMap[progSpec.Name] = progSpec
		}
		if err := probes.CheckUnspecReturnCode(progSpecsMap); err != nil {
			t.Error(err)
		}
		if err := checkTCActUnspec(progSpecsMap); err != nil {
			t.Error(err)
		}
	})
}
