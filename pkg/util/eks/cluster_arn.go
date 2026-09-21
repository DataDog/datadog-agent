// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eks provides utilities for EKS cluster identity.
package eks

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clusterARNCacheKey = "eks_cluster_arn"

	// MaxClusterNameLength is the maximum length of an EKS cluster name (AWS limit)
	MaxClusterNameLength = 100
)

var (
	// validClusterNamePattern matches valid EKS cluster names per AWS documentation:
	// alphanumeric, hyphens, underscores, 1-100 chars
	validClusterNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)

	// validRegionPattern matches AWS region format
	validRegionPattern = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-\d$`)

	// validAccountIDPattern matches 12-digit AWS account IDs
	validAccountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)
)

// ClusterIdentity holds EKS cluster AWS identity information.
// These values are derived from IMDS and represent the node's AWS identity,
// which is assumed to match the cluster's AWS identity when nodes and cluster
// reside in the same account.
type ClusterIdentity struct {
	// ClusterName is the EKS cluster name
	ClusterName string
	// ClusterARN is the full EKS cluster ARN (arn:aws:eks:region:account:cluster/name)
	ClusterARN string
	// AccountID is the AWS account ID (12 digits)
	AccountID string
	// Region is the AWS region (e.g., us-west-2)
	Region string
}

// BuildClusterARN constructs an EKS cluster ARN from its components.
// Returns empty string if any required component is empty or invalid.
//
// Note: This function assumes the cluster resides in the same account as the
// calling node's IMDS identity. Cross-account node pools are not supported.
func BuildClusterARN(clusterName, accountID, region string) string {
	if clusterName == "" || accountID == "" || region == "" {
		return ""
	}
	if !IsValidClusterName(clusterName) {
		log.Debugf("Invalid EKS cluster name format: %s", clusterName)
		return ""
	}
	if !validAccountIDPattern.MatchString(accountID) {
		log.Debugf("Invalid AWS account ID format: %s", accountID)
		return ""
	}
	if !validRegionPattern.MatchString(region) {
		log.Debugf("Invalid AWS region format: %s", region)
		return ""
	}
	return fmt.Sprintf("arn:aws:eks:%s:%s:cluster/%s", region, accountID, clusterName)
}

// IsValidClusterName checks if a cluster name matches EKS naming rules.
func IsValidClusterName(name string) bool {
	if name == "" || len(name) > MaxClusterNameLength {
		return false
	}
	return validClusterNamePattern.MatchString(name)
}

// GetClusterIdentity retrieves EKS cluster identity from IMDS and cluster name.
// Results are cached indefinitely since cluster identity does not change.
//
// clusterName must be provided (typically from kubernetes.io/cluster/<name> tag
// or DD_CLUSTER_NAME config). This function retrieves account and region from
// the node's IMDS identity document.
//
// Limitation: This assumes the EKS cluster and worker nodes are in the same
// AWS account. Cross-account node pools will produce an incorrect cluster ARN.
func GetClusterIdentity(ctx context.Context, clusterName string) (*ClusterIdentity, error) {
	if clusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}

	cacheKey := cache.BuildAgentKey(clusterARNCacheKey)
	if cached, found := cache.Cache.Get(cacheKey); found {
		return cached.(*ClusterIdentity), nil
	}

	accountID, err := ec2.GetAccountID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS account ID from IMDS: %w", err)
	}

	region, err := ec2.GetRegion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS region from IMDS: %w", err)
	}

	// Normalize inputs
	clusterName = strings.TrimSpace(clusterName)
	accountID = strings.TrimSpace(accountID)
	region = strings.TrimSpace(region)

	clusterARN := BuildClusterARN(clusterName, accountID, region)
	if clusterARN == "" {
		return nil, fmt.Errorf("failed to build valid cluster ARN from name=%s account=%s region=%s",
			clusterName, accountID, region)
	}

	identity := &ClusterIdentity{
		ClusterName: clusterName,
		ClusterARN:  clusterARN,
		AccountID:   accountID,
		Region:      region,
	}

	cache.Cache.Set(cacheKey, identity, cache.NoExpiration)
	log.Infof("EKS cluster identity resolved: %s", clusterARN)

	return identity, nil
}

// ParseClusterARN extracts components from an EKS cluster ARN.
// Returns nil if the ARN is malformed or not an EKS cluster ARN.
func ParseClusterARN(arn string) *ClusterIdentity {
	if arn == "" {
		return nil
	}

	// Expected format: arn:aws:eks:region:account:cluster/name
	// or arn:aws-cn:eks:... for China, arn:aws-us-gov:eks:... for GovCloud
	parts := strings.Split(arn, ":")
	if len(parts) != 6 {
		return nil
	}

	partition := parts[1]
	if partition != "aws" && partition != "aws-cn" && partition != "aws-us-gov" {
		return nil
	}

	service := parts[2]
	if service != "eks" {
		return nil
	}

	region := parts[3]
	accountID := parts[4]
	resource := parts[5]

	if !strings.HasPrefix(resource, "cluster/") {
		return nil
	}

	clusterName := strings.TrimPrefix(resource, "cluster/")
	if clusterName == "" {
		return nil
	}

	return &ClusterIdentity{
		ClusterName: clusterName,
		ClusterARN:  arn,
		AccountID:   accountID,
		Region:      region,
	}
}
