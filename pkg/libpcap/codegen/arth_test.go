// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestGenLoadi(t *testing.T) {
	cs := newEthernetCS()
	a := GenLoadi(cs, 42)
	if a == nil {
		t.Fatal("GenLoadi returned nil")
	}
	if a.S == nil {
		t.Fatal("no statements")
	}
	// First stmt: LD|IMM with k=42
	if a.S.S.Code != int(bpf.BPF_LD|bpf.BPF_IMM) {
		t.Errorf("code = %#x, want LD|IMM", a.S.S.Code)
	}
	if a.S.S.K != 42 {
		t.Errorf("k = %d, want 42", a.S.S.K)
	}
	// Second stmt: ST to register
	if a.S.Next == nil {
		t.Fatal("expected store statement")
	}
	if a.S.Next.S.Code != int(bpf.BPF_ST) {
		t.Errorf("store code = %#x, want ST", a.S.Next.S.Code)
	}
	if a.Regno < 0 {
		t.Error("no register allocated")
	}
}

func TestGenLoadlen(t *testing.T) {
	cs := newEthernetCS()
	a := GenLoadlen(cs)
	if a == nil {
		t.Fatal("GenLoadlen returned nil")
	}
	if a.S.S.Code != int(bpf.BPF_LD|bpf.BPF_LEN) {
		t.Errorf("code = %#x, want LD|LEN", a.S.S.Code)
	}
}

func TestGenNegArth(t *testing.T) {
	cs := newEthernetCS()
	a := GenLoadi(cs, 10)
	a = GenNeg(cs, a)
	if a == nil {
		t.Fatal("GenNeg returned nil")
	}
}

func TestGenArthAdd(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 10)
	a1 := GenLoadi(cs, 5)
	result := GenArth(cs, int(bpf.BPF_ADD), a0, a1)
	if result == nil {
		t.Fatal("GenArth(ADD) returned nil")
	}
	if cs.Err != nil {
		t.Fatalf("unexpected error: %v", cs.Err)
	}
}

func TestGenArthDivByZero(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 10)
	a1 := GenLoadi(cs, 0)
	result := GenArth(cs, int(bpf.BPF_DIV), a0, a1)
	if result != nil {
		t.Error("expected nil for division by zero")
	}
	if cs.Err == nil {
		t.Error("expected error for division by zero")
	}
}

func TestGenArthShiftTooLarge(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 1)
	a1 := GenLoadi(cs, 32)
	result := GenArth(cs, int(bpf.BPF_LSH), a0, a1)
	if result != nil {
		t.Error("expected nil for shift > 31")
	}
	if cs.Err == nil {
		t.Error("expected error for shift > 31")
	}
}

func TestGenRelationEq(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 10)
	a1 := GenLoadi(cs, 10)
	b := GenRelation(cs, int(bpf.BPF_JEQ), a0, a1, 0)
	if b == nil {
		t.Fatal("GenRelation(JEQ) returned nil")
	}
}

func TestGenRelationGt(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 10)
	a1 := GenLoadi(cs, 5)
	b := GenRelation(cs, int(bpf.BPF_JGT), a0, a1, 0)
	if b == nil {
		t.Fatal("GenRelation(JGT) returned nil")
	}
}

func TestGenRelationReversed(t *testing.T) {
	cs := newEthernetCS()
	a0 := GenLoadi(cs, 10)
	a1 := GenLoadi(cs, 5)
	b := GenRelation(cs, int(bpf.BPF_JGT), a0, a1, 1)
	if b == nil {
		t.Fatal("GenRelation(JGT reversed) returned nil")
	}
	if !b.Sense {
		t.Error("expected reversed sense")
	}
}

func TestGenLess(t *testing.T) {
	cs := newEthernetCS()
	b := GenLess(cs, 100)
	if b == nil {
		t.Fatal("GenLess returned nil")
	}
	// less 100 = NOT (len > 100) = NOT (len JGT 100)
	if !b.Sense {
		t.Error("expected negated block")
	}
}

func TestGenGreater(t *testing.T) {
	cs := newEthernetCS()
	b := GenGreater(cs, 1000)
	if b == nil {
		t.Fatal("GenGreater returned nil")
	}
	if b.S.K != 1000 {
		t.Errorf("k = %d, want 1000", b.S.K)
	}
}

func TestGenBroadcastEther(t *testing.T) {
	cs := newEthernetCS()
	b := GenBroadcast(cs, QDefault)
	if b == nil {
		t.Fatal("GenBroadcast(default) returned nil")
	}
}

func TestGenMulticastEther(t *testing.T) {
	cs := newEthernetCS()
	b := GenMulticast(cs, QDefault)
	if b == nil {
		t.Fatal("GenMulticast(default) returned nil")
	}
}

func TestGenMulticastIP(t *testing.T) {
	cs := newEthernetCS()
	b := GenMulticast(cs, QIP)
	if b == nil {
		t.Fatal("GenMulticast(IP) returned nil")
	}
}

func TestGenMulticastIPv6(t *testing.T) {
	cs := newEthernetCS()
	b := GenMulticast(cs, QIPv6)
	if b == nil {
		t.Fatal("GenMulticast(IPv6) returned nil")
	}
}

func TestGenLoadLinkProto(t *testing.T) {
	cs := newEthernetCS()
	inst := GenLoadi(cs, 0)
	a := GenLoad(cs, QLink, inst, 1)
	if a == nil {
		t.Fatal("GenLoad(Q_LINK) returned nil")
	}
}

func TestGenLoadIPProto(t *testing.T) {
	cs := newEthernetCS()
	inst := GenLoadi(cs, 9) // offset 9 = IP protocol field
	a := GenLoad(cs, QIP, inst, 1)
	if a == nil {
		t.Fatal("GenLoad(Q_IP) returned nil")
	}
	// Should have a protocol check block
	if a.B == nil {
		t.Error("expected protocol check block")
	}
}

func TestGenLoadBadSize(t *testing.T) {
	cs := newEthernetCS()
	inst := GenLoadi(cs, 0)
	a := GenLoad(cs, QLink, inst, 3) // invalid size
	if a != nil {
		t.Error("expected nil for invalid size")
	}
	if cs.Err == nil {
		t.Error("expected error for invalid size")
	}
}
