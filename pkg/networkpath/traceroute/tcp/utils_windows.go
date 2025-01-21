// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// MatchTCP parses a TCP packet from a header and packet bytes and compares the information
// contained in the packet to what's expected and returns the source IP of the incoming packet
// if it's successful or a MismatchError if the packet can be read but doesn't match
func (tp *tcpParser) MatchTCP(header *ipv4.Header, packet []byte, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, seqNum uint32) (net.IP, error) {
	if header.Protocol != windows.IPPROTO_TCP {
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
	if !tcpMatch(localIP, localPort, remoteIP, remotePort, seqNum, tcpResp) {
		return net.IP{}, common.MismatchError("TCP packet doesn't match")
	}

	return tcpResp.SrcIP, nil
}
