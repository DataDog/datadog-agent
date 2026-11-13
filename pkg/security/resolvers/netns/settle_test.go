// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

func attrs(name string, index int) netlink.LinkAttrs {
	a := netlink.NewLinkAttrs()
	a.Name = name
	a.Index = index
	return a
}

func TestSettlesBeforeAttach(t *testing.T) {
	loopback := attrs("lo", 1)
	loopback.Flags = net.FlagLoopback | net.FlagUp

	for _, test := range []struct {
		device   netlink.Link
		expected bool
	}{
		{device: &netlink.Netkit{LinkAttrs: attrs("lxc1234", 10)}, expected: true},
		{device: &netlink.Macvlan{LinkAttrs: attrs("macvlan0", 11)}, expected: true},
		{device: &netlink.Macvtap{Macvlan: netlink.Macvlan{LinkAttrs: attrs("macvtap0", 12)}}, expected: true},
		{device: &netlink.IPVlan{LinkAttrs: attrs("ipvlan0", 13)}, expected: true},
		{device: &netlink.IPVtap{IPVlan: netlink.IPVlan{LinkAttrs: attrs("ipvtap0", 14)}}, expected: true},
		{device: &netlink.Vlan{LinkAttrs: attrs("eth0.100", 15)}, expected: true},
		{device: &netlink.Veth{LinkAttrs: attrs("cilium_host", 16)}, expected: true},
		// physical devices and VFs
		{device: &netlink.Device{LinkAttrs: attrs("ens5", 2)}, expected: true},
		{device: &netlink.Device{LinkAttrs: loopback}, expected: false},
		{device: &netlink.Bridge{LinkAttrs: attrs("docker0", 3)}, expected: false},
		{device: &netlink.Vxlan{LinkAttrs: attrs("vxlan0", 4)}, expected: false},
		{device: &netlink.Wireguard{LinkAttrs: attrs("wg0", 5)}, expected: false},
		{device: &netlink.Dummy{LinkAttrs: attrs("dummy0", 6)}, expected: false},
		{device: &netlink.Tuntap{LinkAttrs: attrs("tun0", 7)}, expected: false},
		{device: &netlink.Ifb{LinkAttrs: attrs("ifb0", 8)}, expected: false},
		{device: &netlink.GenericLink{LinkAttrs: attrs("geneve0", 9), LinkType: "geneve"}, expected: false},
	} {
		t.Run(test.device.Attrs().Name, func(t *testing.T) {
			assert.Equal(t, test.expected, settlesBeforeAttach(test.device))
		})
	}
}

func TestDeviceSettlerQueue(t *testing.T) {
	s := newDeviceSettler(5 * time.Second)
	now := time.Now()

	first := model.NetDevice{Name: "lxc1", IfIndex: 10, NetNS: 1}
	second := model.NetDevice{Name: "lxc2", IfIndex: 11, NetNS: 1}
	s.push(first, false, now)
	// a device is queued once until it's due
	s.push(first, true, now.Add(time.Second))
	s.push(second, true, now.Add(2*time.Second))

	assert.True(t, s.isPending(first))
	assert.True(t, s.isPending(model.NetDevice{Name: "renamed", IfIndex: 10, NetNS: 1}))
	assert.False(t, s.isPending(model.NetDevice{Name: "lxc1", IfIndex: 10, NetNS: 2}))

	assert.Empty(t, s.popDue(now.Add(4*time.Second)))

	due := s.popDue(now.Add(5 * time.Second))
	if assert.Len(t, due, 1) {
		assert.Equal(t, first, due[0].device)
		assert.False(t, due[0].fromHostSnapshot)
	}
	assert.False(t, s.isPending(first))
	assert.True(t, s.isPending(second))

	due = s.popDue(now.Add(time.Minute))
	if assert.Len(t, due, 1) {
		assert.Equal(t, second, due[0].device)
		assert.True(t, due[0].fromHostSnapshot)
	}
	assert.Empty(t, s.popDue(now.Add(time.Hour)))
}

