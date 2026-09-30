// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

func TestGetClusterAgentEKSIdentityTags(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	getEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return &eks.ClusterIdentity{
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
		"aws_account:123456789012",
		"region:us-west-2",
	}, tags)
}

func TestGetClusterAgentEKSIdentityTagsRejectsIncompleteIdentity(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	getEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return &eks.ClusterIdentity{ClusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/orders"}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.Error(t, err)
}

func TestGetClusterAgentEKSIdentityTagsReturnsClientError(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	expected := errors.New("Cluster Agent unavailable")
	getEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return nil, expected
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.ErrorIs(t, err, expected)
}
