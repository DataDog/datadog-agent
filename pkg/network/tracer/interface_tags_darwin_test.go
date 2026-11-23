// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/route"
	"golang.org/x/sys/unix"
)

// TestInterfaceTypeFor checks that BSD interface names map to the Windows type strings.
func TestInterfaceTypeFor(t *testing.T) {
	cases := []struct {
		name string
		typ  string
	}{
		{name: "lo0", typ: "software_loopback"},
		{name: "utun3", typ: "tunnel"},
		{name: "gif0", typ: "tunnel"},
		{name: "stf0", typ: "tunnel"},
		{name: "ipsec0", typ: "tunnel"},
		{name: "ppp0", typ: "ppp"},
		{name: "bridge0", typ: "bridge"},
		{name: "vlan1", typ: "vlan"},
		{name: "en0", typ: "ethernet_csmacd"},
		{name: "awdl0", typ: "other"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.typ, interfaceTypeFor(tc.name), tc.name)
	}
}

// TestAddInterfaceAddressOmitsDuplicates checks that an address on two interfaces is omitted.
func TestAddInterfaceAddressOmitsDuplicates(t *testing.T) {
	byAddr := map[netip.Addr]darwinInterface{}
	ambiguous := map[netip.Addr]struct{}{}
	addr := netip.MustParseAddr("10.1.1.1")
	addInterfaceAddress(byAddr, ambiguous, addr, darwinInterface{name: "en0", typ: "ethernet_csmacd", index: 14})
	addInterfaceAddress(byAddr, ambiguous, addr, darwinInterface{name: "en1", typ: "ethernet_csmacd", index: 15})
	_, ok := byAddr[addr]
	assert.False(t, ok, "an address on two interfaces is omitted")
}

// TestInterfaceTaggerClassify checks address, zone, route, and miss lookups.
func TestInterfaceTaggerClassify(t *testing.T) {
	en0 := darwinInterface{name: "en0", typ: "ethernet_csmacd", index: 14}
	utun := darwinInterface{name: "utun3", typ: "tunnel", index: 22}
	tagger := &interfaceTagger{
		byAddr: map[netip.Addr]darwinInterface{
			netip.MustParseAddr("10.0.0.8"): en0,
			netip.MustParseAddr("10.8.0.2"): utun,
		},
		byName:  map[string]darwinInterface{"en0": en0, "utun3": utun},
		byIndex: map[int]darwinInterface{14: en0, 22: utun},
		routes: []darwinRoute{
			{prefix: netip.MustParsePrefix("10.8.0.0/16"), index: 22},
			{prefix: netip.MustParsePrefix("0.0.0.0/0"), index: 14},
			{prefix: netip.MustParsePrefix("0.0.0.0/0"), index: 22, ifScope: true},
		},
	}
	orderRoutes(tagger.routes)

	name, typ, index := tagger.classify(netip.MustParseAddr("10.8.0.2"), netip.MustParseAddr("1.1.1.1"))
	assert.Equal(t, "utun3", name)
	assert.Equal(t, "tunnel", typ)
	assert.Equal(t, 22, index)

	zoned := netip.MustParseAddr("fe80::1").WithZone("utun3")
	name, typ, index = tagger.classify(zoned, netip.Addr{})
	assert.Equal(t, "utun3", name)
	assert.Equal(t, "tunnel", typ)
	assert.Equal(t, 22, index)

	name, _, _ = tagger.classify(netip.MustParseAddr("9.9.9.9"), netip.MustParseAddr("1.1.1.1"))
	assert.Empty(t, name, "a specific local address the cache does not own is not guessed from the route")

	name, typ, index = tagger.classify(netip.IPv4Unspecified(), netip.MustParseAddr("10.8.1.9"))
	assert.Equal(t, "utun3", name)
	assert.Equal(t, "tunnel", typ)
	assert.Equal(t, 22, index)

	name, typ, index = tagger.classify(netip.IPv4Unspecified(), netip.MustParseAddr("1.1.1.1"))
	assert.Equal(t, "en0", name)
	assert.Equal(t, "ethernet_csmacd", typ)
	assert.Equal(t, 14, index, "a non-scoped default route wins a tie with an interface-scoped default")

	name, _, _ = tagger.classify(netip.IPv4Unspecified(), netip.IPv4Unspecified())
	assert.Empty(t, name)

	linkLocal := netip.MustParsePrefix("fe80::/64")
	scoped := &interfaceTagger{
		byIndex: map[int]darwinInterface{1: {name: "lo0", typ: "software_loopback", index: 1}, 14: en0},
		routes: []darwinRoute{
			{prefix: linkLocal, index: 1, ifScope: true},
			{prefix: linkLocal, index: 14, ifScope: true},
		},
	}
	orderRoutes(scoped.routes)
	name, _, _ = scoped.classify(netip.IPv6Unspecified(), netip.MustParseAddr("fe80::1"))
	assert.Empty(t, name, "scoped routes of the same length that name different interfaces are not tagged")

	onlyEn0 := &interfaceTagger{
		byIndex: map[int]darwinInterface{14: en0},
		routes:  []darwinRoute{{prefix: linkLocal, index: 14, ifScope: true}},
	}
	name, typ, index = onlyEn0.classify(netip.IPv6Unspecified(), netip.MustParseAddr("fe80::1"))
	assert.Equal(t, "en0", name)
	assert.Equal(t, "ethernet_csmacd", typ)
	assert.Equal(t, 14, index)

	var nilTagger *interfaceTagger
	name, _, _ = nilTagger.classify(netip.MustParseAddr("10.0.0.8"), netip.MustParseAddr("1.1.1.1"))
	assert.Empty(t, name)
}

