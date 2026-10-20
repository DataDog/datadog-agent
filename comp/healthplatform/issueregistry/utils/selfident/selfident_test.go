// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package selfident

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/common/namespace"
)

const testPodName = "dd-agent-abc12"

// testNamespace mirrors what resolveDeploymentID actually queries
// (namespace.GetMyNamespace()) instead of assuming "default", since that
// function falls back to "default" only when
// /var/run/secrets/kubernetes.io/serviceaccount/namespace doesn't exist —
// which isn't true on every test runner (e.g. CI executors that run inside a
// real Kubernetes pod).
var testNamespace = namespace.GetMyNamespace()

func newMockStore(t *testing.T) workloadmetamock.Mock {
	t.Helper()
	env.SetFeatures(t, env.Kubernetes)

	return workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: workloadmeta.NewParams(),
	})
}

func setSelfPod(mockStore workloadmetamock.Mock, owners []workloadmeta.KubernetesPodOwner) {
	mockStore.Set(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   "self-pod-uid",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      testPodName,
			Namespace: testNamespace,
		},
		Owners: owners,
	})
}

func TestDeploymentID_ResolvesFromDaemonSetOwner(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(mockStore)

	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())
}

func TestDeploymentID_NoDaemonSetOwner(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "ReplicaSet", Name: "some-rs", ID: "rs-uid"},
	})

	s := New(mockStore)

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_PodNotFound(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)

	s := New(mockStore)
	// Pod is never added in this test, so it's genuinely absent; keep the
	// retry loop from actually waiting out the default backoff.
	s.resolveTimeout = time.Millisecond
	s.resolveRetryDelay = time.Millisecond

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_RetriesUntilPodAppearsInWorkloadmeta(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)

	s := New(mockStore)
	// A wide retry budget relative to the goroutine's delay below, so the
	// assertion isn't sensitive to scheduling jitter under CI load or -race.
	s.resolveTimeout = 500 * time.Millisecond
	s.resolveRetryDelay = time.Millisecond

	// Simulates workloadmeta's kubelet collector not having synced the
	// agent's own pod yet at the moment the startup health check runs.
	go func() {
		time.Sleep(5 * time.Millisecond)
		setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
			{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
		})
	}()

	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())
}

func TestDeploymentID_NoPodNameEnvVar(t *testing.T) {
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(mockStore)

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_NoWorkloadmeta(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	env.SetFeatures(t, env.Kubernetes)

	s := New(nil)

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_ResolvedOnce(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(mockStore)
	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())

	mockStore.Unset(&workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "self-pod-uid"},
	})

	// Cached from the first resolution; does not re-query workloadmeta.
	assert.Equal(t, "daemonset-uid-123", s.DeploymentID())
}

// If the pod hasn't synced into workloadmeta by the time the retry budget is
// exhausted, that miss must not be cached permanently — a later call (e.g.
// from a different issue module reporting after this one) must get a fresh
// attempt and succeed once the pod has appeared.
func TestDeploymentID_TransientMissIsNotCachedPermanently(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)

	s := New(mockStore)
	s.resolveTimeout = time.Millisecond
	s.resolveRetryDelay = time.Millisecond

	assert.Empty(t, s.DeploymentID(), "pod not yet in workloadmeta, retry budget exhausted")

	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	assert.Equal(t, "daemonset-uid-123", s.DeploymentID(), "a later call must retry rather than replay the stale empty result")
}

func TestIssueDiscriminator_ReportsDeploymentID(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)
	setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
		{Kind: "DaemonSet", Name: "datadog-agent", ID: "daemonset-uid-123"},
	})

	s := New(mockStore)

	assert.Equal(t, "daemonset-uid-123", s.IssueDiscriminator())
}

// IssueDiscriminator must report nothing rather than invent a per-host id when
// no DaemonSet owns this agent — the per-host fallback is the caller's job, so
// that it stays identical on flavors where selfident is a no-op.
func TestIssueDiscriminator_EmptyWithoutDaemonSet(t *testing.T) {
	s := New(nil)

	assert.Empty(t, s.IssueDiscriminator())
}

func TestNew_NoopOutsideKubernetes(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)

	s := New(nil)

	assert.Empty(t, s.DeploymentID())
	assert.Empty(t, s.ClusterID())
}

