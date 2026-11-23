// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func podGroup(created time.Time) *workloadmeta.KubernetesKAIPodGroup {
	return &workloadmeta.KubernetesKAIPodGroup{
		EntityID:          workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKAIPodGroup, ID: "ns/pg"},
		EntityMeta:        workloadmeta.EntityMeta{Name: "pg", Namespace: "ns", UID: "uid-1"},
		QueueName:         "research",
		PriorityClassName: "train",
		CreationTimestamp: created,
	}
}

func unschedulable(nodePool string, at time.Time, reasons ...workloadmeta.KAISchedulingExplanation) workloadmeta.KAISchedulingCondition {
	return workloadmeta.KAISchedulingCondition{
		Type: kaiUnschedulableCondition, Status: "True", NodePool: nodePool, LastTransitionTime: at, Explanations: reasons,
	}
}

func newTestKAI(t *testing.T, evictEvents []corev1.Event) (*kaiEvents, *collected) {
	t.Helper()
	out := &collected{}
	kai := newKAIEvents(newTestIndex(), started, out.submit, func(string) ([]corev1.Event, error) {
		return evictEvents, nil
	})
	kai.now = func() time.Time { return after(1000) }
	return kai, out
}

func TestKAIPodGroupThatExistedBeforeStartIsOnlyRecorded(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	existing := podGroup(started.Add(-time.Hour))
	existing.LastStartTime = after(-60)
	kai.set(existing, true)
	assert.Empty(t, out.events)

	restarted := podGroup(started.Add(-time.Hour))
	restarted.LastStartTime = after(20)
	kai.set(restarted, true)
	assert.Equal(t, []string{"kai.podgroup.admitted"}, out.types())
}

func TestKAICreatedAndPendingWithReasons(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(after(5))
	pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{
		unschedulable("gpu", after(6),
			workloadmeta.KAISchedulingExplanation{Reason: "OverLimit", Message: "queue limit reached"},
			workloadmeta.KAISchedulingExplanation{Reason: "NodeSelector", Message: "no matching node"},
		),
	}
	kai.set(pg, true)

	require.Equal(t, []string{"kai.podgroup.created", "kai.podgroup.pending"}, out.types())
	pending := out.events[1]
	assert.Equal(t, event.AlertTypeWarning, pending.AlertType)
	assert.Equal(t, "PodGroup ns/pg pending. queue limit reached no matching node", pending.Text)
	assert.Equal(t, "KAI PodGroup ns/pg pending", pending.Title)
	assert.Equal(t, "kai", pending.SourceTypeName)
	for _, tag := range []string{
		"kube_namespace:ns", tags.Scheduler + ":kai", tags.SchedulerWorkload + ":pg", tags.SchedulerQueue + ":research",
		"kai_pod_group:pg", "kai_pod_group_uid:uid-1", "kai_queue:research",
		"kai_nodepool:gpu", "kai_pod_group_priority_class:train", "kai_transition:pending",
		"kai_pending_reason:OverLimit", "kai_pending_reason:NodeSelector", "kai_pod_group_event_index:" + strconv.Itoa(firstIndex+1),
	} {
		requireTag(t, pending, tag)
	}
}

func TestKAIPendingIsNotRepeatedWhileTheMessageChanges(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(started.Add(-time.Hour))
	kai.set(pg, true)

	pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{unschedulable("gpu", after(6))}
	kai.set(pg, true)
	require.Len(t, out.events, 1)

	// KAI resets the transition time when only the message changes.
	pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{unschedulable("gpu", after(9))}
	kai.set(pg, true)
	assert.Len(t, out.events, 1)
}

func TestKAIPendingAgainAfterANewStart(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(started.Add(-time.Hour))
	pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{unschedulable("gpu", after(-100))}
	kai.set(pg, true)

	pg.LastStartTime = after(10)
	pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{unschedulable("gpu", after(20))}
	kai.set(pg, true)

	// The condition came back at 20s, after the start at 10s, so the events follow that order.
	assert.Equal(t, []string{"kai.podgroup.admitted", "kai.podgroup.pending"}, out.types())
}

func TestKAIAdmittedWaitCountsFromCreation(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(after(5))
	kai.set(pg, true)
	pg.LastStartTime = after(45)
	kai.set(pg, true)

	require.Equal(t, []string{"kai.podgroup.created", "kai.podgroup.admitted"}, out.types())
	assert.Equal(t, "PodGroup ns/pg admitted. Queued wait time was 40s.", out.events[1].Text)
}

func TestKAIAdmittedAfterEvictionWaitsFromTheEviction(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(started.Add(-time.Hour))
	pg.LastStartTime = after(-300)
	kai.set(pg, true)

	pg.LastEvictionTime = after(10)
	pg.LastStartTime = after(70)
	kai.set(pg, true)

	require.Equal(t, []string{"kai.podgroup.evicted", "kai.podgroup.admitted"}, out.types())
	assert.Equal(t, "PodGroup ns/pg admitted. Queued wait time was 60s.", out.events[1].Text)
}

func evictEvent(uid string, at time.Time, annotations map[string]string, message string) corev1.Event {
	return corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Annotations: annotations},
		InvolvedObject: corev1.ObjectReference{UID: k8stypes.UID(uid)},
		LastTimestamp:  metav1.NewTime(at),
		Message:        message,
	}
}

