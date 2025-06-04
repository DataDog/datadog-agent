// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tcp adds a TCP traceroute implementation to the agent
package tcp

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/icmp"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/winconn"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TracerouteSequentialSocket runs a traceroute sequentially where a packet is
// sent and we wait for a response before sending the next packet
// This method uses socket options to set the TTL and get the hop IP
func (t *TCPv4) TracerouteSequentialSocket() (*common.Results, error) {
	log.Debugf("Running traceroute to %+v", t)
	// Get local address for the interface that connects to this
	// host and store in in the probe
	addr, conn, err := common.LocalAddrForHost(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	defer conn.Close()
	t.srcIP = addr.IP
	t.srcPort = addr.AddrPort().Port()

	hops := make([]*common.Hop, 0, int(t.MaxTTL-t.MinTTL)+1)

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		s, err := winconn.NewConn()
		if err != nil {
			return nil, fmt.Errorf("failed to create raw socket: %w", err)
		}
		hop, err := t.sendAndReceiveSocket(s, i, t.Timeout)
		s.Close()
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
		Tags:       []string{"tcp_method:syn_socket"},
	}, nil
}

func (t *TCPv4) sendAndReceiveSocket(s winconn.ConnWrapper, ttl int, timeout time.Duration) (*common.Hop, error) {
	// set the TTL
	err := s.SetTTL(ttl)
	if err != nil {
		return nil, fmt.Errorf("failed to set TTL: %w", err)
	}

	start := time.Now() // TODO: is this the best place to start?
	hopIP, end, icmpType, icmpCode, err := s.GetHop(timeout, t.Target, t.DestPort)
	if err != nil {
		log.Errorf("failed to get hop: %s", err.Error())
		return nil, fmt.Errorf("failed to get hop: %w", err)
	}

	rtt := time.Duration(0)
	if !hopIP.Equal(net.IP{}) {
		rtt = end.Sub(start)
	}

	return &common.Hop{
		IP:       hopIP,
		Port:     0, // TODO: fix this
		ICMPType: icmpType,
		ICMPCode: icmpCode,
		RTT:      rtt,
		IsDest:   hopIP.Equal(t.Target),
	}, nil
}

// TracerouteSequential runs a traceroute sequentially where a packet is
// sent and we wait for a response before sending the next packet
func (t *TCPv4) TracerouteSequential() (*common.Results, error) {
	log.Debugf("Running traceroute to %+v", t)
	// Get local address for the interface that connects to this
	// host and store in in the probe
	addr, conn, err := common.LocalAddrForHost(t.Target, t.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	defer conn.Close()
	t.srcIP = addr.IP
	t.srcPort = addr.AddrPort().Port()

	rs, err := winconn.NewRawConn()
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	defer rs.Close()

	hops := make([]*common.Hop, 0, int(t.MaxTTL-t.MinTTL)+1)

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		seqNumber, packetID := t.nextSeqNumAndPacketID()
		hop, err := t.sendAndReceive(rs, i, seqNumber, packetID, t.Timeout)
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
		Tags:       []string{"tcp_method:syn", fmt.Sprintf("paris_traceroute_mode_enabled:%t", t.ParisTracerouteMode)},
	}, nil
}

func (t *TCPv4) sendAndReceive(rs winconn.RawConnWrapper, ttl int, seqNum uint32, packetID uint16, timeout time.Duration) (*common.Hop, error) {
	_, buffer, _, err := t.createRawTCPSynBuffer(packetID, seqNum, ttl)
	if err != nil {
		log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	err = rs.SendRawPacket(t.Target, t.DestPort, buffer)
	if err != nil {
		log.Errorf("failed to send TCP packet: %s", err.Error())
		return nil, err
	}

	icmpParser := icmp.NewICMPTCPParser()
	tcpParser := newParser()
	matcherFuncs := map[int]common.MatcherFunc{
		windows.IPPROTO_ICMP: icmpParser.Match,
		windows.IPPROTO_TCP:  tcpParser.MatchTCP,
	}
	start := time.Now() // TODO: is this the best place to start?
	hopIP, end, err := rs.ListenPackets(timeout, t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum, packetID, matcherFuncs)
	if err != nil {
		log.Errorf("failed to listen for packets: %s", err.Error())
		return nil, err
	}

	rtt := time.Duration(0)
	if !hopIP.Equal(net.IP{}) {
		rtt = end.Sub(start)
	}

	return &common.Hop{
		IP:       hopIP,
		Port:     0, // TODO: fix this
		ICMPType: 0, // TODO: fix this
		RTT:      rtt,
		IsDest:   hopIP.Equal(t.Target),
	}, nil
}
