package util

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetIPToAddress(t *testing.T) {
	// V4
	addr := V4Address(889192575)
	addrFromIP := AddressFromNetIP(net.ParseIP("127.0.0.53"))

	_, ok := addrFromIP.(v4Address)
	assert.True(t, ok)
	assert.Equal(t, addrFromIP, addr)

	// V6
	addr = V6Address(889192575, 0)
	addrFromIP = AddressFromNetIP(net.ParseIP("::7f00:35:0:0"))

	_, ok = addrFromIP.(v6Address)
	assert.True(t, ok)
	assert.Equal(t, addrFromIP, addr)

	// Mismatch tests
	a := AddressFromNetIP(net.ParseIP("127.0.0.1"))
	b := AddressFromNetIP(net.ParseIP("::7f00:35:0:0"))
	assert.NotEqual(t, a, b)

	a = AddressFromNetIP(net.ParseIP("127.0.0.1"))
	b = AddressFromNetIP(net.ParseIP("127.0.0.2"))
	assert.NotEqual(t, a, b)

	a = AddressFromNetIP(net.ParseIP("::7f00:35:0:1"))
	b = AddressFromNetIP(net.ParseIP("::7f00:35:0:0"))
	assert.NotEqual(t, a, b)
}

func TestNetIPFromAddress(t *testing.T) {
	// v4
	addr := V4Address(889192575)
	ip := net.ParseIP("127.0.0.53").To4()
	ipFromAddr := NetIPFromAddress(addr)
	assert.Equal(t, ip, ipFromAddr)

	// v6
	addr = V6Address(889192575, 0)
	ip = net.ParseIP("::7f00:35:0:0")
	ipFromAddr = NetIPFromAddress(addr)
	assert.Equal(t, ip, ipFromAddr)

	// v4 + v6 mismatched
	addr = V4Address(889192575)
	ip = net.ParseIP("::7f00:35:0:0")
	ipFromAddr = NetIPFromAddress(addr)
	assert.NotEqual(t, ip, ipFromAddr)
}

func TestAddressUsageInMaps(t *testing.T) {
	addrMap := make(map[Address]struct{})

	addrMap[V4Address(889192575)] = struct{}{}
	addrMap[V6Address(889192575, 0)] = struct{}{}

	_, ok := addrMap[AddressFromString("127.0.0.53")]
	assert.True(t, ok)

	_, ok = addrMap[AddressFromString("127.0.0.1")]
	assert.False(t, ok)

	_, ok = addrMap[AddressFromString("::7f00:35:0:0")]
	assert.True(t, ok)

	_, ok = addrMap[AddressFromString("::")]
	assert.False(t, ok)
}

func TestAddressV4(t *testing.T) {
	addr := V4Address(889192575)

	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V4AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("127.0.0.53"))
	assert.Equal(t, "127.0.0.53", addr.String())

	addr = V4Address(0)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V4AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("0.0.0.0"))
	assert.Equal(t, "0.0.0.0", addr.String())

	addr = V4Address(16820416)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V4AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("192.168.0.1"))
	assert.Equal(t, "192.168.0.1", addr.String())
	// Should be able to recreate addr from IP Byte string
	assert.Equal(t, "\xc0\xa8\x00\x01", addr.ByteString())
	assert.Equal(t, AddressFromByteString("\xc0\xa8\x00\x01"), addr)
}

func TestAddressV6(t *testing.T) {
	addr := V6Address(889192575, 0)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::7f00:35:0:0"))
	assert.Equal(t, "::7f00:35:0:0", addr.String())

	addr = V6Address(0, 0)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::"))
	assert.Equal(t, "::", addr.String())

	addr = V6Address(72057594037927936, 0)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::1"))
	assert.Equal(t, "::1", addr.String())

	addr = V6Address(72059793061183488, 3087860000)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("2001:db8::2:1"))
	assert.Equal(t, "2001:db8::2:1", addr.String())
	// Should be able to recreate addr from IP Byte string
	assert.Equal(t, " \x01\r\xb8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x01", addr.ByteString())
	assert.Equal(t, AddressFromByteString(" \x01\r\xb8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x01"), addr)
}

func TestAddressByteStrings(t *testing.T) {
	// v4
	addr := V4Address(16820416)
	assert.Len(t, addr.ByteString(), 4) // Should only be 4 bytes
	assert.Equal(t, "\xc0\xa8\x00\x01", addr.ByteString())
	assert.Equal(t, addr, AddressFromByteString("\xc0\xa8\x00\x01"))
	assert.Equal(t, NetIPFromAddress(addr), NetIPFromIPByteString("\xc0\xa8\x00\x01"))

	addr = V4Address(0)
	assert.Len(t, addr.ByteString(), 4) // Should only be 4 bytes
	assert.Equal(t, "\x00\x00\x00\x00", addr.ByteString())
	assert.Equal(t, addr, AddressFromByteString("\x00\x00\x00\x00"))
	assert.Equal(t, NetIPFromAddress(addr), NetIPFromIPByteString("\x00\x00\x00\x00"))

	// v6
	addr = V6Address(72059793061183488, 3087860000)
	assert.Len(t, addr.ByteString(), 16) // Should only be 16 bytes
	assert.Equal(t, " \x01\r\xb8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x01", addr.ByteString())
	assert.Equal(t, addr, AddressFromByteString(" \x01\r\xb8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x01"))
	assert.Equal(t, NetIPFromAddress(addr), NetIPFromIPByteString(" \x01\r\xb8\x00\x00\x00\x00\x00\x00\x00\x00\x00\x02\x00\x01"))

	addr = V6Address(0, 0)
	assert.Len(t, addr.ByteString(), 16) // Should only be 16 bytes
	assert.Equal(t, "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00", addr.ByteString())
	assert.Equal(t, addr, AddressFromByteString("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	assert.Equal(t, NetIPFromAddress(addr), NetIPFromIPByteString("\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
}
