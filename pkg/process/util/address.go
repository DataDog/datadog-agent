// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NET) Fix revive linter
package util

import (
	"encoding/binary"
	"net"
	"net/netip"

	"go4.org/netipx"

	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// Address is an IP abstraction that is family (v4/v6) agnostic
type Address struct {
	netip.Addr
}

// WriteTo writes the address byte representation into the supplied buffer
func (a Address) WriteTo(b []byte) int {
	if a.Is4() {
		v := a.As4()
		return copy(b, v[:])
	}

	v := a.As16()
	return copy(b, v[:])

}

// Bytes returns a byte slice representing the Address.
// You may want to consider using `WriteTo` instead to avoid allocations
func (a Address) Bytes() []byte {
	// Note: this implicitly converts IPv4-in-6 to IPv4
	if a.Is4() || a.Is4In6() {
		v := a.As4()
		return v[:]
	}

	v := a.As16()
	return v[:]
}

// Len returns the number of bytes required to represent this IP
func (a Address) Len() int {
	return int(a.BitLen()) / 8
}

// IsZero reports whether a is its zero value
func (a Address) IsZero() bool {
	return a.Addr == netip.Addr{}
}

// AddressFromNetIP returns an Address from a provided net.IP
func AddressFromNetIP(ip net.IP) Address {
	addr, _ := netipx.FromStdIP(ip)
	return Address{addr}
}

// AddressFromString creates an Address using the string representation of an IP
func AddressFromString(s string) Address {
	ip, _ := netip.ParseAddr(s)
	return Address{ip}
}

// NetIPFromAddress returns a net.IP from an Address
// Warning: the returned `net.IP` will share the same underlying
// memory as the given `buf` argument.
func NetIPFromAddress(addr Address, buf []byte) net.IP {
	n := addr.WriteTo(buf)
	return net.IP(buf[:n])
}

// FromLowHigh creates an address from a pair of uint64 numbers
func FromLowHigh(l, h uint64) Address {
	if h > 0 {
		return V6Address(l, h)
	}

	return V4Address(uint32(l))
}

// ToLowHigh converts an address into a pair of uint64 numbers
func ToLowHigh(addr Address) (l, h uint64) {
	return ToLowHighIP(addr.Addr)
}

// ToLowHighIP converts a netaddr.IP into a pair of uint64 numbers
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

// V4Address creates an Address using the uint32 representation of an v4 IP
func V4Address(ip uint32) Address {
	return Address{
		netip.AddrFrom4([4]byte{
			uint8(ip),
			uint8(ip >> 8),
			uint8(ip >> 16),
			uint8(ip >> 24),
		}),
	}
}

// V4AddressFromBytes creates an Address using the byte representation of an v4 IP
func V4AddressFromBytes(buf []byte) Address {
	return Address{netip.AddrFrom4(*(*[4]byte)(buf))}
}

// V6Address creates an Address using the uint128 representation of an v6 IP
func V6Address(low, high uint64) Address {
	var a [16]byte
	binary.LittleEndian.PutUint64(a[:8], high)
	binary.LittleEndian.PutUint64(a[8:], low)
	return Address{netip.AddrFrom16(a)}
}

// V6AddressFromBytes creates an Address using the byte representation of an v6 IP
func V6AddressFromBytes(buf []byte) Address {
	return Address{netip.AddrFrom16(*(*[16]byte)(buf))}
}

// IPBufferPool is meant to be used in conjunction with `NetIPFromAddress`
var IPBufferPool = ddsync.NewTypedPool(func() *[]byte {
	b := make([]byte, net.IPv6len)
	return &b
})
