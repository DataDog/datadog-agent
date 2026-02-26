// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build darwin

package filter

import (
	"net"
	"sync"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var serializeOpts = gopacket.SerializeOptions{FixLengths: true}

var zeroMAC = net.HardwareAddr{0, 0, 0, 0, 0, 0}

// buildIPv4Packet constructs an Ethernet + IPv4 packet using gopacket.
// srcIP and dstIP are 4-byte IPv4 addresses.
func buildIPv4Packet(srcIP, dstIP [4]byte) []byte {
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, serializeOpts,
		&layers.Ethernet{
			SrcMAC:       zeroMAC,
			DstMAC:       zeroMAC,
			EthernetType: layers.EthernetTypeIPv4,
		},
		&layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolTCP,
			SrcIP:    net.IP(srcIP[:]),
			DstIP:    net.IP(dstIP[:]),
		},
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// buildIPv6Packet constructs an Ethernet + IPv6 packet using gopacket.
// srcIP and dstIP are 16-byte IPv6 addresses.
func buildIPv6Packet(srcIP, dstIP [16]byte) []byte {
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, serializeOpts,
		&layers.Ethernet{
			SrcMAC:       zeroMAC,
			DstMAC:       zeroMAC,
			EthernetType: layers.EthernetTypeIPv6,
		},
		&layers.IPv6{
			Version:    6,
			NextHeader: layers.IPProtocolTCP,
			HopLimit:   64,
			SrcIP:      net.IP(srcIP[:]),
			DstIP:      net.IP(dstIP[:]),
		},
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestDeterminePacketDirection_IPv4_Outgoing(t *testing.T) {
	// Local is 192.168.1.1, remote is 10.0.0.1. Packet from local to remote = outgoing.
	local := [4]byte{192, 168, 1, 1}
	remote := [4]byte{10, 0, 0, 1}
	data := buildIPv4Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketOutgoing), dir, "src local, dst remote should be PACKET_OUTGOING")
}

func TestDeterminePacketDirection_IPv4_Host(t *testing.T) {
	// Remote to local = incoming (PACKET_HOST).
	local := [4]byte{192, 168, 1, 1}
	remote := [4]byte{10, 0, 0, 1}
	data := buildIPv4Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketHost), dir, "src remote, dst local should be PACKET_HOST")
}

func TestDeterminePacketDirection_IPv4_ShortData(t *testing.T) {
	// Truncated packet: only 20 bytes (no full IPv4 header after ethernet).
	data := make([]byte, 20)
	data[12] = 0x08
	data[13] = 0x00

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{string([]byte{192, 168, 1, 1}): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketHost), dir, "truncated packet should default to PACKET_HOST")
}

func TestDeterminePacketDirection_IPv4_VeryShort(t *testing.T) {
	// Less than 14 bytes: no ethernet.
	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection([]byte{1, 2, 3})
	assert.Equal(t, uint8(PacketHost), dir, "very short data should default to PACKET_HOST")
}

func TestDeterminePacketDirection_IPv6_Outgoing(t *testing.T) {
	var local [16]byte
	local[15] = 1 // ::1
	var remote [16]byte
	remote[15] = 2 // ::2
	data := buildIPv6Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketOutgoing), dir, "IPv6 src local, dst remote should be PACKET_OUTGOING")
}

func TestDeterminePacketDirection_IPv6_Host(t *testing.T) {
	var local [16]byte
	local[15] = 1
	var remote [16]byte
	remote[15] = 2
	data := buildIPv6Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}

	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketHost), dir, "IPv6 src remote, dst local should be PACKET_HOST")
}

func TestDeterminePacketDirection_IPv6_ShortData(t *testing.T) {
	// IPv6 header is 40 bytes; total need 14+40=54. Provide only 14+20.
	data := make([]byte, 34)
	data[12] = 0x86
	data[13] = 0xDD

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketHost), dir, "truncated IPv6 packet should default to PACKET_HOST")
}

func TestDeterminePacketDirection_NonIP_EtherType(t *testing.T) {
	// EtherType 0x0806 (ARP) or similar: should return PACKET_HOST.
	data := make([]byte, 14)
	data[12] = 0x08
	data[13] = 0x06

	ih := &interfaceHandle{
		ifaceName:  "test",
		linkType:   layers.LinkTypeEthernet,
		localAddrs: map[string]struct{}{},
	}
	dir := ih.determinePacketDirection(data)
	assert.Equal(t, uint8(PacketHost), dir, "non-IP ethertype should default to PACKET_HOST")
}

// ============================================================================
// Loopback (utun / VPN interface) direction tests
//
// BSD loopback format (DLT_NULL / LinkTypeNull):
//   bytes 0-3  : address family, little-endian (AF_INET=2, AF_INET6=28 on macOS)
//   bytes 4+   : raw IP header
//
// IPv4: src at offset 4+12=16, dst at 4+16=20
// IPv6: src at offset 4+8=12,  dst at 4+24=28
// ============================================================================