// TestMatchRouteLongestPrefix checks that a longer scoped route beats a shorter
// unscoped route, and that an unscoped route wins at the same length.
func TestMatchRouteLongestPrefix(t *testing.T) {
	en0 := darwinInterface{name: "en0", typ: "ethernet_csmacd", index: 14}
	utun := darwinInterface{name: "utun3", typ: "tunnel", index: 22}
	peer := netip.MustParseAddr("10.8.1.9")
	host := netip.MustParsePrefix("10.8.1.9/32")
	net16 := netip.MustParsePrefix("10.8.0.0/16")
	defaultRoute := netip.MustParsePrefix("0.0.0.0/0")

	// A longer scoped route wins an unscoped default. The intermediate route
	// names a different interface, so comparing interfaces before lengths
	// would drop the tag.
	tables := [][]darwinRoute{
		{
			{prefix: host, index: 22, ifScope: true},
			{prefix: defaultRoute, index: 14},
		},
		{
			{prefix: host, index: 22, ifScope: true},
			{prefix: net16, index: 14, ifScope: true},
			{prefix: defaultRoute, index: 14},
		},
	}
	for _, routes := range tables {
		orderRoutes(routes)
		tagger := &interfaceTagger{
			byIndex: map[int]darwinInterface{14: en0, 22: utun},
			routes:  routes,
		}
		name, typ, index := tagger.classify(netip.IPv4Unspecified(), peer)
		assert.Equal(t, "utun3", name)
		assert.Equal(t, "tunnel", typ)
		assert.Equal(t, 22, index)
	}

	sameLength := &interfaceTagger{
		byIndex: map[int]darwinInterface{14: en0, 22: utun},
		routes: []darwinRoute{
			{prefix: net16, index: 22, ifScope: true},
			{prefix: net16, index: 14},
			{prefix: defaultRoute, index: 22, ifScope: true},
		},
	}
	orderRoutes(sameLength.routes)
	name, typ, index := sameLength.classify(netip.IPv4Unspecified(), peer)
	assert.Equal(t, "en0", name)
	assert.Equal(t, "ethernet_csmacd", typ)
	assert.Equal(t, 14, index, "an unscoped route wins a scoped route of the same length")
}