func TestIdentityWithContextWaitsForReadiness(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	for _, test := range []struct {
		name    string
		resolve func(*SelfIdent, context.Context) string
		report  func(*SelfIdent) string
	}{
		{"deployment", (*SelfIdent).DeploymentIDWithContext, (*SelfIdent).DeploymentID},
		{"cluster", (*SelfIdent).ClusterIDWithContext, (*SelfIdent).ClusterID},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				mockStore := newMockStore(t)
				s := New(mockStore)
				ready := make(chan struct{})
				stubClusterIDFuncs(t, func() (string, error) {
					select {
					case <-ready:
						return "identity", nil
					default:
						return "", errors.New("not ready")
					}
				}, func() (string, error) { return "", errors.New("unused") })
				go func() {
					time.Sleep(2100 * time.Millisecond)
					setSelfPod(mockStore, []workloadmeta.KubernetesPodOwner{
						{Kind: "DaemonSet", ID: "identity"},
					})
					close(ready)
				}()

				start := time.Now()
				reportWait := make(chan time.Duration, 1)
				go func() {
					assert.Empty(t, test.report(s))
					reportWait <- time.Since(start)
				}()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				assert.Equal(t, "identity", test.resolve(s, ctx))
				assert.Equal(t, 2200*time.Millisecond, time.Since(start))
				assert.Equal(t, time.Second, <-reportWait)
				synctest.Wait()
			})
		})
	}
}

func TestIdentityWithContextStopsWaiting(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	for _, test := range []struct {
		name    string
		resolve func(*SelfIdent, context.Context) string
	}{
		{"deployment", (*SelfIdent).DeploymentIDWithContext},
		{"cluster", (*SelfIdent).ClusterIDWithContext},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, cancelEarly := range []bool{false, true} {
				synctest.Test(t, func(t *testing.T) {
					s := New(newMockStore(t))
					stubClusterIDFuncs(t,
						func() (string, error) { return "", errors.New("not ready") },
						func() (string, error) { return "", errors.New("unused") },
					)
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					wantWait := 10 * time.Second
					if cancelEarly {
						wantWait = 500 * time.Millisecond
						time.AfterFunc(wantWait, cancel)
					}
					start := time.Now()
					assert.Empty(t, test.resolve(s, ctx))
					assert.Equal(t, wantWait, time.Since(start))
					synctest.Wait()
				})
			}
		})
	}
}

func TestClusterID_BlocksUpToDefaultTimeout(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	synctest.Test(t, func(t *testing.T) {
		stubClusterIDFuncs(t,
			func() (string, error) { return "", errors.New("cluster agent unreachable") },
			func() (string, error) { return "", errors.New("unused") },
		)

		s := New(nil)
		start := time.Now()
		first := s.ClusterID()
		elapsed := time.Since(start)

		assert.Empty(t, first, "every lookup failed, so resolution settles on empty")
		assert.Equal(t, time.Second, elapsed)

		// Let the resolver goroutine finish before t.Cleanup restores the stub
		// globals it reads.
		synctest.Wait()
	})
}

// A blocked lookup must not extend the caller's wait.
func TestClusterID_BlockedLookupDoesNotBlockCallerBeyondTimeout(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		stubClusterIDFuncs(t,
			func() (string, error) {
				<-release // block as a hung Cluster Agent/API server call would
				return "node-agent-id", nil
			},
			func() (string, error) { return "", errors.New("unused") },
		)

		s := New(nil)
		s.resolveTimeout = 20 * time.Millisecond

		start := time.Now()
		id := s.ClusterID()
		elapsed := time.Since(start)
		assert.Empty(t, id, "a blocked lookup must not surface an id")
		assert.Equal(t, s.resolveTimeout, elapsed, "caller must return once the bounded wait elapses, not wait for the blocked lookup")

		// Unblock the lookup and let the resolver settle: proves a later call
		// picks up the id the timed-out one missed, and lets the resolver finish
		// (synctest.Wait) before t.Cleanup restores the stub globals it reads.
		close(release)
		synctest.Wait()
		assert.Equal(t, "node-agent-id", s.ClusterID(), "a later call must pick up the id the timed-out caller missed")
	})
}

// TestClusterID_CachesSuccessfulResolution verifies that once resolution
// succeeds, later calls return the cached id without re-running the lookup.
func TestClusterID_CachesSuccessfulResolution(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	var calls int
	stubClusterIDFuncs(t,
		func() (string, error) { calls++; return "node-agent-id", nil },
		func() (string, error) { return "", errors.New("unused") },
	)

	s := New(nil)

	assert.Equal(t, "node-agent-id", s.ClusterID())
	assert.Equal(t, "node-agent-id", s.ClusterID())
	assert.Equal(t, 1, calls, "second call must return from cache, not re-run the lookup")
}

