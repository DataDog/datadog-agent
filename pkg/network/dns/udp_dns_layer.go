// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || (linux && bpf) || darwin

package dns

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var _ gopacket.DecodingLayer = &udpWithDNSSupport{}

// udpWithDNSSupport wraps layers.UDP to decode the payload as DNS regardless of
// the port number. This is necessary because gopacket's UDP layer has
// port-based heuristics that may try to decode port 5353 as mDNS or other
// protocols instead of standard DNS. Payloads cut short by the snaplen are not
// decoded (see NextLayerType).
type udpWithDNSSupport struct {
	layers.UDP
}

// NextLayerType returns LayerTypeDNS to ensure the payload is decoded as DNS,
// unless the payload was cut short by the snaplen: decoding would fail, and the
// parser reads just the response header instead.
func (m *udpWithDNSSupport) NextLayerType() gopacket.LayerType {
	if int(m.Length)-8 > len(m.Payload) {
		return gopacket.LayerTypeZero
	}
	return layers.LayerTypeDNS
}
