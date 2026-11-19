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

func TestParseClusterARNValidation(t *testing.T) {
	valid := func(clusterARN string) bool { _, ok := ParseClusterARN(clusterARN); return ok }
	assert.True(t, valid("arn:aws:eks:us-west-2:123456789012:cluster/orders"))
	assert.True(t, valid("arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/orders"))
	assert.False(t, valid(""))
	assert.False(t, valid("not-an-arn"))
	assert.False(t, valid("arn:aws:ec2:us-west-2:123456789012:cluster/orders"))   // wrong service
	assert.False(t, valid("arn:aws:eks:us-west-2:12345:cluster/orders"))          // bad account
	assert.False(t, valid("arn:aws:eks:us-west-2:123456789012:nodegroup/orders")) // wrong resource
	// Region/partition charset must be constrained so a value with a comma or space cannot be
	// emitted verbatim as a tag value (which would corrupt comma-joined static tags).
	assert.False(t, valid("arn::eks:us-west-2 oops,tag:123456789012:cluster/orders")) // bad region + empty partition
	assert.False(t, valid("arn:aws:eks:us-west-2,x:123456789012:cluster/orders"))     // comma in region
	assert.False(t, valid("arn:AWS:eks:us-west-2:123456789012:cluster/orders"))       // upper-case partition
}

func TestParseClusterARN(t *testing.T) {
	id, ok := ParseClusterARN("arn:aws-cn:eks:cn-north-1:123456789012:cluster/Orders_Prod")
	assert.True(t, ok)
	assert.Equal(t, ClusterIdentity{
		ClusterName: "Orders_Prod",
		ClusterARN:  "arn:aws-cn:eks:cn-north-1:123456789012:cluster/Orders_Prod",
		AccountID:   "123456789012",
		Region:      "cn-north-1",
	}, id)

	for _, bad := range []string{
		"arn:aws:eks:us-west-2:123456789012",                    // too few sections
		"urn:aws:eks:us-west-2:123456789012:cluster/orders",     // not an ARN
		"arn:aws:eks:us-west-2:123456789012:cluster/",           // empty name
		"arn:aws:eks:us-west-2:123456789012:cluster/orders:x",   // colon in the name
		"arn:aws:eks:us-west-2:123456789012:cluster/orders/sub", // slash in the name
		"arn:aws:eks::123456789012:cluster/orders",              // empty Region
	} {
		_, ok := ParseClusterARN(bad)
		assert.False(t, ok, bad)
	}
}
