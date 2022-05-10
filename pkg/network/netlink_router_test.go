// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf
// +build linux_bpf

package network

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/network/testutil"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
)

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
