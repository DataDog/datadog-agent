// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// buildIPv4Packet constructs a minimal Ethernet + IPv4 packet (14 + 20 bytes).
// srcIP and dstIP are 4-byte IPv4 addresses. EtherType is set to 0x0800 (IPv4).
func buildIPv4Packet(srcIP, dstIP [4]byte) []byte {
	// Ethernet: 6 dst MAC + 6 src MAC + 2 ethertype (0x0800)
	eth := make([]byte, 14)
	eth[12] = 0x08
	eth[13] = 0x00

	// IPv4 minimal header: 20 bytes, src at offset 12, dst at offset 16
	ip := make([]byte, 20)
	ip[0] = 0x45 // version 4, IHL 5
	copy(ip[12:16], srcIP[:])
	copy(ip[16:20], dstIP[:])

	return append(eth, ip...)
}

// buildIPv6Packet constructs a minimal Ethernet + IPv6 packet (14 + 40 bytes).
// srcIP and dstIP are 16-byte IPv6 addresses.
func buildIPv6Packet(srcIP, dstIP [16]byte) []byte {
	eth := make([]byte, 14)
	eth[12] = 0x86
	eth[13] = 0xDD
	ip := make([]byte, 40)
	copy(ip[8:24], srcIP[:])
	copy(ip[24:40], dstIP[:])
	return append(eth, ip...)
}

func TestDeterminePacketDirection_IPv4_Outgoing(t *testing.T) {
	// Local is 192.168.1.1, remote is 10.0.0.1. Packet from local to remote = outgoing.
	local := [4]byte{192, 168, 1, 1}
	remote := [4]byte{10, 0, 0, 1}
	data := buildIPv4Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_OUTGOING), dir, "src local, dst remote should be PACKET_OUTGOING")
}

func TestDeterminePacketDirection_IPv4_Host(t *testing.T) {
	// Remote to local = incoming (PACKET_HOST).
	local := [4]byte{192, 168, 1, 1}
	remote := [4]byte{10, 0, 0, 1}
	data := buildIPv4Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_HOST), dir, "src remote, dst local should be PACKET_HOST")
}

func TestDeterminePacketDirection_IPv4_ShortData(t *testing.T) {
	// Truncated packet: only 20 bytes (no full IPv4 header after ethernet).
	data := make([]byte, 20)
	data[12] = 0x08
	data[13] = 0x00

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{string([]byte{192, 168, 1, 1}): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_HOST), dir, "truncated packet should default to PACKET_HOST")
}

func TestDeterminePacketDirection_IPv4_VeryShort(t *testing.T) {
	// Less than 14 bytes: no ethernet.
	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection([]byte{1, 2, 3})
	assert.Equal(t, uint8(PACKET_HOST), dir, "very short data should default to PACKET_HOST")
}

func TestDeterminePacketDirection_IPv6_Outgoing(t *testing.T) {
	var local [16]byte
	local[15] = 1 // ::1
	var remote [16]byte
	remote[15] = 2 // ::2
	data := buildIPv6Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_OUTGOING), dir, "IPv6 src local, dst remote should be PACKET_OUTGOING")
}

func TestDeterminePacketDirection_IPv6_Host(t *testing.T) {
	var local [16]byte
	local[15] = 1
	var remote [16]byte
	remote[15] = 2
	data := buildIPv6Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_HOST), dir, "IPv6 src remote, dst local should be PACKET_HOST")
}

func TestDeterminePacketDirection_IPv6_ShortData(t *testing.T) {
	// IPv6 header is 40 bytes; total need 14+40=54. Provide only 14+20.
	data := make([]byte, 34)
	data[12] = 0x86
	data[13] = 0xDD

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_HOST), dir, "truncated IPv6 packet should default to PACKET_HOST")
}

func TestDeterminePacketDirection_NonIP_EtherType(t *testing.T) {
	// EtherType 0x0806 (ARP) or similar: should return PACKET_HOST.
	data := make([]byte, 14)
	data[12] = 0x08
	data[13] = 0x06

	ih := &interfaceHandle{
		ifaceName:  "test",
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PACKET_HOST), dir, "non-IP ethertype should default to PACKET_HOST")
}
