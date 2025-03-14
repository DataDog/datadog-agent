// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	sendPacketFunc    = sendPacket    // for testing
	listenPacketsFunc = listenPackets // for testing
)

type (
	rawConnWrapper interface {
		SetReadDeadline(t time.Time) error
		ReadFrom(b []byte) (*ipv4.Header, []byte, *ipv4.ControlMessage, error)
		WriteTo(h *ipv4.Header, p []byte, cm *ipv4.ControlMessage) error
	}
)

// TracerouteSequential runs a traceroute sequentially where a packet is
// sent and we wait for a response before sending the next packet
func (t *TCPv4) TracerouteSequential() (*common.Results, error) {
	// Get local address for the interface that connects to this
	// host and store in in the probe
	addr, conn, err := common.LocalAddrForHost(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	conn.Close() // we don't need the UDP port here
	t.srcIP = addr.IP

	// So far I haven't had success trying to simply create a socket
	// using syscalls directly, but in theory doing so would allow us
	// to avoid creating two listeners since we could see all IP traffic
	// this way
	//
	// Create a raw ICMP listener to catch ICMP responses
	icmpConn, err := net.ListenPacket("ip4:icmp", addr.IP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create ICMP listener: %w", err)
	}
	defer icmpConn.Close()
	// RawConn is necessary to set the TTL and ID fields
	rawIcmpConn, err := ipv4.NewRawConn(icmpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw ICMP listener: %w", err)
	}

	// Create a TCP listener with port 0 to get a random port from the OS
	// and reserve it for the duration of the traceroute
	port, tcpListener, err := reserveLocalPort()
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	defer tcpListener.Close()
	t.srcPort = port

	// Create a raw TCP listener to catch the TCP response from our final
	// hop if we get one
	tcpConn, err := net.ListenPacket("ip4:tcp", addr.IP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	defer tcpConn.Close()
	log.Tracef("Listening for TCP on: %s\n", addr.IP.String()+":"+addr.AddrPort().String())
	// RawConn is necessary to set the TTL and ID fields
	rawTCPConn, err := ipv4.NewRawConn(tcpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw TCP listener: %w", err)
	}

	// hops should be of length # of hops
	hops := make([]*common.Hop, 0, t.MaxTTL-t.MinTTL)

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		seqNumber := rand.Uint32()
		hop, err := t.sendAndReceive(rawIcmpConn, rawTCPConn, i, seqNumber, t.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to run traceroute: %w", err)
		}
		hops = append(hops, hop)
		log.Tracef("Discovered hop: %+v", hop)
		// if we've reached our destination,
		// we're done
		if hop.IsDest {
			break
		}
	}

	return &common.Results{
		Source:     t.srcIP,
		SourcePort: t.srcPort,
		Target:     t.Target,
		DstPort:    t.DestPort,
		Hops:       hops,
	}, nil
}

func (t *TCPv4) sendAndReceive(rawIcmpConn rawConnWrapper, rawTCPConn rawConnWrapper, ttl int, seqNum uint32, timeout time.Duration) (*common.Hop, error) {
	tcpHeader, tcpPacket, err := t.createRawTCPSyn(seqNum, ttl)
	if err != nil {
		log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	err = sendPacketFunc(rawTCPConn, tcpHeader, tcpPacket)
	if err != nil {
		log.Errorf("failed to send TCP SYN: %s", err.Error())
		return nil, err
	}

	start := time.Now()
	resp := listenPacketsFunc(rawIcmpConn, rawTCPConn, timeout, t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum)
	if resp.Err != nil {
		log.Errorf("failed to listen for packets: %s", resp.Err.Error())
		return nil, resp.Err
	}

	rtt := time.Duration(0)
	if !resp.IP.Equal(net.IP{}) {
		rtt = resp.Time.Sub(start)
	}

	return &common.Hop{
		IP:       resp.IP,
		Port:     resp.Port,
		ICMPType: resp.Type,
		ICMPCode: resp.Code,
		RTT:      rtt,
		IsDest:   resp.IP.Equal(t.Target),
	}, nil
}
