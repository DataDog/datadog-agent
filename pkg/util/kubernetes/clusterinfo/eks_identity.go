// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"

	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

var getEKSClusterIdentity = func(ctx context.Context) (*eks.ClusterIdentity, error) {
	client, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return nil, err
	}
	return client.GetEKSClusterIdentity(ctx)
}

// GetClusterAgentEKSIdentityTags returns authoritative EKS identity as low-cardinality tags.
func GetClusterAgentEKSIdentityTags(ctx context.Context) ([]string, error) {
	identity, err := getEKSClusterIdentity(ctx)
	if err != nil {
		return nil, err
	}
	tags := identity.Tags()
	if tags == nil {
		return nil, errors.New("Cluster Agent returned incomplete EKS identity")
	}
	return tags, nil
}
