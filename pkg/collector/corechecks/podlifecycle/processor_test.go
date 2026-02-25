// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package podlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

func newTestProcessor(t *testing.T, name string) (*processor, *mocksender.MockSender, taggermock.Mock) {
	t.Helper()
	s := mocksender.NewMockSender(checkid.ID(name))
	s.On("Distribution", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	s.On("Commit").Return()
	tagger := taggerfxmock.SetupFakeTagger(t)
	return newProcessor(s, tagger), s, tagger
}

func makeReadyRunningPod(uid, namespace string, createdAt time.Time, readyAt time.Time, containerStartedAt time.Time) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   uid,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Namespace: namespace,
		},
		Phase: "Running",
		Ready: true,
		Conditions: []workloadmeta.KubernetesPodCondition{
			{
				Type:               "Ready",
				Status:             "True",
				LastTransitionTime: readyAt,
			},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{
			{
				Name:  "app",
				Ready: true,
				State: workloadmeta.KubernetesContainerState{
					Running: &workloadmeta.KubernetesContainerStateRunning{
						StartedAt: containerStartedAt,
					},
				},
			},
		},
		CreationTimestamp: createdAt,
	}
}

func makePendingPod(uid, namespace string, createdAt time.Time) *workloadmeta.KubernetesPod {
	return &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   uid,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Namespace: namespace,
		},
		Phase: "Pending",
		Conditions: []workloadmeta.KubernetesPodCondition{
			{Type: "Ready", Status: "False"},
		},
		CreationTimestamp: createdAt,
	}
}

// TestProcessEvents_PendingThenReady verifies the normal path: pod first appears
// as Pending, then transitions to Ready+Running. Metrics are emitted exactly once
// with tagger-provided low-cardinality tags (no pod_name).
func TestProcessEvents_PendingThenReady(t *testing.T) {
	proc, sender, tagger := newTestProcessor(t, "pending_then_ready")

	uid := "pod-uid-1"
	now := time.Now().UTC().Truncate(time.Second)
	createdAt := now.Add(-30 * time.Second)
	readyAt := now.Add(-5 * time.Second)
	startedAt := now.Add(-8 * time.Second)

	// Pre-populate tagger with low-cardinality tags for this pod.
	// pod_name is intentionally absent (it would be OrchestratorCardinality).
	tagger.SetTags(
		taggertypes.NewEntityID(taggertypes.KubernetesPodUID, uid),
		"test",
		[]string{"kube_namespace:default", "kube_deployment:my-deploy"}, // low
		[]string{"pod_name:my-pod-abc"},                                  // orch – must NOT appear
		nil, nil,
	)

	// First event: pod is Pending – no metrics emitted.
	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makePendingPod(uid, "default", createdAt)}},
		Ch:     make(chan struct{}),
	})
	sender.AssertNotCalled(t, "Distribution")

	proc.mu.Lock()
	rec := proc.podStates[uid]
	proc.mu.Unlock()
	assert.Equal(t, podStatusPending, rec.status)
	assert.Equal(t, createdAt, rec.createdAt)

	// Second event: pod becomes Ready+Running – metrics emitted exactly once.
	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makeReadyRunningPod(uid, "default", createdAt, readyAt, startedAt)}},
		Ch:     make(chan struct{}),
	})

	proc.mu.Lock()
	rec2 := proc.podStates[uid]
	proc.mu.Unlock()
	assert.Equal(t, podStatusDone, rec2.status)

	expectedTTR := readyAt.Sub(createdAt).Seconds()
	expectedTTRun := startedAt.Sub(createdAt).Seconds()

	tagsContainDeployment := mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if tag == "kube_deployment:my-deploy" {
				return true
			}
		}
		return false
	})
	tagsDoNotContainPodName := mock.MatchedBy(func(tags []string) bool {
		for _, tag := range tags {
			if len(tag) >= 9 && tag[:9] == "pod_name:" {
				return false
			}
		}
		return true
	})

	sender.AssertCalled(t, "Distribution", metricTimeToReady, expectedTTR, "", tagsContainDeployment)
	sender.AssertCalled(t, "Distribution", metricTimeToRunning, expectedTTRun, "", tagsDoNotContainPodName)

	// Third event: another Ready+Running update – no additional metric emitted.
	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makeReadyRunningPod(uid, "default", createdAt, now, now.Add(-1*time.Second))}},
		Ch:     make(chan struct{}),
	})
	sender.AssertNumberOfCalls(t, "Distribution", 2) // still only 2 total
}

