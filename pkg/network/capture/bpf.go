// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"fmt"

	"github.com/cilium/ebpf/asm"
	"github.com/cloudflare/cbpfc"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
	"golang.org/x/net/bpf"
)

// compileBPFFilter compiles a tcpdump-style BPF filter string into classic BPF
// raw instructions using libpcap via gopacket/pcap.
//
// snapLen limits which packets the filter considers "long enough" for offset
// accesses. An empty filter string returns a nil slice (match-all).
func compileBPFFilter(filter string, snapLen uint32) ([]bpf.RawInstruction, error) {
	if filter == "" {
		// Empty filter — match everything. Return nil so callers can skip cbpfc.
		return nil, nil
	}

	// pcap.CompileBPFFilter uses libpcap to parse tcpdump syntax and returns
	// a slice of pcap.BPFInstruction (which mirrors struct bpf_insn).
	pcapInsts, err := pcap.CompileBPFFilter(layers.LinkTypeEthernet, int(snapLen), filter)
	if err != nil {
		return nil, fmt.Errorf("compiling BPF filter %q: %w", filter, err)
	}

	raw := make([]bpf.RawInstruction, len(pcapInsts))
	for i, pi := range pcapInsts {
		raw[i] = bpf.RawInstruction{
			Op: pi.Code,
			Jt: pi.Jt,
			Jf: pi.Jf,
			K:  pi.K,
		}
	}
	return raw, nil
}

// bpfToEBPF converts classic BPF raw instructions into eBPF assembly instructions
// suitable for inline use inside the capture TC SchedCLS program.
//
// Register contract (must match buildProgram's register allocation):
//   - R1  = packet data start (PacketStart, set by caller before inline jump)
//   - R2  = packet data end   (PacketEnd)
//   - R3  = filter result on exit (non-zero = match)
//   - R1,R2,R3,R4 = scratch (Working) — cbpfc may clobber these
//   - R5–R9 are NOT touched by cbpfc, which lets the caller preserve:
//     R6=skb, R7=ring buf reservation, R8=skb->len, R9=skb->ifindex
//
// The caller must set R1/R2 before the inline filter block, and check R3 after.
func bpfToEBPF(raw []bpf.RawInstruction) (asm.Instructions, error) {
	// Disassemble raw instructions into the richer bpf.Instruction type that
	// cbpfc understands. This mirrors the pattern in the Security Agent's
	// rawpacket/pcap.go FilterToInsts function.
	bpfInsts := make([]bpf.Instruction, len(raw))
	for i, ri := range raw {
		bpfInsts[i] = ri.Disassemble()
	}

	opts := cbpfc.EBPFOpts{
		PacketStart: asm.R1,
		PacketEnd:   asm.R2,
		Result:      asm.R3,
		// Working registers: R1–R4 only. R5–R9 are off-limits so cbpfc
		// never clobbers our long-lived skb/reservation/metadata values.
		Working: [4]asm.Register{
			asm.R1,
			asm.R2,
			asm.R3,
			asm.R4,
		},
		// StackOffset = 0: the surrounding program does not use the stack
		// before entering the filter, so cbpfc can start at offset 0.
		StackOffset: 0,
		LabelPrefix: "cbpfc_dd_pcap_",
		ResultLabel: "cbpfc_dd_pcap_result",
	}

	insts, err := cbpfc.ToEBPF(bpfInsts, opts)
	if err != nil {
		return nil, fmt.Errorf("converting BPF to eBPF: %w", err)
	}
	return insts, nil
}
