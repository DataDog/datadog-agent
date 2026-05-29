// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package address provides an IP address abstraction that is family (v4/v6) agnostic.
// This package is extracted as a standalone module to enable independent consumption
// by packages like pkg/network and pkg/ebpf without pulling in the full agent.
package address

import (
	"encoding/binary"
	"net"
	"net/netip"

	"go4.org/netipx"
)

// Address is an IP abstraction that is family (v4/v6) agnostic
type Address struct {
	netip.Addr
}

// FromNetIP returns an Address from a provided net.IP
func FromNetIP(ip net.IP) Address {
	addr, _ := netipx.FromStdIP(ip)
	return Address{addr}
}

// FromString creates an Address using the string representation of an IP
func FromString(s string) Address {
	ip, _ := netip.ParseAddr(s)
	return Address{ip}
}

// NetIPFromAddress returns a net.IP from an Address
// Warning: the returned `net.IP` will share the same underlying
// memory as the given `buf` argument.
func NetIPFromAddress(addr Address, buf []byte) net.IP {
	n := copy(buf, addr.AsSlice())
	return net.IP(buf[:n])
}

// FromLowHigh creates an address from a pair of uint64 numbers
func FromLowHigh(l, h uint64) Address {
	if h > 0 {
		return V6(l, h)
	}
	return V4(uint32(l))
}

// ToLowHigh converts an address into a pair of uint64 numbers
func ToLowHigh(addr Address) (l, h uint64) {
	return ToLowHighIP(addr.Addr)
}

// ToLowHighIP converts a netip.Addr into a pair of uint64 numbers
func ToLowHighIP(a netip.Addr) (l, h uint64) {
	if a.Is6() {
		return toLowHigh16(a.As16())
	}
	return toLowHigh4(a.As4())
}

func toLowHigh4(b [4]byte) (l, h uint64) {
	return uint64(binary.LittleEndian.Uint32(b[:4])), uint64(0)
}

func toLowHigh16(b [16]byte) (l, h uint64) {
	return binary.LittleEndian.Uint64(b[8:]), binary.LittleEndian.Uint64(b[:8])
}

// V4 creates an Address using the uint32 representation of an v4 IP
func V4(ip uint32) Address {
	return Address{
		netip.AddrFrom4([4]byte{
			uint8(ip),
			uint8(ip >> 8),
			uint8(ip >> 16),
			uint8(ip >> 24),
		}),
	}
}

// V6 creates an Address using the uint128 representation of an v6 IP
func V6(low, high uint64) Address {
	var a [16]byte
	binary.LittleEndian.PutUint64(a[:8], high)
	binary.LittleEndian.PutUint64(a[8:], low)
	return Address{netip.AddrFrom16(a)}
}