// TestProcessEvents_AlreadyReady verifies that a pod seen for the first time
// already Ready+Running is marked done and no metric is emitted.
func TestProcessEvents_AlreadyReady(t *testing.T) {
	proc, sender, _ := newTestProcessor(t, "already_ready")

	now := time.Now().UTC().Truncate(time.Second)
	uid := "pod-uid-2"
	ready := makeReadyRunningPod(uid, "kube-system", now.Add(-60*time.Second), now.Add(-10*time.Second), now.Add(-12*time.Second))

	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: ready}},
		Ch:     make(chan struct{}),
	})

	proc.mu.Lock()
	rec := proc.podStates[uid]
	proc.mu.Unlock()
	assert.Equal(t, podStatusDone, rec.status)
	sender.AssertNotCalled(t, "Distribution")
}

// TestProcessEvents_UnsetRemoves verifies that an Unset event removes the pod
// from podStates.
func TestProcessEvents_UnsetRemoves(t *testing.T) {
	proc, _, _ := newTestProcessor(t, "unset_removes")

	now := time.Now().UTC().Truncate(time.Second)
	uid := "pod-uid-3"
	createdAt := now.Add(-30 * time.Second)

	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makePendingPod(uid, "ns", createdAt)}},
		Ch:     make(chan struct{}),
	})
	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeUnset, Entity: makePendingPod(uid, "ns", createdAt)}},
		Ch:     make(chan struct{}),
	})

	proc.mu.Lock()
	_, inStates := proc.podStates[uid]
	proc.mu.Unlock()
	assert.False(t, inStates)
}

// TestProcessEvents_DoneIsIdempotent verifies that subsequent Ready+Running
// events for an already-done pod emit no further metrics.
func TestProcessEvents_DoneIsIdempotent(t *testing.T) {
	proc, sender, tagger := newTestProcessor(t, "done_idempotent")

	uid := "pod-uid-4"
	now := time.Now().UTC().Truncate(time.Second)
	createdAt := now.Add(-60 * time.Second)
	readyAt := now.Add(-10 * time.Second)
	startedAt := now.Add(-15 * time.Second)

	tagger.SetTags(taggertypes.NewEntityID(taggertypes.KubernetesPodUID, uid), "test",
		[]string{"kube_namespace:ns"}, nil, nil, nil)

	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makePendingPod(uid, "ns", createdAt)}},
		Ch:     make(chan struct{}),
	})
	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makeReadyRunningPod(uid, "ns", createdAt, readyAt, startedAt)}},
		Ch:     make(chan struct{}),
	})
	sender.AssertNumberOfCalls(t, "Distribution", 2)

	proc.processEvents(workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Type: workloadmeta.EventTypeSet, Entity: makeReadyRunningPod(uid, "ns", createdAt, now, now.Add(-2*time.Second))}},
		Ch:     make(chan struct{}),
	})
	sender.AssertNumberOfCalls(t, "Distribution", 2) // unchanged
}

// TestCommitViaGoroutine verifies that the start goroutine calls Commit on
// context cancellation.
func TestCommitViaGoroutine(t *testing.T) {
	proc, sender, _ := newTestProcessor(t, "goroutine_commit")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // trigger immediate Commit on ctx.Done()

	proc.start(ctx, time.Hour)

	sender.AssertCalled(t, "Commit")
}
