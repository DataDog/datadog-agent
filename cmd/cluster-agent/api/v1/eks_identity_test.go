// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

func TestGetEKSClusterIdentity(t *testing.T) {
	original := resolveEKSClusterIdentity
	t.Cleanup(func() { resolveEKSClusterIdentity = original })
	resolveEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return &eks.ClusterIdentity{
			ClusterName: "orders",
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}

	recorder := httptest.NewRecorder()
	getEKSClusterIdentity(recorder, httptest.NewRequest(http.MethodGet, "/cluster/eks-identity", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{
		"cluster_name":"orders",
		"cluster_arn":"arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"aws_account_id":"123456789012",
		"region":"us-west-2"
	}`, recorder.Body.String())
}

func TestGetEKSClusterIdentityUnavailable(t *testing.T) {
	original := resolveEKSClusterIdentity
	t.Cleanup(func() { resolveEKSClusterIdentity = original })
	resolveEKSClusterIdentity = func(context.Context) (*eks.ClusterIdentity, error) {
		return nil, errors.New("identity unavailable")
	}

	recorder := httptest.NewRecorder()
	getEKSClusterIdentity(recorder, httptest.NewRequest(http.MethodGet, "/cluster/eks-identity", nil))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}
