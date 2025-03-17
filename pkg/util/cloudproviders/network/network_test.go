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
