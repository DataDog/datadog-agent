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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// clusterNameRetryTTL bounds how often cluster name discovery is retried when the
// EC2 kubernetes.io/cluster/<name> tag was not available. Both a missing name and a
// configured-name fallback are cached with this TTL so a transient EC2 tag failure at
// startup cannot pin the wrong name for the lifetime of the Cluster Agent. Variable so
// tests can exercise expiry without sleeping.
var clusterNameRetryTTL = 5 * time.Minute

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

	// The EC2 tag carries the real EKS cluster name (which may contain upper-case
	// characters the Datadog cluster name cannot). Only that source is cached permanently.
	clusterName, err := ec2ClusterName(ctx)
	if err == nil && clusterName != "" {
		cache.Cache.Set(cacheKey, clusterName, cache.NoExpiration)
		return clusterName
	}
	log.Debugf("EKS cluster name is not available from EC2 tags (retry in %s), falling back to the configured cluster name: %v", clusterNameRetryTTL, err)

	// Fallback to the configured Datadog cluster name. It may differ from the real EKS
	// name, so it is only cached briefly and EC2 tag discovery is retried afterwards.
	clusterName = configuredCluster(ctx, "")
	cache.Cache.Set(cacheKey, clusterName, clusterNameRetryTTL)
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
