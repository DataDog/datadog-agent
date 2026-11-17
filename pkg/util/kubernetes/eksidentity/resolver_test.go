// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eksidentity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/eks"
)

type stubState struct {
	requested   string
	requestedCA []byte
	ec2Lookups  int
	resolveCall int
}

var testClusterCA = []byte("-----BEGIN CERTIFICATE-----\nY2x1c3Rlcg==\n-----END CERTIFICATE-----\n")

func stub(t *testing.T, distribution string, ec2Name string, ec2Err error, configured string) *stubState {
	t.Helper()
	cache.Cache.Delete(cache.BuildAgentKey("eks", "cluster-name"))
	origDistribution, origEC2, origConfigured, origResolve, origCA := detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey, readClusterCA
	t.Cleanup(func() {
		detectDistribution, ec2ClusterName, configuredCluster, resolveIdentityByKey, readClusterCA = origDistribution, origEC2, origConfigured, origResolve, origCA
		cache.Cache.Delete(cache.BuildAgentKey("eks", "cluster-name"))
	})

	state := &stubState{}
	detectDistribution = func(context.Context) (string, bool) { return distribution, true }
	ec2ClusterName = func(context.Context) (string, error) { state.ec2Lookups++; return ec2Name, ec2Err }
	configuredCluster = func(context.Context, string) string { return configured }
	readClusterCA = func() ([]byte, error) { return testClusterCA, nil }
	resolveIdentityByKey = func(_ context.Context, name string, clusterCA []byte) (*eks.ClusterIdentity, error) {
		state.requested = name
		state.requestedCA = clusterCA
		state.resolveCall++
		return &eks.ClusterIdentity{
			ClusterName: name,
			ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/" + name,
			AccountID:   "123456789012",
			Region:      "us-west-2",
		}, nil
	}
	return state
}

func TestResolveRefusesNonEKS(t *testing.T) {
	state := stub(t, "gke", "orders", nil, "orders")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrNotEKS)
	assert.Empty(t, state.requested)
	assert.Equal(t, 0, state.ec2Lookups, "non-EKS clusters must not run cluster name discovery")
	assert.Nil(t, Tags(t.Context()))
}

func TestResolvePrefersEC2ClusterTag(t *testing.T) {
	state := stub(t, "eks", "Orders_Prod", nil, "orders-prod")
	identity, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Orders_Prod", state.requested)
	assert.Equal(t, "arn:aws:eks:us-west-2:123456789012:cluster/Orders_Prod", identity.ClusterARN)
}

func TestResolveFallsBackToConfiguredName(t *testing.T) {
	state := stub(t, "eks", "", errors.New("no ec2 tags"), "orders")
	_, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "orders", state.requested)
	assert.ElementsMatch(t, []string{
		"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
		"eks_cluster_account:123456789012",
		"eks_cluster_region:us-west-2",
	}, Tags(t.Context()))
}

func TestResolveFailsWithoutClusterName(t *testing.T) {
	state := stub(t, "eks", "", nil, "")
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrClusterNameUnavailable)
	assert.Empty(t, state.requested)

	// The absence is cached so repeated node requests do not re-run discovery.
	_, err = Resolve(t.Context())
	assert.ErrorIs(t, err, ErrClusterNameUnavailable)
	assert.Equal(t, 1, state.ec2Lookups)
}

func TestResolveCachesDiscoveredClusterName(t *testing.T) {
	state := stub(t, "eks", "orders", nil, "")
	for range 3 {
		_, err := Resolve(t.Context())
		require.NoError(t, err)
	}
	assert.Equal(t, 1, state.ec2Lookups, "cluster name must be discovered once")
	assert.Equal(t, 3, state.resolveCall, "identity resolution itself is cached by pkg/util/eks")
}

func TestResolveRetriesEC2TagAfterConfiguredFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ec2Err := errors.New("ec2 tags unavailable")
		state := stub(t, "eks", "", ec2Err, "orders-display-name")
		_, err := Resolve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "orders-display-name", state.requested, "configured name is the fallback")

		// Once the EC2 tag becomes available, the fallback must not pin the display name.
		ec2ClusterName = func(context.Context) (string, error) { state.ec2Lookups++; return "OrdersProd", nil }
		_, err = Resolve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "orders-display-name", state.requested, "fallback is served from cache until the retry TTL")
		assert.Equal(t, 1, state.ec2Lookups)

		time.Sleep(clusterNameRetryTTL + time.Second)
		_, err = Resolve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "OrdersProd", state.requested, "EC2 tag name must replace the configured fallback")
		assert.Equal(t, 2, state.ec2Lookups, "EC2 tag discovery must be retried after the fallback TTL")

		// The EC2 derived name is cached permanently.
		time.Sleep(clusterNameRetryTTL + time.Second)
		_, err = Resolve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, 2, state.ec2Lookups)
	})
}

