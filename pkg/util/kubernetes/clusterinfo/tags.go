// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package clusterinfo provides utilities to retrieve cluster information from the Cluster Agent
package clusterinfo

import (
	"context"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	"github.com/cenkalti/backoff/v7"
)

// GetClusterAgentStaticTags gets the tags from the cluster-agent
// Currently only ClusterID for orch_cluster_id tag
// Context is passed to match the host tags interface
func GetClusterAgentStaticTags(context.Context) ([]string, error) {
	clusterID, err := clustername.GetClusterID()
	if err != nil {
		return nil, err
	}

	if clusterID != "" {
		return []string{tags.OrchClusterID + ":" + clusterID}, nil
	}

	return nil, nil
}

// clusterIDCall is the one Cluster Agent cluster ID call in flight for the process.
var clusterIDCall sharedCall[[]string]

// GetClusterAgentStaticTagsWithRetry gets the tags from the cluster-agent with a constant backoff
// policy, for at most StartupRetryBudget and never past ctx's deadline. The cluster ID query does
// not take a ctx, so each attempt runs as a shared call and the budget still holds when the
// Cluster Agent hangs.
func GetClusterAgentStaticTagsWithRetry(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, startupRetryBudget)
	defer cancel()

	backoffPolicy := backoff.NewConstantBackOff(1 * time.Second)
	res, err := backoff.Retry(ctx, func() ([]string, error) {
		return clusterIDCall.Do(ctx, GetClusterAgentStaticTags)
	}, backoff.WithBackOff(backoffPolicy))

	return res, err
}
