// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"
	pkgerrors "github.com/DataDog/datadog-agent/pkg/errors"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/eks/identity"
)

func resetStartupIdentity(t *testing.T) {
	t.Helper()
	reset := func() {
		_ = startupIdentity.lock.Acquire(context.Background(), 1)
		defer startupIdentity.lock.Release(1)
		startupIdentity.tags, startupIdentity.budgetSpent = nil, false
		startupIdentity.lastErr, startupIdentity.lastErrAt = nil, time.Time{}
		identityCall.reset()
	}
	reset()
	t.Cleanup(reset)
}

func stubEvidence(t *testing.T, onEKS bool) {
	t.Helper()
	orig := hasEKSEvidence
	t.Cleanup(func() { hasEKSEvidence = orig })
	hasEKSEvidence = func() bool { return onEKS }
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
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
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
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
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
				tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
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
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		assert.ErrorIs(t, err, unreachable)
		assert.Equal(t, StartupRetryBudget, time.Since(start))
		// Let an attempt started at the deadline end in the background.
		synctest.Wait()
		assert.Greater(t, calls, 1)

		// Later startup callers (DogStatsD, OTLP) right after get the same answer at once,
		// without asking the unreachable Cluster Agent again.
		for range 2 {
			callsBefore := calls
			start = time.Now()
			_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
			assert.ErrorIs(t, err, unreachable)
			assert.Zero(t, time.Since(start))
			assert.Equal(t, callsBefore, calls)
		}

		// After a short while, a later caller makes one attempt again.
		time.Sleep(startupFailureTTL)
		callsBefore := calls
		_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		assert.ErrorIs(t, err, unreachable)
		assert.Equal(t, callsBefore+1, calls)
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

		_, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.Error(t, err)
		// Let an attempt started at the deadline end in the background.
		synctest.Wait()

		time.Sleep(startupFailureTTL)
		reachable = true
		callsBefore := calls
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.NoError(t, err)
		assert.Contains(t, tags, "eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders")
		assert.Equal(t, callsBefore+1, calls)

		_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.NoError(t, err)
		assert.Equal(t, callsBefore+1, calls)
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryReusesSuccess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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
				tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
				assert.NoError(t, err)
				results[i] = tags
			})
		}
		// Every caller is now either inside the one lookup or waiting for it.
		synctest.Wait()
		assert.Equal(t, 1, calls, "concurrent callers share one lookup")
		close(release)
		wg.Wait()

		assert.Equal(t, 1, calls)
		for _, tags := range results {
			assert.ElementsMatch(t, ordersIdentity.Tags(), tags)
		}
	})
}

func TestGetClusterAgentEKSIdentityHostTags(t *testing.T) {
	original := getEKSClusterIdentity
	t.Cleanup(func() { getEKSClusterIdentity = original })
	calls := 0
	answer := func(id *identity.ClusterIdentity, err error) {
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) { calls++; return id, err }
	}
	statusErr := func(code int, retryLater bool) error {
		return &clusteragent.StatusError{StatusCode: code, RetryLater: retryLater, Err: pkgerrors.NewRemoteServiceError("dca", http.StatusText(code))}
	}

	t.Run("node without EKS evidence makes no identity call", func(t *testing.T) {
		// kops, self-managed Kubernetes or ROSA on EC2, kind, AKS, GKE, on-prem.
		calls = 0
		stubEvidence(t, false)
		answer(ordersIdentity, nil)
		tags, err := GetClusterAgentEKSIdentityHostTags(t.Context())
		require.NoError(t, err, "an empty success, so the provider is not retried or logged")
		assert.Empty(t, tags)
		assert.Equal(t, 0, calls)
	})

	t.Run("identity on an EKS node", func(t *testing.T) {
		stubEvidence(t, true)
		answer(ordersIdentity, nil)
		tags, err := GetClusterAgentEKSIdentityHostTags(t.Context())
		require.NoError(t, err)
		assert.Len(t, tags, 3)
	})

	t.Run("a 404 is an empty success", func(t *testing.T) {
		// Not EKS, or an older Cluster Agent: must not log on every refresh.
		stubEvidence(t, true)
		answer(nil, statusErr(http.StatusNotFound, false))
		tags, err := GetClusterAgentEKSIdentityHostTags(t.Context())
		require.NoError(t, err)
		assert.Empty(t, tags)
	})

	t.Run("answers that can change are errors, retried on the next refresh", func(t *testing.T) {
		stubEvidence(t, true)
		answer(nil, statusErr(http.StatusServiceUnavailable, true))
		_, err := GetClusterAgentEKSIdentityHostTags(t.Context())
		assert.Error(t, err)
		answer(nil, errors.New("connection refused"))
		_, err = GetClusterAgentEKSIdentityHostTags(t.Context())
		assert.Error(t, err)
	})
}

