// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/dmi"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/cloudprovider"
	"github.com/cenkalti/backoff/v7"
	"golang.org/x/sync/semaphore"
)

var errIncompleteIdentity = errors.New("cluster agent returned incomplete EKS identity")

// StartupRetryBudget bounds how long startup waits for the Cluster Agent to answer.
const StartupRetryBudget = 10 * time.Second

// laterAttemptTimeout bounds the single attempt made once the startup budget is spent, so a
// later caller (another startup component) never waits long on a Cluster Agent that hangs.
const laterAttemptTimeout = time.Second

// startupFailureTTL is how long a failed startup lookup is reused. The tagger, DogStatsD and OTLP
// compute their static tags within seconds of each other, so once the budget is spent the
// callers right after it return the same failure at once instead of adding a second each.
const startupFailureTTL = 5 * time.Second

// startupIdentity is shared by every GetClusterAgentEKSIdentityTagsWithRetry caller in the process.
// lock is a one-slot semaphore rather than a mutex so that a caller waiting for the in-flight
// lookup can give up when its own ctx is done.
var startupIdentity = struct {
	lock        *semaphore.Weighted
	tags        []string
	budgetSpent bool
	lastErr     error
	lastErrAt   time.Time
}{lock: semaphore.NewWeighted(1)}

var getEKSClusterIdentity = func(ctx context.Context) (*identity.ClusterIdentity, error) {
	client, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return nil, err
	}
	return client.GetEKSClusterIdentity(ctx)
}

// getNodeDistribution is the node's kube distribution, the value of its kube_distribution tag
// (cloudprovider.GetName): from the node's kubelet version (v1.31.0-eks-...) and its labels
// (eks.amazonaws.com/nodegroup, eks.amazonaws.com/compute-type, set on managed, Auto Mode,
// Fargate and hybrid nodes), read through the Cluster Agent once and cached for the process.
// It is the evidence that a node is on EKS. It does not read DMI or call IMDS, so it works on
// Windows nodes, with ec2_use_dmi: false and on EKS Hybrid nodes, and it is not fooled by kops,
// self-managed Kubernetes, ROSA or kind on EC2.
var getNodeDistribution = cloudprovider.GetName

// hostMayBeAWS reports false only when local DMI says the node is not an EC2 instance. It is
// used only while the Cluster Agent cannot be reached yet, to decide whether waiting for it is
// worth the startup budget. Without DMI (Windows, a container without /sys) it cannot tell, so
// it says yes. It reads only local files.
var hostMayBeAWS = defaultHostMayBeAWS

func defaultHostMayBeAWS() bool {
	vendor := dmi.GetBoardVendor()
	return vendor == "" || vendor == ec2.DMIBoardVendor || ec2.IsRunningOnDMI()
}

// errNotEKSNode is returned when the node's distribution is known and is not EKS.
var errNotEKSNode = errors.New("node is not on EKS")

// checkEKSNode returns nil when the node is on EKS. Answers that will not change within the
// startup budget are backoff.Permanent: a known non-EKS distribution, a client configuration
// error, or a Cluster Agent that cannot be reached from a node DMI places off EC2.
func checkEKSNode(ctx context.Context) error {
	// An EKS Fargate sidecar is on EKS by definition. It has no node to ask about.
	if env.IsFeaturePresent(env.EKSFargate) {
		return nil
	}
	distribution, err := getNodeDistribution(ctx)
	if err != nil {
		if clusteragent.IsConfigError(err) {
			return backoff.Permanent(err)
		}
		if !hostMayBeAWS() {
			return backoff.Permanent(fmt.Errorf("cannot reach the Cluster Agent and the node is not on EC2: %w", err))
		}
		return err
	}
	if distribution != "eks" {
		return backoff.Permanent(errNotEKSNode)
	}
	return nil
}

