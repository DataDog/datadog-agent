// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testMAC1    = net.HardwareAddr{0x02, 0, 0, 0, 0, 1}
	testMAC2    = net.HardwareAddr{0x02, 0, 0, 0, 0, 2}
	testIPv4Src = net.IPv4(10, 0, 0, 1)
	testIPv4Dst = net.IPv4(10, 0, 0, 2)
	testIPv6Src = net.ParseIP("fd00::1")
	testIPv6Dst = net.ParseIP("fd00::2")

	// testSecret stands in for application payload. No trimmed packet may
	// contain it.
	testSecret = gopacket.Payload("GET /secret HTTP/1.1\r\nAuthorization: Bearer abc\r\n\r\n")
)

func serialize(t *testing.T, l ...gopacket.SerializableLayer) []byte {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	require.NoError(t, gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, l...))
	return append([]byte(nil), buf.Bytes()...)
}

func eth(etherType layers.EthernetType) *layers.Ethernet {
	return &layers.Ethernet{SrcMAC: testMAC1, DstMAC: testMAC2, EthernetType: etherType}
}

func ipv4(protocol layers.IPProtocol) *layers.IPv4 {
	return &layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: protocol, SrcIP: testIPv4Src, DstIP: testIPv4Dst}
}

func ipv6(next layers.IPProtocol) *layers.IPv6 {
	return &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: next, SrcIP: testIPv6Src, DstIP: testIPv6Dst}
}

