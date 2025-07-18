// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package packets

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	"golang.org/x/sys/windows"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
)

// RawConn is a struct that encapsulates a raw socket
// on Windows that can be used to listen to traffic on a host
// or send raw packets from a host
type RawConn struct {
	closeOnce sync.Once
	deadline  time.Time
	socket    windows.Handle
}

// NewRawConn creates a Winrawsocket
func NewRawConn(family gopacket.LayerType) (*RawConn, error) {
	switch family {
	case layers.LayerTypeIPv4:
		// supported
	case layers.LayerTypeIPv6:
		// https://learn.microsoft.com/en-us/windows/win32/winsock/tcp-ip-raw-sockets-2
		// For IPv6 (address family of AF_INET6), an application receives everything
		// after the last IPv6 header in each received datagram regardless of the
		// IPV6_HDRINCL socket option. The application does not receive any IPv6 headers
		// using a raw socket.
		return nil, fmt.Errorf("NewRawConn: IPv6 is not supported")
	default:
		return nil, fmt.Errorf("NewRawConn: unknown IP family %s", family)
	}

	s, err := windows.Socket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	err = windows.SetsockoptInt(s, windows.IPPROTO_IP, windows.IP_HDRINCL, 1)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set IP_HDRINCL: %w", err)
	}

	return &RawConn{socket: s}, nil
}

var _ Source = &RawConn{}
var _ Sink = &RawConn{}

// Close closes the socket
func (r *RawConn) Close() error {
	var err error
	r.closeOnce.Do(func() {
		err = windows.Closesocket(r.socket)
		r.socket = windows.InvalidHandle
	})
	return err
}

func (r *RawConn) getReadTimeout() time.Duration {
	const (
		defaultTimeout = 1000 * time.Millisecond
		minTimeout     = 100 * time.Millisecond
	)
	// always return a timeout because we don't want the syscall to block forever
	if r.deadline.IsZero() {
		return defaultTimeout
	}

	timeout := time.Until(r.deadline)
	// I don't think SO_RCVTIMEO is going to be that precise, so add a min timeout
	// to avoid making a syscall that is doomed to fail
	if timeout < minTimeout {
		return minTimeout
	}
	return timeout

}

// Read reads a packet (starting with the IP frame)
func (r *RawConn) Read(buf []byte) (int, error) {
	timeoutMs := r.getReadTimeout().Milliseconds()

	err := windows.SetsockoptInt(r.socket, windows.SOL_SOCKET, windows.SO_RCVTIMEO, int(timeoutMs))
	if err != nil {
		return 0, fmt.Errorf("failed to set SO_RCVTIMEO: %w", err)
	}

	n, _, err := windows.Recvfrom(r.socket, buf, 0)
	if err == windows.WSAETIMEDOUT || err == windows.WSAEMSGSIZE {
		return 0, &common.ReceiveProbeNoPktError{Err: err}
	}

	// windows returns -1 on errors, unlike unix
	if n < 0 {
		n = 0
	}
	return n, err
}

// SetReadDeadline sets the deadline for when a Read() call must finish
func (r *RawConn) SetReadDeadline(t time.Time) error {
	r.deadline = t
	return nil
}

// WriteTo writes the given packet (buffer starts at the IP layer) to addrPort.
// (the port is required for compatibility with Windows)
func (r *RawConn) WriteTo(buf []byte, addrPort netip.AddrPort) error {
	sa := &windows.SockaddrInet4{
		Port: int(addrPort.Port()),
	}
	copy(sa.Addr[:], addrPort.Addr().AsSlice())
	err := windows.Sendto(r.socket, buf, 0, sa)
	if err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}
	return nil
}
