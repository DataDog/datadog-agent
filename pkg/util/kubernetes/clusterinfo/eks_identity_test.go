// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
)

func resetStartupIdentity(t *testing.T) {
	t.Helper()
	reset := func() {
		startupIdentity.Lock()
		defer startupIdentity.Unlock()
		startupIdentity.tags, startupIdentity.budgetSpent = nil, false
	}
	reset()
	t.Cleanup(reset)
}

var ordersIdentity = &identity.ClusterIdentity{
	ClusterName: "orders",
	ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
	AccountID:   "123456789012",
	Region:      "us-west-2",
}

func TestGetClusterAgentEKSIdentityTags(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	resetStartupIdentity(t)
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		return &identity.ClusterIdentity{
			ClusterName: "orders",
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"eks_cluster_account:123456789012",
		"eks_cluster_region:us-west-2",
	}, tags)
}

func TestGetClusterAgentEKSIdentityTagsRejectsIncompleteIdentity(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	resetStartupIdentity(t)
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		// No valid cluster ARN: the account and Region are derived from the ARN, so an
		// identity that cannot be anchored to a well-formed ARN yields no tags and must be
		// rejected rather than emitting partial, unauthenticated tag values.
		return &identity.ClusterIdentity{ClusterName: "orders"}, nil
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.Error(t, err)
}

func TestGetClusterAgentEKSIdentityTagsReturnsClientError(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	resetStartupIdentity(t)
	expected := errors.New("Cluster Agent unavailable")
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		return nil, expected
	}

	tags, err := GetClusterAgentEKSIdentityTags(t.Context())
	assert.Nil(t, tags)
	assert.ErrorIs(t, err, expected)
}

func TestGetClusterAgentEKSIdentityTagsWithRetryStopsOnClusterAgentAnswer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		expected := &clusteragent.StatusError{
			StatusCode: http.StatusNotFound,
			Err:        pkgerrors.NewRemoteServiceError("https://dca:5005/api/v1/cluster/eks-identity", "404 Not Found"),
		}
		calls := 0
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls++
			return nil, expected
		}

		start := time.Now()
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
		assert.Nil(t, tags)
		assert.ErrorIs(t, err, expected)
		assert.Equal(t, 1, calls)
		assert.Zero(t, time.Since(start))
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryStopsOnIncompleteIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		calls := 0
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls++
			return &identity.ClusterIdentity{ClusterName: "orders"}, nil
		}

		start := time.Now()
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
		assert.Nil(t, tags)
		assert.ErrorIs(t, err, errIncompleteIdentity)
		assert.Equal(t, 1, calls)
		assert.Zero(t, time.Since(start))
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryRetriesTransportFailures(t *testing.T) {
	for name, transportErr := range map[string]error{
		"client init":       errors.New("temporary failure in clusterAgentClient"),
		"after client init": pkgerrors.NewRemoteServiceError("https://dca:5005/api/v1/cluster/eks-identity", "dial tcp 10.0.0.1:5005: connect: connection refused"),
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				original := getEKSClusterIdentity
				t.Cleanup(func() { getEKSClusterIdentity = original })
				resetStartupIdentity(t)
				calls := 0
				getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
					calls++
					if calls == 1 {
						return nil, transportErr
					}
					return &identity.ClusterIdentity{
						ClusterName: "orders",
						ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/orders",
						AccountID:   "123456789012",
						Region:      "us-west-2",
					}, nil
				}

				start := time.Now()
				tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
				require.NoError(t, err)
				assert.Equal(t, 2, calls)
				assert.Equal(t, time.Second, time.Since(start))
				assert.Contains(t, tags, "eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders")
			})
		})
	}
}

func TestGetClusterAgentEKSIdentityTagsWithRetrySpendsBudgetOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		unreachable := pkgerrors.NewRemoteServiceError("https://dca:5005/api/v1/cluster/eks-identity", "dial tcp 10.0.0.1:5005: connect: connection refused")
		calls := 0
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls++
			return nil, unreachable
		}

		start := time.Now()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry()
		assert.ErrorIs(t, err, unreachable)
		assert.Equal(t, startupRetryBudget, time.Since(start))
		assert.Greater(t, calls, 1)

		for range 2 {
			callsBefore := calls
			start = time.Now()
			_, err = GetClusterAgentEKSIdentityTagsWithRetry()
			assert.ErrorIs(t, err, unreachable)
			assert.Zero(t, time.Since(start))
			assert.Equal(t, callsBefore+1, calls)
		}
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryRecoversAfterBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		reachable := false
		calls := 0
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls++
			if !reachable {
				return nil, errors.New("temporary failure in clusterAgentClient")
			}
			return ordersIdentity, nil
		}

		_, err := GetClusterAgentEKSIdentityTagsWithRetry()
		require.Error(t, err)

		reachable = true
		callsBefore := calls
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
		require.NoError(t, err)
		assert.Contains(t, tags, "eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders")
		assert.Equal(t, callsBefore+1, calls)

		_, err = GetClusterAgentEKSIdentityTagsWithRetry()
		require.NoError(t, err)
		assert.Equal(t, callsBefore+1, calls)
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryReusesSuccess(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	resetStartupIdentity(t)
	release := make(chan struct{})
	var calls int
	getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
		calls++
		<-release
		return ordersIdentity, nil
	}

	results := make([][]string, 3)
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
			assert.NoError(t, err)
			results[i] = tags
		})
	}
	close(release)
	wg.Wait()

	assert.Equal(t, 1, calls)
	for _, tags := range results {
		assert.ElementsMatch(t, ordersIdentity.Tags(), tags)
	}

	results[0][0] = "mutated"
	tags, err := GetClusterAgentEKSIdentityTagsWithRetry()
	require.NoError(t, err)
	assert.ElementsMatch(t, ordersIdentity.Tags(), tags)
}
