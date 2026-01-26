// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package dns

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/bpf"

	"github.com/DataDog/datadog-agent/pkg/network/config"
)

func TestGenerateBPFFilter(t *testing.T) {
	tests := []struct {
		name             string
		ports            []int
		collectDNSStats  bool
		expectAssembleOK bool
	}{
		{
			name:             "default port 53",
			ports:            []int{53},
			collectDNSStats:  true,
			expectAssembleOK: true,
		},
		{
			name:             "multiple ports",
			ports:            []int{53, 5353},
			collectDNSStats:  true,
			expectAssembleOK: true,
		},
		{
			name:             "custom port only",
			ports:            []int{8053},
			collectDNSStats:  true,
			expectAssembleOK: true,
		},
		{
			name:             "stats disabled",
			ports:            []int{53},
			collectDNSStats:  false,
			expectAssembleOK: true,
		},
		{
			name:             "multiple ports stats disabled",
			ports:            []int{53, 5353, 8053},
			collectDNSStats:  false,
			expectAssembleOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				DNSMonitoringPortList: tt.ports,
				CollectDNSStats:       tt.collectDNSStats,
			}

			rawIns, err := generateBPFFilter(cfg)
			if tt.expectAssembleOK {
				require.NoError(t, err)
				require.NotEmpty(t, rawIns)
			}
		})
	}
}

