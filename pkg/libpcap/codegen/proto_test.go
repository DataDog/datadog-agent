// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestGenProtoAbbrevIP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QIP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_IP) returned nil")
	}
	// "ip" should generate ethertype == 0x0800
	if b.S.K != EthertypeIP {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeIP)
	}
}

func TestGenProtoAbbrevIPv6(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QIPv6)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_IPV6) returned nil")
	}
	if b.S.K != EthertypeIPv6 {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeIPv6)
	}
}

func TestGenProtoAbbrevARP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QARP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_ARP) returned nil")
	}
	if b.S.K != EthertypeARP {
		t.Errorf("k = %#x, want %#x", b.S.K, EthertypeARP)
	}
}

func TestGenProtoAbbrevTCP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QTCP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_TCP) returned nil")
	}
	if cs.Err != nil {
		t.Fatalf("unexpected error: %v", cs.Err)
	}
	// "tcp" generates: (ethertype == IP && proto == 6) || (ethertype == IPv6 && next-header == 6)
	// The result is a complex CFG. The outermost block should be a protocol check.
	// Just verify it's non-nil and has a jump instruction
	if b.S.Code != JmpCode(int(bpf.BPF_JEQ)) {
		t.Errorf("outer block code = %#x, want JMP|JEQ|K", b.S.Code)
	}
}

func TestGenProtoAbbrevUDP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QUDP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_UDP) returned nil")
	}
}

func TestGenProtoAbbrevSCTP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QSCTP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_SCTP) returned nil")
	}
}

func TestGenProtoAbbrevICMP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QICMP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_ICMP) returned nil")
	}
}

func TestGenProtoAbbrevICMPv6(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QICMPv6)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_ICMPV6) returned nil")
	}
}

func TestGenProtoAbbrevAH(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QAH)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_AH) returned nil")
	}
}

func TestGenProtoAbbrevESP(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QESP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(Q_ESP) returned nil")
	}
}

func TestGenProtoAbbrevLinkError(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QLink)
	if b != nil {
		t.Error("GenProtoAbbrev(Q_LINK) should return nil (error)")
	}
	if cs.Err == nil {
		t.Error("expected error for Q_LINK in wrong context")
	}
}

func TestGenProtoAbbrevRadioError(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QRadio)
	if b != nil {
		t.Error("GenProtoAbbrev(Q_RADIO) should return nil (error)")
	}
	if cs.Err == nil {
		t.Error("expected error for Q_RADIO")
	}
}

func TestGenProtoIPProto(t *testing.T) {
	cs := newEthernetCS()
	// gen_proto(6, Q_IP, Q_DEFAULT) → ethertype=IP && proto=TCP
	b := genProto(cs, IPProtoTCP, QIP, QDefault)
	if b == nil {
		t.Fatal("genProto(TCP, Q_IP) returned nil")
	}
	// The innermost block should check protocol byte == 6
	if b.S.K != IPProtoTCP {
		t.Errorf("k = %d, want %d (TCP)", b.S.K, IPProtoTCP)
	}
}

func TestGenProtoIPv6Proto(t *testing.T) {
	cs := newEthernetCS()
	// gen_proto(6, Q_IPV6, Q_DEFAULT) → ethertype=IPv6 && (next-header=6 || fragment-then-6)
	b := genProto(cs, IPProtoTCP, QIPv6, QDefault)
	if b == nil {
		t.Fatal("genProto(TCP, Q_IPV6) returned nil")
	}
}

func TestGenProtoDefault(t *testing.T) {
	cs := newEthernetCS()
	// gen_proto(6, Q_DEFAULT, Q_DEFAULT) → IP variant || IPv6 variant
	b := genProto(cs, IPProtoTCP, QDefault, QDefault)
	if b == nil {
		t.Fatal("genProto(TCP, Q_DEFAULT) returned nil")
	}
}

func TestGenProtoTCPEndToEnd(t *testing.T) {
	cs := newEthernetCS()
	b := GenProtoAbbrev(cs, QTCP)
	if b == nil {
		t.Fatal("GenProtoAbbrev(TCP) returned nil")
	}
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil after compiling 'tcp'")
	}
}
