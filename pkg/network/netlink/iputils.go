// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import "net/netip"

// AddrIsZero reports whether addr is its zero value
func AddrIsZero(addr netip.Addr) bool {
	return addr == netip.Addr{}
}

// AddrPortIsZero reports whether addrPort is its zero value
func AddrPortIsZero(addrPort netip.AddrPort) bool {
	return addrPort == netip.AddrPort{}
}

// AddrPortWithAddr returns an AddrPort with Addr addr and port addrPort.Port()
func AddrPortWithAddr(addrPort netip.AddrPort, addr netip.Addr) netip.AddrPort {
	return netip.AddrPortFrom(addr, addrPort.Port())
}

// AddrPortWithPort returns an AddrPort with Addr addrPort.Addr() and port port
func AddrPortWithPort(addrPort netip.AddrPort, port uint16) netip.AddrPort {
	return netip.AddrPortFrom(addrPort.Addr(), port)
}
