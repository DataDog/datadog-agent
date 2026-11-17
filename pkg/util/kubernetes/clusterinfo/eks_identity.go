// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/cenkalti/backoff/v7"
)

var errIncompleteIdentity = errors.New("cluster agent returned incomplete EKS identity")

// StartupRetryBudget bounds how long startup waits for the Cluster Agent to answer.
const StartupRetryBudget = 10 * time.Second

// Startup budgets. Variables only so that tests can shorten them.
var (
	startupRetryBudget = StartupRetryBudget
	// attemptTimeout bounds one Cluster Agent call that callers share (see sharedCall).
	attemptTimeout = 10 * time.Second
)

// startupFetch is the one background fetch of the identity in this process, started by the first
// GetClusterAgentEKSIdentityTagsWithRetry call.
var startupFetch struct {
	mu sync.Mutex
	f  *identityFetch
}

// identityFetch is one fetch of the identity, bounded to startupRetryBudget and retried within
// it. It does not belong to the caller that started it: it runs on its own context, so a caller
// that leaves early neither fails nor shortens it. Its result is final for the process.
type identityFetch struct {
	ctx  context.Context // ends when the budget is spent or the fetch is over
	done chan struct{}   // closed once tags and err are set
	tags []string
	err  error
}

var getEKSClusterIdentity = func(ctx context.Context) (*identity.ClusterIdentity, error) {
	client, err := clusteragent.GetClusterAgentClient()
	if err != nil {
		return nil, err
	}
	return client.GetEKSClusterIdentity(ctx)
}

// serviceAccountTokenPath is where Kubernetes mounts the pod's service account token, on Linux
// and Windows nodes alike. It is the path rest.InClusterConfig reads.
// #nosec G101 -- a path, not a credential
const serviceAccountTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token" //nolint:gosec // G101, see above

// eksIssuerPattern matches the service account issuer EKS sets for every cluster, its OIDC
// provider URL, for example https://oidc.eks.us-west-2.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE.
var eksIssuerPattern = regexp.MustCompile(`^https://oidc\.eks\.[a-z0-9-]+\.[a-z0-9.-]+/id/[A-Za-z0-9]+$`)

// hasEKSEvidence reports whether this node or sidecar positively runs on EKS, from local data
// only. It makes no network call, so it costs nothing at startup on other clusters. Variable for
// tests.
var hasEKSEvidence = func() bool {
	// An EKS Fargate sidecar is on EKS by definition.
	if env.IsFeaturePresent(env.EKSFargate) {
		return true
	}
	// The kube distribution, if the host tags have already resolved it through the Cluster
	// Agent. Only the cache is read here.
	if distribution, found := cache.Cache.Get(cache.BuildAgentKey(constants.NodeKubeDistributionKey)); found && distribution == "eks" {
		return true
	}
	return serviceAccountTokenIsEKS()
}

// serviceAccountTokenIsEKS reads the issuer of the pod's service account token once. EKS issues
// every pod's token from the cluster's OIDC provider, on EC2, Windows, Fargate and Hybrid nodes,
// with ec2_use_dmi set or not, while kops, kind, self-managed clusters, AKS and GKE use other
// issuers. The token is not verified: it only decides whether startup waits for the Cluster
// Agent, and the identity itself comes from the Cluster Agent.
var serviceAccountTokenIsEKS = sync.OnceValue(func() bool {
	return isEKSServiceAccountToken(serviceAccountTokenPath)
})

