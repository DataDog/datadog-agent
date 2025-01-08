// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package udp

import (
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Traceroute runs a traceroute in parallel where all packets
// are sent simultaneously and all responses are received in parallel
func (u *UDPv4) TracerouteSequential() (*common.Results, error) {
	log.Debugf("Running UDP traceroute to %+v", u)
	// Get local address for the interface that connects to this
	// host and store in in the probe
	//
	// TODO: ensure we hold the UDP port created here since we can
	addr, conn, err := common.LocalAddrForHost(u.Target, u.DestPort)
	if err != nil {
		return nil, fmt.Errorf("failed to get local address for target: %w", err)
	}
	defer conn.Close()
	u.srcIP = addr.IP
	u.srcPort = addr.AddrPort().Port()

	rs, err := common.CreateRawSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	defer rs.Close()

	hops := make([]*common.Hop, 0, int(u.MaxTTL-u.MinTTL)+1)

	for i := int(u.MinTTL); i <= int(u.MaxTTL); i++ {
		seqNumber := rand.Uint32()
		hop, err := u.sendAndReceive(rs, i, seqNumber, u.Timeout)
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
		DstPort:    u.DestPort,
		Hops:       hops,
	}, nil
}

func (u *UDPv4) sendAndReceive(rs *common.Winrawsocket, ttl int, seqNum uint32, timeout time.Duration) (*common.Hop, error) {
	// _, buffer, udpChecksum, _, err := createRawUDPBuffer(u.srcIP, u.srcPort, u.Target, u.DestPort, ttl)
	// if err != nil {
	// 	log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
	// 	return nil, err
	// }

	// err = rs.SendRawPacket(u.Target, u.DestPort, buffer)
	// if err != nil {
	// 	log.Errorf("failed to send UDP packet: %s", err.Error())
	// 	return nil, err
	// }

	// matcherFuncs := map[int]common.MatcherFunc{
	// 	windows.IPPROTO_ICMP: common.MatchICMP,
	// }
	// start := time.Now() // TODO: is this the best place to start?
	// hopIP, end, err := rs.ListenPackets(timeout, u.srcIP, u.srcPort, u.Target, u.DestPort)
	// if err != nil {
	// 	log.Errorf("failed to listen for packets: %s", err.Error())
	// 	return nil, err
	// }

	// rtt := time.Duration(0)
	// if !hopIP.Equal(net.IP{}) {
	// 	rtt = end.Sub(start)
	// }

	// return &common.Hop{
	// 	IP:     hopIP,
	// 	RTT:    rtt,
	// 	IsDest: hopIP.Equal(u.Target),
	// }, nil
	return nil, nil
}
