// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipfilter

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tcpAddr(ip string) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: 1234}
}

func udpAddr(ip string) *net.UDPAddr {
	return &net.UDPAddr{IP: net.ParseIP(ip), Port: 1234}
}

func TestEmptyFilterAllowsAll(t *testing.T) {
	f, err := New(nil, nil)
	require.NoError(t, err)
	assert.False(t, f.Active())
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.True(t, f.Allow(tcpAddr("192.168.1.1")))
	assert.True(t, f.Allow(udpAddr("::1")))
}

func TestAllowListSingleIP(t *testing.T) {
	f, err := New([]string{"10.0.0.1"}, nil)
	require.NoError(t, err)
	assert.True(t, f.Active())
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.False(t, f.Allow(tcpAddr("10.0.0.2")))
}

func TestAllowListCIDR(t *testing.T) {
	f, err := New([]string{"10.0.0.0/24"}, nil)
	require.NoError(t, err)
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.True(t, f.Allow(tcpAddr("10.0.0.254")))
	assert.False(t, f.Allow(tcpAddr("10.0.1.1")))
}

func TestDenyListSingleIP(t *testing.T) {
	f, err := New(nil, []string{"10.0.0.99"})
	require.NoError(t, err)
	assert.True(t, f.Active())
	assert.False(t, f.Allow(tcpAddr("10.0.0.99")))
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
}

func TestDenyListCIDR(t *testing.T) {
	f, err := New(nil, []string{"192.168.0.0/16"})
	require.NoError(t, err)
	assert.False(t, f.Allow(tcpAddr("192.168.1.1")))
	assert.False(t, f.Allow(tcpAddr("192.168.255.255")))
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
}

func TestDenyTakesPrecedenceOverAllow(t *testing.T) {
	f, err := New([]string{"10.0.0.0/24"}, []string{"10.0.0.99"})
	require.NoError(t, err)
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.False(t, f.Allow(tcpAddr("10.0.0.99")))
	assert.False(t, f.Allow(tcpAddr("192.168.1.1")))
}

func TestIPv6(t *testing.T) {
	f, err := New([]string{"::1", "fd00::/64"}, nil)
	require.NoError(t, err)
	assert.True(t, f.Allow(tcpAddr("::1")))
	assert.True(t, f.Allow(tcpAddr("fd00::1")))
	assert.False(t, f.Allow(tcpAddr("fd01::1")))
}

func TestUDPAddr(t *testing.T) {
	f, err := New([]string{"10.0.0.0/8"}, nil)
	require.NoError(t, err)
	assert.True(t, f.Allow(udpAddr("10.0.0.1")))
	assert.False(t, f.Allow(udpAddr("192.168.1.1")))
}

func TestInvalidCIDRRejected(t *testing.T) {
	_, err := New([]string{"not-an-ip"}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "allowed_ips")

	_, err = New(nil, []string{"999.999.999.999"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "denied_ips")
}

func TestInvalidAddrReturnsFalse(t *testing.T) {
	f, err := New(nil, nil)
	require.NoError(t, err)

	badAddr := &net.UnixAddr{Name: "/tmp/sock", Net: "unix"}
	assert.False(t, f.Allow(badAddr))
}

func TestMultipleEntries(t *testing.T) {
	f, err := New(
		[]string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
		[]string{"10.0.0.0/24"},
	)
	require.NoError(t, err)
	assert.False(t, f.Allow(tcpAddr("10.0.0.5")))
	assert.True(t, f.Allow(tcpAddr("10.0.1.5")))
	assert.True(t, f.Allow(tcpAddr("172.16.0.1")))
	assert.True(t, f.Allow(tcpAddr("192.168.1.1")))
	assert.False(t, f.Allow(tcpAddr("8.8.8.8")))
}
