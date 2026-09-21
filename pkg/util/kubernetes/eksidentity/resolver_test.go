// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eksidentity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

type stubState struct {
	requested   string
	ec2Lookups  int
	resolveCall int
}

func stub(t *testing.T, distribution string, ec2Name string, ec2Err error, configured string) *stubState {
	t.Helper()
	cache.Cache.Delete(cache.BuildAgentKey("eks", "cluster-name"))
	origDistribution, origEC2, origConfigured, origResolve := detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey
	t.Cleanup(func() {
		detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey = origDistribution, origEC2, origConfigured, origResolve
		cache.Cache.Delete(cache.BuildAgentKey("eks", "cluster-name"))
	})

	state := &stubState{}
	detectDistribution = func(context.Context) string { return distribution }
	ec2ClusterName = func(context.Context) (string, error) { state.ec2Lookups++; return ec2Name, ec2Err }
	configuredCluster = func(context.Context, string) string { return configured }
	resolveIdentityByKey = func(_ context.Context, name string) (*eks.ClusterIdentity, error) {
		state.requested = name
		state.resolveCall++
		return &eks.ClusterIdentity{
			ClusterName: name,
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/" + name,
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}
	return state
}

func TestResolveRefusesNonEKS(t *testing.T) {
	state := stub(t, "gke", "orders", nil, "orders")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrNotEKS)
	assert.Empty(t, state.requested)
	assert.Equal(t, 0, state.ec2Lookups, "non-EKS clusters must not run cluster name discovery")
	assert.Nil(t, Tags(t.Context()))
}

func TestResolvePrefersEC2ClusterTag(t *testing.T) {
	state := stub(t, "eks", "Orders_Prod", nil, "orders-prod")
	identity, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Orders_Prod", state.requested)
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/Orders_Prod", identity.ClusterARN)
}

func TestResolveFallsBackToConfiguredName(t *testing.T) {
	state := stub(t, "eks", "", errors.New("no ec2 tags"), "orders")
	_, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "orders", state.requested)
	assert.ElementsMatch(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"aws_account:123456789012",
		"region:us-west-2",
	}, Tags(t.Context()))
}

func TestResolveFailsWithoutClusterName(t *testing.T) {
	state := stub(t, "eks", "", nil, "")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrClusterNameUnavailable)
	assert.Empty(t, state.requested)

	// The absence is cached so repeated node requests do not re-run discovery.
	_, err = Resolve(t.Context())
	assert.ErrorIs(t, err, ErrClusterNameUnavailable)
	assert.Equal(t, 1, state.ec2Lookups)
}

func TestResolveCachesDiscoveredClusterName(t *testing.T) {
	state := stub(t, "eks", "orders", nil, "")
	for range 3 {
		_, err := Resolve(t.Context())
		require.NoError(t, err)
	}
	assert.Equal(t, 1, state.ec2Lookups, "cluster name must be discovered once")
	assert.Equal(t, 3, state.resolveCall, "identity resolution itself is cached by pkg/util/eks")
}

func TestResolveRetriesEC2TagAfterConfiguredFallback(t *testing.T) {
	origTTL := clusterNameRetryTTL
	clusterNameRetryTTL = time.Nanosecond
	t.Cleanup(func() { clusterNameRetryTTL = origTTL })

	ec2Err := errors.New("ec2 tags unavailable")
	state := stub(t, "eks", "", ec2Err, "orders-display-name")
	_, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "orders-display-name", state.requested, "configured name is the fallback")

	// Once the EC2 tag becomes available, the fallback must not pin the display name.
	ec2ClusterName = func(context.Context) (string, error) { state.ec2Lookups++; return "OrdersProd", nil }
	_, err = Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "OrdersProd", state.requested, "EC2 tag name must replace the configured fallback")
	assert.Equal(t, 2, state.ec2Lookups, "EC2 tag discovery must be retried after the fallback TTL")

	// The EC2 derived name is cached permanently.
	_, err = Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 2, state.ec2Lookups)
}
