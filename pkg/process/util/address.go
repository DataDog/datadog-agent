// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"encoding/binary"
	"net"
	"sync"
)

// Address is an IP abstraction that is family (v4/v6) agnostic
type Address interface {
	Bytes() []byte
	WriteTo([]byte) int
	String() string
	IsLoopback() bool
	Len() int
}

// AddressFromNetIP returns an Address from a provided net.IP
func AddressFromNetIP(ip net.IP) Address {
	if v4 := ip.To4(); v4 != nil {
		var a v4Address
		copy(a[:], v4)
		return a
	}

	var a v6Address
	copy(a[:], ip)
	return a
}

// AddressFromString creates an Address using the string representation of an v4 IP
func AddressFromString(ip string) Address {
	return AddressFromNetIP(net.ParseIP(ip))
}

// NetIPFromAddress returns a net.IP from an Address
// Warning: the returned `net.IP` will share the same underlying
// memory as the given `buf` argument.
func NetIPFromAddress(addr Address, buf []byte) net.IP {
	if addrLen := addr.Len(); len(buf) < addrLen {
		// if the function is misused we allocate
		buf = make([]byte, addrLen)
	}

	n := addr.WriteTo(buf)
	return net.IP(buf[:n])
}

// ToLowHigh converts an address into a pair of uint64 numbers
func ToLowHigh(addr Address) (l, h uint64) {
	if addr == nil {
		return
	}

	switch b := addr.Bytes(); len(b) {
	case 4:
		return uint64(binary.LittleEndian.Uint32(b[:4])), uint64(0)
	case 16:
		return binary.LittleEndian.Uint64(b[8:]), binary.LittleEndian.Uint64(b[:8])
	}

	return
}

type v4Address [4]byte

// V4Address creates an Address using the uint32 representation of an v4 IP
func V4Address(ip uint32) Address {
	var a v4Address
	a[0] = byte(ip)
	a[1] = byte(ip >> 8)
	a[2] = byte(ip >> 16)
	a[3] = byte(ip >> 24)
	return a
}

// V4AddressFromBytes creates an Address using the byte representation of an v4 IP
func V4AddressFromBytes(buf []byte) Address {
	var a v4Address
	copy(a[:], buf)
	return a
}

// Bytes returns a byte array of the underlying array
func (a v4Address) Bytes() []byte {
	return a[:]
}

// WriteTo writes the address byte representation into the supplied buffer
func (a v4Address) WriteTo(b []byte) int {
	return copy(b, a[:])
}

// String returns the human readable string representation of an IP
func (a v4Address) String() string {
	return net.IPv4(a[0], a[1], a[2], a[3]).String()
}

// IsLoopback returns true if this address is the loopback address
func (a v4Address) IsLoopback() bool {
	return net.IP(a[:]).IsLoopback()
}

// Len returns the number of bytes required to represent this IP
func (a v4Address) Len() int {
	return 4
}

type v6Address [16]byte

// V6Address creates an Address using the uint128 representation of an v6 IP
func V6Address(low, high uint64) Address {
	var a v6Address
	binary.LittleEndian.PutUint64(a[:8], high)
	binary.LittleEndian.PutUint64(a[8:], low)
	return a
}

// V6AddressFromBytes creates an Address using the byte representation of an v6 IP
func V6AddressFromBytes(buf []byte) Address {
	var a v6Address
	copy(a[:], buf)
	return a
}

// Bytes returns a byte array of the underlying array
func (a v6Address) Bytes() []byte {
	return a[:]
}

// WriteTo writes the address byte representation into the supplied buffer
func (a v6Address) WriteTo(b []byte) int {
	return copy(b, a[:])
}

// String returns the human readable string representation of an IP
func (a v6Address) String() string {
	return net.IP(a[:]).String()
}

// IsLoopback returns true if this address is the loopback address
func (a v6Address) IsLoopback() bool {
	return net.IP(a[:]).IsLoopback()
}

// Len returns the number of bytes required to represent this IP
func (a v6Address) Len() int {
	return 16
}

// IPBufferPool is meant to be used in conjunction with `NetIPFromAddress`
var IPBufferPool = sync.Pool{
	New: func() interface{} {
		b := make([]byte, net.IPv6len)
		return &b
	},
}
