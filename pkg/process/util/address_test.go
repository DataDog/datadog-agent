// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetIPToAddress(t *testing.T) {
	// V4
	addr := V4Address(889192575)
	addrFromIP := AddressFromNetIP(net.ParseIP("127.0.0.53"))

	assert.Equal(t, addrFromIP, addr)

	// V6
	addr = V6Address(889192575, 0)
	addrFromIP = AddressFromNetIP(net.ParseIP("::7f00:35:0:0"))

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
	buf := make([]byte, 16)

	// v4
	addr := V4Address(889192575)
	ip := net.ParseIP("127.0.0.53").To4()
	ipFromAddr := NetIPFromAddress(addr, buf)
	assert.Equal(t, ip, ipFromAddr)

	// v6
	addr = V6Address(889192575, 0)
	ip = net.ParseIP("::7f00:35:0:0")
	ipFromAddr = NetIPFromAddress(addr, buf)
	assert.Equal(t, ip, ipFromAddr)

	// v4 + v6 mismatched
	addr = V4Address(889192575)
	ip = net.ParseIP("::7f00:35:0:0")
	ipFromAddr = NetIPFromAddress(addr, buf)
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

	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("127.0.0.53"))
	assert.Equal(t, "127.0.0.53", addr.String())

	addr = V4Address(0)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("0.0.0.0"))
	assert.Equal(t, "0.0.0.0", addr.String())

	addr = V4Address(16820416)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("192.168.0.1"))
	assert.Equal(t, "192.168.0.1", addr.String())
}

func TestAddressV6(t *testing.T) {
	addr := V6Address(889192575, 0)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::7f00:35:0:0"))
	assert.Equal(t, "::7f00:35:0:0", addr.String())
	assert.False(t, addr.IsLoopback())

	addr = V6Address(0, 0)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::"))
	assert.Equal(t, "::", addr.String())

	addr = V6Address(72057594037927936, 0)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("::1"))
	assert.Equal(t, "::1", addr.String())
	assert.True(t, addr.IsLoopback())

	addr = V6Address(72059793061183488, 3087860000)
	// Should be able to recreate addr from IP string
	assert.Equal(t, addr, AddressFromString("2001:db8::2:1"))
	assert.Equal(t, "2001:db8::2:1", addr.String())
	assert.False(t, addr.IsLoopback())
}

func TestV6AddressAllocation(t *testing.T) {
	allocs := int(testing.AllocsPerRun(100, func() {
		_ = V6Address(889192575, 0)
	}))

	assert.Equalf(t, 0, allocs, "V6Address should not allocate: got %d allocations", allocs)
}

func BenchmarkNetIPFromAddress(b *testing.B) {
	var (
		buf  = make([]byte, 16)
		addr = AddressFromString("8.8.8.8")
		ip   net.IP
	)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip = NetIPFromAddress(addr, buf)
	}
	runtime.KeepAlive(ip)
}

func BenchmarkV6Address(b *testing.B) {
	var addr Address

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addr = V6Address(889192575, 0)
	}
	runtime.KeepAlive(addr)
}

func BenchmarkToLowHigh(b *testing.B) {
	addr := AddressFromString("8.8.8.8")
	var l, h uint64
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// this method shouldn't allocate
		l, h = ToLowHigh(addr)
	}

	runtime.KeepAlive(l)
	runtime.KeepAlive(h)
}
