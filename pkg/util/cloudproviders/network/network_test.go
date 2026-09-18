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

// mockNoCloudDetected forces provider detection to be inconclusive, so
// GetNetworkID falls through to racing GCE/EC2 directly, regardless of what
// cloud (if any) the machine running the test is actually on.
func mockNoCloudDetected(t *testing.T) {
	t.Cleanup(func() { cache.Cache.Delete(networkIDProviderCacheKey) })
	origEC2, origGCE := isRunningOnEC2, isRunningOnGCE
	origAzure, origOracle := isRunningOnAzure, isRunningOnOracle
	t.Cleanup(func() { isRunningOnEC2 = origEC2 })
	t.Cleanup(func() { isRunningOnGCE = origGCE })
	t.Cleanup(func() { isRunningOnAzure = origAzure })
	t.Cleanup(func() { isRunningOnOracle = origOracle })
	isRunningOnEC2 = func(context.Context) bool { return false }
	isRunningOnGCE = func(context.Context) bool { return false }
	isRunningOnAzure = func(context.Context) bool { return false }
	isRunningOnOracle = func(context.Context) bool { return false }
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
			mockNoCloudDetected(t)

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

func TestGetNetworkIDDetectedProviderIsUsedDirectly(t *testing.T) {
	t.Cleanup(func() { cache.Cache.Delete(networkIDCacheKey) })
	t.Cleanup(func() { cache.Cache.Delete(networkIDProviderCacheKey) })

	cfg := configmock.New(t)
	cfg.SetInTest("cloud_provider_metadata", []string{"gcp", "aws"})

	origEC2, origGCE := isRunningOnEC2, isRunningOnGCE
	origAzure, origOracle := isRunningOnAzure, isRunningOnOracle
	t.Cleanup(func() { isRunningOnEC2 = origEC2 })
	t.Cleanup(func() { isRunningOnGCE = origGCE })
	t.Cleanup(func() { isRunningOnAzure = origAzure })
	t.Cleanup(func() { isRunningOnOracle = origOracle })
	isRunningOnGCE = func(context.Context) bool { return false }
	isRunningOnAzure = func(context.Context) bool { return false }
	isRunningOnOracle = func(context.Context) bool { return false }
	ec2Detections := 0
	isRunningOnEC2 = func(context.Context) bool { ec2Detections++; return true }

	gceCalls, ec2Calls := 0, 0
	origGCEFn, origEC2Fn := getGCENetworkID, getEC2NetworkID
	t.Cleanup(func() { getGCENetworkID = origGCEFn })
	t.Cleanup(func() { getEC2NetworkID = origEC2Fn })
	getGCENetworkID = func(_ context.Context) (string, error) { gceCalls++; return "", errors.New("should not be called") }
	getEC2NetworkID = func(_ context.Context) (string, error) { ec2Calls++; return "", errors.New("ec2 error") }

	// call GetNetworkID twice; resolution fails both times so the networkID
	// cache never gets populated, but the detected provider should still only
	// be probed once, since a positive detection is cached
	_, err1 := GetNetworkID(context.Background())
	_, err2 := GetNetworkID(context.Background())
	require.ErrorContains(t, err1, "ec2 error")
	require.ErrorContains(t, err2, "ec2 error")
	require.Equal(t, 1, ec2Detections, "cloud provider detection should be cached")
	require.Equal(t, 2, ec2Calls, "network ID fetch should be retried")
	require.Equal(t, 0, gceCalls, "GCE should not be probed once EC2 is positively detected")
}

func TestGetNetworkIDUnsupportedProviderShortCircuits(t *testing.T) {
	t.Cleanup(func() { cache.Cache.Delete(networkIDCacheKey) })
	t.Cleanup(func() { cache.Cache.Delete(networkIDProviderCacheKey) })

	cfg := configmock.New(t)
	cfg.SetInTest("cloud_provider_metadata", []string{"gcp", "aws"})

	origEC2, origGCE := isRunningOnEC2, isRunningOnGCE
	origAzure, origOracle := isRunningOnAzure, isRunningOnOracle
	t.Cleanup(func() { isRunningOnEC2 = origEC2 })
	t.Cleanup(func() { isRunningOnGCE = origGCE })
	t.Cleanup(func() { isRunningOnAzure = origAzure })
	t.Cleanup(func() { isRunningOnOracle = origOracle })
	isRunningOnEC2 = func(context.Context) bool { return false }
	isRunningOnGCE = func(context.Context) bool { return false }
	isRunningOnAzure = func(context.Context) bool { return false }
	isRunningOnOracle = func(context.Context) bool { return true }

	gceCalls, ec2Calls := 0, 0
	origGCEFn, origEC2Fn := getGCENetworkID, getEC2NetworkID
	t.Cleanup(func() { getGCENetworkID = origGCEFn })
	t.Cleanup(func() { getEC2NetworkID = origEC2Fn })
	getGCENetworkID = func(_ context.Context) (string, error) { gceCalls++; return "", nil }
	getEC2NetworkID = func(_ context.Context) (string, error) { ec2Calls++; return "", nil }

	_, err := GetNetworkID(context.Background())
	require.ErrorContains(t, err, "does not support network ID resolution")
	require.Equal(t, 0, gceCalls, "GCE metadata endpoint should not be probed on a host known not to support it")
	require.Equal(t, 0, ec2Calls, "EC2 metadata endpoint should not be probed on a host known not to support it")
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