// tcpWithTimestamps is a 32-byte TCP header: NOP, NOP, timestamps.
func tcpWithTimestamps() *layers.TCP {
	return &layers.TCP{
		SrcPort: 40000, DstPort: 8080, ACK: true, PSH: true, Window: 512,
		Options: []layers.TCPOption{
			{OptionType: layers.TCPOptionKindNop, OptionLength: 1},
			{OptionType: layers.TCPOptionKindNop, OptionLength: 1},
			{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
		},
	}
}

// tcpSYN is a 40-byte TCP header with the option set Linux sends on connect.
func tcpSYN() *layers.TCP {
	return &layers.TCP{
		SrcPort: 40000, DstPort: 443, SYN: true, Window: 64240,
		Options: []layers.TCPOption{
			{OptionType: layers.TCPOptionKindMSS, OptionLength: 4, OptionData: []byte{0x05, 0xb4}},
			{OptionType: layers.TCPOptionKindSACKPermitted, OptionLength: 2},
			{OptionType: layers.TCPOptionKindTimestamps, OptionLength: 10, OptionData: make([]byte, 8)},
			{OptionType: layers.TCPOptionKindNop, OptionLength: 1},
			{OptionType: layers.TCPOptionKindWindowScale, OptionLength: 3, OptionData: []byte{7}},
		},
	}
}

func udp(dstPort layers.UDPPort) *layers.UDP {
	return &layers.UDP{SrcPort: 40000, DstPort: dstPort}
}

// linuxSLL builds the 16-byte Linux cooked capture header that libpcap
// prepends on the "any" device.
func linuxSLL(etherType layers.EthernetType) []byte {
	hdr := []byte{
		0x00, 0x04, // packet type: outgoing
		0x00, 0x01, // ARPHRD_ETHER
		0x00, 0x06, // address length
		0x02, 0, 0, 0, 0, 1, 0, 0, // address, padded to 8 bytes
		byte(etherType >> 8), byte(etherType),
	}
	return hdr
}

// ipv6DestinationOptions is an 8-byte destination options extension header
// (a single PadN option) followed by next.
func ipv6DestinationOptions(next layers.IPProtocol) []byte {
	return []byte{byte(next), 0x00, 0x01, 0x04, 0, 0, 0, 0}
}

// ipv6FragmentHeader builds the 8-byte IPv6 fragment extension header (RFC
// 8200 §4.5), matching gopacket's decodeIPv6Fragment field layout.
func ipv6FragmentHeader(next layers.IPProtocol, fragOffset uint16, moreFragments bool) []byte {
	var mf uint16
	if moreFragments {
		mf = 1
	}
	word := fragOffset<<3 | mf
	return []byte{
		byte(next), 0x00,
		byte(word >> 8), byte(word),
		0, 0, 0, 1, // Identification
	}
}

// tcpRawHeader builds a 20-byte TCP header with an explicit (possibly
// invalid) data offset nibble, bypassing layers.TCP's own validation so
// malformed-offset packets can be crafted for headerLen.
func tcpRawHeader(dataOffset uint8) []byte {
	h := make([]byte, 20)
	binary.BigEndian.PutUint16(h[0:2], 40000) // src port
	binary.BigEndian.PutUint16(h[2:4], 8080)  // dst port
	h[12] = dataOffset << 4
	return h
}

func TestHeaderLen(t *testing.T) {
	ipv4TCP := serialize(t, ipv4(layers.IPProtocolTCP), tcpWithTimestamps(), testSecret)
	tcpSegment := serialize(t, tcpWithTimestamps(), testSecret)
	// An IHL of 15 (60 bytes) exceeds the IP total length, so IPv4 fails its
	// own decode after gopacket has set its Contents to the whole remainder.
	badIHL := serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(9999), gopacket.Payload("secret"))
	badIHL[14] = 0x4f
	// A Geneve header declaring 4 bytes of options whose single option claims
	// 64 bytes: the decoder would run the option over the payload behind it.
	geneveOverrun := append([]byte{
		0x01, 0x00, 0x08, 0x00, 0, 0, 7, 0, // version 0, Opt Len 1, IPv4, VNI 7
		0x01, 0x02, 0x03, 0x0f, // option class/type, length 15
	}, make([]byte, 10)...)
	geneveOverrun = append(geneveOverrun, testSecret...)

	tests := []struct {
		name       string
		linkType   layers.LinkType
		packet     []byte
		wantLen    int
		wantFailed bool
	}{
		{
			name:     "ethernet IPv4 TCP with timestamps",
			linkType: layers.LinkTypeEthernet,
			packet:   serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP), tcpWithTimestamps(), testSecret),
			wantLen:  14 + 20 + 32,
		},
		{
			name:     "linux cooked capture IPv4 TCP",
			linkType: layers.LinkTypeLinuxSLL,
			packet:   append(linuxSLL(layers.EthernetTypeIPv4), ipv4TCP...),
			wantLen:  16 + 20 + 32,
		},
		{
			name:     "VLAN IPv6 TCP SYN",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeDot1Q),
				&layers.Dot1Q{VLANIdentifier: 42, Type: layers.EthernetTypeIPv6},
				ipv6(layers.IPProtocolTCP), tcpSYN()),
			wantLen: 14 + 4 + 40 + 40,
		},
		{
			name:     "IPv6 destination options extension header",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv6),
				ipv6(layers.IPProtocolIPv6Destination),
				gopacket.Payload(append(ipv6DestinationOptions(layers.IPProtocolTCP), tcpSegment...))),
			wantLen: 14 + 40 + 8 + 32,
		},
		{
			name:     "UDP payload",
			linkType: layers.LinkTypeEthernet,
			packet:   serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(9999), testSecret),
			wantLen:  14 + 20 + 8,
		},
		{
			name:     "DNS is application data",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(53),
				&layers.DNS{ID: 1, RD: true, Questions: []layers.DNSQuestion{{Name: []byte("secret.example.com"), Type: layers.DNSTypeA, Class: layers.DNSClassIN}}}),
			wantLen: 14 + 20 + 8,
		},
		{
			name:     "ICMP echo keeps the ICMP header only",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolICMPv4),
				&layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0), Id: 1, Seq: 1}, testSecret),
			wantLen: 14 + 20 + 8,
		},
		{
			name:     "VXLAN keeps outer and inner headers",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(4789),
				&layers.VXLAN{ValidIDFlag: true, VNI: 7},
				eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP), tcpWithTimestamps(), testSecret),
			wantLen: 14 + 20 + 8 + 8 + 14 + 20 + 32,
		},
		{
			name:     "Geneve keeps outer and inner headers",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(6081),
				&layers.Geneve{Protocol: layers.EthernetTypeIPv4, VNI: 7},
				ipv4(layers.IPProtocolUDP), udp(9999), testSecret),
			wantLen: 14 + 20 + 8 + 8 + 20 + 8,
		},
		{
			name:       "Geneve option overrunning the options length fails closed",
			linkType:   layers.LinkTypeEthernet,
			packet:     serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(6081), gopacket.Payload(geneveOverrun)),
			wantLen:    14 + 20 + 8,
			wantFailed: true,
		},
		{
			name:     "GRE keeps outer and inner headers",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolGRE),
				&layers.GRE{Protocol: layers.EthernetTypeIPv4},
				ipv4(layers.IPProtocolUDP), udp(9999), testSecret),
			wantLen: 14 + 20 + 4 + 20 + 8,
		},
		{
			name:     "ESP keeps outer headers only",
			linkType: layers.LinkTypeEthernet,
			packet:   serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolESP), gopacket.Payload(append([]byte{0, 0, 0, 1, 0, 0, 0, 1}, testSecret...))),
			wantLen:  14 + 20,
		},
		{
			name:     "non-first IPv4 fragment keeps the IP header only",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4),
				&layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, FragOffset: 100, SrcIP: testIPv4Src, DstIP: testIPv4Dst},
				testSecret),
			wantLen: 14 + 20,
		},
		{
			name:     "non-first IPv4 fragment UDP keeps the IP header only",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4),
				&layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolUDP, FragOffset: 100, SrcIP: testIPv4Src, DstIP: testIPv4Dst},
				testSecret),
			wantLen: 14 + 20,
		},
		{
			// The first fragment (offset 0, MoreFragments set) still carries a
			// real TCP header, but IPv4.NextLayerType treats any fragmented
			// packet as opaque, so headerLen fails closed and keeps only the
			// IP header — favoring the privacy guarantee over completeness.
			name:     "first IPv4 fragment (more fragments set) keeps the IP header only",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4),
				&layers.IPv4{Version: 4, IHL: 5, TTL: 64, Protocol: layers.IPProtocolTCP, Flags: layers.IPv4MoreFragments, SrcIP: testIPv4Src, DstIP: testIPv4Dst},
				tcpWithTimestamps(), testSecret),
			wantLen: 14 + 20,
		},
		{
			// gopacket's IPv6 fragment decoder always hands off to the opaque
			// Fragment layer type regardless of offset, so both the first and a
			// later fragment stop at the fragment extension header.
			name:     "IPv6 fragment extension header (first fragment) keeps headers up to the fragment header",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv6),
				ipv6(layers.IPProtocolIPv6Fragment),
				gopacket.Payload(append(ipv6FragmentHeader(layers.IPProtocolTCP, 0, true), serialize(t, tcpWithTimestamps(), testSecret)...))),
			wantLen: 14 + 40 + 8,
		},
		{
			name:     "IPv6 fragment extension header (non-first fragment) keeps headers up to the fragment header",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t,
				eth(layers.EthernetTypeIPv6),
				ipv6(layers.IPProtocolIPv6Fragment),
				gopacket.Payload(append(ipv6FragmentHeader(layers.IPProtocolUDP, 200, false), testSecret...))),
			wantLen: 14 + 40 + 8,
		},
		{
			// TCP data offset > remaining packet length: gopacket sets
			// Contents to the whole remainder (header+payload) but also nils
			// Payload, so the DecodeFailure that follows has zero LayerContents
			// and headerLen's fail-closed correction (n -= last) undoes the
			// bogus TCP contribution, stopping at the IP header.
			name:     "TCP data offset beyond packet length fails closed at the IP header",
			linkType: layers.LinkTypeEthernet,
			// 20-byte raw TCP header + a short payload: total length (26)
			// is less than the claimed data offset * 4 (60), so the data
			// offset claims to reach past the end of the packet.
			packet: serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP),
				gopacket.Payload(append(tcpRawHeader(15), []byte("secret")...))),
			wantLen:    14 + 20,
			wantFailed: true,
		},
		{
			name:     "TCP data offset less than 5 fails closed at the IP header",
			linkType: layers.LinkTypeEthernet,
			packet: serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP),
				gopacket.Payload(append(tcpRawHeader(2), []byte("secret")...))),
			wantLen:    14 + 20,
			wantFailed: true,
		},
		{
			name:       "unknown ethertype keeps the link-layer header only",
			linkType:   layers.LinkTypeEthernet,
			packet:     serialize(t, eth(layers.EthernetType(0x88b5)), testSecret),
			wantLen:    14,
			wantFailed: true,
		},
		{
			name:       "TCP header cut short fails closed at the IP header",
			linkType:   layers.LinkTypeEthernet,
			packet:     serialize(t, eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP), tcpWithTimestamps(), testSecret)[:14+20+24],
			wantLen:    14 + 20,
			wantFailed: true,
		},
		{
			name:       "malformed IPv4 header fails closed at the link layer",
			linkType:   layers.LinkTypeEthernet,
			packet:     badIHL,
			wantLen:    14,
			wantFailed: true,
		},
		{
			name:       "unsupported link type writes nothing",
			linkType:   layers.LinkType(250),
			packet:     ipv4TCP,
			wantLen:    0,
			wantFailed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, failed := headerLen(tt.packet, tt.linkType)
			assert.Equal(t, tt.wantLen, n)
			assert.Equal(t, tt.wantFailed, failed)
			assert.LessOrEqual(t, n, len(tt.packet))
			assert.False(t, bytes.Contains(tt.packet[:n], []byte("secret")), "trimmed packet must not contain payload")
		})
	}
}