func writeToken(t *testing.T, claims string) string {
	t.Helper()
	enc := base64.RawURLEncoding.EncodeToString
	path := filepath.Join(t.TempDir(), "token")
	require.NoError(t, os.WriteFile(path, []byte(enc([]byte(`{"alg":"RS256"}`))+"."+enc([]byte(claims))+".c2lnbmF0dXJl\n"), 0o600))
	return path
}

func TestIsEKSServiceAccountToken(t *testing.T) {
	for _, tc := range []struct {
		name  string
		path  string
		onEKS bool
	}{
		{"EKS", writeToken(t, `{"iss":"https://oidc.eks.us-west-2.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE","sub":"system:serviceaccount:datadog:datadog-agent"}`), true},
		{"EKS in China", writeToken(t, `{"iss":"https://oidc.eks.cn-north-1.amazonaws.com.cn/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"kops or kind", writeToken(t, `{"iss":"https://kubernetes.default.svc.cluster.local"}`), false},
		{"GKE", writeToken(t, `{"iss":"https://container.googleapis.com/v1/projects/p/locations/us-central1/clusters/c"}`), false},
		{"AKS", writeToken(t, `{"iss":"https://eastus.oic.prod-aks.azure.com/tenant/uuid/"}`), false},
		{"legacy token", writeToken(t, `{"iss":"kubernetes/serviceaccount"}`), false},
		{"lookalike host", writeToken(t, `{"iss":"https://oidc.eks.evil/id/x/y"}`), false},
		{"no token", filepath.Join(t.TempDir(), "missing"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.onEKS, isEKSServiceAccountToken(tc.path))
		})
	}

	garbage := filepath.Join(t.TempDir(), "garbage")
	require.NoError(t, os.WriteFile(garbage, []byte("not a token"), 0o600))
	assert.False(t, isEKSServiceAccountToken(garbage))
}

func TestHasEKSEvidence(t *testing.T) {
	origToken := serviceAccountTokenIsEKS
	t.Cleanup(func() { serviceAccountTokenIsEKS = origToken })
	key := cache.BuildAgentKey(constants.NodeKubeDistributionKey)
	t.Cleanup(func() { cache.Cache.Delete(key) })

	serviceAccountTokenIsEKS = func() bool { return false }
	assert.False(t, HasEKSEvidence())

	cache.Cache.Set(key, "eks", cache.NoExpiration)
	assert.True(t, HasEKSEvidence(), "a distribution the host tags already resolved")
	cache.Cache.Set(key, "gke", cache.NoExpiration)
	assert.False(t, HasEKSEvidence())
	cache.Cache.Delete(key)

	serviceAccountTokenIsEKS = func() bool { return true }
	assert.True(t, HasEKSEvidence(), "an EKS service account token")

	serviceAccountTokenIsEKS = func() bool { return false }
	env.SetFeatures(t, env.EKSFargate)
	assert.True(t, HasEKSEvidence(), "an EKS Fargate sidecar")
}

func TestGetClusterAgentEKSIdentityTagsWithRetryHonorsCallerContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			return nil, errors.New("connection refused")
		}

		// A caller that stops (Agent shutdown) gets its answer at its own deadline, not
		// after the full startup budget.
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		defer cancel()
		start := time.Now()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(ctx)
		require.Error(t, err)
		assert.Equal(t, 2*time.Second, time.Since(start))
		synctest.Wait()
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryWaiterHonorsItsContext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			return nil, errors.New("connection refused")
		}

		// The first caller holds the retry for the whole budget.
		first := make(chan struct{})
		go func() {
			defer close(first)
			_, _ = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		}()
		synctest.Wait()

		// A second caller waiting on it stops when its own ctx is done.
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		start := time.Now()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(ctx)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, time.Second, time.Since(start))
		<-first
		synctest.Wait()
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryBoundsLaterAttempts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		hang := false
		getEKSClusterIdentity = func(ctx context.Context) (*identity.ClusterIdentity, error) {
			if hang {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return nil, errors.New("connection refused")
		}
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.Error(t, err)
		synctest.Wait()

		// Once the budget is spent, a later caller makes one attempt with a short timeout.
		time.Sleep(startupFailureTTL)
		hang = true
		start := time.Now()
		_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.Error(t, err)
		assert.Equal(t, laterAttemptTimeout, time.Since(start))
		// Let the hanging shared call end before the bubble does.
		time.Sleep(attemptTimeout)
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryRetriesNotDeterminedYet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		calls := 0
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls++
			if calls < 3 {
				// What the client returns for a 503 carrying identity.RetryLaterHeader
				// (see clusteragent TestGetEKSClusterIdentityRetryLaterAnswer).
				return nil, &clusteragent.StatusError{
					StatusCode: http.StatusServiceUnavailable,
					RetryLater: true,
					Err:        pkgerrors.NewRemoteServiceError("https://dca:5005/api/v1/cluster/eks-identity", "503 Service Unavailable"),
				}
			}
			return ordersIdentity, nil
		}

		tags, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 3, calls)
		assert.ElementsMatch(t, ordersIdentity.Tags(), tags)
	})
}