func TestResolveDoesNotPinFallbackWhenCallerCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		state := stub(t, "eks", "", nil, "orders-display-name")
		// The EC2 tag lookup only answers when its ctx is done, like a slow IMDS call.
		ec2ClusterName = func(ctx context.Context) (string, error) {
			state.ec2Lookups++
			<-ctx.Done()
			return "", ctx.Err()
		}

		// The node that triggered discovery disconnects right away. The shared resolve goes on
		// and its lookup times out.
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err := Resolve(ctx)
		require.ErrorIs(t, err, context.Canceled)
		// A second node joins the same shared resolve and waits for it.
		_, _ = Resolve(t.Context())
		assert.Equal(t, 1, state.ec2Lookups, "the second node shared the first lookup")

		// EC2 tags are now reachable. A cancelled or timed out lookup must not have cached the
		// configured name for clusterNameRetryTTL, only for the short timeout TTL.
		ec2ClusterName = func(context.Context) (string, error) { state.ec2Lookups++; return "OrdersProd", nil }
		time.Sleep(clusterNameTimeoutTTL + time.Second)
		_, err = Resolve(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "OrdersProd", state.requested)
	})
}

func TestResolveBoundsEC2TagDiscovery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub(t, "eks", "", nil, "orders")
		ec2ClusterName = func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}
		start := time.Now()
		_, err := Resolve(t.Context())
		require.NoError(t, err, "the configured name is used for this request")
		assert.LessOrEqual(t, time.Since(start), clusterNameDiscoveryTimeout, "discovery has its own short budget")
	})
}

func TestResolveVerifiesAgainstTheClusterCA(t *testing.T) {
	for _, tc := range []struct {
		name     string
		ec2Name  string
		ec2Err   error
		wantName string
	}{
		{name: "EC2 tag name", ec2Name: "Orders_Prod", wantName: "Orders_Prod"},
		{name: "configured fallback", ec2Err: errors.New("no ec2 tags"), wantName: "orders"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := stub(t, "eks", tc.ec2Name, tc.ec2Err, "orders")
			_, err := Resolve(t.Context())
			require.NoError(t, err)
			assert.Equal(t, tc.wantName, state.requested)
			assert.Equal(t, testClusterCA, state.requestedCA, "the pod's cluster CA is passed for verification")
		})
	}
}

func TestResolveRefusesUnverifiedConfiguredName(t *testing.T) {
	// Without the cluster CA, a configured name can name another cluster (a stale value, a
	// blue/green pair), so it is not used.
	state := stub(t, "eks", "", errors.New("no ec2 tags"), "orders")
	readClusterCA = func() ([]byte, error) { return nil, os.ErrNotExist }
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	assert.ErrorIs(t, err, ErrClusterUnverified)
	assert.Equal(t, 0, state.resolveCall)
}

func TestResolveUsesEC2NameWithoutClusterCA(t *testing.T) {
	// The EC2 kubernetes.io/cluster/<name> tag is set by EKS on the node this pod runs on, so it
	// names this cluster even when the CA cannot be read.
	state := stub(t, "eks", "Orders_Prod", nil, "orders")
	readClusterCA = func() ([]byte, error) { return nil, os.ErrNotExist }
	_, err := Resolve(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Orders_Prod", state.requested)
	assert.Nil(t, state.requestedCA)
}

func TestResolveTimedOutLookupWithoutCAIsNotDeterminedYet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		state := stub(t, "eks", "", nil, "orders")
		ec2ClusterName = func(ctx context.Context) (string, error) {
			state.ec2Lookups++
			<-ctx.Done()
			return "", ctx.Err()
		}
		readClusterCA = func() ([]byte, error) { return nil, os.ErrNotExist }

		// The EC2 tag lookup timing out says nothing about the cluster name, so this is not
		// the final "cannot verify the configured name" answer.
		_, err := Resolve(t.Context())
		assert.ErrorIs(t, err, ErrNotDetermined)
		assert.NotErrorIs(t, err, ErrClusterUnverified)
	})
}

