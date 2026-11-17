// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eks resolves authoritative Amazon EKS cluster identity through eks:DescribeCluster.
// The identity data type and its tag projection live in the SDK-free sub-package
// pkg/util/eks/identity so that node-Agent packages which only need the type do not link the
// AWS SDK; this package loads the AWS configuration and is reached only from the Cluster Agent.
// DescribeCluster is called through a minimal SigV4-signed client (describe_cluster.go) rather
// than the generated EKS service client, to keep the Cluster Agent binary small.
package eks

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
)

const (
	// identityTimeout bounds the DescribeCluster call. It is kept below the Cluster Agent
	// client's 3s ResponseHeaderTimeout so a cold resolve completes (and caches) within the
	// node Agent's single request attempt rather than surfacing as a client timeout.
	identityTimeout = 2 * time.Second
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
)

// ClusterIdentity is the authoritative identity returned by EKS DescribeCluster. It is defined in
// the SDK-free identity sub-package and re-exported here for the resolver's callers.
type ClusterIdentity = identity.ClusterIdentity

var newDescribeClusterAPI = newSignedDescribeClusterClient

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
	if id, ok, err := lookupCache(cacheKey); ok {
		return id, err
	}

	// Coalesce concurrent misses: after a Cluster Agent restart or a negative-TTL expiry every
	// node Agent polls at once, and without this each would issue its own DescribeCluster and
	// trip EKS throttling. singleflight collapses them into one resolve per cluster name; the
	// shared result is then served from the cache by all waiters.
	v, err, _ := resolveGroup.Do(clusterName, func() (interface{}, error) {
		// Re-check the cache inside the flight: the winner of a prior flight may have just
		// populated it while this goroutine was waiting to enter.
		if id, ok, err := lookupCache(cacheKey); ok {
			return id, err
		}
		return resolveAndCache(ctx, clusterName, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	id := v.(*ClusterIdentity)
	// Return a copy so no caller aliases the value shared by the cache and the other waiters.
	out := *id
	return &out, nil
}

// resolveGroup coalesces concurrent GetClusterIdentity misses by cluster name.
var resolveGroup singleflight.Group

// lookupCache returns a cached identity (as a fresh copy) or cached error. ok is false on a miss.
func lookupCache(cacheKey string) (*ClusterIdentity, bool, error) {
	cached, found := cache.Cache.Get(cacheKey)
	if !found {
		return nil, false, nil
	}
	switch v := cached.(type) {
	case *ClusterIdentity:
		out := *v
		return &out, true, nil
	case error:
		return nil, true, v
	}
	return nil, false, nil
}

// resolveAndCache performs the DescribeCluster resolve and populates the cache. The returned
// pointer is the cached value; callers copy before handing it out.
func resolveAndCache(ctx context.Context, clusterName, cacheKey string) (*ClusterIdentity, error) {
	resolveCtx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()

	cfg, err := loadAWSConfig(resolveCtx)
	if err != nil {
		err = fmt.Errorf("load AWS configuration: %w", err)
		cache.Cache.Set(cacheKey, err, failureTTL(err))
		return nil, err
	}

	id, err := resolveClusterIdentity(resolveCtx, clusterName, cfg.Region, newDescribeClusterAPI(cfg))
	if err != nil {
		// Negative cache: node Agents query the Cluster Agent repeatedly at startup, and a
		// missing eks:DescribeCluster permission must not turn into one API call per request.
		// Transient failures use a short TTL so a blip does not strip the tag for 5 minutes.
		cache.Cache.Set(cacheKey, err, failureTTL(err))
		return nil, err
	}
	cache.Cache.Set(cacheKey, id, cache.NoExpiration)
	return id, nil
}

// failureTTL classifies a resolution error so that failures which can clear on their own are
// retried soon, while failures that are stable for the cluster's configuration are cached longer.
// A context deadline/cancellation, HTTP 429 and expired credentials (the common IRSA
// projected-token staleness) are transient, and the AWS SDK's retry classifier decides the rest:
// it treats throttling codes, 5xx, timeouts, connection resets and DNS/TLS failures as
// retryable, so those use the short TTL.
// Everything the retryer considers terminal (AccessDenied, ResourceNotFound, malformed identity,
// invalid name) uses the long TTL.
func failureTTL(err error) time.Duration {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return transientFailureCacheTTL
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		// HTTP 429 is throttling whatever error code accompanies it.
		if apiErr.StatusCode == http.StatusTooManyRequests {
			return transientFailureCacheTTL
		}
		// The SDK retryer does not classify expired credentials as retryable, but they clear
		// once the credential provider refreshes (for example a rotated IRSA token).
		switch apiErr.Code {
		case "ExpiredToken", "ExpiredTokenException", "RequestExpired":
			return transientFailureCacheTTL
		}
	}
	if retryableClassifier.IsErrorRetryable(err) == aws.TrueTernary {
		return transientFailureCacheTTL
	}
	return failureCacheTTL
}

// retryableClassifier reuses the AWS SDK's standard retryable-error set so that throttling, 5xx,
// timeouts and connection resets are all treated as transient without hand-maintaining a code
// list. apiError exposes ErrorCode and HTTPStatusCode, which is what the classifier inspects.
var retryableClassifier = retry.IsErrorRetryables(retry.DefaultRetryables)

func resolveClusterIdentity(ctx context.Context, clusterName, requestRegion string, client describeClusterAPI) (*ClusterIdentity, error) {
	output, err := client.DescribeCluster(ctx, clusterName)
	if err != nil {
		return nil, fmt.Errorf("describe EKS cluster %q: %w", clusterName, err)
	}
	if output == nil || output.Cluster == nil || output.Cluster.Arn == nil {
		return nil, fmt.Errorf("%w: DescribeCluster returned no ARN", ErrInvalidClusterIdentity)
	}
	if output.Cluster.Name != nil && aws.ToString(output.Cluster.Name) != clusterName {
		return nil, fmt.Errorf("%w: requested cluster %q but received %q", ErrInvalidClusterIdentity, clusterName, aws.ToString(output.Cluster.Name))
	}

	// ParseClusterARN checks the service, partition, Region and account fields and that the
	// resource is cluster/<name>; the same check runs again on the node side.
	parsed, ok := identity.ParseClusterARN(aws.ToString(output.Cluster.Arn))
	if !ok {
		return nil, fmt.Errorf("%w: unexpected EKS ARN %q", ErrInvalidClusterIdentity, aws.ToString(output.Cluster.Arn))
	}
	if parsed.ClusterName != clusterName {
		return nil, fmt.Errorf("%w: ARN resource does not match cluster %q", ErrInvalidClusterIdentity, clusterName)
	}
	if requestRegion != "" && parsed.Region != requestRegion {
		return nil, fmt.Errorf("%w: requested Region %q but received %q", ErrInvalidClusterIdentity, requestRegion, parsed.Region)
	}
	return &parsed, nil
}
