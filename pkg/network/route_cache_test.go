// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package network

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	gomock "github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

func TestRouteCacheGet(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRouter(ctrl)

	tests := []struct {
		source, dest string
		netns        uint32

		route Route
		ok    bool

		times int
	}{
		{source: "127.0.0.1", dest: "127.0.0.1", route: Route{IfIndex: 0}, ok: true, times: 1},
		{source: "10.0.2.2", dest: "8.8.8.8", route: Route{Gateway: util.AddressFromString("10.0.2.1"), IfIndex: 1}, ok: true, times: 1},
		{source: "1.2.3.4", dest: "5.6.7.8", route: Route{}, ok: false, times: 2}, // 2 calls expected here since this is not going to be cached
	}

	cache := NewRouteCache(10, m)
	defer cache.Close()

	m.EXPECT().Close()

	// run through to fill up cache
	for _, te := range tests {
		source := util.AddressFromString(te.source)
		dest := util.AddressFromString(te.dest)
		m.EXPECT().Route(gomock.Eq(source), gomock.Eq(dest), gomock.Eq(te.netns)).
			Return(te.route, te.ok).
			Times(te.times)

		r, ok := cache.Get(source, dest, te.netns)
		require.Equal(t, te.route, r)
		require.Equal(t, te.ok, ok)
	}

	for _, te := range tests {
		source := util.AddressFromString(te.source)
		dest := util.AddressFromString(te.dest)
		r, ok := cache.Get(source, dest, te.netns)
		require.Equal(t, te.route, r)
		require.Equal(t, te.ok, ok)
	}
}

func TestRouteCacheTTL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	m := NewMockRouter(ctrl)

	route := Route{Gateway: util.AddressFromString("1.1.1.1"), IfIndex: 0}
	m.EXPECT().Route(gomock.Any(), gomock.Any(), gomock.Any()).Return(route, true).Times(2)

	cache := newRouteCache(10, m, time.Millisecond)
	defer cache.Close()

	m.EXPECT().Close()

	source := util.AddressFromString("1.1.1.1")
	dest := util.AddressFromString("1.2.3.4")
	r, ok := cache.Get(source, dest, 0)
	require.True(t, ok)
	require.Equal(t, route, r)

	time.Sleep(2 * time.Millisecond)

	r, ok = cache.Get(source, dest, 0)
	require.True(t, ok)
	require.Equal(t, route, r)
}

func TestNetlinkRouterNonRootNamespace(t *testing.T) {
	// setup a network namespace
	cmds := []string{
		"ip link add br0 type bridge",
		"ip addr add 2.2.2.1/24 broadcast 2.2.2.255 dev br0",
		"ip netns add test1",
		"ip link add veth1 type veth peer name veth2",
		"ip link set veth1 master br0",
		"ip link set veth2 netns test1",
		"ip -n test1 addr add 2.2.2.2/24 broadcast 2.2.2.255 dev veth2",
		"ip link set br0 up",
		"ip link set veth1 up",
		"ip -n test1 link set veth2 up",
		"ip -n test1 route add default via 2.2.2.1",
		"iptables -I POSTROUTING 1 -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
		"iptables -I FORWARD -i br0 -j ACCEPT",
		"iptables -I FORWARD -o br0 -j ACCEPT",
		"sysctl -w net.ipv4.ip_forward=1",
	}
	defer func() {
		testutil.RunCommands(t, []string{
			"iptables -D FORWARD -o br0 -j ACCEPT",
			"iptables -D FORWARD -i br0 -j ACCEPT",
			"iptables -D POSTROUTING -t nat -s 2.2.2.0/24 ! -d 2.2.2.0/24 -j MASQUERADE",
			"ip link del veth1",
			"ip link del br0",
			"ip netns del test1",
		}, true)
	}()
	testutil.RunCommands(t, cmds, false)

	test1Ns, err := netns.GetFromName("test1")
	require.NoError(t, err)
	defer test1Ns.Close()

	ino, err := util.GetInoForNs(test1Ns)
	require.NoError(t, err)

	// create the router in a different namespace than the root network namespace
	ns, err := netns.NewNamed("router")
	require.NoError(t, err)
	defer netns.DeleteNamed("router")

	var router *netlinkRouter
	err = util.WithNS("/proc", ns, func() error {
		router, err = newNetlinkRouter("/proc")
		return err
	})

	// do a route lookup for a connection from test1 namespace
	r, ok := router.Route(util.AddressFromString("2.2.2.2"), util.AddressFromString("8.8.8.8"), ino)
	assert.True(t, ok, "route not found")
	assert.False(t, r.Gateway.IsZero(), "gateway should not be empty")
	assert.NotZero(t, r.IfIndex, "route link index should be != 0")
}
