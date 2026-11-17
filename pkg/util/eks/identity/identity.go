// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package identity holds the authoritative EKS cluster identity data type and its tag
// projection. It is deliberately free of the AWS SDK so that the many node-Agent packages that
// only need the data type (the Cluster Agent client interface, the clusterinfo bridge) do not
// link any AWS SDK package. The resolver, which loads the AWS configuration and calls
// DescribeCluster, lives in the parent pkg/util/eks.
package identity

import (
	"regexp"
	"strings"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
)

var (
	clusterNamePattern = regexp.MustCompile(`^[0-9A-Za-z][A-Za-z0-9_-]{0,99}$`)
	accountIDPattern   = regexp.MustCompile(`^[0-9]{12}$`)
	// partitionPattern and regionPattern constrain the ARN fields that become tag values.
	// Splitting an ARN on colons does no charset validation, so without these a
	// Region or partition containing a comma or space would be emitted verbatim as a tag value
	// (and would corrupt consumers that comma-join static tags). They match the e2e contract
	// ^arn:aws[a-z-]*:eks:[a-z0-9-]+:[0-9]{12}:cluster/...$.
	partitionPattern = regexp.MustCompile(`^aws[a-z0-9-]*$`)
	regionPattern    = regexp.MustCompile(`^[a-z0-9-]+$`)
)

// RetryLaterHeader is part of the identity endpoint's contract: the Cluster Agent sets it to
// "true" or "false" on every non-200 answer it produces, and node Agents rely on it to tell
// those answers from errors returned by a proxy or mesh sidecar in front of it.
//   - "true": the answer can change within a node's startup budget, so the node retries it.
//     The identity is not determined yet (only in the Cluster Agent's first minute), or a cached
//     DescribeCluster failure expires within 5s.
//   - "false": the answer is final for the node's startup.
//
// Without the header, a 408, 429 or 5xx is retried and any other status is final. A 404, 401
// or 403 is final with or without it: a Cluster Agent built before the endpoint answers 403.
const RetryLaterHeader = "X-Datadog-EKS-Identity-Retry"

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
	if c == nil {
		return nil
	}
	parsed, ok := ParseClusterARN(c.ClusterARN)
	if !ok {
		return nil
	}
	return []string{
		taggertags.EksClusterARN + ":" + c.ClusterARN,
		taggertags.EksClusterAccount + ":" + parsed.AccountID,
		taggertags.EksClusterRegion + ":" + parsed.Region,
	}
}

// IsValidClusterName reports whether name can be an EKS cluster name: 1 to 100 characters,
// letters, digits, hyphens and underscores, starting with a letter or digit.
func IsValidClusterName(name string) bool {
	return clusterNamePattern.MatchString(name)
}

// ParseClusterARN validates an EKS cluster ARN of the form
// arn:<partition>:eks:<region>:<account>:cluster/<name> and returns the identity it encodes.
// It is a small local parser so that node-side binaries, which only need to validate the ARN
// received from the Cluster Agent, do not link an AWS SDK package for it. It runs on both sides
// of the Cluster Agent boundary: on DescribeCluster output, and again on the node before the ARN
// becomes a tag, so a version-skewed or misbehaving Cluster Agent cannot inject tag values.
func ParseClusterARN(clusterARN string) (ClusterIdentity, bool) {
	// Same split as the AWS SDK ARN parser: six colon separated sections, the last one being
	// the resource, which may itself contain colons.
	sections := strings.SplitN(clusterARN, ":", 6)
	if len(sections) != 6 || sections[0] != "arn" {
		return ClusterIdentity{}, false
	}
	partition, service, region, accountID, resource := sections[1], sections[2], sections[3], sections[4], sections[5]
	if service != "eks" || !partitionPattern.MatchString(partition) ||
		!regionPattern.MatchString(region) || !accountIDPattern.MatchString(accountID) {
		return ClusterIdentity{}, false
	}
	const clusterPrefix = "cluster/"
	name, found := strings.CutPrefix(resource, clusterPrefix)
	if !found || !clusterNamePattern.MatchString(name) {
		return ClusterIdentity{}, false
	}
	return ClusterIdentity{
		ClusterName: name,
		ClusterARN:  clusterARN,
		AccountID:   accountID,
		Region:      region,
	}, true
}
