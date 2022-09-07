// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !android
// +build linux,!android

package netlink

import "net/netip"

func AddrIsZero(addr netip.Addr) bool {
	return addr == netip.Addr{}
}

func AddrPortIsZero(addrPort netip.AddrPort) bool {
	return addrPort == netip.AddrPort{}
}

func AddrPortWithAddr(addrPort netip.AddrPort, addr netip.Addr) netip.AddrPort {
	return netip.AddrPortFrom(addr, addrPort.Port())
}

func AddrPortWithPort(addrPort netip.AddrPort, port uint16) netip.AddrPort {
	return netip.AddrPortFrom(addrPort.Addr(), port)
}
