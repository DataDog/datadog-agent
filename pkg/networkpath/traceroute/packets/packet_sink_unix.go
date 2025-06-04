// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package packets

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

// sinkUnix is an implementation of the packet sink interface for unix OSes
type sinkUnix struct {
	rawConn syscall.RawConn
}

var _ Sink = &sinkUnix{}

// NewSinkUnix returns a new sinkUnix implementing packet sink
func NewSinkUnix(addr netip.Addr) (Sink, error) {
	var domain, protocol int

	if addr.Is4() {
		domain = unix.AF_INET
		protocol = unix.IPPROTO_RAW
	} else if addr.Is6() {
		domain = unix.AF_INET6
		protocol = unix.IPPROTO_RAW
	} else {
		return nil, fmt.Errorf("SinkUnix supports only IPv4 or IPv6 addresses")
	}

	fd, err := unix.Socket(domain, unix.SOCK_RAW|unix.SOCK_NONBLOCK, protocol)
	if err != nil {
		return nil, fmt.Errorf("NewSinkUnix failed to create socket: %w", err)
	}
	if addr.Is4() {
		err = unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_HDRINCL, 1)
		if err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("failed to set IP_HDRINCL: %w", err)
		}
	} else {
		err = unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_HDRINCL, 1)
		if err != nil {
			unix.Close(fd)
			return nil, fmt.Errorf("failed to set IPV6_HDRINCL: %w", err)
		}
	}
	sock := os.NewFile(uintptr(fd), "")
	rawConn, err := sock.SyscallConn()
	if err != nil {
		sock.Close()
		return nil, fmt.Errorf("failed to get raw connection: %w", err)
	}

	return &sinkUnix{
		rawConn: rawConn,
	}, nil
}

// WriteTo writes the given packet (buffer starts at the IP layer) to addrPort.
func (p *sinkUnix) WriteTo(buf []byte, addr netip.Addr) error {
	sa, err := getSockAddr(addr)
	if err != nil {
		return err
	}

	var sendtoErr error
	err = p.rawConn.Write(func(fd uintptr) (done bool) {
		err := unix.Sendto(int(fd), buf, 0, sa)
		if err == nil {
			return true
		}

		return err == syscall.EAGAIN || err == syscall.EWOULDBLOCK
	})

	return errors.Join(sendtoErr, err)
}

func getSockAddr(addr netip.Addr) (unix.Sockaddr, error) {
	switch {
	case addr.Is4():
		var sa4 unix.SockaddrInet4
		b := addr.As4()
		copy(sa4.Addr[:], b[:])
		return &sa4, nil
	case addr.Is6():
		var sa6 unix.SockaddrInet6
		b := addr.As16()
		copy(sa6.Addr[:], b[:])
		return &sa6, nil
	default:
		return nil, fmt.Errorf("invalid IP address")
	}
}

// Close closes the socket
func (p *sinkUnix) Close() error {
	var closeErr error
	err := p.rawConn.Control(func(fd uintptr) {
		closeErr = unix.Close(int(fd))
	})

	return errors.Join(closeErr, err)
}
