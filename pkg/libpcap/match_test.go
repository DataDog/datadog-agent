// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package libpcap

import (
	"encoding/binary"
	"testing"
)

// buildEthernetIPv4TCP builds a minimal Ethernet+IPv4+TCP packet.
func buildEthernetIPv4TCP(srcPort, dstPort uint16) []byte {
	pkt := make([]byte, 54) // 14 (eth) + 20 (ipv4) + 20 (tcp)

	// Ethernet header
	// dst MAC: 00:00:00:00:00:01
	pkt[5] = 0x01
	// src MAC: 00:00:00:00:00:02
	pkt[11] = 0x02
	// Ethertype: 0x0800 (IPv4)
	pkt[12] = 0x08
	pkt[13] = 0x00

	// IPv4 header (offset 14)
	pkt[14] = 0x45          // version=4, IHL=5 (20 bytes)
	pkt[16] = 0x00          // total length high
	pkt[17] = 40            // total length = 40 (20 IP + 20 TCP)
	pkt[23] = 6             // protocol = TCP
	// src IP: 192.168.1.1
	pkt[26] = 192
	pkt[27] = 168
	pkt[28] = 1
	pkt[29] = 1
	// dst IP: 10.0.0.1
	pkt[30] = 10
	pkt[31] = 0
	pkt[32] = 0
	pkt[33] = 1

	// TCP header (offset 34)
	binary.BigEndian.PutUint16(pkt[34:], srcPort)
	binary.BigEndian.PutUint16(pkt[36:], dstPort)
	pkt[46] = 0x50 // data offset = 5 (20 bytes), no flags

	return pkt
}

// buildEthernetIPv4UDP builds a minimal Ethernet+IPv4+UDP packet.
func buildEthernetIPv4UDP(srcPort, dstPort uint16) []byte {
	pkt := make([]byte, 42) // 14 (eth) + 20 (ipv4) + 8 (udp)

	// Ethernet
	pkt[12] = 0x08
	pkt[13] = 0x00

	// IPv4
	pkt[14] = 0x45
	pkt[17] = 28 // 20 IP + 8 UDP
	pkt[23] = 17 // UDP

	// UDP
	binary.BigEndian.PutUint16(pkt[34:], srcPort)
	binary.BigEndian.PutUint16(pkt[36:], dstPort)

	return pkt
}

// buildEthernetARP builds a minimal Ethernet+ARP packet.
func buildEthernetARP() []byte {
	pkt := make([]byte, 42) // 14 (eth) + 28 (arp)
	pkt[12] = 0x08
	pkt[13] = 0x06 // Ethertype: ARP
	return pkt
}

func TestNewBPF(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "tcp")
	if err != nil {
		t.Fatalf("NewBPF('tcp') = %v", err)
	}
	if filter == nil {
		t.Fatal("NewBPF returned nil")
	}
}

func TestNewBPFError(t *testing.T) {
	_, err := NewBPF(LinkTypeEthernet, 256, "((((")
	if err == nil {
		t.Error("expected error for syntax error")
	}
}

func TestMatchesTCPPort80(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "tcp dst port 80")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// TCP packet to port 80 — should match
	pkt := buildEthernetIPv4TCP(12345, 80)
	ci := CaptureInfo{Length: len(pkt), CaptureLength: len(pkt)}
	if !filter.Matches(ci, pkt) {
		t.Error("tcp dst port 80 should match packet to port 80")
	}

	// TCP packet to port 443 — should NOT match
	pkt2 := buildEthernetIPv4TCP(12345, 443)
	if filter.Matches(ci, pkt2) {
		t.Error("tcp dst port 80 should NOT match packet to port 443")
	}
}

func TestMatchesTCP(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "tcp")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// TCP packet — should match
	tcpPkt := buildEthernetIPv4TCP(1234, 80)
	ci := CaptureInfo{Length: len(tcpPkt), CaptureLength: len(tcpPkt)}
	if !filter.Matches(ci, tcpPkt) {
		t.Error("'tcp' should match TCP packet")
	}

	// UDP packet — should NOT match
	udpPkt := buildEthernetIPv4UDP(1234, 53)
	ci2 := CaptureInfo{Length: len(udpPkt), CaptureLength: len(udpPkt)}
	if filter.Matches(ci2, udpPkt) {
		t.Error("'tcp' should NOT match UDP packet")
	}
}

