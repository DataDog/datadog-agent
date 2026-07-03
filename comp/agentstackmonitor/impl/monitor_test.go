// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nooptelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// newTestComponent builds a component with a no-op telemetry sink and the
// clock pinned to a fixed instant. wmeta / scheduler / store / provider are
// deliberately left nil — the tests only exercise state and gauge cleanup
// paths that don't call them.
func newTestComponent(t *testing.T) *component {
	t.Helper()
	return &component{
		gauges:        newGauges(nooptelemetry.GetCompatComponent()),
		tickInterval:  defaultTickInterval,
		cacheValidity: defaultCacheValidity,
		now:           func() time.Time { return time.Unix(1000, 0) },
		subjects:      make(map[stateKey]*subjectState),
	}
}

func TestGetOrCreateState_ReturnsSameEntryAndRefreshesController(t *testing.T) {
	c := newTestComponent(t)
	key := stateKey{podUID: "pod-uid-1", containerName: "cluster-agent"}

	// Seed with an outdated controller (e.g. from a prior tick).
	first := c.getOrCreateState(key, fakePod("pod-uid-1"), SubjectKindClusterAgent,
		controllerRef{Namespace: "monitoring", Kind: "ReplicaSet", Name: "old"}, "cluster-agent")
	require.NotNil(t, first)

	// A subsequent call with a different controller must reuse the same
	// entry (preserving buffered state) but reflect the new controller.
	second := c.getOrCreateState(key, fakePod("pod-uid-1"), SubjectKindClusterAgent,
		controllerRef{Namespace: "monitoring", Kind: "Deployment", Name: "datadog-cluster-agent"}, "cluster-agent")
	assert.Same(t, first, second, "getOrCreateState should return the same subjectState pointer")
	assert.Equal(t, "datadog-cluster-agent", second.controller.Name, "controller should be refreshed")
}

func TestPurgeStale_KeepsRecentAndDropsExpired(t *testing.T) {
	c := newTestComponent(t)
	now := c.now()

	recentKey := stateKey{podUID: "recent", containerName: "cluster-agent"}
	expiredKey := stateKey{podUID: "expired", containerName: "agent"}

	c.subjects[recentKey] = &subjectState{
		subjectKind:   SubjectKindClusterAgent,
		controller:    controllerRef{Namespace: "monitoring", Kind: "Deployment", Name: "cluster-agent"},
		containerName: "cluster-agent",
		lastSeenAt:    now.Add(-staleness / 2),
	}
	c.subjects[expiredKey] = &subjectState{
		subjectKind:   SubjectKindClusterCheckRunner,
		controller:    controllerRef{Namespace: "monitoring", Kind: "Deployment", Name: "cluster-checks-runner"},
		containerName: "agent",
		lastSeenAt:    now.Add(-2 * staleness),
	}

	// Simulate a tick that only observed the recent entry.
	c.purgeStale(map[stateKey]struct{}{recentKey: {}})

	assert.Contains(t, c.subjects, recentKey, "recent entry should be preserved")
	assert.NotContains(t, c.subjects, expiredKey, "expired entry should be purged")
}

// fakePod returns a KubernetesPod stub with just enough identity for
// observePod to populate the subjectState.
func fakePod(uid string) *workloadmeta.KubernetesPod {
	pod := &workloadmeta.KubernetesPod{}
	pod.EntityID.ID = uid
	pod.EntityID.Kind = workloadmeta.KindKubernetesPod
	pod.EntityMeta.Name = "some-pod"
	pod.EntityMeta.Namespace = "monitoring"
	return pod
}
