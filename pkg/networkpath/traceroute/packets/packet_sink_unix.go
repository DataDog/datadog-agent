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

// NewSinkUnix returns a new SinkUnix implementing packet sink
func NewSinkUnix(addr netip.Addr) (Sink, error) {
	if !addr.Is4() {
		return nil, fmt.Errorf("SinkUnix only supports IPv4 addresses (for now)")
	}
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_RAW|unix.SOCK_NONBLOCK, unix.IPPROTO_RAW)
	if err != nil {
		return nil, fmt.Errorf("NewSinkUnix failed to create socket: %s", err)
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
func (p *sinkUnix) WriteTo(buf []byte, addrPort netip.AddrPort) error {
	if !addrPort.Addr().Is4() {
		return fmt.Errorf("SinkUnix only supports IPv4 addresses (for now)")
	}
	sa := &unix.SockaddrInet4{
		Addr: addrPort.Addr().As4(),
	}

	var sendtoErr error
	err := p.rawConn.Write(func(fd uintptr) (done bool) {
		err := unix.Sendto(int(fd), buf, 0, sa)
		if err == nil {
			return true
		}

		return err == syscall.EAGAIN || err == syscall.EWOULDBLOCK
	})

	return errors.Join(sendtoErr, err)
}

// Close closes the socket
func (p *sinkUnix) Close() error {
	var closeErr error
	err := p.rawConn.Control(func(fd uintptr) {
		closeErr = unix.Close(int(fd))
	})

	return errors.Join(closeErr, err)
}
