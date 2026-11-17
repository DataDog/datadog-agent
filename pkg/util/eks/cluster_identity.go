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
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"

	"golang.org/x/sync/singleflight"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/ec2"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// identityTimeout bounds one resolve (AWS configuration and DescribeCluster). It is kept
	// below the Cluster Agent client's 3s ResponseHeaderTimeout so a cold resolve usually
	// completes within the node Agent's request. The resolve does not depend on that request, so
	// when it takes longer the result is still cached for the node's next attempt.
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
	// ErrClusterMismatch indicates that the cluster DescribeCluster answered for is not the
	// cluster this pod runs in.
	ErrClusterMismatch = errors.New("EKS cluster does not match the running cluster")
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
// Node account metadata is never used as cluster ownership evidence. When clusterCA is set, the
// cluster DescribeCluster answers for must have that certificate authority (see verifyClusterCA).
func GetClusterIdentity(ctx context.Context, clusterName string, clusterCA []byte) (*ClusterIdentity, error) {
	if !identity.IsValidClusterName(clusterName) {
		return nil, ErrInvalidClusterName
	}

	// The CA is part of the key: a mismatch cached for one CA must not answer for another.
	flightKey, cacheKey := identityKeys(clusterName, clusterCA)
	if id, ok, err := lookupCache(cacheKey); ok {
		return id, err
	}

	// Coalesce concurrent misses: after a Cluster Agent restart or a negative-TTL expiry every
	// node Agent polls at once, and without this each would issue its own DescribeCluster and
	// trip EKS throttling. singleflight collapses them into one resolve per cluster name; the
	// shared result is then served from the cache by all waiters.
	//
	// The resolve runs on a context detached from the caller that started it: that caller is
	// one node Agent's HTTP request, and its disconnect (restart, its own response timeout)
	// must not fail the resolve for every other waiter. Each waiter still honors its own ctx.
	results := resolveGroup.DoChan(flightKey, func() (v interface{}, err error) {
		defer func() {
			// DoChan re-panics in a new goroutine, which would crash the Cluster Agent.
			if r := recover(); r != nil {
				log.Errorf("EKS DescribeCluster resolution panicked: %v", r)
				v, err = nil, fmt.Errorf("EKS DescribeCluster resolution panicked: %v", r)
			}
		}()
		// Re-check the cache inside the flight: the winner of a prior flight may have just
		// populated it while this goroutine was waiting to enter.
		if id, ok, err := lookupCache(cacheKey); ok {
			return id, err
		}
		resolveCtx, cancel := detachedContext(ctx, identityTimeout)
		defer cancel()
		return resolveAndCache(resolveCtx, clusterName, clusterCA, cacheKey)
	})

	var res singleflight.Result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res = <-results:
	}
	if res.Err != nil {
		return nil, res.Err
	}
	id := res.Val.(*ClusterIdentity)
	// Return a copy so no caller aliases the value shared by the cache and the other waiters.
	out := *id
	return &out, nil
}

// detachedContext returns a context that is not cancelled with ctx, so one caller leaving does
// not fail the shared resolve, but that keeps ctx's deadline when it is earlier than timeout:
// a caller with an overall budget (eksidentity.Resolve) bounds the whole resolve.
func detachedContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	return context.WithDeadline(context.WithoutCancel(ctx), deadline)
}

