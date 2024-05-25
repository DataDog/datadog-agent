// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"fmt"
	"math/rand"
	"net"
	"time"

	"golang.org/x/net/ipv4"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/google/gopacket/layers"
)

type (
	// TCPv4 encapsulates the data needed to run
	// a TCPv4 traceroute
	TCPv4 struct {
		Target   net.IP
		srcIP    net.IP // calculated internally
		srcPort  uint16 // calculated internally
		DestPort uint16
		NumPaths uint16
		MinTTL   uint8
		MaxTTL   uint8
		Delay    time.Duration // delay between sending packets (not applicable if we go the serial send/receive route)
		Timeout  time.Duration // full timeout for all packets
	}

	// Results encapsulates a response from the TCP
	// traceroute
	Results struct {
		Source     net.IP
		SourcePort uint16
		Target     net.IP
		DstPort    uint16
		Hops       []*Hop
	}

	// Hop encapsulates information about a single
	// hop in a TCP traceroute
	Hop struct {
		IP       net.IP
		Port     uint16
		ICMPType layers.ICMPv4TypeCode
		RTT      time.Duration
		IsDest   bool
	}
)

// TracerouteSequential runs a traceroute sequentially where a packet is
// sent and we wait for a response before sending the next packet
func (t *TCPv4) TracerouteSequential() (*Results, error) {
	// Get local address for the interface that connects to this
	// host and store in in the probe
	//
	// TODO: do this once for the probe and hang on to the
	// listener until we decide to close the probe
	addr, err := localAddrForHost(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	t.srcIP = addr.IP
	t.srcPort = addr.AddrPort().Port()

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

	// Create a raw TCP listener to catch the TCP response from our final
	// hop if we get one
	tcpConn, err := net.ListenPacket("ip4:tcp", addr.IP.String())
	if err != nil {
		return nil, fmt.Errorf("failed to create TCP listener: %w", err)
	}
	defer tcpConn.Close()
	log.Debugf("Listening for TCP on: %s\n", addr.IP.String()+":"+addr.AddrPort().String())
	// RawConn is necessary to set the TTL and ID fields
	rawTCPConn, err := ipv4.NewRawConn(tcpConn)
	if err != nil {
		return nil, fmt.Errorf("failed to get raw TCP listener: %w", err)
	}

	// hops should be of length # of hops
	hops := make([]*Hop, 0, t.MaxTTL-t.MinTTL)

	// TODO: better logic around timeout for sequential is needed
	// right now we're just hacking around the existing
	// need to convert uint8 to int for proper conversion to
	// time.Duration
	timeout := t.Timeout / time.Duration(int(t.MaxTTL-t.MinTTL))

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		seqNumber := rand.Uint32()
		hop, err := t.sendAndReceive(rawIcmpConn, rawTCPConn, i, seqNumber, timeout)
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

	return &Results{
		Source:     t.srcIP,
		SourcePort: t.srcPort,
		Target:     t.Target,
		DstPort:    t.DestPort,
		Hops:       hops,
	}, nil
}

func (t *TCPv4) sendAndReceive(rawIcmpConn *ipv4.RawConn, rawTCPConn *ipv4.RawConn, ttl int, seqNum uint32, timeout time.Duration) (*Hop, error) {
	flags := byte(0)
	flags |= SYN
	tcpHeader, tcpPacket, err := createRawTCPPacket(t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum, ttl, flags)
	if err != nil {
		log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	err = sendPacket(rawTCPConn, tcpHeader, tcpPacket)
	if err != nil {
		log.Errorf("failed to send TCP SYN: %s", err.Error())
		return nil, err
	}

	start := time.Now() // TODO: is this the best place to start?
	hopIP, hopPort, icmpType, end, err := listenAnyPacket(rawIcmpConn, rawTCPConn, timeout, t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum)
	if err != nil {
		log.Errorf("failed to listen for packets: %s", err.Error())
		return nil, err
	}
	log.Debugf("Finished loop for TTL %d", ttl)

	rtt := time.Duration(0)
	if !hopIP.Equal(net.IP{}) {
		rtt = end.Sub(start)
	}

	return &Hop{
		IP:       hopIP,
		Port:     hopPort,
		ICMPType: icmpType,
		RTT:      rtt,
		IsDest:   hopIP.Equal(t.Target),
	}, nil
}

// Close doesn't to anything yet, but we should
// use this to close out long running sockets
// when we're done with a path test
func (t *TCPv4) Close() error {
	return nil
}
