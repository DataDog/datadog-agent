// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/cenkalti/backoff/v7"
)

var errIncompleteIdentity = errors.New("cluster agent returned incomplete EKS identity")

var getEKSClusterIdentity = func(ctx context.Context) (*identity.ClusterIdentity, error) {
	client, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return nil, err
	}
	return client.GetEKSClusterIdentity(ctx)
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
// host-metadata provider path, which re-runs on its own refresh interval and is self-healing,
// keeps using the non-retrying call above with a single attempt.
func GetClusterAgentEKSIdentityTagsWithRetry() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	backoffPolicy := backoff.NewConstantBackOff(1 * time.Second)
	return backoff.Retry(ctx, func() ([]string, error) {
		tags, err := GetClusterAgentEKSIdentityTags(ctx)
		var statusErr *clusteragent.StatusError
		if errors.As(err, &statusErr) || errors.Is(err, errIncompleteIdentity) {
			return nil, backoff.Permanent(err)
		}
		return tags, err
	}, backoff.WithBackOff(backoffPolicy))
}