// TestRoutesFromMessages checks that reject, blackhole, and unknown-interface routes are dropped.
func TestRoutesFromMessages(t *testing.T) {
	byIndex := map[int]darwinInterface{
		14: {name: "en0", typ: "ethernet_csmacd", index: 14},
		22: {name: "utun3", typ: "tunnel", index: 22},
	}
	routes := routesFromMessages([]route.Message{
		&route.RouteMessage{
			Flags: unix.RTF_UP | unix.RTF_REJECT,
			Index: 14,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet4Addr{},
				unix.RTAX_NETMASK: &route.Inet4Addr{},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP | unix.RTF_BLACKHOLE,
			Index: 14,
			Addrs: []route.Addr{
				unix.RTAX_DST: &route.Inet4Addr{IP: [4]byte{192, 0, 2, 0}},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP,
			Index: 99,
			Addrs: []route.Addr{
				unix.RTAX_DST: &route.Inet4Addr{IP: [4]byte{203, 0, 113, 1}},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP | unix.RTF_IFSCOPE,
			Index: 22,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet4Addr{},
				unix.RTAX_NETMASK: &route.Inet4Addr{},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP | unix.RTF_GATEWAY,
			Index: 14,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet4Addr{},
				unix.RTAX_NETMASK: &route.Inet4Addr{},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP | unix.RTF_HOST,
			Index: 22,
			Addrs: []route.Addr{
				unix.RTAX_DST: &route.Inet4Addr{IP: [4]byte{10, 8, 0, 5}},
			},
		},
	}, byIndex)

	require.Len(t, routes, 3)
	assert.Equal(t, 32, routes[0].prefix.Bits())
	assert.Equal(t, 22, routes[0].index)
	assert.False(t, routes[1].ifScope)
	assert.Equal(t, 14, routes[1].index)
	assert.True(t, routes[2].ifScope)
	assert.Equal(t, 22, routes[2].index)
}

// TestRoutePrefixLengths checks /24, /20, and IPv6 /64 prefixes.
func TestRoutePrefixLengths(t *testing.T) {
	byIndex := map[int]darwinInterface{
		1: {name: "en0", typ: "ethernet_csmacd", index: 1},
	}
	var v6mask [16]byte
	for i := 0; i < 8; i++ {
		v6mask[i] = 0xff
	}
	routes := routesFromMessages([]route.Message{
		&route.RouteMessage{
			Flags: unix.RTF_UP,
			Index: 1,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet4Addr{IP: [4]byte{192, 168, 1, 0}},
				unix.RTAX_NETMASK: &route.Inet4Addr{IP: [4]byte{255, 255, 255, 0}},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP,
			Index: 1,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet4Addr{IP: [4]byte{172, 16, 0, 0}},
				unix.RTAX_NETMASK: &route.Inet4Addr{IP: [4]byte{255, 255, 240, 0}},
			},
		},
		&route.RouteMessage{
			Flags: unix.RTF_UP,
			Index: 1,
			Addrs: []route.Addr{
				unix.RTAX_DST:     &route.Inet6Addr{IP: [16]byte{0x20, 0x01, 0x0d, 0xb8}},
				unix.RTAX_NETMASK: &route.Inet6Addr{IP: v6mask},
			},
		},
	}, byIndex)

	require.Len(t, routes, 3)
	assert.Equal(t, "2001:db8::/64", routes[0].prefix.String())
	assert.Equal(t, "192.168.1.0/24", routes[1].prefix.String())
	assert.Equal(t, "172.16.0.0/20", routes[2].prefix.String())
}

// TestInterfaceTaggerCloseIsIdempotent checks that a second Close is safe and stops refresh.
func TestInterfaceTaggerCloseIsIdempotent(t *testing.T) {
	tagger := newInterfaceTagger()
	tagger.Close()
	tagger.Close()
	select {
	case <-tagger.done:
	default:
		t.Fatal("Close did not stop the refresh loop")
	}
}
