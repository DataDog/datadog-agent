// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package identity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClusterIdentityTags(t *testing.T) {
	id := &ClusterIdentity{
		ClusterName: "orders",
		ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
		AccountID:   "123456789012",
		Region:      "us-west-2",
	}
	assert.Equal(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"eks_cluster_account:123456789012",
		"eks_cluster_region:us-west-2",
	}, id.Tags())

	var nilIdentity *ClusterIdentity
	assert.Nil(t, nilIdentity.Tags())
	// A malformed ARN yields no tag, even if the other fields are set.
	assert.Nil(t, (&ClusterIdentity{ClusterARN: "not-an-arn", AccountID: "123456789012", Region: "us-west-2"}).Tags())
	// A valid ARN alone is sufficient; account and Region are derived from it.
	assert.Equal(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"eks_cluster_account:123456789012",
		"eks_cluster_region:us-west-2",
	}, (&ClusterIdentity{ClusterARN: id.ClusterARN}).Tags())
}

func TestIsValidClusterARN(t *testing.T) {
	assert.True(t, IsValidClusterARN("arn:aws:eks:us-west-2:123456789012:cluster/orders"))
	assert.True(t, IsValidClusterARN("arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/orders"))
	assert.False(t, IsValidClusterARN(""))
	assert.False(t, IsValidClusterARN("not-an-arn"))
	assert.False(t, IsValidClusterARN("arn:aws:ec2:us-west-2:123456789012:cluster/orders"))   // wrong service
	assert.False(t, IsValidClusterARN("arn:aws:eks:us-west-2:12345:cluster/orders"))          // bad account
	assert.False(t, IsValidClusterARN("arn:aws:eks:us-west-2:123456789012:nodegroup/orders")) // wrong resource
}
