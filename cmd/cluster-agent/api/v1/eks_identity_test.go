// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
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
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/eksidentity"
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
	for _, sentinel := range []error{eksidentity.ErrNotEKS, eksidentity.ErrClusterNameUnavailable} {
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