func TestKAIEvictedWithDetailsFromTheEvictEvent(t *testing.T) {
	annotations := map[string]string{
		kaiEvictorActionAnnotation:    "preempt",
		kaiEvictorNameAnnotation:      "big-job",
		kaiEvictorNamespaceAnnotation: "other",
	}
	kai, out := newTestKAI(t, []corev1.Event{
		evictEvent("uid-1", after(100), annotations, "Preempted by big-job"), // belongs to an earlier eviction
		evictEvent("uid-2", after(20), annotations, "Another PodGroup"),      // another PodGroup
		evictEvent("uid-1", after(20), nil, "Combined event"),                // without KAI annotations
		evictEvent("uid-1", after(20), annotations, "Evicted to make room."), // the match
	})

	pg := podGroup(started.Add(-time.Hour))
	kai.set(pg, true)
	pg.LastEvictionTime = after(20)
	kai.set(pg, true)

	require.Len(t, out.events, 1)
	e := out.events[0]
	assert.Equal(t, "PodGroup ns/pg evicted. Evicted to make room. Eviction action: preempt.", e.Text)
	requireTag(t, e, "kai_eviction_action:preempt")
	requireTag(t, e, "kai_evicted_by:other/big-job")
}

func TestKAIEvictedWithoutMatchingEvictEvent(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(started.Add(-time.Hour))
	kai.set(pg, true)
	pg.LastEvictionTime = after(20)
	kai.set(pg, true)

	require.Len(t, out.events, 1)
	assert.Equal(t, "PodGroup ns/pg evicted.", out.events[0].Text)
	rejectTagPrefix(t, out.events[0], "kai_eviction_action")
}

func TestKAIEvictEventListFailureDoesNotStopTheEvent(t *testing.T) {
	out := &collected{}
	kai := newKAIEvents(newTestIndex(), started, out.submit, func(string) ([]corev1.Event, error) {
		return nil, errors.New("forbidden")
	})

	pg := podGroup(started.Add(-time.Hour))
	kai.set(pg, true)
	pg.LastEvictionTime = after(20)
	kai.set(pg, true)

	require.Len(t, out.events, 1)
	assert.Equal(t, "kai.podgroup.evicted", out.events[0].EventType)
}

func TestKAIFollowerRecordsStateWithoutEvents(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(after(5))
	pg.LastStartTime = after(10)
	kai.set(pg, false)
	assert.Empty(t, out.events)

	kai.set(pg, true)
	assert.Empty(t, out.events)

	pg.LastEvictionTime = after(30)
	kai.set(pg, true)
	assert.Equal(t, []string{"kai.podgroup.evicted"}, out.types())
}

func TestKAIDefaultNodePoolFromLabel(t *testing.T) {
	kai, out := newTestKAI(t, nil)

	pg := podGroup(after(5))
	pg.Labels = map[string]string{kaiNodePoolLabel: "pool-b"}
	kai.set(pg, true)

	require.Len(t, out.events, 1)
	requireTag(t, out.events[0], "kai_nodepool:pool-b")

	other := podGroup(after(6))
	other.UID = "uid-2"
	kai.set(other, true)
	requireTag(t, out.events[1], "kai_nodepool:default")
}

func TestKAIUnsetForgetsThePodGroup(t *testing.T) {
	kai, out := newTestKAI(t, nil)
	kai.set(podGroup(after(5)), true)
	require.Len(t, out.events, 1)

	kai.unset("uid-1")

	assert.NotContains(t, kai.states, "uid-1")
	assert.NotContains(t, kai.index.last, "uid-1")
}

func TestKAIPendingOnSeveralNodePoolsIsSentInNodePoolOrder(t *testing.T) {
	// Map iteration order is random, so repeat to catch an unsorted result.
	for i := 0; i < 20; i++ {
		kai, out := newTestKAI(t, nil)
		pg := podGroup(after(5))
		pg.SchedulingConditions = []workloadmeta.KAISchedulingCondition{
			unschedulable("pool-c", after(6)),
			unschedulable("pool-a", after(6)),
			unschedulable("pool-b", after(6)),
		}

		kai.set(pg, true)

		require.Len(t, out.events, 4)
		var pools []string
		for _, e := range out.events[1:] {
			for _, tag := range e.Tags {
				if strings.HasPrefix(tag, "kai_nodepool:") {
					pools = append(pools, strings.TrimPrefix(tag, "kai_nodepool:"))
				}
			}
		}
		require.Equal(t, []string{"pool-a", "pool-b", "pool-c"}, pools)
	}
}

func TestKAIPodGroupCreatedInTheStartSecondIsReported(t *testing.T) {
	// Creation timestamps have a precision of one second, so a PodGroup created
	// after the check started in the same second looks as old as the start.
	out := &collected{}
	startedInTheSecond := started.Add(700 * time.Millisecond)
	kai := newKAIEvents(newTestIndex(), startedInTheSecond, out.submit, func(string) ([]corev1.Event, error) { return nil, nil })

	kai.set(podGroup(started), true)
	require.Equal(t, []string{"kai.podgroup.created"}, out.types())

	// One created in the second before the start still counts as existing.
	before := podGroup(started.Add(-time.Second))
	before.UID = "uid-2"
	kai.set(before, true)
	assert.Len(t, out.events, 1)
}
