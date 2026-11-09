// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
)

func TestGetClusterAgentEKSIdentityTags(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		return &identity.ClusterIdentity{
			ClusterName: "orders",
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"eks_cluster_account:123456789012",
		"eks_cluster_region:us-west-2",
	}, tags)
}

func TestGetClusterAgentEKSIdentityTagsRejectsIncompleteIdentity(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		// No valid cluster ARN: the account and Region are derived from the ARN, so an
		// identity that cannot be anchored to a well-formed ARN yields no tags and must be
		// rejected rather than emitting partial, unauthenticated tag values.
		return &identity.ClusterIdentity{ClusterName: "orders"}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.Error(t, err)
}

func TestGetClusterAgentEKSIdentityTagsReturnsClientError(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	expected := errors.New("Cluster Agent unavailable")
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		return nil, expected
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.ErrorIs(t, err, expected)
}

func TestGetClusterAgentEKSIdentityTagsWithRetryStopsOnClusterAgentAnswer(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	expected := errors.New(`"https://dca:5005/api/v1/cluster/eks-identity" is unavailable: 404 Not Found`)
	calls := 0
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		calls++
		return nil, expected
	}

	start := time.Now()
	tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
	assert.Nil(t, tags)
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, 1, calls)
	assert.Less(t, time.Since(start), time.Second)
}

func TestGetClusterAgentEKSIdentityTagsWithRetryRetriesUnreachableClusterAgent(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	calls := 0
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("%w: temporary failure in clusterAgentClient", errClusterAgentUnreachable)
		}
		return &identity.ClusterIdentity{
			ClusterName: "orders",
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
	assert.Contains(t, tags, "eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders")
}
