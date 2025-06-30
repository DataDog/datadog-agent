// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package udp

import (
	"fmt"
	"net"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/winconn"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sys/windows"
)

// TracerouteSequential runs a traceroute
func (u *UDPv4) TracerouteSequential() (*common.Results, error) {
	log.Debugf("Running UDP traceroute to %+v", u)
	// Get local address for the interface that connects to this
	// host and store in in the probe
	addr, conn, err := common.LocalAddrForHost(u.Target, u.TargetPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	// TODO: Need to call bind on our port?
	// When the UDP socket for this remains claimed, ICMP messages that we wish
	// to read on the raw socket created below are not received with the raw socket
	// This makes a case to investigate using 2 separate sockets for
	// Windows implementations in the future.
	conn.Close()
	u.srcIP = addr.IP
	u.srcPort = addr.AddrPort().Port()

	rs, err := winconn.NewRawConn()
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	defer rs.Close()

	hops := make([]*common.Hop, 0, int(u.MaxTTL-u.MinTTL)+1)

	for i := int(u.MinTTL); i <= int(u.MaxTTL); i++ {
		hop, err := u.sendAndReceive(rs, i, u.Timeout)
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
		Source:     u.srcIP,
		SourcePort: u.srcPort,
		Target:     u.Target,
		DstPort:    u.TargetPort,
		Hops:       hops,
	}, nil
}

func (u *UDPv4) sendAndReceive(rs winconn.RawConnWrapper, ttl int, timeout time.Duration) (*common.Hop, error) {
	ipHdrID, buffer, udpChecksum, err := u.createRawUDPBuffer(u.srcIP, u.srcPort, u.Target, u.TargetPort, ttl)
	if err != nil {
		log.Errorf("failed to create UDP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	err = rs.SendRawPacket(u.Target, u.TargetPort, buffer)
	if err != nil {
		log.Errorf("failed to send UDP packet: %s", err.Error())
		return nil, err
	}

	matcherFuncs := map[int]common.MatcherFunc{
		windows.IPPROTO_ICMP: u.icmpParser.Match,
	}
	start := time.Now() // TODO: is this the best place to start?
	hopIP, end, err := rs.ListenPackets(timeout, u.srcIP, u.srcPort, u.Target, u.TargetPort, uint32(udpChecksum), ipHdrID, matcherFuncs)
	if err != nil {
		log.Errorf("failed to listen for packets: %s", err.Error())
		return nil, err
	}

	rtt := time.Duration(0)
	if !hopIP.Equal(net.IP{}) {
		rtt = end.Sub(start)
	}

	return &common.Hop{
		IP:     hopIP,
		RTT:    rtt,
		IsDest: hopIP.Equal(u.Target),
	}, nil
}
