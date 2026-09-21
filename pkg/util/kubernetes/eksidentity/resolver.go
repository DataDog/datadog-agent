// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eksidentity resolves authoritative EKS cluster identity from the Cluster Agent.
package eksidentity

import (
	"context"
	"errors"

	ec2tags "github.com/DataDog/datadog-agent/pkg/util/ec2/tags"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// ErrNotEKS indicates the Cluster Agent is not running on an EKS cluster.
var ErrNotEKS = errors.New("cluster is not EKS")

// ErrClusterNameUnavailable indicates that no EKS cluster name could be determined.
var ErrClusterNameUnavailable = errors.New("EKS cluster name is unavailable")

// Injectable for tests; production values are the real detectors.
var (
	detectDistribution   = cloudprovider.DCAGetName
	ec2ClusterName       = ec2tags.GetClusterName
	configuredCluster    = clustername.GetClusterName
	resolveIdentityByKey = eks.GetClusterIdentity
)

// Resolve resolves authoritative EKS identity from the Cluster Agent.
// The EC2 kubernetes.io/cluster/<name> tag preserves the real EKS name; the configured
// Datadog cluster name is only a fallback because customers may choose a different display name.
func Resolve(ctx context.Context) (*eks.ClusterIdentity, error) {
	if detectDistribution(ctx) != "eks" {
		return nil, ErrNotEKS
	}

	clusterName, err := ec2ClusterName(ctx)
	if err != nil || clusterName == "" {
		clusterName = configuredCluster(ctx, "")
	}
	if clusterName == "" {
		return nil, ErrClusterNameUnavailable
	}
	return resolveIdentityByKey(ctx, clusterName)
}

// Tags returns the identity as low-cardinality tags, or nil when unavailable.
func Tags(ctx context.Context) []string {
	identity, err := Resolve(ctx)
	if err != nil {
		return nil
	}
	return identity.Tags()
}
