// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"net"
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/util/address"
	ddsync "github.com/DataDog/datadog-agent/pkg/util/sync"
)

// Address is an IP abstraction that is family (v4/v6) agnostic.
// This is a type alias for address.Address from the standalone pkg/util/address module.
// New code should import pkg/util/address directly.
type Address = address.Address

// AddressFromNetIP returns an Address from a provided net.IP
func AddressFromNetIP(ip net.IP) Address {
	return address.FromNetIP(ip)
}

// AddressFromString creates an Address using the string representation of an IP
func AddressFromString(s string) Address {
	return address.FromString(s)
}

// NetIPFromAddress returns a net.IP from an Address
// Warning: the returned `net.IP` will share the same underlying
// memory as the given `buf` argument.
func NetIPFromAddress(addr Address, buf []byte) net.IP {
	return address.NetIPFromAddress(addr, buf)
}

// FromLowHigh creates an address from a pair of uint64 numbers
func FromLowHigh(l, h uint64) Address {
	return address.FromLowHigh(l, h)
}

// ToLowHigh converts an address into a pair of uint64 numbers
func ToLowHigh(addr Address) (l, h uint64) {
	return address.ToLowHigh(addr)
}

// ToLowHighIP converts a netip.Addr into a pair of uint64 numbers
func ToLowHighIP(a netip.Addr) (l, h uint64) {
	return address.ToLowHighIP(a)
}

// V4Address creates an Address using the uint32 representation of an v4 IP
func V4Address(ip uint32) Address {
	return address.V4(ip)
}

// V6Address creates an Address using the uint128 representation of an v6 IP
func V6Address(low, high uint64) Address {
	return address.V6(low, high)
}

// IPBufferPool is meant to be used in conjunction with `NetIPFromAddress`
var IPBufferPool = ddsync.NewSlicePool[byte](net.IPv6len, net.IPv6len)
