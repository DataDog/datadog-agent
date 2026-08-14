// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalIPCacheRefreshesAfterTTL(t *testing.T) {
	firstIP := netip.MustParseAddr("10.0.0.1")
	secondIP := netip.MustParseAddr("10.0.0.2")
	discoveryCalls := 0

	localIPs := newLocalIPCache(func() (map[netip.Addr]struct{}, error) {
		discoveryCalls++
		if discoveryCalls == 1 {
			return map[netip.Addr]struct{}{firstIP: {}}, nil
		}
		return map[netip.Addr]struct{}{secondIP: {}}, nil
	})

	containsFirst, err := localIPs.contains(firstIP)
	require.NoError(t, err)
	assert.True(t, containsFirst)

	containsSecond, err := localIPs.contains(secondIP)
	require.NoError(t, err)
	assert.False(t, containsSecond)
	assert.Equal(t, 1, discoveryCalls)

	localIPs.items.Delete(localIPCacheRefreshAttemptKey)

	containsSecond, err = localIPs.contains(secondIP)
	require.NoError(t, err)
	assert.True(t, containsSecond)
	assert.Equal(t, 2, discoveryCalls)
}

func TestLocalIPCacheUsesStaleSnapshotOnDiscoveryFailure(t *testing.T) {
	localIP := netip.MustParseAddr("10.0.0.1")
	discoveryErr := errors.New("boom")
	failDiscovery := false
	discoveryCalls := 0

	localIPs := newLocalIPCache(func() (map[netip.Addr]struct{}, error) {
		discoveryCalls++
		if failDiscovery {
			return nil, discoveryErr
		}
		return map[netip.Addr]struct{}{localIP: {}}, nil
	})
	localIPs.maxStaleAge = time.Minute

	containsLocal, err := localIPs.contains(localIP)
	require.NoError(t, err)
	assert.True(t, containsLocal)

	failDiscovery = true
	localIPs.items.Delete(localIPCacheRefreshAttemptKey)

	containsLocal, err = localIPs.contains(localIP)
	require.ErrorIs(t, err, discoveryErr)
	assert.True(t, containsLocal)

	containsLocal, err = localIPs.contains(localIP)
	require.NoError(t, err)
	assert.True(t, containsLocal)
	assert.Equal(t, 2, discoveryCalls)
}