// TestClusterID_FailedResolutionIsNotCachedPermanently verifies that a startup
// Cluster Agent/API server outage doesn't permanently blank out cluster_id: a
// later call must retry rather than replay the stale empty result (cf.
// TestDeploymentID_TransientMissIsNotCachedPermanently).
func TestClusterID_FailedResolutionIsNotCachedPermanently(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	synctest.Test(t, func(t *testing.T) {
		var ready bool
		stubClusterIDFuncs(t,
			func() (string, error) {
				if !ready {
					return "", errors.New("api server unreachable")
				}
				return "node-agent-id", nil
			},
			func() (string, error) { return "", errors.New("unused") },
		)

		s := New(nil)
		assert.Empty(t, s.ClusterID(), "lookup failed, retry budget exhausted")
		synctest.Wait()
		ready = true
		assert.Equal(t, "node-agent-id", s.ClusterID(), "a later call must retry rather than replay the stale empty result")
		synctest.Wait()
	})
}

// stubClusterIDFuncs overrides the node-agent/cluster-agent cluster id
// lookups for the duration of the test, restoring the real functions on
// cleanup.
func stubClusterIDFuncs(t *testing.T, nodeAgent, clusterAgent func() (string, error)) {
	t.Helper()
	origNodeAgent, origClusterAgent := nodeAgentClusterIDFunc, clusterAgentClusterIDFunc
	nodeAgentClusterIDFunc, clusterAgentClusterIDFunc = nodeAgent, clusterAgent
	t.Cleanup(func() {
		nodeAgentClusterIDFunc, clusterAgentClusterIDFunc = origNodeAgent, origClusterAgent
	})
}

// TestClusterID_ClusterAgentFlavorUsesClusterAgentLookup verifies that on the
// Cluster Agent flavor, ClusterID dispatches to the Cluster-Agent-specific
// lookup rather than the node-agent-only clustername.GetClusterID (broken when
// the DCA calls it on itself).
func TestClusterID_ClusterAgentFlavorUsesClusterAgentLookup(t *testing.T) {
	origFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.ClusterAgent)
	t.Cleanup(func() { flavor.SetFlavor(origFlavor) })

	env.SetFeatures(t, env.Kubernetes)
	stubClusterIDFuncs(t,
		func() (string, error) { return "node-agent-id", nil },
		func() (string, error) { return "cluster-agent-id", nil },
	)

	s := New(nil)
	assert.Equal(t, "cluster-agent-id", s.ClusterID())
}

// TestClusterID_NonClusterAgentFlavorUsesNodeAgentLookup verifies that a
// non-Cluster-Agent flavor (e.g. the node agent) keeps using
// clustername.GetClusterID rather than the Cluster-Agent-specific lookup.
func TestClusterID_NonClusterAgentFlavorUsesNodeAgentLookup(t *testing.T) {
	origFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.DefaultAgent)
	t.Cleanup(func() { flavor.SetFlavor(origFlavor) })

	env.SetFeatures(t, env.Kubernetes)
	stubClusterIDFuncs(t,
		func() (string, error) { return "node-agent-id", nil },
		func() (string, error) { return "cluster-agent-id", nil },
	)

	s := New(nil)
	assert.Equal(t, "node-agent-id", s.ClusterID())
}

// TestClusterID_ClusterAgentFlavorLookupError verifies that an error from the
// Cluster-Agent-specific lookup (e.g. apiserver.GetAPIClient failing) is
// retried and ultimately settles on empty, the same as the node-agent path.
func TestClusterID_ClusterAgentFlavorLookupError(t *testing.T) {
	origFlavor := flavor.GetFlavor()
	flavor.SetFlavor(flavor.ClusterAgent)
	t.Cleanup(func() { flavor.SetFlavor(origFlavor) })

	env.SetFeatures(t, env.Kubernetes)
	synctest.Test(t, func(t *testing.T) {
		stubClusterIDFuncs(t,
			func() (string, error) { return "node-agent-id", nil },
			func() (string, error) { return "", errors.New("api server unreachable") },
		)

		s := New(nil)
		assert.Empty(t, s.ClusterID())
		synctest.Wait()
	})
}
