// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
	identitytypes "github.com/DataDog/datadog-agent/pkg/util/eks/identity"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clusterinfo"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/eksidentity"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func stubEKSIdentityResolver(t *testing.T, identity *eks.ClusterIdentity, err error) {
	t.Helper()
	original := resolveEKSClusterIdentity
	t.Cleanup(func() { resolveEKSClusterIdentity = original })
	resolveEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return identity, err
	}
}

func serveEKSIdentity(t *testing.T) (*httptest.ResponseRecorder, *mocktracer.Span) {
	t.Helper()
	mt := mocktracer.Start()
	defer mt.Stop()

	recorder := httptest.NewRecorder()
	getEKSClusterIdentity(recorder, httptest.NewRequest(http.MethodGet, "/cluster/eks-identity", nil))

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, "cluster_agent.metadata.eks_cluster_identity", spans[0].OperationName())
	return recorder, spans[0]
}

func TestGetEKSClusterIdentity(t *testing.T) {
	stubEKSIdentityResolver(t, &eks.ClusterIdentity{
		ClusterName: "orders",
		ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
		AccountID:   "123456789012",
		Region:      "us-west-2",
	}, nil)

	recorder, span := serveEKSIdentity(t)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	// Only what the node uses: it derives the account and Region from the ARN.
	assert.JSONEq(t, `{"cluster_arn":"arn:aws:eks:us-west-2:123456789012:cluster/orders"}`, recorder.Body.String())
	assert.Nil(t, span.Tag("error.message"))

	// The node's type reads it and emits all three tags.
	var id identitytypes.ClusterIdentity
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &id))
	assert.Len(t, id.Tags(), 3)
}

func TestGetEKSClusterIdentityNotApplicableIsNotAnError(t *testing.T) {
	for _, sentinel := range []error{eksidentity.ErrNotEKS, eksidentity.ErrClusterNameUnavailable, eksidentity.ErrClusterUnverified} {
		t.Run(sentinel.Error(), func(t *testing.T) {
			// Wrapped sentinels must still be recognized.
			stubEKSIdentityResolver(t, nil, fmt.Errorf("resolve: %w", sentinel))

			recorder, span := serveEKSIdentity(t)

			assert.Equal(t, http.StatusNotFound, recorder.Code)
			assert.Nil(t, span.Tag("error.message"), "expected outcome must not mark the span errored")
		})
	}
}

func TestGetEKSClusterIdentityTransientFailure(t *testing.T) {
	stubEKSIdentityResolver(t, nil, errors.New("AccessDeniedException"))

	recorder, span := serveEKSIdentity(t)

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "AccessDeniedException", span.Tag("error.message"))
}

func captureWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(&buf, log.WarnLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "warn")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })
	return &buf
}

func TestGetEKSClusterIdentityDoesNotWarnPerRequest(t *testing.T) {
	// While a failure is cached, every node request is answered from the cache. The failure
	// is logged once where it is resolved, not once per node request.
	stubEKSIdentityResolver(t, nil, errors.New("api error AccessDeniedException"))
	logs := captureWarnings(t)

	for range 3 {
		recorder, _ := serveEKSIdentity(t)
		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	}
	log.Flush()
	assert.NotContains(t, logs.String(), "WARN")
}

func TestGetEKSClusterIdentityNotDeterminedYetIsRetryable(t *testing.T) {
	// The Cluster Agent serves its API before it reaches the API server, so the distribution
	// can be unknown for a while. That is not a final answer, and nodes must try again.
	stubEKSIdentityResolver(t, nil, fmt.Errorf("check distribution: %w", eksidentity.ErrNotDetermined))
	recorder, _ := serveEKSIdentity(t)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "true", recorder.Header().Get(identitytypes.RetryLaterHeader))
}

func TestGetEKSClusterIdentityMarksEveryAnswer(t *testing.T) {
	// Every answer the Cluster Agent produces says whether it is final, so a node can tell it
	// from an unmarked error a proxy or a mesh sidecar returns, which it retries.
	for name, tc := range map[string]struct {
		err    error
		status int
		mark   string
	}{
		"not EKS":        {fmt.Errorf("resolve: %w", eksidentity.ErrNotEKS), http.StatusNotFound, "false"},
		"no name":        {eksidentity.ErrClusterNameUnavailable, http.StatusNotFound, "false"},
		"not determined": {fmt.Errorf("check distribution: %w", eksidentity.ErrNotDetermined), http.StatusServiceUnavailable, "true"},
		// A throttled DescribeCluster is cached for 30s, longer than any node's startup
		// budget, so asking again would only get the same answer. A missing permission is
		// final too.
		"throttled": {fmt.Errorf("describe: %w", throttlingError{}), http.StatusServiceUnavailable, "false"},
		"denied":    {errors.New("api error AccessDeniedException"), http.StatusServiceUnavailable, "false"},
	} {
		stubEKSIdentityResolver(t, nil, tc.err)
		recorder, _ := serveEKSIdentity(t)
		assert.Equal(t, tc.status, recorder.Code, name)
		assert.Equal(t, tc.mark, recorder.Header().Get(identitytypes.RetryLaterHeader), name)
	}
}

