// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package network

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

// addCleanupForSubnets clears the subnet cache after the tests.
// otherwise, tests will interfere with each other.
func addCleanupForSubnets(t *testing.T) {
	t.Cleanup(func() {
		cache.Cache.Delete(vpcSubnetsForHostCacheKey)
	})
}

// mockGetVPCSubnetsForHostImpl replaces the implementation of getVPCSubnetsForHost with a mock
func mockGetVPCSubnetsForHostImpl(t *testing.T, mock func(context.Context) ([]string, error)) {
	t.Cleanup(func() { getVPCSubnetsForHost = getVPCSubnetsForHostImpl })
	getVPCSubnetsForHost = mock
}

func TestGetVPCSubnetsForHost(t *testing.T) {
	addCleanupForSubnets(t)
	expectedSubnets := []string{"192.168.1.0/24", "beef::/64"}
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return expectedSubnets, nil
	})
	subnets, err := GetVPCSubnetsForHost(context.Background())
	require.NoError(t, err)

	var actualSubnets []string
	for _, subnet := range subnets {
		actualSubnets = append(actualSubnets, subnet.String())
	}
	// check that it parsed the subnets correctly
	require.ElementsMatch(t, expectedSubnets, actualSubnets)
}

func TestGetVPCSubnetsForHostInvalid(t *testing.T) {
	addCleanupForSubnets(t)
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return []string{"not a valid subnet"}, nil
	})
	_, err := GetVPCSubnetsForHost(context.Background())
	require.Error(t, err)
}

func TestGetVPCSubnetsForHostError(t *testing.T) {
	addCleanupForSubnets(t)
	errMock := errors.New("mock error")
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return nil, errMock
	})
	_, err := GetVPCSubnetsForHost(context.Background())
	require.ErrorIs(t, err, errMock)
}

// addCleanupForNetworkID clears the network ID cache after the tests.
func addCleanupForNetworkID(t *testing.T) {
	t.Cleanup(func() {
		cache.Cache.Delete(networkIDCacheKey)
	})
}

func TestGetNetworkIDFromConfig(t *testing.T) {
	addCleanupForNetworkID(t)
	cfg := configmock.New(t)
	cfg.SetWithoutSource("network.id", "configured-network-123")

	networkID, err := GetNetworkID(context.Background())
	require.NoError(t, err)
	require.Equal(t, "configured-network-123", networkID)
}

func TestGetNetworkIDCaching(t *testing.T) {
	addCleanupForNetworkID(t)
	cfg := configmock.New(t)
	cfg.SetWithoutSource("network.id", "cached-network-id")

	// First call
	networkID1, err := GetNetworkID(context.Background())
	require.NoError(t, err)
	require.Equal(t, "cached-network-id", networkID1)

	// Change config - should still return cached value
	cfg.SetWithoutSource("network.id", "new-network-id")

	networkID2, err := GetNetworkID(context.Background())
	require.NoError(t, err)
	require.Equal(t, "cached-network-id", networkID2, "should return cached value")
}

func TestGetNetworkIDEmptyConfig(t *testing.T) {
	addCleanupForNetworkID(t)
	cfg := configmock.New(t)
	cfg.SetWithoutSource("network.id", "")

	// When config is empty, it will try GCE and EC2 which will fail in test env
	_, err := GetNetworkID(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "could not detect network ID")
}

func TestGetVPCSubnetsForHostEmptyList(t *testing.T) {
	addCleanupForSubnets(t)
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return []string{}, nil
	})
	subnets, err := GetVPCSubnetsForHost(context.Background())
	require.NoError(t, err)
	require.Empty(t, subnets)
}

func TestGetVPCSubnetsForHostIPv4Only(t *testing.T) {
	addCleanupForSubnets(t)
	expectedSubnets := []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return expectedSubnets, nil
	})
	subnets, err := GetVPCSubnetsForHost(context.Background())
	require.NoError(t, err)

	var actualSubnets []string
	for _, subnet := range subnets {
		actualSubnets = append(actualSubnets, subnet.String())
	}
	require.ElementsMatch(t, expectedSubnets, actualSubnets)
}

func TestGetVPCSubnetsForHostIPv6Only(t *testing.T) {
	addCleanupForSubnets(t)
	expectedSubnets := []string{"2001:db8::/32", "fe80::/10"}
	mockGetVPCSubnetsForHostImpl(t, func(_ context.Context) ([]string, error) {
		return expectedSubnets, nil
	})
	subnets, err := GetVPCSubnetsForHost(context.Background())
	require.NoError(t, err)

	var actualSubnets []string
	for _, subnet := range subnets {
		actualSubnets = append(actualSubnets, subnet.String())
	}
	require.ElementsMatch(t, expectedSubnets, actualSubnets)
}
