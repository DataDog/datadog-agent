// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eksidentity

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

func stub(t *testing.T, distribution string, ec2Name string, ec2Err error, configured string) *string {
	t.Helper()
	origDistribution, origEC2, origConfigured, origResolve := detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey
	t.Cleanup(func() {
		detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey = origDistribution, origEC2, origConfigured, origResolve
	})

	requested := new(string)
	detectDistribution = func(context.Context) string { return distribution }
	ec2ClusterName = func(context.Context) (string, error) { return ec2Name, ec2Err }
	configuredCluster = func(context.Context, string) string { return configured }
	resolveIdentityByKey = func(_ context.Context, name string) (*eks.ClusterIdentity, error) {
		*requested = name
		return &eks.ClusterIdentity{
			ClusterName: name,
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/" + name,
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}
	return requested
}

func TestResolveRefusesNonEKS(t *testing.T) {
	requested := stub(t, "gke", "orders", nil, "orders")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrNotEKS)
	assert.Empty(t, *requested)
	assert.Nil(t, Tags(t.Context()))
}

func TestResolvePrefersEC2ClusterTag(t *testing.T) {
	requested := stub(t, "eks", "Orders_Prod", nil, "orders-prod")
	identity, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Orders_Prod", *requested)
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/Orders_Prod", identity.ClusterARN)
}

func TestResolveFallsBackToConfiguredName(t *testing.T) {
	requested := stub(t, "eks", "", errors.New("no ec2 tags"), "orders")
	_, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "orders", *requested)
	assert.ElementsMatch(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"aws_account:123456789012",
		"region:us-west-2",
	}, Tags(t.Context()))
}

func TestResolveFailsWithoutClusterName(t *testing.T) {
	requested := stub(t, "eks", "", nil, "")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrClusterNameUnavailable)
	assert.Empty(t, *requested)
}
