// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eks resolves authoritative Amazon EKS cluster identity.
package eks

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsarn "github.com/aws/aws-sdk-go-v2/aws/arn"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awseks "github.com/aws/aws-sdk-go-v2/service/eks"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

const (
	identityTimeout = 5 * time.Second
	// failureCacheTTL bounds how often a failing resolution is retried against the EKS API.
	failureCacheTTL = 5 * time.Minute
)

var (
	// ErrInvalidClusterName indicates that the supplied cluster name cannot be an EKS cluster name.
	ErrInvalidClusterName = errors.New("invalid EKS cluster name")
	// ErrInvalidClusterIdentity indicates that DescribeCluster returned an unusable identity.
	ErrInvalidClusterIdentity = errors.New("invalid EKS cluster identity")

	clusterNamePattern = regexp.MustCompile(`^[0-9A-Za-z][A-Za-z0-9_-]{0,99}$`)
	accountIDPattern   = regexp.MustCompile(`^[0-9]{12}$`)
)

// ClusterIdentity is the authoritative identity returned by EKS DescribeCluster.
type ClusterIdentity struct {
	ClusterName string `json:"cluster_name"`
	ClusterARN  string `json:"cluster_arn"`
	AccountID   string `json:"aws_account_id"`
	Region      string `json:"region"`
}

// Tags returns the identity as low-cardinality tags. It returns nil for an incomplete identity.
func (c *ClusterIdentity) Tags() []string {
	if c == nil || c.ClusterARN == "" || c.AccountID == "" || c.Region == "" {
		return nil
	}
	return []string{
		taggertags.EksClusterARN + ":" + c.ClusterARN,
		taggertags.AwsAccount + ":" + c.AccountID,
		taggertags.Region + ":" + c.Region,
	}
}

type describeClusterAPI interface {
	DescribeCluster(context.Context, *awseks.DescribeClusterInput, ...func(*awseks.Options)) (*awseks.DescribeClusterOutput, error)
}

var newDescribeClusterAPI = func(cfg aws.Config) describeClusterAPI {
	return awseks.NewFromConfig(cfg)
}

var loadAWSConfig = func(ctx context.Context) (aws.Config, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		return aws.Config{}, err
	}
	if cfg.Region == "" {
		cfg.Region, err = ec2.GetRegion(ctx)
		if err != nil {
			return aws.Config{}, fmt.Errorf("resolve EKS region: %w", err)
		}
	}
	return cfg, nil
}

// GetClusterIdentity resolves a cluster ARN through EKS DescribeCluster and caches successful results.
// Node account metadata is never used as cluster ownership evidence.
func GetClusterIdentity(ctx context.Context, clusterName string) (*ClusterIdentity, error) {
	if !clusterNamePattern.MatchString(clusterName) {
		return nil, ErrInvalidClusterName
	}

	cacheKey := cache.BuildAgentKey("eks", "cluster-identity", clusterName)
	if cached, found := cache.Cache.Get(cacheKey); found {
		switch v := cached.(type) {
		case *ClusterIdentity:
			return v, nil
		case error:
			return nil, v
		}
	}

	resolveCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()

	cfg, err := loadAWSConfig(resolveCtx)
	if err != nil {
		err = fmt.Errorf("load AWS configuration: %w", err)
		cache.Cache.Set(cacheKey, err, failureCacheTTL)
		return nil, err
	}

	identity, err := resolveClusterIdentity(resolveCtx, clusterName, cfg.Region, newDescribeClusterAPI(cfg))
	if err != nil {
		// Negative cache: node Agents query the Cluster Agent repeatedly at startup, and a
		// missing eks:DescribeCluster permission must not turn into one API call per request.
		cache.Cache.Set(cacheKey, err, failureCacheTTL)
		return nil, err
	}
	cache.Cache.Set(cacheKey, identity, cache.NoExpiration)
	return identity, nil
}

func resolveClusterIdentity(ctx context.Context, clusterName, requestRegion string, client describeClusterAPI) (*ClusterIdentity, error) {
	output, err := client.DescribeCluster(ctx, &awseks.DescribeClusterInput{Name: aws.String(clusterName)})
	if err != nil {
		return nil, fmt.Errorf("describe EKS cluster %q: %w", clusterName, err)
	}
	if output == nil || output.Cluster == nil || output.Cluster.Arn == nil {
		return nil, fmt.Errorf("%w: DescribeCluster returned no ARN", ErrInvalidClusterIdentity)
	}
	if output.Cluster.Name != nil && aws.ToString(output.Cluster.Name) != clusterName {
		return nil, fmt.Errorf("%w: requested cluster %q but received %q", ErrInvalidClusterIdentity, clusterName, aws.ToString(output.Cluster.Name))
	}

	parsed, err := awsarn.Parse(aws.ToString(output.Cluster.Arn))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidClusterIdentity, err)
	}
	if parsed.Service != "eks" || parsed.Region == "" || !accountIDPattern.MatchString(parsed.AccountID) {
		return nil, fmt.Errorf("%w: unexpected EKS ARN fields", ErrInvalidClusterIdentity)
	}
	if requestRegion != "" && parsed.Region != requestRegion {
		return nil, fmt.Errorf("%w: requested Region %q but received %q", ErrInvalidClusterIdentity, requestRegion, parsed.Region)
	}

	const clusterPrefix = "cluster/"
	if !strings.HasPrefix(parsed.Resource, clusterPrefix) || strings.TrimPrefix(parsed.Resource, clusterPrefix) != clusterName {
		return nil, fmt.Errorf("%w: ARN resource does not match cluster %q", ErrInvalidClusterIdentity, clusterName)
	}

	return &ClusterIdentity{
		ClusterName: clusterName,
		ClusterARN:  parsed.String(),
		AccountID:   parsed.AccountID,
		Region:      parsed.Region,
	}, nil
}
