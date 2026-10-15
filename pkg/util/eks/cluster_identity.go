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
	smithy "github.com/aws/smithy-go"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
)

const (
	identityTimeout = 5 * time.Second
	// failureCacheTTL bounds how often a permanent failure (AccessDenied, ResourceNotFound,
	// malformed ARN) is retried against the EKS API. These do not resolve on their own, so a
	// node Agent must not turn them into one API call per request.
	failureCacheTTL = 5 * time.Minute
	// transientFailureCacheTTL is a short TTL for failures that can clear on their own
	// (throttling, timeouts, credential refresh). Caching these as hard as permanent ones
	// would strip the identity tag from the whole fleet for the full failureCacheTTL after a
	// single blip, so they are retried much sooner.
	transientFailureCacheTTL = 30 * time.Second
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
//
// Only eks_cluster_arn is emitted. The ARN already encodes the partition, Region and owning
// account, so emitting separate aws_account/region tags would both duplicate that payload on
// every span and collide with the node-derived aws_account/region host tags: on a cross-account
// node group the node's account and the cluster's account differ, leaving two values under one
// key on the same entity. Consumers that need the account or Region parse them out of the ARN.
func (c *ClusterIdentity) Tags() []string {
	if c == nil || !isValidClusterARN(c.ClusterARN) {
		return nil
	}
	return []string{
		taggertags.EksClusterARN + ":" + c.ClusterARN,
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
			// Return a copy so a caller cannot mutate the value shared by the process-global cache.
			out := *v
			return &out, nil
		case error:
			return nil, v
		}
	}

	resolveCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()

	cfg, err := loadAWSConfig(resolveCtx)
	if err != nil {
		err = fmt.Errorf("load AWS configuration: %w", err)
		cache.Cache.Set(cacheKey, err, failureTTL(err))
		return nil, err
	}

	identity, err := resolveClusterIdentity(resolveCtx, clusterName, cfg.Region, newDescribeClusterAPI(cfg))
	if err != nil {
		// Negative cache: node Agents query the Cluster Agent repeatedly at startup, and a
		// missing eks:DescribeCluster permission must not turn into one API call per request.
		// Transient failures use a short TTL so a blip does not strip the tag for 5 minutes.
		cache.Cache.Set(cacheKey, err, failureTTL(err))
		return nil, err
	}
	cache.Cache.Set(cacheKey, identity, cache.NoExpiration)
	return identity, nil
}

// failureTTL classifies a resolution error so that failures which can clear on their own are
// retried soon, while failures that are stable for the cluster's configuration are cached longer.
// A context deadline/cancellation, an AWS throttling error, or any error carrying a 5xx HTTP
// status is treated as transient; everything else (AccessDenied, ResourceNotFound, malformed
// identity, invalid name) is treated as permanent.
func failureTTL(err error) time.Duration {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return transientFailureCacheTTL
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorFault() == smithy.FaultServer {
			return transientFailureCacheTTL
		}
		switch apiErr.ErrorCode() {
		case "ThrottlingException", "Throttling", "ThrottlingError", "RequestLimitExceeded",
			"TooManyRequestsException", "ServiceUnavailable", "ServiceUnavailableException",
			"InternalServerErrorException", "InternalFailure":
			return transientFailureCacheTTL
		}
	}
	var respErr interface{ HTTPStatusCode() int }
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() >= 500 {
		return transientFailureCacheTTL
	}
	return failureCacheTTL
}

// isValidClusterARN reports whether arn is a well-formed EKS cluster ARN. It is used on both
// sides of the Cluster Agent boundary: the resolver validates DescribeCluster output, and the
// node Agent re-validates the ARN it receives over HTTP before turning it into a tag, so a
// version-skewed or misbehaving Cluster Agent cannot inject arbitrary tag values.
func isValidClusterARN(clusterARN string) bool {
	if clusterARN == "" {
		return false
	}
	parsed, err := awsarn.Parse(clusterARN)
	if err != nil {
		return false
	}
	if parsed.Service != "eks" || parsed.Region == "" || !accountIDPattern.MatchString(parsed.AccountID) {
		return false
	}
	const clusterPrefix = "cluster/"
	return strings.HasPrefix(parsed.Resource, clusterPrefix) &&
		clusterNamePattern.MatchString(strings.TrimPrefix(parsed.Resource, clusterPrefix))
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