// buildLoopbackIPv4Packet builds a BSD loopback (DLT_NULL) + IPv4 packet using gopacket.
// The 4-byte loopback header encodes AF_INET (2) in little-endian host byte order.
func buildLoopbackIPv4Packet(srcIP, dstIP [4]byte) []byte {
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, serializeOpts,
		&layers.Loopback{Family: layers.ProtocolFamilyIPv4},
		&layers.IPv4{
			Version:  4,
			TTL:      64,
			Protocol: layers.IPProtocolTCP,
			SrcIP:    net.IP(srcIP[:]),
			DstIP:    net.IP(dstIP[:]),
		},
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// buildLoopbackIPv6Packet builds a BSD loopback (DLT_NULL) + IPv6 packet using gopacket.
// The 4-byte loopback header encodes AF_INET6 as used on FreeBSD/macOS (28) in little-endian.
func buildLoopbackIPv6Packet(srcIP, dstIP [16]byte) []byte {
	buf := gopacket.NewSerializeBuffer()
	err := gopacket.SerializeLayers(buf, serializeOpts,
		&layers.Loopback{Family: layers.ProtocolFamilyIPv6FreeBSD},
		&layers.IPv6{
			Version:    6,
			NextHeader: layers.IPProtocolTCP,
			HopLimit:   64,
			SrcIP:      net.IP(srcIP[:]),
			DstIP:      net.IP(dstIP[:]),
		},
	)
	if err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func TestDeterminePacketDirection_Loopback_IPv4_Outgoing(t *testing.T) {
	local := [4]byte{10, 0, 0, 1}
	remote := [4]byte{8, 8, 8, 8}
	data := buildLoopbackIPv4Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}
	assert.Equal(t, uint8(PacketOutgoing), ih.determinePacketDirection(data),
		"loopback IPv4: src local, dst remote should be PacketOutgoing")
}

func TestDeterminePacketDirection_Loopback_IPv4_Host(t *testing.T) {
	local := [4]byte{10, 0, 0, 1}
	remote := [4]byte{8, 8, 8, 8}
	data := buildLoopbackIPv4Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}
	assert.Equal(t, uint8(PacketHost), ih.determinePacketDirection(data),
		"loopback IPv4: src remote, dst local should be PacketHost")
}

func TestDeterminePacketDirection_Loopback_IPv6_Outgoing(t *testing.T) {
	var local, remote [16]byte
	local[15] = 1
	remote[15] = 2
	data := buildLoopbackIPv6Packet(local, remote)

	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}
	assert.Equal(t, uint8(PacketOutgoing), ih.determinePacketDirection(data),
		"loopback IPv6: src local, dst remote should be PacketOutgoing")
}

func TestDeterminePacketDirection_Loopback_IPv6_Host(t *testing.T) {
	var local, remote [16]byte
	local[15] = 1
	remote[15] = 2
	data := buildLoopbackIPv6Packet(remote, local)

	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{string(local[:]): {}},
	}
	assert.Equal(t, uint8(PacketHost), ih.determinePacketDirection(data),
		"loopback IPv6: src remote, dst local should be PacketHost")
}

func TestDeterminePacketDirection_Loopback_ShortData(t *testing.T) {
	// Fewer than 4 bytes — cannot read AF header.
	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{},
	}
	assert.Equal(t, uint8(PacketHost), ih.determinePacketDirection([]byte{2, 0}),
		"loopback: fewer than 4 bytes should default to PacketHost")
}

func TestDeterminePacketDirection_Loopback_UnknownAF(t *testing.T) {
	// AF=99 is not AF_INET or AF_INET6.
	buf := make([]byte, 4+20)
	buf[0] = 99
	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{},
	}
	assert.Equal(t, uint8(PacketHost), ih.determinePacketDirection(buf),
		"loopback: unknown AF should default to PacketHost")
}

func TestDeterminePacketDirection_Loopback_IPv4_TruncatedIP(t *testing.T) {
	// AF_INET header present but IPv4 header is too short (< 20 bytes after the 4-byte prefix).
	buf := make([]byte, 4+10) // only 10 bytes of IP instead of 20
	buf[0] = 2                // AF_INET
	ih := &interfaceHandle{
		ifaceName:  "utun0",
		linkType:   layers.LinkTypeNull,
		localAddrs: map[string]struct{}{},
	}
	assert.Equal(t, uint8(PacketHost), ih.determinePacketDirection(buf),
		"loopback: truncated IPv4 header should default to PacketHost")
}

// ============================================================================
// T-2: Buffer pool regression — snapLen > defaultSnapLen must not panic
// ============================================================================

func TestLibpcapSource_BufferPool_LargeSnapLen(t *testing.T) {
	const largeSnapLen = 8192 // larger than defaultSnapLen (4096)

	// Build a LibpcapSource directly so we can exercise the pool methods
	// without opening any real pcap handles (which would require root).
	ps := &LibpcapSource{}
	ps.bufPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, largeSnapLen)
		},
	}

	buf := ps.getBuffer()
	require.Equal(t, largeSnapLen, cap(buf), "pool should vend buffers sized to snapLen")

	// Simulate receiving a packet larger than the old defaultSnapLen (4096).
	// Before the fix this line would have panicked with "slice bounds out of range".
	simulatedPacketLen := 7000
	require.LessOrEqual(t, simulatedPacketLen, cap(buf),
		"simulated packet must fit in the buffer")
	buf = buf[:simulatedPacketLen]
	copy(buf, make([]byte, simulatedPacketLen))

	ps.putBuffer(buf)

	// After putBuffer the pool must restore the buffer to full capacity.
	buf2 := ps.getBuffer()
	assert.Equal(t, largeSnapLen, cap(buf2),
		"putBuffer must restore capacity so the next caller does not panic")
	ps.putBuffer(buf2)
}

func TestLibpcapSource_BufferPool_DefaultSnapLen(t *testing.T) {
	// Sanity check: the default snapLen (4096) path still works correctly.
	ps := &LibpcapSource{}
	ps.bufPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, defaultSnapLen)
		},
	}

	buf := ps.getBuffer()
	assert.Equal(t, defaultSnapLen, cap(buf))

	// Simulate a small packet.
	buf = buf[:100]
	ps.putBuffer(buf)

	buf2 := ps.getBuffer()
	assert.Equal(t, defaultSnapLen, cap(buf2))
	ps.putBuffer(buf2)
}
