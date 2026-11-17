// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eksidentity resolves authoritative EKS cluster identity from the Cluster Agent.
package eksidentity

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	ec2tags "github.com/DataDog/datadog-agent/pkg/util/ec2/tags"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"golang.org/x/sync/singleflight"
)

// clusterNameRetryTTL bounds how often cluster name discovery is retried when the
// EC2 kubernetes.io/cluster/<name> tag was not available. Both a missing name and a
// configured-name fallback are cached with this TTL so a transient EC2 tag failure at
// startup cannot pin the wrong name for the lifetime of the Cluster Agent.
const clusterNameRetryTTL = 5 * time.Minute

// clusterNameDiscoveryTimeout bounds the EC2 tag lookup (IMDS, then the EC2 API) inside the
// resolve budget, leaving DescribeCluster the rest.
const clusterNameDiscoveryTimeout = 1500 * time.Millisecond

// clusterNameTimeoutTTL is how long a timed out EC2 tag lookup is remembered.
const clusterNameTimeoutTTL = 5 * time.Second

// resolveBudget bounds one whole resolve: the distribution check, the EC2 tag lookup and
// DescribeCluster. It is below the node Agent client's 3s ResponseHeaderTimeout, so a cold
// resolve answers the request that started it.
const resolveBudget = 2500 * time.Millisecond

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

// ErrNotDetermined indicates that the identity is not known yet and a later attempt can find it:
// the distribution could not be determined (the API server is not reachable yet), or the EC2
// tag lookup ran out of time. It is not cached.
var ErrNotDetermined = errors.New("EKS cluster identity is not determined yet")

// Injectable for tests; production values are the real detectors.
var (
	detectDistribution   = cloudprovider.DCAGetDistribution
	ec2ClusterName       = clusterNameFromEC2Tags
	ec2Tags              = ec2tags.GetTags
	configuredCluster    = clustername.GetClusterName
	resolveIdentityByKey = eks.GetClusterIdentity
	readClusterCA        = func() ([]byte, error) { return os.ReadFile(serviceAccountCAPath) }
)

// discoveredName is a cluster name and whether it came from the node's EC2 tags.
type discoveredName struct {
	name    string
	fromEC2 bool
	// timedOut is set when the EC2 tag lookup ran out of time, so the tags may hold the name.
	timedOut bool
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
	// Every node Agent asks at once after a Cluster Agent restart. They share one resolve, so
	// they make one EC2 tag lookup and one DescribeCluster instead of one each. The resolve runs
	// detached from the node request that started it, so that node disconnecting does not fail
	// it for the others, and within resolveBudget, so it answers before the node gives up.
	results := resolveGroup.DoChan("resolve", func() (v interface{}, err error) {
		defer func() {
			// DoChan re-panics in a new goroutine, which would crash the Cluster Agent.
			if r := recover(); r != nil {
				log.Errorf("EKS cluster identity resolution panicked: %v", r)
				v, err = nil, fmt.Errorf("EKS cluster identity resolution panicked: %v", r)
			}
		}()
		resolveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), resolveBudget)
		defer cancel()
		return resolve(resolveCtx)
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-results:
		if res.Err != nil {
			return nil, res.Err
		}
		id := *res.Val.(*eks.ClusterIdentity)
		return &id, nil
	}
}

// resolveGroup coalesces concurrent Resolve calls.
var resolveGroup singleflight.Group

func resolve(ctx context.Context) (*eks.ClusterIdentity, error) {
	distribution, determined := detectDistribution(ctx)
	if !determined {
		return nil, fmt.Errorf("kube distribution: %w", ErrNotDetermined)
	}
	if distribution != "eks" {
		return nil, ErrNotEKS
	}

	discovered := discoverClusterName(ctx)
	if discovered.name == "" {
		return nil, ErrClusterNameUnavailable
	}
	clusterCA, err := readClusterCA()
	if err == nil && !containsCertificate(clusterCA) {
		// An empty or PEM-less file cannot verify anything. Treat it as unreadable rather than
		// failing every lookup as a mismatch.
		err = errors.New("the cluster CA file holds no certificate")
	}
	if err != nil {
		if !discovered.fromEC2 {
			if discovered.timedOut {
				return nil, fmt.Errorf("EC2 tag lookup timed out: %w", ErrNotDetermined)
			}
			return nil, ErrClusterUnverified
		}
		log.Debugf("Cannot read the cluster CA to verify the EKS cluster, using the EC2 tag cluster name: %v", err)
		clusterCA = nil
	}
	id, err := resolveIdentityByKey(ctx, discovered.name, clusterCA)
	if errors.Is(err, eks.ErrClusterMismatch) && discovered.fromEC2 {
		// The EC2 tag name is otherwise cached for good. Look it up again next time, in case
		// the node's tags change.
		cache.Cache.Delete(cache.BuildAgentKey("eks", "cluster-name"))
	}
	return id, err
}

// containsCertificate reports whether a PEM bundle holds at least one certificate.
func containsCertificate(bundle []byte) bool {
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return false
		}
		if block.Type == "CERTIFICATE" {
			return true
		}
	}
}

// clusterNameFromEC2Tags returns the EKS cluster name from the node's kubernetes.io/cluster/<name>
// EC2 tags. A node can carry several (a migrated node group, a tag copied from another
// cluster) and their order is not stable, so the smallest name is picked to get the same answer
// every time. The shared ec2tags.GetClusterName keeps its first-tag behavior, which other
// callers (kube_cluster_name) depend on.
func clusterNameFromEC2Tags(ctx context.Context) (string, error) {
	tags, err := ec2Tags(ctx)
	if err != nil {
		return "", err
	}
	const prefix = "kubernetes.io/cluster/"
	name := ""
	for _, tag := range tags {
		key, _, _ := strings.Cut(tag, ":")
		candidate, ok := strings.CutPrefix(key, prefix)
		if !ok || candidate == "" {
			continue
		}
		if name == "" || candidate < name {
			name = candidate
		}
	}
	if name == "" {
		return "", errors.New("no kubernetes.io/cluster tag in the EC2 tags")
	}
	return name, nil
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
	// ctx is the shared resolve's own context (see Resolve), so a node disconnecting does not
	// cut the lookup short.
	lookupCtx, cancel := context.WithTimeout(ctx, clusterNameDiscoveryTimeout)
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
	// that ran out of time says nothing about the EC2 tags, so it is cached for a few seconds
	// only, enough to keep a burst of node requests from repeating it.
	discovered := discoveredName{name: configuredCluster(ctx, "")}
	if timedOut {
		log.Debugf("EKS cluster name lookup from EC2 tags timed out, retrying in %s: %v", clusterNameTimeoutTTL, err)
		discovered.timedOut = true
		cache.Cache.Set(cacheKey, discovered, clusterNameTimeoutTTL)
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
