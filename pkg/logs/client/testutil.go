// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package client

import (
	"net"
)

// AddrToHostPort converts a net.Addr to a (string, int).
func AddrToHostPort(remoteAddr net.Addr) (string, int) {
	switch addr := remoteAddr.(type) {
	case *net.UDPAddr:
		return addr.IP.String(), int(addr.Port)
	case *net.TCPAddr:
		return addr.IP.String(), int(addr.Port)
	}
	return "", 0
}

// AddrToEndPoint creates an EndPoint from an Addr.
func AddrToEndPoint(addr net.Addr) Endpoint {
	host, port := AddrToHostPort(addr)
	return Endpoint{Host: host, Port: port}
}

// AddrToDestination creates a Destination from an Addr
func AddrToDestination(addr net.Addr, ctx *DestinationsContext) *Destination {
	return NewDestination(AddrToEndPoint(addr), ctx)
}
