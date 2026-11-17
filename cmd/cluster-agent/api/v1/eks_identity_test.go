// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"

	"github.com/DataDog/datadog-agent/pkg/util/eks"
	identitytypes "github.com/DataDog/datadog-agent/pkg/util/eks/identity"
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
	assert.JSONEq(t, `{
		"cluster_name":"orders",
		"cluster_arn":"arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"aws_account_id":"123456789012",
		"region":"us-west-2"
	}`, recorder.Body.String())
	assert.Nil(t, span.Tag("error.message"))
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

func TestGetEKSClusterIdentityTransientFailureIsRetryable(t *testing.T) {
	// A throttled or timed out DescribeCluster is cached for 30s and can clear within a node's
	// startup budget. A missing permission is final.
	stubEKSIdentityResolver(t, nil, fmt.Errorf("describe: %w", context.DeadlineExceeded))
	recorder, _ := serveEKSIdentity(t)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Equal(t, "true", recorder.Header().Get(identitytypes.RetryLaterHeader))

	stubEKSIdentityResolver(t, nil, errors.New("api error AccessDeniedException"))
	recorder, _ = serveEKSIdentity(t)
	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	assert.Empty(t, recorder.Header().Get(identitytypes.RetryLaterHeader))
}
