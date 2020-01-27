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
	assert.True(t, a.IsLoopback())
	assert.True(t, b.IsLoopback())

	a = AddressFromNetIP(net.ParseIP("::7f00:35:0:1"))
	b = AddressFromNetIP(net.ParseIP("::7f00:35:0:0"))
	assert.NotEqual(t, a, b)
	assert.False(t, a.IsLoopback())
	assert.False(t, b.IsLoopback())
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
}

func TestAddressV6(t *testing.T) {
	addr := V6Address(889192575, 0)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::7f00:35:0:0"))
	assert.Equal(t, "::7f00:35:0:0", addr.String())
	assert.False(t, addr.IsLoopback())

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
	assert.True(t, addr.IsLoopback())

	addr = V6Address(72059793061183488, 3087860000)
	// Should be able to recreate addr from bytes alone
	assert.Equal(t, addr, V6AddressFromBytes(addr.Bytes()))
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("2001:db8::2:1"))
	assert.Equal(t, "2001:db8::2:1", addr.String())
	assert.False(t, addr.IsLoopback())
}
