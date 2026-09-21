// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ec2

// Package eks provides utilities for resolving EKS cluster identity.
package eks

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/eks"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	clusterIdentityCacheKey = "eks_cluster_identity"

	// apiTimeout is the timeout for EKS API calls
	apiTimeout = 10 * time.Second

	// MaxClusterNameLength is the maximum length of an EKS cluster name (AWS limit)
	MaxClusterNameLength = 100
)

var (
	// validClusterNamePattern matches valid EKS cluster names per AWS documentation:
	// alphanumeric, hyphens, underscores, 1-100 chars, must start with alphanumeric
	validClusterNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,99}$`)

	// validAccountIDPattern matches 12-digit AWS account IDs
	validAccountIDPattern = regexp.MustCompile(`^[0-9]{12}$`)

	// validRegionPattern matches AWS region format
	validRegionPattern = regexp.MustCompile(`^[a-z]{2}(-gov)?-[a-z]+-\d$`)

	// allowedPartitions restricts ARN parsing to known AWS partitions
	allowedPartitions = map[string]bool{
		"aws":        true,
		"aws-cn":     true,
		"aws-us-gov": true,
	}

	// ErrNoPermission indicates the agent lacks eks:DescribeCluster permission
	ErrNoPermission = errors.New("eks:DescribeCluster permission denied")

	// ErrClusterNotFound indicates the cluster does not exist
	ErrClusterNotFound = errors.New("EKS cluster not found")

	// ErrInvalidClusterName indicates an invalid cluster name format
	ErrInvalidClusterName = errors.New("invalid EKS cluster name format")

	// ErrNameMismatch indicates the returned ARN doesn't match the requested cluster name
	ErrNameMismatch = errors.New("cluster name mismatch between request and response")
)

// ClusterIdentity holds authoritative EKS cluster AWS identity from DescribeCluster.
// All fields are derived from the EKS control plane response, not node IMDS.
type ClusterIdentity struct {
	// ClusterName is the EKS cluster name
	ClusterName string
	// ClusterARN is the full EKS cluster ARN from DescribeCluster
	ClusterARN string
	// AccountID is the cluster owner's AWS account ID (12 digits)
	AccountID string
	// Region is the AWS region where the cluster is hosted
	Region string
}

// eksClient abstracts the EKS API for testing
type eksClient interface {
	DescribeCluster(ctx context.Context, params *eks.DescribeClusterInput, optFns ...func(*eks.Options)) (*eks.DescribeClusterOutput, error)
}

// clientFactory creates EKS clients; replaceable for testing
var clientFactory = func(ctx context.Context, region string) (eksClient, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return eks.NewFromConfig(cfg), nil
}

// GetClusterIdentity retrieves authoritative EKS cluster identity by calling
// eks:DescribeCluster. This requires the node IAM role to have eks:DescribeCluster
// permission, which is included in AmazonEKSWorkerNodePolicy by default.
//
// The function:
//   - Validates cluster name format before making API calls
//   - Calls DescribeCluster to get the authoritative cluster ARN
//   - Parses the ARN to extract account ID and region
//   - Validates the response cluster name matches the request
//   - Caches successful results indefinitely (cluster identity does not change)
//
// Fails closed: returns an error (not partial data) on any failure including
// missing permissions, API errors, or validation failures.
//
// Parameters:
//   - ctx: context for cancellation
//   - clusterName: EKS cluster name (from DD_CLUSTER_NAME or discovered)
//
// Returns ErrNoPermission if the agent lacks eks:DescribeCluster IAM permission.
func GetClusterIdentity(ctx context.Context, clusterName string) (*ClusterIdentity, error) {
	if clusterName == "" {
		return nil, ErrInvalidClusterName
	}

	clusterName = strings.TrimSpace(clusterName)
	if !IsValidClusterName(clusterName) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidClusterName, clusterName)
	}

	// Check cache first
	cacheKey := cache.BuildAgentKey(clusterIdentityCacheKey)
	if cached, found := cache.Cache.Get(cacheKey); found {
		identity := cached.(*ClusterIdentity)
		// Validate cached identity matches requested cluster
		if identity.ClusterName == clusterName {
			return identity, nil
		}
		// Cache mismatch - cluster name changed, re-fetch
		log.Debugf("Cached EKS identity for %s doesn't match requested %s, re-fetching",
			identity.ClusterName, clusterName)
	}

	// Get region from IMDS (only used to know which EKS regional endpoint to call)
	region, err := ec2.GetRegion(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get AWS region from IMDS: %w", err)
	}

	// Create EKS client
	client, err := clientFactory(ctx, region)
	if err != nil {
		return nil, err
	}

	// Call DescribeCluster with timeout
	apiCtx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()

	output, err := client.DescribeCluster(apiCtx, &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		// Check for specific error types
		errStr := err.Error()
		if strings.Contains(errStr, "AccessDenied") || strings.Contains(errStr, "not authorized") {
			log.Warnf("EKS DescribeCluster permission denied for cluster %s - ensure node IAM role has eks:DescribeCluster", clusterName)
			return nil, fmt.Errorf("%w: %v", ErrNoPermission, err)
		}
		if strings.Contains(errStr, "ResourceNotFoundException") || strings.Contains(errStr, "cluster not found") {
			return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, clusterName)
		}
		return nil, fmt.Errorf("EKS DescribeCluster failed: %w", err)
	}

	if output.Cluster == nil || output.Cluster.Arn == nil {
		return nil, fmt.Errorf("EKS DescribeCluster returned nil cluster or ARN")
	}

	// Parse the authoritative ARN
	identity := ParseClusterARN(aws.ToString(output.Cluster.Arn))
	if identity == nil {
		return nil, fmt.Errorf("failed to parse cluster ARN from DescribeCluster: %s",
			aws.ToString(output.Cluster.Arn))
	}

	// Validate the response matches our request (defense in depth)
	if identity.ClusterName != clusterName {
		return nil, fmt.Errorf("%w: requested %s, got %s",
			ErrNameMismatch, clusterName, identity.ClusterName)
	}

	// Cache successful result
	cache.Cache.Set(cacheKey, identity, cache.NoExpiration)
	log.Infof("EKS cluster identity resolved via DescribeCluster: %s (account=%s, region=%s)",
		identity.ClusterARN, identity.AccountID, identity.Region)

	return identity, nil
}

// ParseClusterARN extracts components from an EKS cluster ARN.
// Returns nil if the ARN is malformed, not an EKS cluster ARN, or uses
// an unknown partition.
//
// Expected format: arn:<partition>:eks:<region>:<account>:cluster/<name>
// Supported partitions: aws, aws-cn, aws-us-gov
func ParseClusterARN(arn string) *ClusterIdentity {
	if arn == "" {
		return nil
	}

	// Expected format: arn:aws:eks:region:account:cluster/name
	parts := strings.Split(arn, ":")
	if len(parts) != 6 {
		return nil
	}

	if parts[0] != "arn" {
		return nil
	}

	partition := parts[1]
	if !allowedPartitions[partition] {
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

	// Validate extracted components
	if !validAccountIDPattern.MatchString(accountID) {
		return nil
	}
	if !validRegionPattern.MatchString(region) {
		return nil
	}

	return &ClusterIdentity{
		ClusterName: clusterName,
		ClusterARN:  arn,
		AccountID:   accountID,
		Region:      region,
	}
}

// IsValidClusterName checks if a cluster name matches EKS naming rules:
// - 1-100 characters
// - Alphanumeric, hyphens, underscores only
// - Must start with alphanumeric character
func IsValidClusterName(name string) bool {
	if name == "" || len(name) > MaxClusterNameLength {
		return false
	}
	return validClusterNamePattern.MatchString(name)
}
