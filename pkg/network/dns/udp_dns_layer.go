// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package dns

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var _ gopacket.DecodingLayer = &udpWithDNSSupport{}

// udpWithDNSSupport wraps layers.UDP to always decode the payload as DNS,
// regardless of the port number. This is necessary because gopacket's UDP layer
// has port-based heuristics that may try to decode port 5353 as mDNS or other
// protocols instead of standard DNS.
type udpWithDNSSupport struct {
	layers.UDP
}

// NextLayerType always returns LayerTypeDNS to ensure the payload is decoded as DNS
func (m *udpWithDNSSupport) NextLayerType() gopacket.LayerType {
	return layers.LayerTypeDNS
}
