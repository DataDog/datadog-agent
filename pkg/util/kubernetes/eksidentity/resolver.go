// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eksidentity resolves authoritative EKS cluster identity from the Cluster Agent.
package eksidentity

import (
	"context"
	"errors"
	"os"
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
// startup cannot pin the wrong name for the lifetime of the Cluster Agent.
const clusterNameRetryTTL = 5 * time.Minute

// clusterNameDiscoveryTimeout bounds the EC2 tag lookup (IMDS, then the EC2 API). Together with
// the DescribeCluster budget it keeps a cold resolve close to the node Agent's request timeout.
const clusterNameDiscoveryTimeout = 1500 * time.Millisecond

// serviceAccountCAPath is where Kubernetes mounts the cluster CA into every pod, including
// Fargate pods. It is the path rest.InClusterConfig reads.
const serviceAccountCAPath = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

// ErrNotEKS indicates the Cluster Agent is not running on an EKS cluster.
var ErrNotEKS = errors.New("cluster is not EKS")

// ErrClusterNameUnavailable indicates that no EKS cluster name could be determined.
var ErrClusterNameUnavailable = errors.New("EKS cluster name is unavailable")

// ErrClusterUnverified indicates that the only cluster name available is the configured one and
// the cluster CA needed to check it against EKS could not be read.
var ErrClusterUnverified = errors.New("configured EKS cluster name cannot be verified without the cluster CA")

// Injectable for tests; production values are the real detectors.
var (
	detectDistribution   = cloudprovider.DCAGetName
	ec2ClusterName       = ec2tags.GetClusterName
	configuredCluster    = clustername.GetClusterName
	resolveIdentityByKey = eks.GetClusterIdentity
	readClusterCA        = func() ([]byte, error) { return os.ReadFile(serviceAccountCAPath) }
)

// discoveredName is a cluster name and whether it came from the node's EC2 tags.
type discoveredName struct {
	name    string
	fromEC2 bool
}

// Resolve resolves authoritative EKS identity from the Cluster Agent.
// The EC2 kubernetes.io/cluster/<name> tag preserves the real EKS name; the configured
// Datadog cluster name is only a fallback because customers may choose a different display name.
// The distribution check is served by cloudprovider's permanent cache; the discovered name
// (or its absence) is cached here so node requests do not re-run EC2 tag discovery.
//
// DescribeCluster is trusted only for the cluster this pod runs in: its certificate authority
// must be the CA mounted into this pod. A name can be stale or reused (a blue/green pair, a
// recreated cluster, a copied values file), and the ARN of such a cluster passes every other
// check. When the CA cannot be read, only the EC2 tag name, which EKS sets on this cluster's
// nodes, is used.
func Resolve(ctx context.Context) (*eks.ClusterIdentity, error) {
	if detectDistribution(ctx) != "eks" {
		return nil, ErrNotEKS
	}

	discovered := discoverClusterName(ctx)
	if discovered.name == "" {
		return nil, ErrClusterNameUnavailable
	}
	clusterCA, err := readClusterCA()
	if err != nil {
		if !discovered.fromEC2 {
			return nil, ErrClusterUnverified
		}
		log.Debugf("Cannot read the cluster CA to verify the EKS cluster, using the EC2 tag cluster name: %v", err)
		clusterCA = nil
	}
	return resolveIdentityByKey(ctx, discovered.name, clusterCA)
}

func discoverClusterName(ctx context.Context) discoveredName {
	cacheKey := cache.BuildAgentKey("eks", "cluster-name")
	if cached, found := cache.Cache.Get(cacheKey); found {
		if name, ok := cached.(discoveredName); ok {
			return name
		}
	}

	// The EC2 tag carries the real EKS cluster name (which may contain upper-case
	// characters the Datadog cluster name cannot). Only that source is cached permanently.
	//
	// The lookup runs on a context detached from the node request that triggered it, with its
	// own short budget, so a disconnecting node cannot make it fail.
	lookupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), clusterNameDiscoveryTimeout)
	clusterName, err := ec2ClusterName(lookupCtx)
	timedOut := lookupCtx.Err() != nil
	cancel()
	if err == nil && clusterName != "" {
		discovered := discoveredName{name: clusterName, fromEC2: true}
		cache.Cache.Set(cacheKey, discovered, cache.NoExpiration)
		return discovered
	}

	// Fallback to the configured Datadog cluster name. It may differ from the real EKS
	// name, so it is only cached briefly and EC2 tag discovery is retried afterwards. A lookup
	// that ran out of time says nothing about the EC2 tags, so its fallback is not cached.
	discovered := discoveredName{name: configuredCluster(ctx, "")}
	if timedOut {
		log.Debugf("EKS cluster name lookup from EC2 tags timed out, using the configured cluster name for this request: %v", err)
		return discovered
	}
	log.Debugf("EKS cluster name is not available from EC2 tags (retry in %s), falling back to the configured cluster name: %v", clusterNameRetryTTL, err)
	cache.Cache.Set(cacheKey, discovered, clusterNameRetryTTL)
	return discovered
}

// Tags returns the identity as low-cardinality tags, or nil when unavailable.
func Tags(ctx context.Context) []string {
	identity, err := Resolve(ctx)
	if err != nil {
		log.Debugf("EKS cluster identity is not available for the Cluster Agent's own tags: %v", err)
		return nil
	}
	return identity.Tags()
}
