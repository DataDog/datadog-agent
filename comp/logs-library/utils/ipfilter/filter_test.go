// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipfilter

import (
	"net"
	"sync"
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
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.True(t, f.Allow(tcpAddr("192.168.1.1")))
	assert.True(t, f.Allow(udpAddr("::1")))
}

func TestAllowListSingleIP(t *testing.T) {
	f, err := New([]string{"10.0.0.1"}, nil)
	require.NoError(t, err)
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

func TestIPv4MappedIPv6PrefixIsNormalized(t *testing.T) {
	f, err := New([]string{"::ffff:10.0.0.0/104"}, nil)
	require.NoError(t, err)
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")), "native IPv4 should match unmapped prefix")
	assert.True(t, f.Allow(tcpAddr("10.255.255.255")), "top of /8 range should match")
	assert.False(t, f.Allow(tcpAddr("11.0.0.1")), "outside range should not match")
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

func TestTrimSpaceInEntries(t *testing.T) {
	f, err := New([]string{" 10.0.0.0/8 ", "  192.168.1.1  "}, nil)
	require.NoError(t, err)
	assert.True(t, f.Allow(tcpAddr("10.0.0.1")))
	assert.True(t, f.Allow(tcpAddr("192.168.1.1")))
	assert.False(t, f.Allow(tcpAddr("172.16.0.1")))
}

// --- Check() / Denial tests ---

func TestCheckDeniedByPrefix(t *testing.T) {
	f, err := New(nil, []string{"192.168.0.0/16"})
	require.NoError(t, err)

	d := f.Check(tcpAddr("192.168.1.1"))
	assert.False(t, d.Allowed())
	assert.Equal(t, "denied by 192.168.0.0/16", d.Reason())
}

func TestCheckNotInAllowList(t *testing.T) {
	f, err := New([]string{"10.0.0.0/8"}, nil)
	require.NoError(t, err)

	d := f.Check(tcpAddr("192.168.1.1"))
	assert.False(t, d.Allowed())
	assert.Equal(t, "not in allow list", d.Reason())
}

func TestCheckAllowed(t *testing.T) {
	f, err := New([]string{"10.0.0.0/8"}, nil)
	require.NoError(t, err)

	d := f.Check(tcpAddr("10.0.0.1"))
	assert.True(t, d.Allowed())
	assert.Empty(t, d.Reason())
}

func TestCheckAllowedNoLists(t *testing.T) {
	f, err := New(nil, nil)
	require.NoError(t, err)

	d := f.Check(tcpAddr("10.0.0.1"))
	assert.True(t, d.Allowed())
	assert.Empty(t, d.Reason())
}

func TestCheckInvalidAddr(t *testing.T) {
	f, err := New(nil, nil)
	require.NoError(t, err)

	d := f.Check(&net.UnixAddr{Name: "/tmp/sock", Net: "unix"})
	assert.False(t, d.Allowed())
	assert.Equal(t, "invalid address", d.Reason())
}

func TestCheckDenyPrecedenceShowsDenyRule(t *testing.T) {
	f, err := New([]string{"10.0.0.0/24"}, []string{"10.0.0.99/32"})
	require.NoError(t, err)

	d := f.Check(tcpAddr("10.0.0.99"))
	assert.False(t, d.Allowed())
	assert.Equal(t, "denied by 10.0.0.99/32", d.Reason())
}

// --- DenialInfo tests ---

func TestDenialInfoEmpty(t *testing.T) {
	di := NewDenialInfo()
	assert.Equal(t, "IP Filter", di.InfoKey())
	assert.Equal(t, []string{"No denials"}, di.Info())
}

func TestDenialInfoRecordsReasons(t *testing.T) {
	di := NewDenialInfo()
	di.Record("denied by 10.0.0.0/24")
	di.Record("denied by 10.0.0.0/24")
	di.Record("not in allow list")

	info := di.Info()
	assert.Len(t, info, 2)
	assert.Contains(t, info, "denied by 10.0.0.0/24: 2")
	assert.Contains(t, info, "not in allow list: 1")
}

func TestDenialInfoSorted(t *testing.T) {
	di := NewDenialInfo()
	di.Record("denied by 192.168.0.0/16")
	di.Record("denied by 10.0.0.0/8")

	info := di.Info()
	require.Len(t, info, 2)
	assert.Equal(t, "denied by 10.0.0.0/8: 1", info[0])
	assert.Equal(t, "denied by 192.168.0.0/16: 1", info[1])
}

func TestDenialInfoConcurrentAccess(t *testing.T) {
	di := NewDenialInfo()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			di.Record("denied by 10.0.0.0/8")
		}()
	}
	wg.Wait()

	info := di.Info()
	require.Len(t, info, 1)
	assert.Equal(t, "denied by 10.0.0.0/8: 100", info[0])
}
