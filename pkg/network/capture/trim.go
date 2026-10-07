// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && pcap && cgo

package capture

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// headerLayers are the layers whose bytes are protocol headers and may be
// written. Decoding stops at the first layer not listed here, so anything the
// decoder classifies as application data (gopacket.Payload, DNS, TLS, ...),
// encrypted (ESP, WireGuard) or non-first IP fragments is never written.
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
		if _, ok := headerLayers[layer.LayerType()]; !ok {
			return n, false
		}
		last = len(layer.LayerContents())
		n += last
	}
	return n, false
}