// TestHeaderLenWithinSnapLen pins the snap length choice: the common packet
// shapes must fit in maxSnapLen with their headers intact.
func TestHeaderLenWithinSnapLen(t *testing.T) {
	vxlanIPv4 := serialize(t,
		eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolUDP), udp(4789),
		&layers.VXLAN{ValidIDFlag: true, VNI: 7},
		eth(layers.EthernetTypeIPv4), ipv4(layers.IPProtocolTCP), tcpWithTimestamps(), testSecret)
	vlanIPv6SYN := append(linuxSLL(layers.EthernetTypeDot1Q), serialize(t,
		&layers.Dot1Q{VLANIdentifier: 42, Type: layers.EthernetTypeIPv6},
		ipv6(layers.IPProtocolTCP), tcpSYN())...)

	tests := []struct {
		name     string
		linkType layers.LinkType
		packet   []byte
	}{
		{"VXLAN IPv4 TCP", layers.LinkTypeEthernet, vxlanIPv4},
		{"cooked VLAN IPv6 SYN", layers.LinkTypeLinuxSLL, vlanIPv6SYN},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full, failed := headerLen(tt.packet, tt.linkType)
			require.False(t, failed)
			require.LessOrEqual(t, full, maxSnapLen)

			cut, failed := headerLen(tt.packet[:min(len(tt.packet), maxSnapLen)], tt.linkType)
			assert.False(t, failed)
			assert.Equal(t, full, cut)
		})
	}
}