func TestDeviceSettlerTimer(t *testing.T) {
	s := newDeviceSettler(50 * time.Millisecond)
	defer s.stop()

	assert.Nil(t, s.arm(time.Now()), "the timer is only armed for pending devices")

	s.push(model.NetDevice{Name: "lxc1", IfIndex: 10, NetNS: 1}, false, time.Now())
	settled := s.arm(time.Now())
	require.NotNil(t, settled)

	select {
	case now := <-settled:
		assert.Len(t, s.popDue(now), 1)
	case <-time.After(5 * time.Second):
		t.Fatal("the timer didn't fire")
	}
	assert.Nil(t, s.arm(time.Now()))

	// a device pushed late fires later, whatever the timer fired for before
	s.push(model.NetDevice{Name: "lxc2", IfIndex: 11, NetNS: 1}, false, time.Now().Add(time.Hour))
	settled = s.arm(time.Now())
	select {
	case <-settled:
		t.Fatal("the timer fired before the deadline")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestDeviceSettlerFailedDevices(t *testing.T) {
	s := newDeviceSettler(time.Second)
	s.hostNetNS = 1

	s.addFailed(model.NetDevice{Name: "cilium_host", IfIndex: 3, NetNS: 1})
	// only host devices are retried by the host snapshot
	s.addFailed(model.NetDevice{Name: "eth0", IfIndex: 2, NetNS: 2})

	failing := &netlink.Veth{LinkAttrs: attrs("cilium_host", 3)}
	assert.True(t, s.hasFailed(failing))
	assert.False(t, s.hasFailed(&netlink.Device{LinkAttrs: attrs("eth0", 2)}))
	// a new name or a reused ifindex gets another try
	assert.False(t, s.hasFailed(&netlink.Veth{LinkAttrs: attrs("renamed", 3)}))
	assert.False(t, s.hasFailed(&netlink.Veth{LinkAttrs: attrs("cilium_host", 4)}))

	s.pruneFailed([]netlink.Link{failing, &netlink.Device{LinkAttrs: attrs("ens5", 2)}})
	assert.True(t, s.hasFailed(failing))

	// a device that disappears is forgotten, so that a device reusing its ifindex and name is tried
	s.pruneFailed([]netlink.Link{&netlink.Device{LinkAttrs: attrs("ens5", 2)}})
	assert.False(t, s.hasFailed(failing))
	assert.Empty(t, s.failed)
}

func TestSettlerOnlyWithTCX(t *testing.T) {
	legacy := &config.Config{NetworkEnabled: true}
	nr, err := NewResolver(legacy, nil, statsdclient.NewStatsdClient(), tc.NewResolver(legacy))
	require.NoError(t, err)
	assert.Nil(t, nr.settler)
	assert.Nil(t, nr.hostSnapshotRequests)
	// a no-op in legacy mode
	nr.requestHostSnapshot()

	tcx := &config.Config{NetworkEnabled: true, NetworkTCXEnabled: true, NetworkTCXSettleDelay: 5 * time.Second}
	nr, err = NewResolver(tcx, nil, statsdclient.NewStatsdClient(), nil)
	require.NoError(t, err)
	assert.Nil(t, nr.settler, "the attach mode is the one of the TC resolver")

	nr, err = NewResolver(tcx, nil, statsdclient.NewStatsdClient(), tc.NewResolver(tcx))
	require.NoError(t, err)
	require.NotNil(t, nr.settler)
	assert.Equal(t, 5*time.Second, nr.settler.delay)

	nr.requestHostSnapshot()
	// requests coalesce while the loop is busy
	nr.requestHostSnapshot()
	assert.Len(t, nr.hostSnapshotRequests, 1)
}

func TestRequestsInUnknownNetNSAreQueued(t *testing.T) {
	for _, tcxEnabled := range []bool{false, true} {
		cfg := &config.Config{NetworkEnabled: true, NetworkTCXEnabled: tcxEnabled, NetworkTCXSettleDelay: 5 * time.Second}
		nr, err := NewResolver(cfg, nil, statsdclient.NewStatsdClient(), tc.NewResolver(cfg))
		require.NoError(t, err)

		for i, requestType := range []TcClassifierRequestType{TcNewDeviceRequestType, TcDeviceUpdateRequestType, TcRegisteredDeviceRequestType} {
			device := model.NetDevice{Name: "eth0", IfIndex: uint32(i + 2), NetNS: 4026532420}
			err := nr.handleTCClassifierRequest(TcClassifierRequest{RequestType: requestType, Device: device})
			var qnde QueuedNetworkDeviceError
			assert.ErrorAs(t, err, &qnde, "request type %d, tcx %v", requestType, tcxEnabled)
		}

		netns := nr.ResolveNetworkNamespace(4026532420)
		require.NotNil(t, netns)
		assert.Len(t, netns.networkDevicesQueue, 3)
		if tcxEnabled {
			assert.Empty(t, nr.settler.pending)
		}
	}
}
