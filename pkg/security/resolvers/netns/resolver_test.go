// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netns

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/tc"
)

// newTestResolver returns a resolver whose manager was never initialized: releasing a namespace without a handle is a
// no-op, anything that needs a netlink socket isn't.
func newTestResolver(t *testing.T, networkEnabled bool) *Resolver {
	cfg := &config.Config{NetworkEnabled: networkEnabled}
	nr, err := NewResolver(cfg, &manager.Manager{}, &statsd.NoOpClient{}, tc.NewResolver(cfg))
	require.NoError(t, err)
	return nr
}

func (nr *Resolver) contains(nsID uint32) bool {
	nr.RLock()
	defer nr.RUnlock()
	return nr.networkNamespaces.Contains(nsID)
}

func TestEvictedNamespacesReleasedAfterUnlock(t *testing.T) {
	nr := newTestResolver(t, true)
	netns := NewNetworkNamespace(42)
	nr.networkNamespaces.Add(42, netns)

	nr.Lock()
	_ = nr.networkNamespaces.Remove(42)
	assert.Equal(t, []*NetworkNamespace{netns}, nr.evicted)
	nr.unlockAndRelease()

	assert.Empty(t, nr.evicted)
	require.True(t, nr.TryLock())
	nr.Unlock()
}

func TestMountRequests(t *testing.T) {
	t.Run("umount flushes", func(t *testing.T) {
		nr := newTestResolver(t, true)
		nr.networkNamespaces.Add(42, NewNetworkNamespace(42))

		nr.PushNetworkNamespaceUmountRequest(42)
		nr.handleNetworkNamespaceMountRequest(<-nr.netnsMountRequests)
		assert.False(t, nr.contains(42))
	})

	t.Run("mount without path", func(t *testing.T) {
		nr := newTestResolver(t, true)
		nr.PushNetworkNamespaceMountRequest(42, nil)
		assert.Empty(t, nr.netnsMountRequests)
	})

	t.Run("network disabled", func(t *testing.T) {
		nr := newTestResolver(t, false)
		nr.PushNetworkNamespaceUmountRequest(42)
		assert.Empty(t, nr.netnsMountRequests)
	})

	t.Run("full queue", func(t *testing.T) {
		nr := newTestResolver(t, true)
		for nsID := 1; nsID <= cap(nr.netnsMountRequests)+1; nsID++ {
			nr.PushNetworkNamespaceUmountRequest(uint32(nsID))
		}
		assert.Len(t, nr.netnsMountRequests, cap(nr.netnsMountRequests))
		assert.EqualValues(t, 1, nr.errorCounters[errorClassMountQueueFull].Load())
	})
}

func TestDriftFlushesExpiredNamespaces(t *testing.T) {
	nr := newTestResolver(t, true)
	for nsID := uint32(1); nsID <= 2; nsID++ {
		netns := NewNetworkNamespace(nsID)
		netns.lonelyTimeout = time.Now().Add(-time.Second)
		nr.networkNamespaces.Add(nsID, netns)
	}

	// the first namespace got a device since it became lonely
	nr.preventNetworkNamespaceDrift(map[uint32]int{1: 1})
	assert.True(t, nr.contains(1))
	assert.False(t, nr.contains(2))
}

func TestRunServesRequests(t *testing.T) {
	nr := newTestResolver(t, true)
	nr.networkNamespaces.Add(42, NewNetworkNamespace(42))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, nr.Start(ctx))

	nr.PushNetworkNamespaceUmountRequest(42)
	assert.Eventually(t, func() bool { return !nr.contains(42) }, 5*time.Second, 10*time.Millisecond)

	nr.Close()
}
