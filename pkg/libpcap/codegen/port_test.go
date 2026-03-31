// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import "testing"

func TestGenPortTCPSrc(t *testing.T) {
	cs := newEthernetCS()
	b := GenPort(cs, 80, IPProtoTCP, QSrc)
	if b == nil {
		t.Fatal("GenPort(80, TCP, SRC) returned nil")
	}
	if cs.Err != nil {
		t.Fatalf("unexpected error: %v", cs.Err)
	}
}

func TestGenPortTCPDst(t *testing.T) {
	cs := newEthernetCS()
	b := GenPort(cs, 443, IPProtoTCP, QDst)
	if b == nil {
		t.Fatal("GenPort(443, TCP, DST) returned nil")
	}
}

func TestGenPortUDPDefault(t *testing.T) {
	cs := newEthernetCS()
	b := GenPort(cs, 53, IPProtoUDP, QDefault)
	if b == nil {
		t.Fatal("GenPort(53, UDP, DEFAULT) returned nil")
	}
}

func TestGenPortAnyProto(t *testing.T) {
	cs := newEthernetCS()
	// ProtoUndef means check TCP, UDP, and SCTP
	b := GenPort(cs, 8080, ProtoUndef, QDefault)
	if b == nil {
		t.Fatal("GenPort(8080, UNDEF, DEFAULT) returned nil")
	}
}

func TestGenPort6(t *testing.T) {
	cs := newEthernetCS()
	b := GenPort6(cs, 80, IPProtoTCP, QDst)
	if b == nil {
		t.Fatal("GenPort6(80, TCP, DST) returned nil")
	}
}

func TestGenPortrange(t *testing.T) {
	cs := newEthernetCS()
	b := GenPortrange(cs, 8000, 9000, ProtoUndef, QDefault)
	if b == nil {
		t.Fatal("GenPortrange(8000-9000) returned nil")
	}
}

func TestGenPortrange6(t *testing.T) {
	cs := newEthernetCS()
	b := GenPortrange6(cs, 8000, 9000, IPProtoTCP, QSrc)
	if b == nil {
		t.Fatal("GenPortrange6(8000-9000, TCP, SRC) returned nil")
	}
}

func TestGenPortTCPEndToEnd(t *testing.T) {
	cs := newEthernetCS()
	// "tcp dst port 80"
	b := GenPort(cs, 80, IPProtoTCP, QDst)
	if b == nil {
		t.Fatal("GenPort returned nil")
	}
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil")
	}
}

func TestGenIPfrag(t *testing.T) {
	cs := newEthernetCS()
	b := genIPfrag(cs)
	if b == nil {
		t.Fatal("genIPfrag returned nil")
	}
	// Should be a JSET with k=0x1fff, negated
	if b.S.K != 0x1fff {
		t.Errorf("k = %#x, want 0x1fff", b.S.K)
	}
	if !b.Sense {
		t.Error("expected sense=true (negated)")
	}
}
