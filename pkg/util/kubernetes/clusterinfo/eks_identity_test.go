// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterinfo

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func resetStartupIdentity(t *testing.T) {
	t.Helper()
	reset := func() {
		startupFetch.mu.Lock()
		startupFetch.f = nil
		startupFetch.mu.Unlock()
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
		var calls atomic.Int32
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls.Add(1)
			return nil, unreachable
		}

		start := time.Now()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, StartupRetryBudget, time.Since(start))
		synctest.Wait()
		assert.Greater(t, calls.Load(), int32(1))

		// Later startup callers (DogStatsD, OTLP) get the same answer at once, without asking
		// the unreachable Cluster Agent again.
		for range 2 {
			before := calls.Load()
			start = time.Now()
			_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Zero(t, time.Since(start))
			assert.Equal(t, before, calls.Load())
		}
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryStopsWaitingWhenTheBudgetIsSpent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		// An attempt that outlives the budget, as the Cluster Agent client's initialization
		// can: it takes no ctx. Callers still stop waiting at the budget.
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			time.Sleep(3 * StartupRetryBudget)
			return ordersIdentity, nil
		}
		start := time.Now()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(context.Background())
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Equal(t, StartupRetryBudget, time.Since(start))
		time.Sleep(3 * StartupRetryBudget)
		synctest.Wait()
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

	t.Run("final answers are an empty success", func(t *testing.T) {
		// 404 (not EKS, an older Cluster Agent) and a final 503 (a missing eks:DescribeCluster
		// permission, which the Cluster Agent logs itself) must not log on every refresh.
		stubEvidence(t, true)
		for _, err := range []error{statusErr(http.StatusNotFound, false), statusErr(http.StatusServiceUnavailable, false)} {
			answer(nil, err)
			tags, err := GetClusterAgentEKSIdentityHostTags(t.Context())
			require.NoError(t, err)
			assert.Empty(t, tags)
		}
	})

	t.Run("auth failures are logged once at Warn", func(t *testing.T) {
		// A 401 or 403 means the node's token is wrong. Not an error to retry every refresh,
		// but not silent either.
		stubEvidence(t, true)
		authWarned = sync.Once{}
		t.Cleanup(func() { authWarned = sync.Once{} })
		var buf bytes.Buffer
		logger, err := log.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(&buf, log.WarnLvl)
		require.NoError(t, err)
		log.SetupLogger(logger, "warn")
		t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })
		for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusUnauthorized} {
			answer(nil, statusErr(code, false))
			tags, err := GetClusterAgentEKSIdentityHostTags(t.Context())
			require.NoError(t, err)
			assert.Empty(t, tags)
		}
		log.Flush()
		assert.Equal(t, 1, strings.Count(buf.String(), "WARN"), buf.String())
		assert.Contains(t, buf.String(), "Unauthorized")
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
		{"EKS in GovCloud", writeToken(t, `{"iss":"https://oidc.eks.us-gov-west-1.amazonaws.com/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"EKS in iso", writeToken(t, `{"iss":"https://oidc.eks.us-iso-east-1.c2s.ic.gov/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"EKS in isob", writeToken(t, `{"iss":"https://oidc.eks.us-isob-east-1.sc2s.sgov.gov/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"EKS in isoe", writeToken(t, `{"iss":"https://oidc.eks.eu-isoe-west-1.cloud.adc-e.uk/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"EKS in isof", writeToken(t, `{"iss":"https://oidc.eks.us-isof-south-1.csp.hci.ic.gov/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"EKS in eusc", writeToken(t, `{"iss":"https://oidc.eks.eusc-de-east-1.amazonaws.eu/id/EXAMPLED539D4633E53DE1B71EXAMPLE"}`), true},
		{"kops or kind", writeToken(t, `{"iss":"https://kubernetes.default.svc.cluster.local"}`), false},
		{"GKE", writeToken(t, `{"iss":"https://container.googleapis.com/v1/projects/p/locations/us-central1/clusters/c"}`), false},
		{"AKS", writeToken(t, `{"iss":"https://eastus.oic.prod-aks.azure.com/tenant/uuid/"}`), false},
		{"legacy token", writeToken(t, `{"iss":"kubernetes/serviceaccount"}`), false},
		{"lookalike host", writeToken(t, `{"iss":"https://oidc.eks.evil/id/x/y"}`), false},
		{"lookalike domain", writeToken(t, `{"iss":"https://oidc.eks.evil.attacker.example/id/ABC123"}`), false},
		{"AWS name as a subdomain", writeToken(t, `{"iss":"https://oidc.eks.x.s3.amazonaws.com.evil.io/id/A"}`), false},
		{"region with dots", writeToken(t, `{"iss":"https://oidc.eks.us-west-2.evil.amazonaws.com/id/A"}`), false},
		{"trailing slash", writeToken(t, `{"iss":"https://oidc.eks.us-west-2.amazonaws.com/id/A/"}`), false},
		{"no token", filepath.Join(t.TempDir(), "missing"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.onEKS, isEKSServiceAccountToken(tc.path))
		})
	}

	garbage := filepath.Join(t.TempDir(), "garbage")
	require.NoError(t, os.WriteFile(garbage, []byte("not a token"), 0o600))
	assert.False(t, isEKSServiceAccountToken(garbage))

	// A file over the read limit is not a token, even with an EKS issuer at the start.
	big := filepath.Join(t.TempDir(), "big")
	valid, err := os.ReadFile(writeToken(t, `{"iss":"https://oidc.eks.us-west-2.amazonaws.com/id/EXAMPLE"}`))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(big, append(valid, bytes.Repeat([]byte(" "), maxServiceAccountTokenSize)...), 0o600))
	assert.False(t, isEKSServiceAccountToken(big))
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
		drainFetch()
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

