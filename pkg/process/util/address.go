package util

import (
	"encoding/binary"
	"net"
)

// Address is an IP abstraction that is family (v4/v6) agnostic
type Address interface {
	Bytes() []byte
	String() string
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

// String returns the human readable string representation of an IP
func (a v4Address) String() string {
	return net.IPv4(a[0], a[1], a[2], a[3]).String()
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

// String returns the human readable string representation of an IP
func (a v6Address) String() string {
	return net.IP(a[:]).String()
}
