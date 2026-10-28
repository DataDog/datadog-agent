// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || (linux && bpf) || darwin

package dns

import (
	"encoding/binary"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

var _ gopacket.DecodingLayer = &tcpWithDNSSupport{}

// source: https://github.com/weaveworks/scope/blob/master/probe/endpoint/dns_snooper.go
// Gopacket doesn't provide direct support for DNS over TCP, see https://github.com/google/gopacket/issues/236
type tcpWithDNSSupport struct {
	layers.TCP
}

// A DNS message in a TCP segment is preceded by extra two bytes that
// contain the size of the message. If the segment starts with a whole
// message, it can be decoded like a UDP payload. Any further messages in the
// same segment are handled by the parser (see appendTrailingResponses).
func (m *tcpWithDNSSupport) startsWithCompleteDNSMessage() bool {
	payload := m.TCP.LayerPayload()
	if len(payload) < 2 {
		return false
	}
	dnsLengthField := int(binary.BigEndian.Uint16(payload))
	return dnsLengthField >= dnsHeaderLen && dnsLengthField <= len(payload)-2 && isStandardQueryHeader(payload[2:])
}

func (m *tcpWithDNSSupport) NextLayerType() gopacket.LayerType {
	if m.startsWithCompleteDNSMessage() {
		return layers.LayerTypeDNS
	}
	// The segment holds part of a message split across segments. Decoding it
	// as DNS would fail, so stop here; the parser can still match a response
	// from its header (see parseResponseHeaderInto).
	// TODO: reassemble split messages so their answers reach the reverse DNS cache
	return gopacket.LayerTypeZero
}

func (m *tcpWithDNSSupport) LayerPayload() []byte {
	payload := m.TCP.LayerPayload()
	if len(payload) > 1 {
		// Omit the DNS length field, only included in TCP, and any messages
		// after the first one, in order to reuse the DNS UDP parser
		end := 2 + int(binary.BigEndian.Uint16(payload))
		payload = payload[2:min(end, len(payload))]
	}
	return payload
}
