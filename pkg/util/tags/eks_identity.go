// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ec2

// Package tags provides utilities for working with tags.
package tags

import (
	"context"
	"time"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// eksIdentityResolutionTimeout bounds how long we wait for EKS identity at startup.
// If resolution fails, we proceed without EKS identity tags rather than blocking.
const eksIdentityResolutionTimeout = 5 * time.Second

// GetEKSClusterIdentityTags resolves EKS cluster identity via DescribeCluster and returns
// tags for eks_cluster_arn, aws_account, and region. Returns nil on failure (fail closed).
//
// This function is designed to be called once during agent startup. The resolved identity
// is cached indefinitely by the eks package since cluster identity does not change.
//
// Parameters:
//   - ctx: context for cancellation
//   - clusterName: EKS cluster name (from DD_CLUSTER_NAME or discovered)
//
// On failure, logs a warning and returns nil. The agent continues without EKS identity tags.
func GetEKSClusterIdentityTags(ctx context.Context, clusterName string) []string {
	if clusterName == "" {
		return nil
	}

	// Apply bounded timeout to avoid blocking startup indefinitely
	resolveCtx, cancel := context.WithTimeout(ctx, eksIdentityResolutionTimeout)
	defer cancel()

	identity, err := eks.GetClusterIdentity(resolveCtx, clusterName)
	if err != nil {
		// Log at debug level - this is expected when:
		// - Not running on EKS (no IMDS region)
		// - Missing eks:DescribeCluster permission
		// - Cluster name is invalid
		log.Debugf("EKS cluster identity resolution failed for %s: %v", clusterName, err)
		return nil
	}

	tags := []string{
		taggertags.EksClusterARN + ":" + identity.ClusterARN,
		taggertags.AwsAccount + ":" + identity.AccountID,
		taggertags.Region + ":" + identity.Region,
	}

	log.Infof("EKS cluster identity tags resolved: eks_cluster_arn=%s, aws_account=%s, region=%s",
		identity.ClusterARN, identity.AccountID, identity.Region)

	return tags
}