func TestResolveCoalescesColdRequests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// 200 node Agents ask a Cluster Agent that just started. They share one EC2 tag
		// lookup (IMDS, then DescribeTags) instead of making 200.
		state := stub(t, "eks", "", nil, "orders")
		release := make(chan struct{})
		var lookups atomic.Int32
		ec2ClusterName = func(context.Context) (string, error) {
			lookups.Add(1)
			<-release
			return "Orders_Prod", nil
		}

		var wg sync.WaitGroup
		for range 200 {
			wg.Go(func() {
				_, err := Resolve(t.Context())
				assert.NoError(t, err)
			})
		}
		synctest.Wait()
		close(release)
		wg.Wait()
		assert.Equal(t, int32(1), lookups.Load())
		assert.Equal(t, "Orders_Prod", state.requested)
	})
}

func TestResolveCachesTimedOutLookupBriefly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stub(t, "eks", "", nil, "orders")
		var lookups atomic.Int32
		ec2ClusterName = func(ctx context.Context) (string, error) {
			lookups.Add(1)
			<-ctx.Done()
			return "", ctx.Err()
		}

		_, _ = Resolve(t.Context())
		_, _ = Resolve(t.Context())
		assert.Equal(t, int32(1), lookups.Load(), "a timed out lookup is not repeated right away")

		time.Sleep(clusterNameTimeoutTTL + time.Second)
		_, _ = Resolve(t.Context())
		assert.Equal(t, int32(2), lookups.Load(), "but it is tried again soon")
	})
}

func TestResolveStaysWithinNodeRequestTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Every step hangs. The whole resolve still answers before the node Agent's 3s
		// ResponseHeaderTimeout, so the node gets an answer it can act on.
		stub(t, "", "", nil, "orders")
		detectDistribution = func(ctx context.Context) (string, bool) {
			<-ctx.Done()
			return "", false
		}
		start := time.Now()
		_, err := Resolve(t.Context())
		assert.ErrorIs(t, err, ErrNotDetermined)
		assert.Equal(t, resolveBudget, time.Since(start))
		assert.Less(t, resolveBudget, 3*time.Second)

		stub(t, "eks", "", nil, "orders")
		ec2ClusterName = func(ctx context.Context) (string, error) {
			<-ctx.Done()
			return "", ctx.Err()
		}
		resolveIdentityByKey = func(ctx context.Context, _ string, _ []byte) (*eks.ClusterIdentity, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		start = time.Now()
		_, err = Resolve(t.Context())
		require.Error(t, err)
		assert.LessOrEqual(t, time.Since(start), resolveBudget)
	})
}

func TestResolveRecoversPanics(t *testing.T) {
	// singleflight.DoChan re-panics in a new goroutine, which would crash the Cluster Agent.
	stub(t, "eks", "orders", nil, "orders")
	resolveIdentityByKey = func(context.Context, string, []byte) (*eks.ClusterIdentity, error) {
		panic("boom")
	}
	identity, err := Resolve(t.Context())
	assert.Nil(t, identity)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boom")
}

func TestResolveDropsEC2NameAfterClusterMismatch(t *testing.T) {
	// An EC2 tag name that verifies against another cluster's CA must not stay cached forever.
	state := stub(t, "eks", "Orders_Old", nil, "orders")
	resolveIdentityByKey = func(_ context.Context, name string, _ []byte) (*eks.ClusterIdentity, error) {
		state.requested = name
		return nil, fmt.Errorf("verify %s: %w", name, eks.ErrClusterMismatch)
	}
	_, err := Resolve(t.Context())
	require.ErrorIs(t, err, eks.ErrClusterMismatch)
	_, err = Resolve(t.Context())
	require.ErrorIs(t, err, eks.ErrClusterMismatch)
	assert.Equal(t, 2, state.ec2Lookups, "the name is looked up again")
}

func TestResolveTreatsCertificateLessCAAsUnreadable(t *testing.T) {
	for name, ca := range map[string][]byte{
		"empty":       {},
		"no PEM":      []byte("not a certificate"),
		"other block": []byte("-----BEGIN PRIVATE KEY-----\nAAAA\n-----END PRIVATE KEY-----\n"),
	} {
		t.Run(name, func(t *testing.T) {
			// With the EC2 tag name, the identity is resolved without the CA check rather
			// than failing every lookup as a mismatch.
			state := stub(t, "eks", "Orders_Prod", nil, "orders")
			readClusterCA = func() ([]byte, error) { return ca, nil }
			_, err := Resolve(t.Context())
			require.NoError(t, err)
			assert.Nil(t, state.requestedCA)

			// With only the configured name, it cannot be verified.
			state = stub(t, "eks", "", errors.New("no ec2 tags"), "orders")
			readClusterCA = func() ([]byte, error) { return ca, nil }
			_, err = Resolve(t.Context())
			assert.ErrorIs(t, err, ErrClusterUnverified)
			assert.Equal(t, 0, state.resolveCall)
		})
	}
}
