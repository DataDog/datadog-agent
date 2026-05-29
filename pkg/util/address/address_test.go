// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package address

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNetIPToAddress(t *testing.T) {
	// V4
	addr := V4(889192575)
	addrFromIP := FromNetIP(net.ParseIP("127.0.0.53"))
	assert.Equal(t, addrFromIP, addr)

	// V6
	addr = V6(889192575, 0)
	addrFromIP = FromNetIP(net.ParseIP("::7f00:35:0:0"))
	assert.Equal(t, addrFromIP, addr)

	// Mismatch tests
	a := FromNetIP(net.ParseIP("127.0.0.1"))
	b := FromNetIP(net.ParseIP("::7f00:35:0:0"))
	assert.NotEqual(t, a, b)

	a = FromNetIP(net.ParseIP("127.0.0.1"))
	b = FromNetIP(net.ParseIP("127.0.0.2"))
	assert.NotEqual(t, a, b)
}

func TestNetIPFromAddress(t *testing.T) {
	buf := make([]byte, 16)

	// v4
	addr := V4(889192575)
	ip := net.ParseIP("127.0.0.53").To4()
	ipFromAddr := NetIPFromAddress(addr, buf)
	assert.Equal(t, ip, ipFromAddr)

	// v6
	addr = V6(889192575, 0)
	ip = net.ParseIP("::7f00:35:0:0")
	ipFromAddr = NetIPFromAddress(addr, buf)
	assert.Equal(t, ip, ipFromAddr)
}

func TestAddressUsageInMaps(t *testing.T) {
	addrMap := make(map[Address]struct{})

	addrMap[V4(889192575)] = struct{}{}
	addrMap[V6(889192575, 0)] = struct{}{}

	_, ok := addrMap[FromString("127.0.0.53")]
	assert.True(t, ok)

	_, ok = addrMap[FromString("127.0.0.1")]
	assert.False(t, ok)

	_, ok = addrMap[FromString("::7f00:35:0:0")]
	assert.True(t, ok)
}

func TestAddressV4(t *testing.T) {
	addr := V4(889192575)
	assert.Equal(t, addr, FromString("127.0.0.53"))
	assert.Equal(t, "127.0.0.53", addr.String())

	addr = V4(0)
	assert.Equal(t, addr, FromString("0.0.0.0"))
	assert.Equal(t, "0.0.0.0", addr.String())

	addr = V4(16820416)
	assert.Equal(t, addr, FromString("192.168.0.1"))
	assert.Equal(t, "192.168.0.1", addr.String())
}

func TestAddressV6(t *testing.T) {
	addr := V6(889192575, 0)
	assert.Equal(t, addr, FromString("::7f00:35:0:0"))
	assert.Equal(t, "::7f00:35:0:0", addr.String())

	addr = V6(0, 0)
	assert.Equal(t, addr, FromString("::"))
	assert.Equal(t, "::", addr.String())

	addr = V6(72057594037927936, 0)
	assert.Equal(t, addr, FromString("::1"))
	assert.Equal(t, "::1", addr.String())
	assert.True(t, addr.IsLoopback())
}

func TestFromLowHigh(t *testing.T) {
	// V4 (high == 0)
	addr := FromLowHigh(889192575, 0)
	assert.Equal(t, V4(889192575), addr)

	// V6 (high > 0)
	addr = FromLowHigh(889192575, 42)
	assert.Equal(t, V6(889192575, 42), addr)
}

func TestToLowHigh(t *testing.T) {
	addr := V4(889192575)
	l, h := ToLowHigh(addr)
	assert.Equal(t, uint64(889192575), l)
	assert.Equal(t, uint64(0), h)
}

func BenchmarkV6(b *testing.B) {
	var addr Address
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		addr = V6(889192575, 0)
	}
	runtime.KeepAlive(addr)
}

func BenchmarkToLowHigh(b *testing.B) {
	addr := FromString("8.8.8.8")
	var l, h uint64
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, h = ToLowHigh(addr)
	}
	runtime.KeepAlive(l)
	runtime.KeepAlive(h)
}