// GetClusterAgentEKSIdentityHostTags is GetClusterAgentEKSIdentityTags for the host-tags
// provider. It refreshes on its own interval, so answers that will not change are an empty
// success rather than an error that is logged on every refresh: a node not on EKS, and a 404
// from a Cluster Agent that is not on EKS, or one too old to serve identity.
func GetClusterAgentEKSIdentityHostTags(ctx context.Context) ([]string, error) {
	distribution, err := getNodeDistribution(ctx)
	if err != nil {
		return nil, err
	}
	if distribution != "eks" {
		return nil, nil
	}
	tags, err := GetClusterAgentEKSIdentityTags(ctx)
	var statusErr *clusteragent.StatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	return tags, err
}

// GetClusterAgentEKSIdentityTags returns authoritative EKS identity as low-cardinality tags.
func GetClusterAgentEKSIdentityTags(ctx context.Context) ([]string, error) {
	id, err := getEKSClusterIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tags := id.Tags()
	if tags == nil {
		return nil, errIncompleteIdentity
	}
	return tags, nil
}

// GetClusterAgentEKSIdentityTagsWithRetry fetches the EKS identity tags with a short constant
// backoff. The static-tags paths compute tags exactly once at Agent startup, where the node
// Agent routinely becomes ready before the Cluster Agent is reachable; without a retry a single
// cold-start miss would leave eks_cluster_arn permanently absent from container tags and APM
// spans for the life of the process. Only failures to get an answer are retried: once the
// Cluster Agent answers (404 on non-EKS clusters or an older Cluster Agent, 503 when
// DescribeCluster fails, or an identity without a valid ARN), the answer will not change within
// the window, and retrying it would block every caller for the full budget on each startup. The
// one exception is an answer the Cluster Agent marks with identity.RetryLaterHeader: it has not
// determined the identity yet, or its last attempt failed in a way that can clear on its own.
// The host-metadata provider path, which re-runs on its own refresh interval and is self-healing,
// keeps using the non-retrying call above with a single attempt.
//
// Several startup components (the tagger, DogStatsD, OTLP) compute static tags, so the retry
// budget is spent at most once per process: concurrent callers wait for the in-flight lookup, a
// success is reused, and once the budget is spent later callers make a single attempt. An
// unreachable Cluster Agent therefore delays startup by one budget rather than one per caller.
// Every caller stops early when its own ctx is done.
func GetClusterAgentEKSIdentityTagsWithRetry(ctx context.Context) ([]string, error) {
	if err := startupIdentity.lock.Acquire(ctx, 1); err != nil {
		return nil, err
	}
	defer startupIdentity.lock.Release(1)

	if startupIdentity.tags != nil {
		return slices.Clone(startupIdentity.tags), nil
	}
	if startupIdentity.lastErr != nil && time.Since(startupIdentity.lastErrAt) < startupFailureTTL {
		return nil, startupIdentity.lastErr
	}

	budget := StartupRetryBudget
	if startupIdentity.budgetSpent {
		budget = laterAttemptTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var tags []string
	var err error
	if startupIdentity.budgetSpent {
		if err = checkEKSNode(ctx); err == nil {
			tags, err = GetClusterAgentEKSIdentityTags(ctx)
		}
	} else {
		startupIdentity.budgetSpent = true
		backoffPolicy := backoff.NewConstantBackOff(1 * time.Second)
		tags, err = backoff.Retry(ctx, func() ([]string, error) {
			// Without EKS evidence the budget is not spent: kops or kind on EC2, other clouds
			// and on-prem clusters never wait on the Cluster Agent here.
			if err := checkEKSNode(ctx); err != nil {
				return nil, err
			}
			tags, err := GetClusterAgentEKSIdentityTags(ctx)
			var statusErr *clusteragent.StatusError
			if (errors.As(err, &statusErr) && !statusErr.RetryLater) || errors.Is(err, errIncompleteIdentity) {
				return nil, backoff.Permanent(err)
			}
			return tags, err
		}, backoff.WithBackOff(backoffPolicy))
	}
	if err != nil {
		// A caller that stopped early (its own ctx) says nothing about the Cluster Agent.
		if !errors.Is(err, context.Canceled) {
			startupIdentity.lastErr, startupIdentity.lastErrAt = err, time.Now()
		}
		return nil, err
	}
	startupIdentity.tags = tags
	return slices.Clone(tags), nil
}
