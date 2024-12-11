// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package common

import (
	"fmt"
	"net"

	"golang.org/x/sys/windows"
)

var (
	sendTo = windows.Sendto
)

type Winrawsocket struct {
	Socket windows.Handle
}

func (w *Winrawsocket) Close() {
	if w.Socket != windows.InvalidHandle {
		windows.Closesocket(w.Socket) // nolint: errcheck
	}
	w.Socket = windows.InvalidHandle
}

func CreateRawSocket() (*Winrawsocket, error) {
	s, err := windows.Socket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_IP)
	if err != nil {
		return nil, fmt.Errorf("failed to create raw socket: %w", err)
	}
	on := int(1)
	err = windows.SetsockoptInt(s, windows.IPPROTO_IP, windows.IP_HDRINCL, on)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set IP_HDRINCL: %w", err)
	}

	err = windows.SetsockoptInt(s, windows.SOL_SOCKET, windows.SO_RCVTIMEO, 100)
	if err != nil {
		windows.Closesocket(s) // nolint: errcheck
		return nil, fmt.Errorf("failed to set SO_RCVTIMEO: %w", err)
	}
	return &Winrawsocket{Socket: s}, nil
}

func sendRawPacket(w *Winrawsocket, destIP net.IP, destPort uint16, payload []byte) error {

	dst := destIP.To4()
	sa := &windows.SockaddrInet4{
		Port: int(destPort),
		Addr: [4]byte{dst[0], dst[1], dst[2], dst[3]},
	}
	if err := sendTo(w.Socket, payload, 0, sa); err != nil {
		return fmt.Errorf("failed to send packet: %w", err)
	}
	return nil
}
