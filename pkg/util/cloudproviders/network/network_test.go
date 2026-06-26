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

func TestGetNetworkIDProviderGating(t *testing.T) {
	gceErr := errors.New("gce error")
	ec2Err := errors.New("ec2 error")

	tests := []struct {
		name           string
		providers      []string
		gceCallCount   int
		ec2CallCount   int
		wantErrContain string
	}{
		{
			name:           "both providers enabled, both fail",
			providers:      []string{"gcp", "aws"},
			gceCallCount:   1,
			ec2CallCount:   1,
			wantErrContain: "could not detect network ID",
		},
		{
			name:           "only gcp enabled",
			providers:      []string{"gcp"},
			gceCallCount:   1,
			ec2CallCount:   0,
			wantErrContain: "could not detect network ID",
		},
		{
			name:           "only aws enabled",
			providers:      []string{"aws"},
			gceCallCount:   0,
			ec2CallCount:   1,
			wantErrContain: "could not detect network ID",
		},
		{
			name:           "no providers enabled",
			providers:      []string{},
			gceCallCount:   0,
			ec2CallCount:   0,
			wantErrContain: "cloud provider metadata is disabled by configuration",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { cache.Cache.Delete(networkIDCacheKey) })

			cfg := configmock.New(t)
			cfg.SetInTest("cloud_provider_metadata", tc.providers)

			gceCalls, ec2Calls := 0, 0
			origGCE, origEC2 := getGCENetworkID, getEC2NetworkID
			t.Cleanup(func() { getGCENetworkID = origGCE })
			t.Cleanup(func() { getEC2NetworkID = origEC2 })
			getGCENetworkID = func(_ context.Context) (string, error) { gceCalls++; return "", gceErr }
			getEC2NetworkID = func(_ context.Context) (string, error) { ec2Calls++; return "", ec2Err }

			_, err := GetNetworkID(context.Background())
			require.ErrorContains(t, err, tc.wantErrContain)
			require.Equal(t, tc.gceCallCount, gceCalls)
			require.Equal(t, tc.ec2CallCount, ec2Calls)
		})
	}
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
