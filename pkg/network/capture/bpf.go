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

// cbpfcResultLabel is the label cbpfc's generated eBPF code jumps to once the
// filter result is computed. buildProgram must place an anchor instruction
// with this exact symbol immediately after splicing in the filter
// instructions, or the program fails to load with "unsatisfied program
// reference".
const cbpfcResultLabel = "cbpfc_dd_pcap_result"

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
//   - R0,R3,R4,R5 = scratch (Working) — cbpfc may clobber these. cbpfc
//     requires PacketStart/PacketEnd to be disjoint from Working, so R1/R2
//     are deliberately excluded from Working; Result (R3) is allowed to
//     overlap Working.
//   - R6–R9 are NOT touched by cbpfc, which lets the caller preserve:
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
		// Working registers must be disjoint from PacketStart/PacketEnd
		// (cbpfc requirement). R0,R4,R5 are free scratch at this point in
		// buildProgram; R3 doubles as Result, which cbpfc allows to overlap
		// Working. R6–R9 are off-limits so cbpfc never clobbers our
		// long-lived skb/reservation/metadata values.
		Working: [4]asm.Register{
			asm.R0,
			asm.R3,
			asm.R4,
			asm.R5,
		},
		// StackOffset = 0: the surrounding program does not use the stack
		// before entering the filter, so cbpfc can start at offset 0.
		StackOffset: 0,
		LabelPrefix: "cbpfc_dd_pcap_",
		ResultLabel: cbpfcResultLabel,
	}

	insts, err := cbpfc.ToEBPF(bpfInsts, opts)
	if err != nil {
		return nil, fmt.Errorf("converting BPF to eBPF: %w", err)
	}
	return insts, nil
}

// ethHeaderLen is the fixed Ethernet II header length in bytes. VLAN tags
// (802.1Q/QinQ) are not parsed in this MVP — a VLAN-tagged frame falls
// through to the "unrecognised EtherType" case below and is truncated at the
// Ethernet boundary. This is a documented under-capture (header-only stays
// safe, just less precise for VLAN traffic), not a correctness bug.
const ethHeaderLen = 14

// ipv6HeaderLen is the fixed IPv6 base header length. IPv6 extension header
// chains are not walked in this MVP: if the Next Header value isn't directly
// TCP or UDP, the packet is truncated at the IPv6 base header boundary
// instead of following the chain to the real L4 header. Same safety
// trade-off as VLANs above.
const ipv6HeaderLen = 40

