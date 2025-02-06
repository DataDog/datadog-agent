// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"golang.org/x/net/ipv4"
)

type (
	// tcpResponse encapsulates the data from a
	// TCP response needed for matching
	tcpResponse struct {
		SrcIP       net.IP
		DstIP       net.IP
		TCPResponse layers.TCP
	}

	// parser encapsulates everything needed to
	// decode TCP packets off the wire into structs
	parser struct {
		layer               layers.TCP
		decoded             []gopacket.LayerType
		decodingLayerParser *gopacket.DecodingLayerParser
	}
)

func newParser() *parser {
	tcpParser := &parser{}
	tcpParser.decodingLayerParser = gopacket.NewDecodingLayerParser(layers.LayerTypeTCP, &tcpParser.layer)
	tcpParser.decodingLayerParser.IgnoreUnsupported = true
	return tcpParser
}

func (tp *parser) parseTCP(header *ipv4.Header, payload []byte) (*tcpResponse, error) {
	if header.Protocol != syscall.IPPROTO_TCP || header.Version != 4 ||
		header.Src == nil || header.Dst == nil {
		return nil, fmt.Errorf("invalid IP header for TCP packet: %+v", header)
	}
	if len(payload) <= 0 {
		return nil, errors.New("received empty TCP payload")
	}

	if err := tp.decodingLayerParser.DecodeLayers(payload, &tp.decoded); err != nil {
		return nil, fmt.Errorf("failed to decode TCP packet: %w", err)
	}

	resp := &tcpResponse{
		SrcIP:       header.Src,
		DstIP:       header.Dst,
		TCPResponse: tp.layer,
	}
	// make sure the TCP layer is cleared between runs
	tp.layer = layers.TCP{}

	return resp, nil
}

// MatchTCP parses a TCP packet from a header and packet bytes and compares the information
// contained in the packet to what's expected and returns the source IP of the incoming packet
// if it's successful or a MismatchError if the packet can be read but doesn't match
func (tp *parser) MatchTCP(header *ipv4.Header, packet []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, error) {
	if header.Protocol != 6 { // TCP
		return net.IP{}, errors.New("expected a TCP packet")
	}
	// don't even bother parsing the packet if the src/dst ip don't match
	if !localIP.Equal(header.Dst) || !remoteIP.Equal(header.Src) {
		return net.IP{}, common.MismatchError("TCP packet doesn't match")
	}
	tcpResp, err := tp.parseTCP(header, packet)
	if err != nil {
		return net.IP{}, fmt.Errorf("TCP parse error: %w", err)
	}
	if !tcpResp.Match(localIP, localPort, remoteIP, remotePort, seqNum) {
		return net.IP{}, common.MismatchError("TCP packet doesn't match")
	}

	return tcpResp.SrcIP, nil
}

func (t *tcpResponse) Match(localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) bool {
	flagsCheck := (t.TCPResponse.SYN && t.TCPResponse.ACK) || t.TCPResponse.RST
	sourcePort := uint16(t.TCPResponse.SrcPort)
	destPort := uint16(t.TCPResponse.DstPort)

	return remoteIP.Equal(t.SrcIP) &&
		remotePort == sourcePort &&
		localIP.Equal(t.DstIP) &&
		localPort == destPort &&
		seqNum == t.TCPResponse.Ack-1 &&
		flagsCheck
}
