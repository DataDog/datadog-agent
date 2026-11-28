// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/proto/api"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/tests/statsdclient"
)

// TestTCProbesLockOrder runs every path that changes the TC programs of the manager concurrently with the activation of
// its probes. The manager isn't initialized, so each call into it only takes its lock before failing.
func TestTCProbesLockOrder(t *testing.T) {
	defaultLonelyNamespaceTimeout := lonelyNamespaceTimeout
	lonelyNamespaceTimeout = 0
	t.Cleanup(func() { lonelyNamespaceTimeout = defaultLonelyNamespaceTimeout })

	// TCX, since moved devices are only detached with TCX links
	cfg := &config.Config{NetworkEnabled: true, NetworkIngressEnabled: true, NetworkRawPacketEnabled: true, NetworkTCXEnabled: true}
	m := &manager.Manager{}
	tcResolver := tc.NewResolver(cfg)
	nr, err := NewResolver(cfg, m, &statsd.NoOpClient{}, tcResolver)
	require.NoError(t, err)

	handle, err := os.Open(os.DevNull)
	require.NoError(t, err)
	t.Cleanup(func() { handle.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	loop := func(f func(nsID uint32)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// more namespaces than the LRU holds, so that queuing devices evicts some of them
			for i := uint32(0); ctx.Err() == nil; i++ {
				f(i%2048 + 1)
			}
		}()
	}

	loop(func(nsID uint32) { nr.QueueNetworkDevice(model.NetDevice{IfIndex: 1, NetNS: nsID}) })
	loop(func(nsID uint32) {
		if netns := nr.ResolveNetworkNamespace(nsID); netns != nil {
			nr.FlushNetworkNamespace(netns)
		}
	})
	loop(func(uint32) { nr.manualFlushNamespaces() })
	loop(func(uint32) {
		running, unfreeze := tcResolver.FreezePrograms()
		_ = m.UpdateActivatedProbes([]manager.ProbesSelector{running})
		unfreeze()
	})
	loop(func(nsID uint32) {
		_ = tcResolver.SetupNewTCClassifierWithNetNSHandle(model.NetDevice{IfIndex: 1, NetNS: nsID}, handle, m)
	})
	loop(func(nsID uint32) { _ = nr.handleMovedDevice(model.NetDevice{IfIndex: 1, NetNS: nsID}) })
	loop(func(nsID uint32) { tcResolver.ResolveNetworkDeviceIfName(1, nsID) })
	loop(func(uint32) { _ = nr.SendStats() })

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("the network namespace and TC resolvers deadlocked")
	}
}

func TestKeepsNetworkNamespace(t *testing.T) {
	nr := &Resolver{config: &config.Config{NetworkLazyInterfacePrefixes: []string{"lo", "dummy"}}}
	withMAC := func(name string) netlink.LinkAttrs {
		a := attrs(name, 2)
		a.HardwareAddr = net.HardwareAddr{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}
		return a
	}

	for _, test := range []struct {
		name     string
		device   netlink.Link
		expected bool
	}{
		{name: "veth", device: &netlink.Veth{LinkAttrs: withMAC("eth0")}, expected: true},
		{name: "netkit_l3", device: &netlink.Netkit{LinkAttrs: attrs("eth0", 2)}, expected: true},
		{name: "netkit_l2", device: &netlink.Netkit{LinkAttrs: withMAC("eth0")}, expected: true},
		{name: "loopback", device: &netlink.Device{LinkAttrs: attrs("lo", 1)}, expected: false},
		{name: "dummy", device: &netlink.Dummy{LinkAttrs: withMAC("dummy0")}, expected: false},
		{name: "ipip_fallback", device: &netlink.Iptun{LinkAttrs: attrs("tunl0", 3)}, expected: false},
		{name: "sit_fallback", device: &netlink.Sittun{LinkAttrs: attrs("sit0", 4)}, expected: false},
		{name: "ip6tnl_fallback", device: &netlink.Ip6tnl{LinkAttrs: attrs("ip6tnl0", 5)}, expected: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, nr.keepsNetworkNamespace(test.device))
		})
	}
}

func TestDumpNamespaceWithoutHandle(t *testing.T) {
	cfg := &config.Config{NetworkEnabled: true}
	nr, err := NewResolver(cfg, nil, statsdclient.NewStatsdClient(), tc.NewResolver(cfg))
	require.NoError(t, err)

	// a device is queued until a handle on its namespace is resolved
	nr.QueueNetworkDevice(model.NetDevice{Name: "eth0", IfIndex: 2, NetNS: 4026532420})

	assert.Equal(t, []NetworkNamespaceDump{
		{
			NsID:           4026532420,
			HandleFD:       -1,
			DevicesInQueue: []NetworkDeviceDump{{IfName: "eth0", IfIndex: 2}},
		},
	}, nr.dump(&api.DumpNetworkNamespaceParams{}))

	// its interfaces can't be listed without a handle
	assert.Empty(t, nr.dump(&api.DumpNetworkNamespaceParams{SnapshotInterfaces: true}))
}