func TestBPFFilterMatchesPackets(t *testing.T) {
	tests := []struct {
		name            string
		ports           []int
		collectDNSStats bool
		packets         []testPacket
	}{
		{
			name:            "default port 53 with stats",
			ports:           []int{53},
			collectDNSStats: true,
			packets: []testPacket{
				{desc: "IPv4 UDP src port 53", ipv6: false, proto: 0x11, srcPort: 53, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 UDP dst port 53", ipv6: false, proto: 0x11, srcPort: 12345, dstPort: 53, shouldMatch: true},
				{desc: "IPv4 UDP port 80", ipv6: false, proto: 0x11, srcPort: 80, dstPort: 8080, shouldMatch: false},
				{desc: "IPv4 TCP src port 53", ipv6: false, proto: 0x6, srcPort: 53, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 TCP dst port 53", ipv6: false, proto: 0x6, srcPort: 12345, dstPort: 53, shouldMatch: true},
				{desc: "IPv6 UDP src port 53", ipv6: true, proto: 0x11, srcPort: 53, dstPort: 12345, shouldMatch: true},
				{desc: "IPv6 UDP dst port 53", ipv6: true, proto: 0x11, srcPort: 12345, dstPort: 53, shouldMatch: true},
				{desc: "IPv6 UDP port 80", ipv6: true, proto: 0x11, srcPort: 80, dstPort: 8080, shouldMatch: false},
				{desc: "IPv4 TCP src port 80", ipv6: false, proto: 0x6, srcPort: 80, dstPort: 12345, shouldMatch: false},
				{desc: "IPv4 TCP dst port 80", ipv6: false, proto: 0x6, srcPort: 12345, dstPort: 80, shouldMatch: false},
				{desc: "IPv4 TCP src port 443", ipv6: false, proto: 0x6, srcPort: 80, dstPort: 12345, shouldMatch: false},
				{desc: "IPv4 TCP dst port 443", ipv6: false, proto: 0x6, srcPort: 12345, dstPort: 80, shouldMatch: false},
			},
		},
		{
			name:            "default port 53 stats disabled",
			ports:           []int{53},
			collectDNSStats: false,
			packets: []testPacket{
				{desc: "IPv4 UDP src port 53", ipv6: false, proto: 0x11, srcPort: 53, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 UDP dst port 53 only", ipv6: false, proto: 0x11, srcPort: 12345, dstPort: 53, shouldMatch: false},
			},
		},
		{
			name:            "multiple ports",
			ports:           []int{53, 5353},
			collectDNSStats: true,
			packets: []testPacket{
				{desc: "IPv4 UDP src port 53", ipv6: false, proto: 0x11, srcPort: 53, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 UDP src port 5353", ipv6: false, proto: 0x11, srcPort: 5353, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 UDP dst port 53", ipv6: false, proto: 0x11, srcPort: 12345, dstPort: 53, shouldMatch: true},
				{desc: "IPv4 UDP dst port 5353", ipv6: false, proto: 0x11, srcPort: 12345, dstPort: 5353, shouldMatch: true},
				{desc: "IPv4 UDP no match", ipv6: false, proto: 0x11, srcPort: 80, dstPort: 8080, shouldMatch: false},
				{desc: "IPv6 UDP src port 5353", ipv6: true, proto: 0x11, srcPort: 5353, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 TCP src port 80", ipv6: false, proto: 0x6, srcPort: 80, dstPort: 12345, shouldMatch: false},
				{desc: "IPv4 TCP dst port 80", ipv6: false, proto: 0x6, srcPort: 12345, dstPort: 80, shouldMatch: false},
				{desc: "IPv4 TCP src port 443", ipv6: false, proto: 0x6, srcPort: 80, dstPort: 12345, shouldMatch: false},
				{desc: "IPv4 TCP dst port 443", ipv6: false, proto: 0x6, srcPort: 12345, dstPort: 80, shouldMatch: false},
			},
		},
		{
			name:            "custom port 8053",
			ports:           []int{8053},
			collectDNSStats: true,
			packets: []testPacket{
				{desc: "IPv4 UDP src port 8053", ipv6: false, proto: 0x11, srcPort: 8053, dstPort: 12345, shouldMatch: true},
				{desc: "IPv4 UDP dst port 8053", ipv6: false, proto: 0x11, srcPort: 12345, dstPort: 8053, shouldMatch: true},
				{desc: "IPv4 UDP port 53 no match", ipv6: false, proto: 0x11, srcPort: 53, dstPort: 12345, shouldMatch: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				DNSMonitoringPortList: tt.ports,
				CollectDNSStats:       tt.collectDNSStats,
			}

			rawIns, err := generateBPFFilter(cfg)
			require.NoError(t, err)

			// Convert RawInstruction to Instruction for VM
			ins := make([]bpf.Instruction, len(rawIns))
			for i, raw := range rawIns {
				ins[i] = raw.Disassemble()
			}

			vm, err := bpf.NewVM(ins)
			require.NoError(t, err)

			for _, pkt := range tt.packets {
				packet := buildTestPacket(pkt.ipv6, pkt.proto, pkt.srcPort, pkt.dstPort)
				result, err := vm.Run(packet)
				require.NoError(t, err, pkt.desc)

				if pkt.shouldMatch {
					assert.Greater(t, result, 0, "expected match for: %s", pkt.desc)
				} else {
					assert.Equal(t, 0, result, "expected no match for: %s", pkt.desc)
				}
			}
		})
	}
}

type testPacket struct {
	desc        string
	ipv6        bool
	proto       uint8 // 0x6=TCP, 0x11=UDP
	srcPort     uint16
	dstPort     uint16
	shouldMatch bool
}

func buildTestPacket(ipv6 bool, proto uint8, srcPort, dstPort uint16) []byte {
	if ipv6 {
		return buildIPv6Packet(proto, srcPort, dstPort)
	}
	return buildIPv4Packet(proto, srcPort, dstPort)
}

func buildIPv4Packet(proto uint8, srcPort, dstPort uint16) []byte {
	packet := make([]byte, 54) // 14 (eth) + 20 (IP) + 20 (TCP/UDP header space)

	// Ethernet header (14 bytes)
	// Dst MAC (6) + Src MAC (6) + Ethertype (2)
	packet[12] = 0x08
	packet[13] = 0x00 // IPv4

	// IPv4 header (20 bytes, starting at offset 14)
	packet[14] = 0x45  // version=4, IHL=5 (20 bytes)
	packet[23] = proto // Protocol (TCP=6, UDP=17)
	packet[20] = 0     // Fragment offset = 0 (no fragmentation)
	packet[21] = 0     // Fragment offset (low bits)

	// TCP/UDP ports start at offset 14 + 20 = 34
	binary.BigEndian.PutUint16(packet[34:36], srcPort)
	binary.BigEndian.PutUint16(packet[36:38], dstPort)

	return packet
}

func buildIPv6Packet(proto uint8, srcPort, dstPort uint16) []byte {
	packet := make([]byte, 74) // 14 (eth) + 40 (IPv6) + 20 (TCP/UDP header space)

	// Ethernet header (14 bytes)
	packet[12] = 0x86
	packet[13] = 0xdd // IPv6

	// IPv6 header (40 bytes, starting at offset 14)
	packet[14] = 0x60  // version=6
	packet[20] = proto // Next Header (TCP=6, UDP=17)

	// TCP/UDP ports start at offset 14 + 40 = 54
	binary.BigEndian.PutUint16(packet[54:56], srcPort)
	binary.BigEndian.PutUint16(packet[56:58], dstPort)

	return packet
}
