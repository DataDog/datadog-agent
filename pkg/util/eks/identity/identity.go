// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package identity holds the authoritative EKS cluster identity data type and its tag
// projection. It is deliberately free of the AWS SDK so that the many node-Agent packages that
// only need the data type (the Cluster Agent client interface, the clusterinfo bridge) do not
// link the EKS service client. The SDK-dependent resolver lives in the parent pkg/util/eks.
package identity

import (
	"regexp"
	"strings"

	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
)

var (
	clusterNamePattern = regexp.MustCompile(`^[0-9A-Za-z][A-Za-z0-9_-]{0,99}$`)
	accountIDPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	// partitionPattern and regionPattern constrain the ARN fields that become tag values.
	// awsarn.Parse only splits on colons and does no charset validation, so without these a
	// Region or partition containing a comma or space would be emitted verbatim as a tag value
	// (and would corrupt consumers that comma-join static tags). They match the e2e contract
	// ^arn:aws[a-z-]*:eks:[a-z0-9-]+:[0-9]{12}:cluster/...$.
	partitionPattern = regexp.MustCompile(`^aws[a-z0-9-]*$`)
	regionPattern    = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// ClusterIdentity is the authoritative identity returned by EKS DescribeCluster.
type ClusterIdentity struct {
	ClusterName string `json:"cluster_name"`
	ClusterARN  string `json:"cluster_arn"`
	AccountID   string `json:"aws_account_id"`
	Region      string `json:"region"`
}

// Tags returns the identity as low-cardinality tags. It returns nil for an incomplete identity.
//
// Three tags are emitted: eks_cluster_arn plus eks_cluster_account and eks_cluster_region. The
// account and Region use cluster-scoped keys distinct from the node-derived aws_account/region
// host tags, so a cross-account node group yields two clearly-labelled values (where the node
// runs vs. who owns the cluster) rather than two values colliding under one key. Account and
// Region are derived from the ARN so a struct carrying only a valid ClusterARN still emits all
// three, and the ARN is re-validated so a value crossing the Cluster Agent HTTP boundary cannot
// inject arbitrary tag content.
func (c *ClusterIdentity) Tags() []string {
	if c == nil || !IsValidClusterARN(c.ClusterARN) {
		return nil
	}
	parsed, err := awsarn.Parse(c.ClusterARN)
	if err != nil {
		return nil
	}
	return []string{
		taggertags.EksClusterARN + ":" + c.ClusterARN,
		taggertags.EksClusterAccount + ":" + parsed.AccountID,
		taggertags.EksClusterRegion + ":" + parsed.Region,
	}
}

// IsValidClusterARN reports whether clusterARN is a well-formed EKS cluster ARN. It is used on
// both sides of the Cluster Agent boundary: the resolver validates DescribeCluster output, and
// the node Agent re-validates the ARN it receives over HTTP before turning it into a tag, so a
// version-skewed or misbehaving Cluster Agent cannot inject arbitrary tag values.
func IsValidClusterARN(clusterARN string) bool {
	if clusterARN == "" {
		return false
	}
	parsed, err := awsarn.Parse(clusterARN)
	if err != nil {
		return false
	}
	if parsed.Service != "eks" || !partitionPattern.MatchString(parsed.Partition) ||
		!regionPattern.MatchString(parsed.Region) || !accountIDPattern.MatchString(parsed.AccountID) {
		return false
	}
	const clusterPrefix = "cluster/"
	return strings.HasPrefix(parsed.Resource, clusterPrefix) &&
		clusterNamePattern.MatchString(strings.TrimPrefix(parsed.Resource, clusterPrefix))
}