func TestMatchesIP(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "ip")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// IPv4 TCP packet — should match
	pkt := buildEthernetIPv4TCP(1234, 80)
	ci := CaptureInfo{Length: len(pkt), CaptureLength: len(pkt)}
	if !filter.Matches(ci, pkt) {
		t.Error("'ip' should match IPv4 packet")
	}

	// ARP packet — should NOT match
	arpPkt := buildEthernetARP()
	ci2 := CaptureInfo{Length: len(arpPkt), CaptureLength: len(arpPkt)}
	if filter.Matches(ci2, arpPkt) {
		t.Error("'ip' should NOT match ARP packet")
	}
}

func TestMatchesARP(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "arp")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	arpPkt := buildEthernetARP()
	ci := CaptureInfo{Length: len(arpPkt), CaptureLength: len(arpPkt)}
	if !filter.Matches(ci, arpPkt) {
		t.Error("'arp' should match ARP packet")
	}

	tcpPkt := buildEthernetIPv4TCP(1234, 80)
	ci2 := CaptureInfo{Length: len(tcpPkt), CaptureLength: len(tcpPkt)}
	if filter.Matches(ci2, tcpPkt) {
		t.Error("'arp' should NOT match TCP packet")
	}
}

func TestMatchesSrcHost(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "src host 192.168.1.1")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// Packet from 192.168.1.1 — should match
	pkt := buildEthernetIPv4TCP(1234, 80)
	ci := CaptureInfo{Length: len(pkt), CaptureLength: len(pkt)}
	if !filter.Matches(ci, pkt) {
		t.Error("src host 192.168.1.1 should match")
	}
}

func TestMatchesUDPPort53(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "udp port 53")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// UDP to port 53 — should match
	pkt := buildEthernetIPv4UDP(1234, 53)
	ci := CaptureInfo{Length: len(pkt), CaptureLength: len(pkt)}
	if !filter.Matches(ci, pkt) {
		t.Error("udp port 53 should match UDP packet to port 53")
	}

	// UDP to port 80 — should NOT match
	pkt2 := buildEthernetIPv4UDP(1234, 80)
	if filter.Matches(ci, pkt2) {
		t.Error("udp port 53 should NOT match UDP packet to port 80")
	}

	// TCP to port 53 — should NOT match
	tcpPkt := buildEthernetIPv4TCP(1234, 53)
	ci2 := CaptureInfo{Length: len(tcpPkt), CaptureLength: len(tcpPkt)}
	if filter.Matches(ci2, tcpPkt) {
		t.Error("udp port 53 should NOT match TCP packet")
	}
}

func TestMatchesEmpty(t *testing.T) {
	// Empty filter matches everything
	filter, err := NewBPF(LinkTypeEthernet, 256, "")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	pkt := buildEthernetIPv4TCP(1234, 80)
	ci := CaptureInfo{Length: len(pkt), CaptureLength: len(pkt)}
	if !filter.Matches(ci, pkt) {
		t.Error("empty filter should match everything")
	}
}

func TestMatchesNot(t *testing.T) {
	filter, err := NewBPF(LinkTypeEthernet, 256, "not tcp")
	if err != nil {
		t.Fatalf("NewBPF: %v", err)
	}

	// TCP should NOT match
	tcpPkt := buildEthernetIPv4TCP(1234, 80)
	ci := CaptureInfo{Length: len(tcpPkt), CaptureLength: len(tcpPkt)}
	if filter.Matches(ci, tcpPkt) {
		t.Error("'not tcp' should NOT match TCP packet")
	}

	// UDP should match
	udpPkt := buildEthernetIPv4UDP(1234, 53)
	ci2 := CaptureInfo{Length: len(udpPkt), CaptureLength: len(udpPkt)}
	if !filter.Matches(ci2, udpPkt) {
		t.Error("'not tcp' should match UDP packet")
	}
}