// identityKeys returns the singleflight and cache keys for a cluster name and expected CA.
func identityKeys(clusterName string, clusterCA []byte) (flightKey, cacheKey string) {
	caSum := sha256.Sum256(clusterCA)
	flightKey = clusterName + "/" + hex.EncodeToString(caSum[:8])
	return flightKey, cache.BuildAgentKey("eks", "cluster-identity", flightKey)
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
// ctx is the resolve's own context, already bounded by identityTimeout.
func resolveAndCache(ctx context.Context, clusterName string, clusterCA []byte, cacheKey string) (*ClusterIdentity, error) {
	client, region, err := getDescribeClient(ctx)
	if err != nil {
		cacheFailure(cacheKey, err)
		return nil, err
	}

	id, err := resolveClusterIdentity(ctx, clusterName, region, clusterCA, client)
	if err != nil {
		cacheFailure(cacheKey, err)
		return nil, err
	}
	cache.Cache.Set(cacheKey, id, cache.NoExpiration)
	return id, nil
}

// describeClient holds the DescribeCluster client built from the first successful AWS
// configuration load. Loading the configuration builds a new credentials cache, so doing it on
// every miss would fetch new credentials (an STS call with IRSA) each time. The credentials
// cache refreshes itself, and the client keeps the clock skew it learned. A failed load is not
// kept, so the next miss tries again.
//
// The mutex is held while the configuration loads. That is bounded by the resolve's context, and
// callers rarely contend: eksidentity.Resolve already runs one resolve at a time, and a waiter
// here would only load the same configuration again.
var describeClient struct {
	sync.Mutex
	client describeClusterAPI
	region string
}

func getDescribeClient(ctx context.Context) (describeClusterAPI, string, error) {
	describeClient.Lock()
	defer describeClient.Unlock()
	if describeClient.client != nil {
		return describeClient.client, describeClient.region, nil
	}
	cfg, err := loadAWSConfig(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("load AWS configuration: %w", err)
	}
	// A FIPS pseudo Region (us-east-1-fips) names the real Region the ARN must carry.
	cfg = normalizeFIPSRegion(cfg)
	describeClient.client, describeClient.region = newDescribeClusterAPI(cfg), cfg.Region
	return describeClient.client, describeClient.region, nil
}

// cacheFailure negatively caches a resolution failure. Node Agents query the Cluster Agent
// repeatedly at startup, and a missing eks:DescribeCluster permission must not turn into one API
// call per request. Transient failures use a short TTL so a blip does not strip the tag for 5
// minutes. A cancellation never comes from the resolve's own budget, so it says nothing about
// EKS and is not cached.
func cacheFailure(cacheKey string, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	ttl := failureTTL(err)
	// Logged here, once per resolution, rather than on every node request served from the
	// cache. At Warn, since a missing eks:DescribeCluster permission is otherwise invisible.
	log.Warnf("Unable to resolve the EKS cluster identity, retrying in %s: %v", ttl, err)
	cache.Cache.Set(cacheKey, err, ttl)
}

// failureTTL classifies a resolution error so that failures which can clear on their own are
// retried soon, while failures that are stable for the cluster's configuration are cached longer.
// The resolve's own deadline, HTTP 429 and the codes in transientErrorCodes are transient, and
// the AWS SDK's retry classifier decides the rest: it treats throttling codes, 5xx, timeouts,
// connection resets and DNS/TLS failures as retryable, so those use the short TTL. Everything
// the retryer considers terminal (AccessDenied, ResourceNotFound, a malformed identity, an
// invalid name, a cluster mismatch) uses the long TTL.
func failureTTL(err error) time.Duration {
	if errors.Is(err, context.DeadlineExceeded) {
		return transientFailureCacheTTL
	}
	// HTTP 429 is throttling whatever error code accompanies it.
	var withStatus interface{ HTTPStatusCode() int }
	if errors.As(err, &withStatus) && withStatus.HTTPStatusCode() == http.StatusTooManyRequests {
		return transientFailureCacheTTL
	}
	// Match the code through the interface rather than *apiError: errors from
	// Credentials.Retrieve (STS AssumeRoleWithWebIdentity for IRSA) are wrapped SDK errors.
	var withCode interface{ ErrorCode() string }
	if errors.As(err, &withCode) && transientErrorCodes[withCode.ErrorCode()] {
		return transientFailureCacheTTL
	}
	if retryableClassifier.IsErrorRetryable(err) == aws.TrueTernary {
		return transientFailureCacheTTL
	}
	return failureCacheTTL
}

// transientErrorCodes are error codes the SDK retryer does not classify as retryable but that
// clear on their own: expired or not yet valid credentials (a rotated IRSA token), a temporarily
// unreachable OIDC identity provider, and signatures rejected for the signing time (a clock
// that is being corrected).
var transientErrorCodes = map[string]bool{
	"ExpiredToken":              true,
	"ExpiredTokenException":     true,
	"RequestExpired":            true,
	"InvalidIdentityToken":      true,
	"IDPCommunicationError":     true,
	"InvalidSignatureException": true,
	"SignatureDoesNotMatch":     true,
	"RequestTimeTooSkewed":      true,
	"RequestInTheFuture":        true,
}

// retryableClassifier reuses the AWS SDK's standard retryable-error set so that throttling, 5xx,
// timeouts and connection resets are all treated as transient without hand-maintaining a code
// list. apiError exposes ErrorCode and HTTPStatusCode, which is what the classifier inspects.
var retryableClassifier = retry.IsErrorRetryables(retry.DefaultRetryables)

func resolveClusterIdentity(ctx context.Context, clusterName, requestRegion string, clusterCA []byte, client describeClusterAPI) (*ClusterIdentity, error) {
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
	if clusterCA != nil {
		if err := verifyClusterCA(output.Cluster, clusterCA); err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}

// verifyClusterCA checks that the cluster DescribeCluster answered for is the cluster this pod
// runs in. Every pod gets the cluster's CA at
// /var/run/secrets/kubernetes.io/serviceaccount/ca.crt, on EC2 nodes and on Fargate alike, and
// EKS creates a new CA for every cluster. So when the name is stale or reused (a blue/green
// pair, a recreated cluster) the ARN still parses, but the CA does not match.
func verifyClusterCA(cluster *describedCluster, clusterCA []byte) error {
	if cluster.CertificateAuthority == nil || cluster.CertificateAuthority.Data == nil {
		return fmt.Errorf("%w: DescribeCluster returned no certificate authority", ErrClusterMismatch)
	}
	eksPEM, err := base64.StdEncoding.DecodeString(aws.ToString(cluster.CertificateAuthority.Data))
	if err != nil {
		return fmt.Errorf("%w: decode the cluster certificate authority: %v", ErrClusterMismatch, err)
	}
	local := certificates(clusterCA)
	for der := range certificates(eksPEM) {
		if local[der] {
			return nil
		}
	}
	return fmt.Errorf("%w: the cluster certificate authority does not match this cluster's", ErrClusterMismatch)
}

// certificates returns the DER bytes of every certificate in a PEM bundle, as set keys.
func certificates(bundle []byte) map[string]bool {
	certs := map[string]bool{}
	for {
		var block *pem.Block
		block, bundle = pem.Decode(bundle)
		if block == nil {
			return certs
		}
		if block.Type == "CERTIFICATE" {
			certs[string(block.Bytes)] = true
		}
	}
}
