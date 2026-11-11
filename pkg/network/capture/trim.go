// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// headerLayers are the layers whose bytes are protocol headers and may be
// written. Decoding stops at the first layer not listed here, so anything the
// decoder classifies as application data (gopacket.Payload, DNS, TLS, ...),
// encrypted (ESP, WireGuard) or non-first IP fragments is never written. A
// first fragment keeps its transport header (see fragmentHeaderLen).
var headerLayers = map[gopacket.LayerType]struct{}{
	layers.LayerTypeLinuxSLL:        {},
	layers.LayerTypeEthernet:        {},
	layers.LayerTypeDot1Q:           {},
	layers.LayerTypeARP:             {},
	layers.LayerTypeIPv4:            {},
	layers.LayerTypeIPv6:            {},
	layers.LayerTypeIPv6HopByHop:    {},
	layers.LayerTypeIPv6Routing:     {},
	layers.LayerTypeIPv6Fragment:    {},
	layers.LayerTypeIPv6Destination: {},
	layers.LayerTypeTCP:             {},
	layers.LayerTypeUDP:             {},
	layers.LayerTypeICMPv4:          {},
	layers.LayerTypeICMPv6:          {},
	layers.LayerTypeICMPv6Echo:      {},
	layers.LayerTypeVXLAN:           {},
	layers.LayerTypeGeneve:          {},
	layers.LayerTypeGRE:             {},
}

// headerLen returns how many leading bytes of data are protocol headers: the
// sum of every decoded header layer up to the first layer that is not one.
// Tunnels with a decodable inner packet therefore keep both the outer and the
// inner headers.
//
// It fails closed: an unknown or malformed layer ends the count at the last
// complete header, so it may return fewer header bytes than the packet has,
// never more. failed reports that decoding stopped on a layer that could not
// be decoded, which for a packet cut by the snap length means its headers did
// not fit.
func headerLen(data []byte, linkType layers.LinkType) (n int, failed bool) {
	packet := gopacket.NewPacket(data, linkType, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	last := 0
	var prev gopacket.Layer
	for _, layer := range packet.Layers() {
		if layer.LayerType() == gopacket.LayerTypeDecodeFailure {
			// gopacket keeps a layer that failed its own decode, and its
			// Contents may then run past the header (IPv4 sets them to the
			// whole remainder before validating). The failure carries the
			// previous layer's payload, so an empty one means the previous
			// layer is the one that failed: drop it.
			if len(layer.LayerContents()) == 0 {
				n -= last
			}
			return n, true
		}
		if layer.LayerType() == gopacket.LayerTypeFragment {
			l4, failed := fragmentHeaderLen(prev, layer.LayerContents())
			return n + l4, failed
		}
		if _, ok := headerLayers[layer.LayerType()]; !ok {
			return n, false
		}
		// gopacket's Geneve decoder trusts each option's own length, so an
		// option longer than the header's declared options length makes the
		// layer swallow bytes past the header. Treat that as malformed.
		if g, ok := layer.(*layers.Geneve); ok && len(g.Contents) != 8+int(g.OptionsLength) {
			return n, true
		}
		last = len(layer.LayerContents())
		n += last
		prev = layer
	}
	return n, false
}

// fragmentHeaderLen returns the length of the transport header at the start
// of an IP fragment's payload. gopacket hands every IPv4 fragment, first
// included, and every IPv6 fragment to an opaque Fragment layer, so the
// header is decoded here from the IP protocol field. Only a first fragment
// (offset 0) starts with a transport header; later ones are all payload.
func fragmentHeaderLen(ip gopacket.Layer, payload []byte) (n int, failed bool) {
	var proto layers.IPProtocol
	switch l := ip.(type) {
	case *layers.IPv4:
		if l.FragOffset != 0 {
			return 0, false
		}
		proto = l.Protocol
	case *layers.IPv6Fragment:
		if l.FragmentOffset != 0 {
			return 0, false
		}
		proto = l.NextHeader
	default:
		return 0, false
	}
	switch proto {
	case layers.IPProtocolTCP, layers.IPProtocolUDP, layers.IPProtocolICMPv4, layers.IPProtocolICMPv6:
	default:
		return 0, false
	}
	l4 := gopacket.NewPacket(payload, proto.LayerType(), gopacket.DecodeOptions{Lazy: true, NoCopy: true}).Layers()
	if len(l4) == 0 || !completeHeader(l4[0]) {
		return 0, true
	}
	return len(l4[0].LayerContents()), false
}

// completeHeader reports whether a decoded transport layer holds its whole
// header. gopacket's TCP decoder keeps a header cut short by the snap length
// without failing, and a first fragment's UDP length always runs past the
// fragment, so the check is on the header itself.
func completeHeader(l gopacket.Layer) bool {
	switch t := l.(type) {
	case *layers.TCP:
		return t.DataOffset >= 5 && len(t.Contents) == int(t.DataOffset)*4
	case *layers.UDP, *layers.ICMPv4:
		return len(l.LayerContents()) == 8
	case *layers.ICMPv6:
		return len(l.LayerContents()) == 4
	default:
		return false
	}
}
