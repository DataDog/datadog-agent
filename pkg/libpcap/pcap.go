// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package libpcap implements a pure Go BPF packet filter compiler,
// providing a drop-in replacement for gopacket/pcap's CompileBPFFilter
// and NewBPF functions.
package libpcap

import (
	"bytes"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
	"github.com/DataDog/datadog-agent/pkg/libpcap/grammar"
	"github.com/DataDog/datadog-agent/pkg/libpcap/nameresolver"
	optim "github.com/DataDog/datadog-agent/pkg/libpcap/optimizer"
)

// LinkTypeEthernet is the DLT value for Ethernet (DLT_EN10MB).
const LinkTypeEthernet = 1

// RawInstruction matches the instruction type used by gopacket/pcap.
type RawInstruction struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// CompileBPFFilter compiles a BPF filter expression into raw instructions
// with optimization enabled. This is a drop-in replacement for
// gopacket/pcap.CompileBPFFilter.
func CompileBPFFilter(linkType int, snaplen int, expr string) ([]RawInstruction, error) {
	return compileBPF(linkType, snaplen, expr, true)
}

// CompileBPFFilterNoOpt compiles without optimization (for testing/debugging).
func CompileBPFFilterNoOpt(linkType int, snaplen int, expr string) ([]RawInstruction, error) {
	return compileBPF(linkType, snaplen, expr, false)
}

func compileBPF(linkType int, snaplen int, expr string, optimize bool) ([]RawInstruction, error) {
	resolver := nameresolver.New()
	cs := codegen.NewCompilerState(linkType, snaplen, 0xFFFFFFFF, resolver)

	if err := codegen.InitLinktype(cs); err != nil {
		return nil, fmt.Errorf("failed to initialize link type %d: %w", linkType, err)
	}

	if err := grammar.Parse(cs, expr); err != nil {
		return nil, fmt.Errorf("failed to compile filter '%s': %w", expr, err)
	}
	if cs.Err != nil {
		return nil, fmt.Errorf("failed to compile filter '%s': %w", expr, cs.Err)
	}

	// Empty filter (or null production) → accept all
	if cs.IC.Root == nil {
		cs.IC.Root = codegen.GenRetBlk(cs, snaplen)
	}

	if optimize {
		if err := optim.Optimize(&cs.IC); err != nil {
			return nil, fmt.Errorf("failed to optimize filter '%s': %w", expr, err)
		}
	}

	insns, err := codegen.IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to linearize filter '%s': %w", expr, err)
	}

	// Convert bpf.Instruction to RawInstruction
	raw := make([]RawInstruction, len(insns))
	for i, insn := range insns {
		raw[i] = RawInstruction{
			Code: insn.Code,
			Jt:   insn.Jt,
			Jf:   insn.Jf,
			K:    insn.K,
		}
	}
	return raw, nil
}

// CompileToProgram compiles a filter expression to a bpf.Program.
func CompileToProgram(linkType int, snaplen int, expr string) (*bpf.Program, error) {
	raw, err := CompileBPFFilter(linkType, snaplen, expr)
	if err != nil {
		return nil, err
	}
	insns := make([]bpf.Instruction, len(raw))
	for i, r := range raw {
		insns[i] = bpf.Instruction{Code: r.Code, Jt: r.Jt, Jf: r.Jf, K: r.K}
	}
	return &bpf.Program{Instructions: insns}, nil
}

// DumpFilter compiles a filter with optimization and returns the human-readable BPF dump.
func DumpFilter(linkType int, snaplen int, expr string) (string, error) {
	return dumpFilter(linkType, snaplen, expr, true)
}

// DumpFilterNoOpt compiles without optimization and returns the dump.
func DumpFilterNoOpt(linkType int, snaplen int, expr string) (string, error) {
	return dumpFilter(linkType, snaplen, expr, false)
}

func dumpFilter(linkType int, snaplen int, expr string, optimize bool) (string, error) {
	raw, err := compileBPF(linkType, snaplen, expr, optimize)
	if err != nil {
		return "", err
	}
	insns := make([]bpf.Instruction, len(raw))
	for i, r := range raw {
		insns[i] = bpf.Instruction{Code: r.Code, Jt: r.Jt, Jf: r.Jf, K: r.K}
	}
	prog := &bpf.Program{Instructions: insns}
	var buf bytes.Buffer
	bpf.FprintDump(&buf, prog, 1)
	return buf.String(), nil
}
