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

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type winrawsocket struct {
	s windows.Handle
}

func (w *winrawsocket) close() {
	if w.s != windows.InvalidHandle {
		windows.Closesocket(w.s)
	}
	w.s = windows.InvalidHandle
}

func (t *TCPv4) sendRawPacket(w *winrawsocket, payload []byte) error {

	sa := &windows.SockaddrInet4{
		Port: int(t.DestPort), 
		Addr: [4]byte{t.Target[12], t.Target[13], t.Target[14], t.Target[15]},
	}
	if err := windows.Sendto(w.s, payload, 0, sa); err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}
	return nil
}

func createRawSocket() (*winrawsocket, error) {
	s, err := windows.Socket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	on := int(1)
	err = windows.SetsockoptInt(s, windows.IPPROTO_IP, windows.IP_HDRINCL, on)
	if err != nil {
		windows.Closesocket(s)
		return nil, fmt.Errorf("failed to set IP_HDRINCL: %w", err)
	}

	err = windows.SetsockoptInt(s, windows.SOL_SOCKET, windows.SO_RCVTIMEO, 100)
	if err != nil {
		windows.Closesocket(s)
		return nil, fmt.Errorf("failed to set SO_RCVTIMEO: %w", err)
	}
	return &winrawsocket{s: s}, nil
}
// TracerouteSequential runs a traceroute sequentially where a packet is
// sent and we wait for a response before sending the next packet
func (t *TCPv4) TracerouteSequential() (*Results, error) {
	log.Debugf("Running traceroute to %+v", t)
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

	rs, err := createRawSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	defer rs.close()

	hops := make([]*Hop, 0, int(t.MaxTTL-t.MinTTL)+1)

	for i := int(t.MinTTL); i <= int(t.MaxTTL); i++ {
		seqNumber := rand.Uint32()
		hop, err := t.sendAndReceive(rs, i, seqNumber, t.Timeout)
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

	return nil, nil
}


func (t *TCPv4) sendAndReceive(rs *winrawsocket, ttl int, seqNum uint32, timeout time.Duration) (*Hop, error) {
	_, buffer, _, err := createRawTCPSynBuffer(t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum, ttl)
	if err != nil {
		log.Errorf("failed to create TCP packet with TTL: %d, error: %s", ttl, err.Error())
		return nil, err
	}

	err = t.sendRawPacket(rs, buffer)
	if err != nil {
		log.Errorf("failed to send TCP packet: %s", err.Error())
		return nil, err
	}

	start := time.Now() // TODO: is this the best place to start?
	hopIP, hopPort, icmpType, end, err := rs.listenPackets(timeout, t.srcIP, t.srcPort, t.Target, t.DestPort, seqNum)
	if err != nil {
		log.Errorf("failed to listen for packets: %s", err.Error())
		return nil, err
	}

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