func TestGetEKSClusterIdentityCachedFailureAboutToExpireIsRetryable(t *testing.T) {
	// A failure the Cluster Agent asks EKS about again soon can clear within a node's
	// startup budget: mark it retry. One that stays cached longer is final.
	orig := cachedFailureRetryIn
	t.Cleanup(func() { cachedFailureRetryIn = orig })
	stubEKSIdentityResolver(t, nil, fmt.Errorf("describe: %w", throttlingError{}))
	for left, mark := range map[time.Duration]string{
		0:                              "true",
		retryLaterWindow:               "true",
		retryLaterWindow + time.Second: "false",
		25 * time.Second:               "false",
	} {
		cachedFailureRetryIn = func(error) (time.Duration, bool) { return left, true }
		recorder, _ := serveEKSIdentity(t)
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Equal(t, mark, recorder.Header().Get(identitytypes.RetryLaterHeader), "cached for %s more", left)
	}
	assert.Less(t, retryLaterWindow, clusterinfo.StartupRetryBudget)
}

// throttlingError is a throttled DescribeCluster as the AWS retry classifier sees it.
type throttlingError struct{}

func (throttlingError) Error() string       { return "api error ThrottlingException: Rate exceeded" }
func (throttlingError) ErrorCode() string   { return "ThrottlingException" }
func (throttlingError) HTTPStatusCode() int { return http.StatusBadRequest }

// serveRealNode runs the real handler behind a TLS listener, in front of which proxy can answer
// first, and sets up the real node client and startup fetch against it with a 3s budget. It
// returns the count of requests that reached the handler.
func serveRealNode(t *testing.T, proxy func(http.ResponseWriter, *http.Request) bool) *atomic.Int32 {
	t.Helper()
	var requests atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = w.Write([]byte(`{"Major":7,"Minor":60,"Patch":0}`))
		case "/api/v1/cluster/eks-identity":
			if proxy != nil && proxy(w, r) {
				return
			}
			requests.Add(1)
			getEKSClusterIdentity(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	cfg := configmock.New(t)
	cfg.SetInTest("cluster_agent.enabled", true)
	cfg.SetInTest("cluster_agent.url", srv.URL)
	cfg.SetInTest("cluster_agent.auth_token", "01234567890123456789012345678901")
	ipcmock.New(t)
	clusteragent.ResetGlobalClusterAgentClientForTest()
	t.Cleanup(clusteragent.ResetGlobalClusterAgentClientForTest)
	clusterinfo.SetupStartupForTest(t, 3*time.Second, true)
	return &requests
}

func TestGetEKSClusterIdentityNodeStopsOnCachedFailure(t *testing.T) {
	stubEKSIdentityResolver(t, nil, fmt.Errorf("describe: %w", throttlingError{}))
	requests := serveRealNode(t, nil)

	start := time.Now()
	_, err := clusterinfo.GetClusterAgentEKSIdentityTagsWithRetry(context.Background())
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second, "the node must not spend its budget on a cached answer")
	assert.Equal(t, int32(1), requests.Load())
}

func TestGetEKSClusterIdentityNodeRetriesProxyErrors(t *testing.T) {
	// A mesh sidecar or a proxy in front of the Cluster Agent answers 502, 503 or 504 during a
	// rollout, without the Cluster Agent's mark. The node retries within its budget and gets
	// the identity once the Cluster Agent answers.
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusTooManyRequests} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			stubEKSIdentityResolver(t, &eks.ClusterIdentity{ClusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/orders"}, nil)
			var proxied atomic.Int32
			requests := serveRealNode(t, func(w http.ResponseWriter, _ *http.Request) bool {
				if proxied.Add(1) <= 2 {
					http.Error(w, "upstream connect error", status)
					return true
				}
				return false
			})

			tags, err := clusterinfo.GetClusterAgentEKSIdentityTagsWithRetry(context.Background())
			require.NoError(t, err)
			assert.Contains(t, tags, "eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders")
			assert.Equal(t, int32(3), proxied.Load())
			assert.Equal(t, int32(1), requests.Load())
		})
	}
}

func TestGetEKSClusterIdentityNodeStopsOnAuthErrors(t *testing.T) {
	// 401 and 403 come from the node's own token and do not change within seconds, and a 404 is
	// what a Cluster Agent without the route answers: all final, unmarked.
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			stubEKSIdentityResolver(t, &eks.ClusterIdentity{ClusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/orders"}, nil)
			var proxied atomic.Int32
			serveRealNode(t, func(w http.ResponseWriter, _ *http.Request) bool {
				proxied.Add(1)
				http.Error(w, http.StatusText(status), status)
				return true
			})
			_, err := clusterinfo.GetClusterAgentEKSIdentityTagsWithRetry(context.Background())
			require.Error(t, err)
			assert.Equal(t, int32(1), proxied.Load())
		})
	}
}

func TestGetEKSClusterIdentityMarksARealCachedFailureByItsExpiry(t *testing.T) {
	// The real pkg/util/eks cache and the real eks.RetryIn behind the handler: a failure that
	// DescribeCluster is asked about again within retryLaterWindow is marked retry, one cached
	// longer is final.
	resolveEKSClusterIdentity = func(ctx context.Context) (*eks.ClusterIdentity, error) {
		return eks.GetClusterIdentity(ctx, "orders-cached", nil)
	}
	t.Cleanup(func() { resolveEKSClusterIdentity = eksidentity.Resolve })
	for ttl, mark := range map[time.Duration]string{
		2 * time.Second:  "true",
		25 * time.Second: "false",
	} {
		eks.CacheFailureForTest(t, "orders-cached", nil, throttlingError{}, ttl)
		recorder, _ := serveEKSIdentity(t)
		assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
		assert.Equal(t, mark, recorder.Header().Get(identitytypes.RetryLaterHeader), "cached for %s", ttl)
	}
}