// buildHeaderOffsetInsts returns eBPF instructions that compute, in R4, the
// dynamic header-only capture length for the packet already loaded into the
// ring buffer reservation at base+recDataOffset (see buildProgram's step 3) —
// i.e. the offset where the packet's own L3/L4 headers end, capped by
// snapLen. This implements the "capped dynamic" scheme from Confluence NET
// "Dynamic Header Snap Length — Implementation" (page 7027392746):
// captured = min(L7_offset, CAP). No application payload is ever included:
// every path below stops at or before the true L7 boundary, and the final
// clamp against loadedLen guarantees the result never claims more bytes than
// were actually loaded.
//
// Register usage (caller contract, matches buildProgram's allocation):
//   - R6 (skb), R7 (reservation ptr), R8 (skb->len), R9 (skb->ifindex) must
//     hold their buildProgram values on entry and are left untouched.
//   - R0-R5 are used as scratch; the result is left in R4.
//
// Every fixed-offset read below is only emitted when snapLen is large enough
// to make it verifier-safe (the ring buffer reservation gives the base
// pointer exactly snapLen bytes of provable range) — this is a Go-time
// decision, not a runtime one, since snapLen is fixed at program-build time.
// The one read at a *dynamic* offset (the TCP data-offset byte, which sits
// after a variable-length IPv4 header) is instead guarded by a runtime bound
// check on the offset register itself, following the same
// prove-it-then-use-it idiom as the bpf_skb_load_bytes clamp in buildProgram.
func buildHeaderOffsetInsts(snapLen uint32) asm.Instructions {
	var insts asm.Instructions

	// R1 = base pointer to the loaded packet bytes.
	insts = append(insts,
		asm.Mov.Reg(asm.R1, asm.R7),
		asm.Add.Imm(asm.R1, int32(ringBufMetaSize)),
	)

	// R5 = loadedLen = min(skb->len, snapLen) — the hard ceiling on how many
	// bytes of this packet actually landed in the reservation.
	insts = append(insts,
		asm.Mov.Reg(asm.R0, asm.R8),
		asm.JLE.Imm(asm.R0, int32(snapLen), "hdr_loadedlen_ok"),
		asm.Mov.Imm(asm.R0, int32(snapLen)),
		asm.Mov.Reg(asm.R5, asm.R0).WithSymbol("hdr_loadedlen_ok"),
	)

	// R4 = running offset estimate. Every branch below sets its best-known
	// boundary and jumps to hdr_finalize, which clamps R4 to R5 — so it is
	// safe for a branch to leave R4 larger than what was actually loaded.
	insts = append(insts, asm.Mov.Imm(asm.R4, 0))

	if snapLen < ethHeaderLen+2 {
		// Cannot even read the EtherType field safely — nothing more to
		// compute. R4 stays 0; hdr_finalize clamps it to loadedLen.
		insts = append(insts, asm.Ja.Label("hdr_finalize"))
		return append(insts, appendFinalizeInsts()...)
	}

	insts = append(insts, asm.Mov.Imm(asm.R4, ethHeaderLen))

	// EtherType is bytes [12:14]. Loaded as a plain memory half-word it comes
	// back in host (little-endian) order, not network order — HostTo(BE, ...)
	// is the standard bpf_ntohs()-equivalent fixup before comparing against
	// the usual big-endian constants (0x0800, 0x86DD).
	insts = append(insts,
		asm.LoadMem(asm.R3, asm.R1, ethHeaderLen-2, asm.Half),
		asm.HostTo(asm.BE, asm.R3, asm.Half),
		asm.JEq.Imm(asm.R3, 0x0800, "hdr_ipv4"),
		asm.JEq.Imm(asm.R3, 0x86DD, "hdr_ipv6"),
		// Neither IPv4 nor IPv6 (ARP, etc.) — stop at the Ethernet boundary.
		asm.Ja.Label("hdr_finalize"),
	)

	insts = append(insts, buildIPv4OffsetInsts(snapLen)...)
	insts = append(insts, buildIPv6OffsetInsts(snapLen)...)
	insts = append(insts, appendFinalizeInsts()...)
	return insts
}

// buildIPv4OffsetInsts appends the "hdr_ipv4" branch: reads the IHL nibble to
// get the L3 length, then (if there's room) the protocol byte to dispatch to
// a TCP- or UDP-specific L4 length. Falls through to hdr_finalize on every
// exit path, leaving its best-known boundary in R4.
func buildIPv4OffsetInsts(snapLen uint32) asm.Instructions {
	const ipv4ProtoOff = ethHeaderLen + 9 // protocol byte position

	insts := asm.Instructions{
		// R4 = ethHeaderLen as a placeholder anchor for the "hdr_ipv4" label;
		// overwritten immediately below once the real IHL-based length is known.
		asm.Mov.Imm(asm.R4, ethHeaderLen).WithSymbol("hdr_ipv4"),
	}

	if snapLen < ethHeaderLen+1 {
		return append(insts, asm.Ja.Label("hdr_finalize"))
	}

	insts = append(insts,
		asm.LoadMem(asm.R3, asm.R1, ethHeaderLen, asm.Byte), // R3 = IP header byte 0
		asm.And.Imm(asm.R3, 0x0F),                           // R3 = IHL
		asm.LSh.Imm(asm.R3, 2),                              // R3 = IHL*4 = ip_len (0..60)
		asm.Mov.Imm(asm.R4, ethHeaderLen),
		asm.Add.Reg(asm.R4, asm.R3), // R4 = ethHeaderLen + ip_len (L3 boundary, our fallback)
	)

	if snapLen < ipv4ProtoOff+1 {
		return append(insts, asm.Ja.Label("hdr_finalize"))
	}

	insts = append(insts,
		asm.LoadMem(asm.R0, asm.R1, ipv4ProtoOff, asm.Byte),
		asm.JEq.Imm(asm.R0, 6, "hdr_ipv4_tcp"),
		asm.JEq.Imm(asm.R0, 17, "hdr_ipv4_udp"),
		// Other L4 protocol — use the L3 boundary already in R4.
		asm.Ja.Label("hdr_finalize"),

		asm.Add.Imm(asm.R4, 8).WithSymbol("hdr_ipv4_udp"), // UDP header is fixed 8 bytes
		asm.Ja.Label("hdr_finalize"),
	)

	// hdr_ipv4_tcp: TCP header starts at a *dynamic* offset (R4 = ethHeaderLen
	// + ip_len). Reading its data-offset byte (at +12) needs a runtime proof
	// that R4+13 <= snapLen before the verifier will accept the pointer
	// arithmetic — mirrors buildProgram's bpf_skb_load_bytes length clamp.
	insts = append(insts,
		asm.Mov.Reg(asm.R0, asm.R4).WithSymbol("hdr_ipv4_tcp"),
		asm.Add.Imm(asm.R0, 13),
		asm.JGT.Imm(asm.R0, int32(snapLen), "hdr_finalize"), // not enough room — use L3 boundary
		asm.Mov.Reg(asm.R2, asm.R1),
		asm.Add.Reg(asm.R2, asm.R4), // R2 = base + (ethHeaderLen + ip_len) = TCP header start
		asm.LoadMem(asm.R3, asm.R2, 12, asm.Byte),
		asm.RSh.Imm(asm.R3, 4), // high nibble = TCP data offset (words)
		asm.LSh.Imm(asm.R3, 2), // *4 = TCP header length (20..60)
		asm.Add.Reg(asm.R4, asm.R3),
		asm.Ja.Label("hdr_finalize"),
	)

	return insts
}

