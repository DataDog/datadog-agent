// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TestTCProbesLockOrder runs every path that changes the TC programs of the manager concurrently with the activation of
// its probes. The manager isn't initialized, so each call into it only takes its lock before failing.
func TestTCProbesLockOrder(t *testing.T) {
	defaultLonelyNamespaceTimeout := lonelyNamespaceTimeout
	lonelyNamespaceTimeout = 0
	t.Cleanup(func() { lonelyNamespaceTimeout = defaultLonelyNamespaceTimeout })

	cfg := &config.Config{NetworkEnabled: true, NetworkIngressEnabled: true, NetworkRawPacketEnabled: true}
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
	loop(func(uint32) { _ = tcResolver.UpdateActivatedProbes(m, nil) })
	loop(func(nsID uint32) {
		_ = tcResolver.SetupNewTCClassifierWithNetNSHandle(model.NetDevice{IfIndex: 1, NetNS: nsID}, handle, m)
	})
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
