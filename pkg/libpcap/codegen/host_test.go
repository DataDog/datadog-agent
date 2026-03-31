// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

func TestGenHostIPSrc(t *testing.T) {
	cs := newEthernetCS()
	// "src host 192.168.1.1" → ethertype=IP && src_ip == 0xC0A80101
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QIP, QSrc, QHost)
	if b == nil {
		t.Fatal("GenHost(IP, SRC) returned nil")
	}
	// The innermost block should compare the IP address
	if b.S.K != 0xC0A80101 {
		t.Errorf("k = %#x, want 0xC0A80101", b.S.K)
	}
}

func TestGenHostIPDst(t *testing.T) {
	cs := newEthernetCS()
	b := GenHost(cs, 0x0A000001, 0xFFFFFFFF, QIP, QDst, QHost)
	if b == nil {
		t.Fatal("GenHost(IP, DST) returned nil")
	}
	if b.S.K != 0x0A000001 {
		t.Errorf("k = %#x, want 0x0A000001", b.S.K)
	}
}

func TestGenHostIPDefault(t *testing.T) {
	cs := newEthernetCS()
	// "host 192.168.1.1" → default → IP src OR IP dst, plus ARP, RARP
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QDefault, QDefault, QHost)
	if b == nil {
		t.Fatal("GenHost(DEFAULT) returned nil")
	}
}

func TestGenHostNet(t *testing.T) {
	cs := newEthernetCS()
	// "net 192.168.0.0/16" → masked comparison
	b := GenHost(cs, 0xC0A80000, 0xFFFF0000, QIP, QDefault, QNet)
	if b == nil {
		t.Fatal("GenHost(NET) returned nil")
	}
}

func TestGenHostARP(t *testing.T) {
	cs := newEthernetCS()
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QARP, QSrc, QHost)
	if b == nil {
		t.Fatal("GenHost(ARP, SRC) returned nil")
	}
}

func TestGenHostTCPError(t *testing.T) {
	cs := newEthernetCS()
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QTCP, QSrc, QHost)
	if b != nil {
		t.Error("expected nil for TCP host")
	}
	if cs.Err == nil {
		t.Error("expected error for 'tcp' modifier applied to host")
	}
}

func TestGenEhostopSrc(t *testing.T) {
	cs := newEthernetCS()
	mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	b := GenEhostop(cs, mac, QSrc)
	if b == nil {
		t.Fatal("GenEhostop(SRC) returned nil")
	}
	// Source MAC comparison at offset 6 in Ethernet header
}

func TestGenEhostopDst(t *testing.T) {
	cs := newEthernetCS()
	mac := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	b := GenEhostop(cs, mac, QDst)
	if b == nil {
		t.Fatal("GenEhostop(DST) returned nil")
	}
	// Dest MAC comparison at offset 0
}

func TestGenEhostopOr(t *testing.T) {
	cs := newEthernetCS()
	mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	b := GenEhostop(cs, mac, QDefault)
	if b == nil {
		t.Fatal("GenEhostop(DEFAULT) returned nil")
	}
}

func TestGenHost6(t *testing.T) {
	cs := newEthernetCS()
	// ::1
	addr := [16]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	mask := [16]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	b := GenHost6(cs, addr, mask, QDefault, QDefault, QHost)
	if b == nil {
		t.Fatal("GenHost6(::1) returned nil")
	}
}

func TestGenHostIPEndToEnd(t *testing.T) {
	cs := newEthernetCS()
	b := GenHost(cs, 0xC0A80101, 0xFFFFFFFF, QIP, QSrc, QHost)
	if b == nil {
		t.Fatal("GenHost returned nil")
	}
	err := FinishParse(cs, b)
	if err != nil {
		t.Fatalf("FinishParse = %v", err)
	}
	if cs.IC.Root == nil {
		t.Fatal("IC.Root is nil")
	}

	// Walk CFG to verify structure:
	// Root → (ethertype check) → (IP src addr check) → accept/reject
	root := cs.IC.Root
	if root.S.Code != JmpCode(int(bpf.BPF_JEQ)) {
		t.Errorf("root code = %#x, want JMP|JEQ|K", root.S.Code)
	}
}
