// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func newEthernetCS() *CompilerState {
	cs := NewCompilerState(DLTEN10MB, 262144, 0, nil)
	if err := InitLinktype(cs); err != nil {
		panic(err)
	}
	return cs
}

func TestInitLinktypeEthernet(t *testing.T) {
	cs := NewCompilerState(DLTEN10MB, 262144, 0, nil)
	err := InitLinktype(cs)
	if err != nil {
		t.Fatalf("InitLinktype(EN10MB) = %v", err)
	}
	if cs.OffLinktype.ConstPart != 12 {
		t.Errorf("OffLinktype.ConstPart = %d, want 12", cs.OffLinktype.ConstPart)
	}
	if cs.OffLinkpl.ConstPart != 14 {
		t.Errorf("OffLinkpl.ConstPart = %d, want 14", cs.OffLinkpl.ConstPart)
	}
	if cs.OffNl != 0 {
		t.Errorf("OffNl = %d, want 0", cs.OffNl)
	}
	if cs.OffNlNosnap != 3 {
		t.Errorf("OffNlNosnap = %d, want 3", cs.OffNlNosnap)
	}
}

func TestInitLinktypeUnknown(t *testing.T) {
	cs := NewCompilerState(999, 256, 0, nil)
	err := InitLinktype(cs)
	if err == nil {
		t.Error("expected error for unknown DLT")
	}
}

func TestGenLinktypeIP(t *testing.T) {
	cs := newEthernetCS()
	b := GenLinktype(cs, EthertypeIP)
	if b == nil {
		t.Fatal("GenLinktype(IP) returned nil")
	}
	// Should compare ethertype at offset 12 with 0x0800
	if b.S.Code != JmpCode(int(bpf.BPF_JEQ)) {
		t.Errorf("code = %#x, want JMP|JEQ|K", b.S.Code)
	}
	if b.S.K != EthertypeIP {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeIP)
	}
	// Load statement should be LDH at offset 12
	if b.Stmts == nil {
		t.Fatal("no stmts")
	}
	if b.Stmts.S.Code != int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H) {
		t.Errorf("load code = %#x, want LD|ABS|H", b.Stmts.S.Code)
	}
	if b.Stmts.S.K != 12 {
		t.Errorf("load k = %d, want 12", b.Stmts.S.K)
	}
}

func TestGenLinktypeIPv6(t *testing.T) {
	cs := newEthernetCS()
	b := GenLinktype(cs, EthertypeIPv6)
	if b == nil {
		t.Fatal("GenLinktype(IPv6) returned nil")
	}
	if b.S.K != EthertypeIPv6 {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeIPv6)
	}
}

func TestGenLinktypeARP(t *testing.T) {
	cs := newEthernetCS()
	b := GenLinktype(cs, EthertypeARP)
	if b == nil {
		t.Fatal("GenLinktype(ARP) returned nil")
	}
	if b.S.K != EthertypeARP {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeARP)
	}
}

func TestGenEtherLinktype802_2(t *testing.T) {
	cs := newEthernetCS()
	// LLCSAP_ISONS (0xfe) is an LLC SAP — triggers 802.2 encapsulation check
	b := genEtherLinktype(cs, LLCSAPISONs)
	if b == nil {
		t.Fatal("genEtherLinktype(ISONS) returned nil")
	}
	// The result should be a block checking DSAP+SSAP at the LLC layer
	// after verifying the frame is 802.3 (length <= 1500)
}

func TestGenSnap(t *testing.T) {
	cs := newEthernetCS()
	b := genSnap(cs, 0x000000, EthertypeIP)
	if b == nil {
		t.Fatal("genSnap returned nil")
	}
	// SNAP header is 8 bytes: AA AA 03 00 00 00 08 00
	// GenBcmp breaks this into 4+4 byte comparisons
}

func TestGenLinktypeWithFinishParse(t *testing.T) {
	// End-to-end: compile "ip" equivalent (ethertype == 0x0800)
	cs := newEthernetCS()
	b := GenLinktype(cs, EthertypeIP)
	if b == nil {
		t.Fatal("GenLinktype(IP) returned nil")
	}

	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil")
	}

	// Walk the CFG: root should be the ethertype check block
	root := cs.IC.Root
	// JT should lead to accept (return snaplen=262144)
	accept := JT(root)
	if accept == nil {
		t.Fatal("JT(root) is nil")
	}
	if accept.S.K != 262144 {
		t.Errorf("accept.K = %d, want 262144", accept.S.K)
	}
	// JF should lead to reject (return 0)
	reject := JF(root)
	if reject == nil {
		t.Fatal("JF(root) is nil")
	}
	if reject.S.K != 0 {
		t.Errorf("reject.K = %d, want 0", reject.S.K)
	}
}
