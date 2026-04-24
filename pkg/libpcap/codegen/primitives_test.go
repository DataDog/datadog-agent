// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestGenCmp(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	// Ethernet: linktype offset at 12, so OffLinktype.ConstPart = 12
	cs.OffLinktype = AbsOffset{ConstPart: 12}

	// Compare ethertype == 0x0800 (IP)
	b := GenCmp(cs, OrLinktype, 0, bpf.BPF_H, EthertypeIP)
	if b == nil {
		t.Fatal("GenCmp returned nil")
	}

	// Block should have a JEQ jump with k=0x0800
	if b.S.Code != JmpCode(int(bpf.BPF_JEQ)) {
		t.Errorf("block code = %#x, want JMP|JEQ|K", b.S.Code)
	}
	if b.S.K != EthertypeIP {
		t.Errorf("block k = %#x, want %#x", b.S.K, EthertypeIP)
	}

	// Statements should load the value
	if b.Stmts == nil {
		t.Fatal("block has no statements")
	}
	// First statement should be LD|ABS|H (load halfword at absolute offset 12)
	stmt := b.Stmts.S
	if stmt.Code != int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H) {
		t.Errorf("stmt code = %#x, want LD|ABS|H (%#x)", stmt.Code, bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H)
	}
	if stmt.K != 12 {
		t.Errorf("stmt k = %d, want 12", stmt.K)
	}
}

func TestGenMcmp(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	// Masked comparison: (byte[0] & 0x01) == 0x01
	b := GenMcmp(cs, OrPacket, 0, bpf.BPF_B, 0x01, 0x01)
	if b == nil {
		t.Fatal("GenMcmp returned nil")
	}

	// Should have load stmt followed by AND mask stmt
	if b.Stmts == nil {
		t.Fatal("no statements")
	}
	// First: load byte at offset 0
	s := b.Stmts
	if s.S.Code != int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_B) {
		t.Errorf("first stmt = %#x, want LD|ABS|B", s.S.Code)
	}
	// Second: AND with mask 0x01
	s = s.Next
	if s == nil {
		t.Fatal("expected AND mask statement")
	}
	if s.S.Code != int(bpf.BPF_ALU|bpf.BPF_AND|bpf.BPF_K) {
		t.Errorf("mask stmt = %#x, want ALU|AND|K", s.S.Code)
	}
	if s.S.K != 0x01 {
		t.Errorf("mask value = %#x, want 0x01", s.S.K)
	}
}

func TestGenBcmp(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	// Compare 6-byte MAC address at offset 0
	mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	b := GenBcmp(cs, OrPacket, 0, mac)
	if b == nil {
		t.Fatal("GenBcmp returned nil")
	}
	// Should produce a block (the last comparison generated is the outer block)
	// 6 bytes = 1 x 4-byte comparison + 1 x 2-byte comparison
	if b.S.Code != JmpCode(int(bpf.BPF_JEQ)) {
		t.Errorf("block code = %#x, want JMP|JEQ|K", b.S.Code)
	}
}

func TestGenCmpGtLt(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	b := GenCmpGt(cs, OrPacket, 0, bpf.BPF_W, 100)
	if b == nil {
		t.Fatal("GenCmpGt returned nil")
	}
	if b.S.Code != JmpCode(int(bpf.BPF_JGT)) {
		t.Errorf("GT block code = %#x, want JMP|JGT|K", b.S.Code)
	}

	b = GenCmpLt(cs, OrPacket, 0, bpf.BPF_W, 100)
	if b == nil {
		t.Fatal("GenCmpLt returned nil")
	}
	// LT is reversed JGE, so sense should be flipped
	if b.S.Code != JmpCode(int(bpf.BPF_JGE)) {
		t.Errorf("LT block code = %#x, want JMP|JGE|K (reversed)", b.S.Code)
	}
	if !b.Sense {
		t.Error("LT block should have sense=true (reversed)")
	}
}

func TestGenLoadAPacket(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)

	s := GenLoadA(cs, OrPacket, 12, bpf.BPF_H)
	if s == nil {
		t.Fatal("GenLoadA returned nil")
	}
	if s.S.Code != int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_H) {
		t.Errorf("code = %#x, want LD|ABS|H", s.S.Code)
	}
	if s.S.K != 12 {
		t.Errorf("k = %d, want 12", s.S.K)
	}
}

func TestGenLoadALinkpl(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	cs.OffLinkpl = AbsOffset{ConstPart: 14} // Ethernet header
	cs.OffNl = 0                            // IP starts at linkpl

	// Load byte at IP protocol field (offset 9 from network layer)
	s := GenLoadA(cs, OrLinkpl, 9, bpf.BPF_B)
	if s == nil {
		t.Fatal("GenLoadA returned nil")
	}
	// Should be absolute load at 14 + 0 + 9 = 23
	if s.S.Code != int(bpf.BPF_LD|bpf.BPF_ABS|bpf.BPF_B) {
		t.Errorf("code = %#x, want LD|ABS|B", s.S.Code)
	}
	if s.S.K != 23 {
		t.Errorf("k = %d, want 23", s.S.K)
	}
}

func TestGenLoadATranIPv4(t *testing.T) {
	cs := NewCompilerState(1, 256, 0, nil)
	cs.OffLinkpl = AbsOffset{ConstPart: 14}
	cs.OffNl = 0

	// Load transport layer port (offset 0 = src port for TCP/UDP)
	s := GenLoadA(cs, OrTranIPv4, 0, bpf.BPF_H)
	if s == nil {
		t.Fatal("GenLoadA returned nil")
	}
	// First instruction should be LDX|MSH|B to load IP header length
	if s.S.Code != int(bpf.BPF_LDX|bpf.BPF_MSH|bpf.BPF_B) {
		t.Errorf("first code = %#x, want LDX|MSH|B (%#x)", s.S.Code, bpf.BPF_LDX|bpf.BPF_MSH|bpf.BPF_B)
	}
	// Should be followed by LD|IND|H (indirect load using X)
	if s.Next == nil {
		t.Fatal("expected second instruction")
	}
	if s.Next.S.Code != int(bpf.BPF_LD|bpf.BPF_IND|bpf.BPF_H) {
		t.Errorf("second code = %#x, want LD|IND|H", s.Next.S.Code)
	}
}
