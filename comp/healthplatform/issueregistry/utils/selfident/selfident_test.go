// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package selfident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/config/env"
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
	s.resolveRetries = 1
	s.resolveRetryDelay = time.Millisecond

	assert.Empty(t, s.DeploymentID())
}

func TestDeploymentID_RetriesUntilPodAppearsInWorkloadmeta(t *testing.T) {
	t.Setenv(podNameEnvVar, testPodName)
	mockStore := newMockStore(t)

	s := New(mockStore)
	// A wide retry budget relative to the goroutine's delay below, so the
	// assertion isn't sensitive to scheduling jitter under CI load or -race.
	s.resolveRetries = 500
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
	s.resolveRetries = 1
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

// ClusterID must bound how long it blocks a caller made before resolution
// settles — long enough to give a one-shot startup check a real chance at
// getting the id, but not indefinitely.
func TestClusterID_BlocksUpToRetryBudget(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)

	s := New(nil)
	s.resolveRetries = 3
	s.resolveRetryDelay = 10 * time.Millisecond

	start := time.Now()
	first := s.ClusterID()
	elapsed := time.Since(start)
	assert.Empty(t, first, "no Cluster Agent is configured in this test, so resolution settles on empty")
	assert.Less(t, elapsed, time.Second, "ClusterID must not block indefinitely")

	// The first ClusterID() call above has its own bounded wait that's
	// independent of the background resolver goroutine, so it can return
	// before that goroutine has actually stored the settled result. Wait
	// for the real settle point directly instead of assuming the first
	// call's return implies it — otherwise the timing assertion below
	// races the resolver goroutine and is flaky under scheduler/GC jitter
	// (see incident: the two durations it compared could land within
	// fractions of a millisecond of each other).
	assert.Eventually(t, func() bool {
		return s.clusterID.Load() != nil
	}, time.Second, time.Millisecond, "resolution must settle (cache populated) within a reasonable time")

	// Cached from the settled resolution; must return immediately without
	// re-running the resolution loop. Compared against a fixed budget
	// instead of the first call's elapsed time: once truly cached, 50
	// atomic loads take microseconds, orders of magnitude below even a
	// single retry delay, while a broken cache would take multiples of it.
	start = time.Now()
	for i := 0; i < 50; i++ {
		assert.Empty(t, s.ClusterID())
	}
	assert.Less(t, time.Since(start), s.resolveRetryDelay, "later calls must return immediately from cache, not re-run resolution")
}
