// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eksidentity resolves authoritative EKS cluster identity from the Cluster Agent.
package eksidentity

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	ec2tags "github.com/DataDog/datadog-agent/pkg/util/ec2/tags"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

// clusterNameFailureTTL bounds how often a missing cluster name is re-discovered.
const clusterNameFailureTTL = 5 * time.Minute

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
// The distribution check is served by cloudprovider's permanent cache; the discovered name
// (or its absence) is cached here so node requests do not re-run EC2 tag discovery.
func Resolve(ctx context.Context) (*eks.ClusterIdentity, error) {
	if detectDistribution(ctx) != "eks" {
		return nil, ErrNotEKS
	}

	clusterName := discoverClusterName(ctx)
	if clusterName == "" {
		return nil, ErrClusterNameUnavailable
	}
	return resolveIdentityByKey(ctx, clusterName)
}

func discoverClusterName(ctx context.Context) string {
	cacheKey := cache.BuildAgentKey("eks", "cluster-name")
	if cached, found := cache.Cache.Get(cacheKey); found {
		if name, ok := cached.(string); ok {
			return name
		}
	}

	clusterName, err := ec2ClusterName(ctx)
	if err != nil || clusterName == "" {
		clusterName = configuredCluster(ctx, "")
	}
	if clusterName == "" {
		// Bounded negative cache: the name may appear later (for example once EC2 tags load).
		cache.Cache.Set(cacheKey, "", clusterNameFailureTTL)
		return ""
	}
	cache.Cache.Set(cacheKey, clusterName, cache.NoExpiration)
	return clusterName
}

// Tags returns the identity as low-cardinality tags, or nil when unavailable.
func Tags(ctx context.Context) []string {
	identity, err := Resolve(ctx)
	if err != nil {
		return nil
	}
	return identity.Tags()
}