func TestGetClusterAgentEKSIdentityTagsWithRetryCallerLeavingEarlyDoesNotSpendTheBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		// The Cluster Agent answers only after 3s of refusing connections.
		start := time.Now()
		var calls atomic.Int32
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls.Add(1)
			if time.Since(start) < 3*time.Second {
				return nil, errors.New("connection refused")
			}
			return ordersIdentity, nil
		}

		// Caller A has a 50ms ctx and leaves. The fetch is not A's: it goes on.
		ctxA, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(ctxA)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// Caller B has no deadline and gets the identity: A's departure neither failed nor
		// shortened the fetch, and B did not start a second one.
		tags, err := GetClusterAgentEKSIdentityTagsWithRetry(context.Background())
		require.NoError(t, err)
		assert.ElementsMatch(t, ordersIdentity.Tags(), tags)
		assert.Equal(t, 3*time.Second, time.Since(start))
		synctest.Wait()
	})
}

func TestGetClusterAgentEKSIdentityTagsWithRetryResultIsFinalForTheProcess(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		original := getEKSClusterIdentity
		t.Cleanup(func() { getEKSClusterIdentity = original })
		resetStartupIdentity(t)
		var calls atomic.Int32
		getEKSClusterIdentity = func(context.Context) (*identity.ClusterIdentity, error) {
			calls.Add(1)
			return nil, errors.New("connection refused")
		}
		_, err := GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.Error(t, err)
		synctest.Wait()
		before := calls.Load()

		// Whatever the fetch ended with is what later callers get, with no new call: host
		// metadata repairs a later miss.
		time.Sleep(time.Minute)
		start := time.Now()
		_, err = GetClusterAgentEKSIdentityTagsWithRetry(t.Context())
		require.Error(t, err)
		assert.Zero(t, time.Since(start))
		assert.Equal(t, before, calls.Load())
	})
}

// drainFetch lets the background fetch of a synctest bubble run out its budget, which a bubble
// needs before it can end.
func drainFetch() {
	time.Sleep(startupRetryBudget + time.Second)
	synctest.Wait()
}
