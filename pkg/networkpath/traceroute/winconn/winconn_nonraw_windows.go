// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package winconn

import (
	"errors"
	"fmt"
	net "net"
	"syscall"
	"time"

	common "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	ipv4 "golang.org/x/net/ipv4"
)

type (
	NonRawConn struct {
		icmpConn net.PacketConn
		tcpFD    syscall.Handle
	}
)

func NewNonRawConn() (*NonRawConn, error) {
	// Open raw socket to listen for ICMP responses

	// TODO: replace 0.0.0.0 with actual IP of the interface
	// we want to listen to
	icmpConn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return nil, fmt.Errorf("failed to create ICMP listener: %w", err)
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, fmt.Errorf("socket creation error: %w", err)
	}

	return &NonRawConn{
		icmpConn: icmpConn,
		tcpFD:    fd,
	}, nil
}

func (n *NonRawConn) Close() {
	n.icmpConn.Close()
	syscall.Close(n.tcpFD)
}

func (n *NonRawConn) SendRawPacket(destIP net.IP, destPort uint16, _ []byte) error {
	// TODO: if this works well, the Windows implementation will need to be re-worked
	// but for now just call connect
	sa := &syscall.SockaddrInet4{Port: int(destPort)}
	copy(sa.Addr[:], destIP.To4())

	// Start connection attempt (non-blocking)
	err := syscall.Connect(n.tcpFD, sa)
	if err != nil && !errors.Is(err, syscall.EINPROGRESS) {
		return err
	}

	return nil
}

func (n *NonRawConn) ReadFrom(b []byte) (*ipv4.Header, []byte, error) {
	now := time.Now()
	err := n.icmpConn.SetReadDeadline(now.Add(100 * time.Millisecond))
	if err != nil {
		return nil, nil, err
	}

	buf := make([]byte, 1500)
	_, addr, err := n.icmpConn.ReadFrom(buf)
	if err != nil {
		return nil, nil, err
	}

	header := ipv4.Header{
		Src: net.ParseIP(addr.String()),
	}

	return &header, buf, nil
}

func (n *NonRawConn) ListenPackets(timeout time.Duration, localIP net.IP, localPort uint16, remoteIP net.IP, remotePort uint16, innerIdentifier uint32, matcherFuncs map[int]common.MatcherFunc) (net.IP, time.Time, error) {
	return net.IP{}, time.Time{}, errors.New("not implemented")
}