func isEKSServiceAccountToken(path string) bool {
	// #nosec G304 -- only called with serviceAccountTokenPath (or a test path)
	token, err := os.ReadFile(path) //nolint:gosec // G304, see above
	if err != nil {
		return false
	}
	parts := strings.Split(strings.TrimSpace(string(token)), ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims struct {
		Issuer string `json:"iss"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	return eksIssuerPattern.MatchString(claims.Issuer)
}

// HasEKSEvidence reports whether local data, with no network call, shows that this node or
// sidecar runs on EKS: an EKS Fargate sidecar, a kube distribution already resolved as eks, or a
// service account token issued by an EKS cluster. Startup waits for the Cluster Agent's EKS
// identity only when it does.
func HasEKSEvidence() bool {
	return hasEKSEvidence()
}

// GetClusterAgentEKSIdentityHostTags is GetClusterAgentEKSIdentityTags for the host-tags
// provider. It refreshes on its own interval, so answers that will not change are an empty
// success rather than an error that is logged on every refresh: a node without EKS evidence,
// a 404 from a Cluster Agent that is not on EKS or too old to serve identity, and a final 503
// (for example a missing eks:DescribeCluster permission, which the Cluster Agent logs itself).
func GetClusterAgentEKSIdentityHostTags(ctx context.Context) ([]string, error) {
	if !hasEKSEvidence() {
		return nil, nil
	}
	tags, err := GetClusterAgentEKSIdentityTags(ctx)
	var statusErr *clusteragent.StatusError
	if errors.As(err, &statusErr) && !statusErr.RetryLater {
		return nil, nil
	}
	return tags, err
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

// GetClusterAgentEKSIdentityTagsWithRetry returns the EKS identity tags from a single background
// fetch per process. Callers check HasEKSEvidence first. The first call starts the fetch and
// every call, concurrent or later, waits for it on its own ctx. The static-tags paths compute
// tags exactly once at Agent startup, where the node Agent routinely becomes ready before the
// Cluster Agent is reachable; without a retry a single cold-start miss would leave
// eks_cluster_arn absent from container tags and APM spans.
//
// The fetch is bounded to StartupRetryBudget and retries with a short constant backoff within
// it. Only failures to get an answer are retried: once the Cluster Agent answers (404 on non-EKS
// clusters or an older Cluster Agent, 503 when DescribeCluster fails, or an identity without a
// valid ARN), the answer will not change within the window, and retrying it would block every
// caller for the full budget on each startup. The one exception is an answer the Cluster Agent
// marks with identity.RetryLaterHeader: it has not determined the identity yet. What the fetch
// ends with, tags or error, is final for the process. The host-tags provider re-runs on its own
// refresh interval and repairs a later miss, with a single attempt per refresh.
//
// The fetch uses no caller's ctx, so a caller that leaves early neither fails nor shortens it,
// and the budget is spent once however many startup components (the tagger, DogStatsD, OTLP)
// ask. A caller whose ctx ends first gets ctx.Err(). Callers also stop waiting when the budget
// is spent, so the bound holds even though the Cluster Agent client's initialization takes no
// ctx.
func GetClusterAgentEKSIdentityTagsWithRetry(ctx context.Context) ([]string, error) {
	f := startIdentityFetch()
	select {
	case <-f.done:
	case <-f.ctx.Done(): // the budget is spent, though an attempt may still hang in the client
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case <-f.done:
		return slices.Clone(f.tags), f.err
	default:
		return nil, f.ctx.Err()
	}
}

// startIdentityFetch starts the process's fetch on the first call and returns it.
func startIdentityFetch() *identityFetch {
	startupFetch.mu.Lock()
	defer startupFetch.mu.Unlock()
	if startupFetch.f == nil {
		ctx, cancel := context.WithTimeout(context.Background(), startupRetryBudget)
		f := &identityFetch{ctx: ctx, done: make(chan struct{})}
		startupFetch.f = f
		go func() {
			defer cancel()
			f.tags, f.err = backoff.Retry(ctx, func() ([]string, error) {
				tags, err := GetClusterAgentEKSIdentityTags(ctx)
				var statusErr *clusteragent.StatusError
				if (errors.As(err, &statusErr) && !statusErr.RetryLater) || errors.Is(err, errIncompleteIdentity) {
					return nil, backoff.Permanent(err)
				}
				return tags, err
			}, backoff.WithBackOff(backoff.NewConstantBackOff(1*time.Second)))
			close(f.done)
		}()
	}
	return startupFetch.f
}

// sharedCall runs one call at a time in its own goroutine and lets every caller wait for it on
// its own ctx. The Cluster Agent client does not honor a ctx everywhere (its initialization, and
// several queries, use context.TODO), so this is what makes the callers' budgets real: a caller
// returns when its ctx is done, and the call goes on in the background, bounded by
// attemptTimeout and the client's own timeouts, without a new one piling up behind it.
type sharedCall[T any] struct {
	mu   sync.Mutex
	call *callResult[T]
}

type callResult[T any] struct {
	done chan struct{}
	val  T
	err  error
}

// Do waits for the call in flight, or starts fn when there is none.
func (s *sharedCall[T]) Do(ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	var zero T
	// A caller that is already done starts nothing.
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	s.mu.Lock()
	c := s.call
	if c == nil {
		c = &callResult[T]{done: make(chan struct{})}
		s.call = c
		// #nosec G118 -- detached on purpose: callers select on their own ctx, see sharedCall
		go func() { //nolint:gosec // G118, see above
			callCtx, cancel := context.WithTimeout(context.Background(), attemptTimeout)
			defer cancel()
			c.val, c.err = fn(callCtx)
			s.mu.Lock()
			if s.call == c {
				s.call = nil
			}
			s.mu.Unlock()
			close(c.done)
		}()
	}
	s.mu.Unlock()

	select {
	case <-c.done:
		return c.val, c.err
	case <-ctx.Done():
		// The call may have ended at the same time; its answer is better than ctx.Err().
		select {
		case <-c.done:
			return c.val, c.err
		default:
			return zero, ctx.Err()
		}
	}
}

// reset forgets the call in flight, so the next Do starts a new one.
func (s *sharedCall[T]) reset() {
	s.mu.Lock()
	s.call = nil
	s.mu.Unlock()
}