// buildIPv6OffsetInsts appends the "hdr_ipv6" branch. The IPv6 base header is
// a fixed 40 bytes, so — unlike IPv4 — every offset here is a compile-time
// constant and no runtime bound check is needed; snapLen is checked at
// Go-time instead to decide whether to emit each read at all.
func buildIPv6OffsetInsts(snapLen uint32) asm.Instructions {
	const (
		nextHeaderOff = ethHeaderLen + 6 // IPv6 Next Header byte position
		l3Boundary    = ethHeaderLen + ipv6HeaderLen
		tcpDataOffOff = l3Boundary + 12 // TCP data-offset byte, past the IPv6 base header
	)

	insts := asm.Instructions{
		asm.Mov.Imm(asm.R4, l3Boundary).WithSymbol("hdr_ipv6"),
	}

	if snapLen < nextHeaderOff+1 {
		return append(insts, asm.Ja.Label("hdr_finalize"))
	}

	// Only dispatch to the TCP branch if snapLen is large enough to safely
	// read its data-offset byte — otherwise treat TCP the same as any other
	// protocol and fall back to the L3 boundary already in R4. This avoids
	// emitting a jump to "hdr_ipv6_tcp" when that label's block is skipped
	// below (an unanchored reference is a verifier/assembler error, not just
	// an imprecise result).
	canReadTCPDataOffset := snapLen >= tcpDataOffOff+1

	insts = append(insts, asm.LoadMem(asm.R0, asm.R1, nextHeaderOff, asm.Byte))
	if canReadTCPDataOffset {
		insts = append(insts, asm.JEq.Imm(asm.R0, 6, "hdr_ipv6_tcp"))
	}
	insts = append(insts,
		asm.JEq.Imm(asm.R0, 17, "hdr_ipv6_udp"),
		// Extension header chain, other protocol, or (if !canReadTCPDataOffset)
		// TCP without enough room to read its header — use the L3 boundary
		// already in R4 rather than following the chain (MVP limitation).
		asm.Ja.Label("hdr_finalize"),

		asm.Add.Imm(asm.R4, 8).WithSymbol("hdr_ipv6_udp"),
		asm.Ja.Label("hdr_finalize"),
	)

	if canReadTCPDataOffset {
		insts = append(insts,
			asm.LoadMem(asm.R3, asm.R1, tcpDataOffOff, asm.Byte).WithSymbol("hdr_ipv6_tcp"),
			asm.RSh.Imm(asm.R3, 4),
			asm.LSh.Imm(asm.R3, 2),
			asm.Add.Reg(asm.R4, asm.R3),
			asm.Ja.Label("hdr_finalize"),
		)
	}

	return insts
}

// appendFinalizeInsts anchors "hdr_finalize" and clamps R4 to R5 (loadedLen),
// guaranteeing the result never claims more bytes than were actually loaded
// into the reservation — the safety net every branch above relies on.
func appendFinalizeInsts() asm.Instructions {
	return asm.Instructions{
		asm.JLE.Reg(asm.R4, asm.R5, "hdr_have_final").WithSymbol("hdr_finalize"),
		asm.Mov.Reg(asm.R4, asm.R5),
		asm.Mov.Reg(asm.R4, asm.R4).WithSymbol("hdr_have_final"),
	}
}
