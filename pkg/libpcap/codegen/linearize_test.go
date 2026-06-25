// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestLinearizeSimpleReturn(t *testing.T) {
	cs := newEthernetCS()
	// Empty filter → accept all (single return block)
	err := FinishParse(cs, nil)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}
	if len(insns) != 1 {
		t.Fatalf("len = %d, want 1", len(insns))
	}
	// Should be: ret #snaplen
	if insns[0].Code != uint16(bpf.BPF_RET|bpf.BPF_K) {
		t.Errorf("code = %#x, want BPF_RET|BPF_K", insns[0].Code)
	}
	if insns[0].K != 262144 {
		t.Errorf("k = %d, want 262144", insns[0].K)
	}
}

func TestLinearizeIP(t *testing.T) {
	cs := newEthernetCS()
	// "ip" → ethertype == 0x0800
	b := GenLinktype(cs, EthertypeIP)
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}

	// Expected structure (unoptimized):
	// (000) ldh [12]              — load ethertype
	// (001) jeq #0x800 jt 2 jf 3 — check IP
	// (002) ret #262144           — accept
	// (003) ret #0                — reject

	if len(insns) < 3 {
		t.Fatalf("len = %d, want >= 3", len(insns))
	}

	// First instruction should be load halfword at offset 12
	if insns[0].Code != uint16(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H) {
		t.Errorf("insns[0].code = %#x, want LD|ABS|H (%#x)",
			insns[0].Code, bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H)
	}
	if insns[0].K != 12 {
		t.Errorf("insns[0].k = %d, want 12", insns[0].K)
	}

	// Second instruction should be JEQ with ethertype IP
	if insns[1].Code != uint16(bpf.BPF_JMP|bpf.BPF_JEQ|bpf.BPF_K) {
		t.Errorf("insns[1].code = %#x, want JMP|JEQ|K", insns[1].Code)
	}
	if insns[1].K != EthertypeIP {
		t.Errorf("insns[1].k = %#x, want %#x", insns[1].K, EthertypeIP)
	}

	// Accept and reject returns
	hasAccept := false
	hasReject := false
	for _, insn := range insns {
		if insn.Code == uint16(bpf.BPF_RET|bpf.BPF_K) {
			if insn.K == 262144 {
				hasAccept = true
			}
			if insn.K == 0 {
				hasReject = true
			}
		}
	}
	if !hasAccept {
		t.Error("missing accept instruction (ret #262144)")
	}
	if !hasReject {
		t.Error("missing reject instruction (ret #0)")
	}
}

func TestLinearizeTCP(t *testing.T) {
	cs := newEthernetCS()
	// "tcp" → (ethertype=IP && proto=6) || (ethertype=IPv6 && next-header=6)
	b := GenProtoAbbrev(cs, QTCP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(TCP) returned nil")
	}
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}

	// Should produce a non-trivial program
	if len(insns) < 5 {
		t.Fatalf("len = %d, want >= 5 for 'tcp'", len(insns))
	}

	// Dump for debugging
	t.Logf("TCP program (%d instructions):", len(insns))
	for i, insn := range insns {
		t.Logf("  (%03d) %s", i, bpf.Image(insn, i))
	}

	// Verify it starts with loading ethertype
	if insns[0].Code != uint16(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H) {
		t.Errorf("insns[0] = %#x, want LD|ABS|H (load ethertype)", insns[0].Code)
	}

	// Verify there's at least one check for protocol 6 (TCP)
	hasTCPCheck := false
	for _, insn := range insns {
		if insn.Code == uint16(bpf.BPF_JMP|bpf.BPF_JEQ|bpf.BPF_K) && insn.K == 6 {
			hasTCPCheck = true
			break
		}
	}
	if !hasTCPCheck {
		t.Error("missing TCP protocol check (jeq #0x6)")
	}
}

func TestLinearizeIPEndToEndWithImage(t *testing.T) {
	cs := newEthernetCS()
	b := GenLinktype(cs, EthertypeIP)
	FinishParse(cs, b)

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}

	// Use bpf.Image to format — this validates our Image formatter matches
	t.Logf("IP program (%d instructions):", len(insns))
	for i, insn := range insns {
		t.Logf("  %s", bpf.Image(insn, i))
	}
}

func TestLinearizeHostIP(t *testing.T) {
	cs := newEthernetCS()
	// "src host 192.168.1.1"
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QIP, QSrc, QHost)
	if b == nil {
		t.Fatal("GenHost returned nil")
	}
	FinishParse(cs, b)

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}

	if len(insns) < 4 {
		t.Fatalf("len = %d, want >= 4", len(insns))
	}

	t.Logf("'src host 192.168.1.1' program (%d instructions):", len(insns))
	for i, insn := range insns {
		t.Logf("  %s", bpf.Image(insn, i))
	}
}

func TestLinearizePort(t *testing.T) {
	cs := newEthernetCS()
	// "tcp dst port 80"
	b := GenPort(cs, 80, IPProtoTCP, QDst)
	if b == nil {
		t.Fatal("GenPort returned nil")
	}
	FinishParse(cs, b)

	insns, err := IcodeToFcode(&cs.IC, cs.IC.Root)
	if err != nil {
		t.Fatalf("IcodeToFcode = %v", err)
	}

	t.Logf("'tcp dst port 80' program (%d instructions):", len(insns))
	for i, insn := range insns {
		t.Logf("  %s", bpf.Image(insn, i))
	}

	// Should have an instruction checking port 80 (0x50)
	hasPort80 := false
	for _, insn := range insns {
		if insn.Code == uint16(bpf.BPF_JMP|bpf.BPF_JEQ|bpf.BPF_K) && insn.K == 80 {
			hasPort80 = true
			break
		}
	}
	if !hasPort80 {
		t.Error("missing port 80 check")
	}
}
