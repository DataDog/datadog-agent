// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func kueueWorkload(created time.Time, conditions ...workloadmeta.KueueWorkloadCondition) *workloadmeta.KubernetesKueueWorkload {
	return &workloadmeta.KubernetesKueueWorkload{
		EntityID:          workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "team-a/job-1"},
		EntityMeta:        workloadmeta.EntityMeta{Name: "job-1", Namespace: "team-a", UID: "uid-1"},
		QueueName:         "local",
		CreationTimestamp: created,
		Conditions:        conditions,
	}
}

func kueueCondition(conditionType, status, reason string, at time.Time) workloadmeta.KueueWorkloadCondition {
	return workloadmeta.KueueWorkloadCondition{Type: conditionType, Status: status, Reason: reason, LastTransitionTime: at}
}

func newTestKueue(t *testing.T, tagger fakeTagger) (*kueueEvents, *collected, memoryCache) {
	t.Helper()
	cache := memoryCache{}
	out := &collected{}
	kueue := newKueueEvents(tagger, newTestIndex(cache, "kueue"), started, out.submit)
	kueue.now = func() time.Time { return after(1000) }
	return kueue, out, cache
}

func TestKueueWorkloadThatExistedBeforeStartIsOnlyRecorded(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)

	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("Admitted", "True", "Admitted", after(-60))), true)
	assert.Empty(t, out.events)

	// A later change of the same Workload is reported.
	kueue.set(kueueWorkload(started.Add(-time.Hour),
		kueueCondition("Admitted", "True", "Admitted", after(-60)),
		kueueCondition("Finished", "True", "Succeeded", after(30)),
	), true)
	assert.Equal(t, []string{"kueue.workload.finished"}, out.types())
}

func TestKueueNewWorkloadSendsCreatedThenTransitionsInOrder(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)

	kueue.set(kueueWorkload(after(5),
		kueueCondition("Admitted", "True", "Admitted", after(12)),
		kueueCondition("QuotaReserved", "True", "QuotaReserved", after(10)),
	), true)

	assert.Equal(t, []string{"kueue.workload.created", "kueue.workload.quota_reserved", "kueue.workload.admitted"}, out.types())
	assert.Equal(t, after(5).Unix(), out.events[0].Ts)
	assert.Equal(t, after(12).Unix(), out.events[2].Ts)
	assert.Equal(t, event.AlertTypeInfo, out.events[2].AlertType)
	assert.Equal(t, "Kueue workload team-a/job-1 admitted", out.events[2].Title)
	assert.Equal(t, "Workload team-a/job-1 admitted. Queued wait time was 7s.", out.events[2].Text)
	assert.Equal(t, "uid-1", out.events[2].AggregationKey)
	assert.Equal(t, "kueue", out.events[2].SourceTypeName)
}

func TestKueueUnchangedConditionIsNotReportedAgain(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)
	workload := kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10)))

	kueue.set(workload, true)
	require.Len(t, out.events, 2)

	kueue.set(workload, true)
	assert.Len(t, out.events, 2)
}

func TestKueueFollowerRecordsStateWithoutEvents(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)

	kueue.set(kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10))), false)
	assert.Empty(t, out.events)

	// Once it is the leader, only what changed after is reported.
	kueue.set(kueueWorkload(after(5),
		kueueCondition("Admitted", "True", "Admitted", after(10)),
		kueueCondition("PodsReady", "True", "PodsReady", after(20)),
	), true)
	assert.Equal(t, []string{"kueue.workload.running"}, out.types())
}

func TestKueuePendingOnlyForWaitingReasons(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)

	kueue.set(kueueWorkload(started.Add(-time.Hour)), true)
	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "Other", after(5))), true)
	assert.Empty(t, out.events)

	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "Inadmissible", after(6))), true)
	require.Len(t, out.events, 1)
	assert.Equal(t, "kueue.workload.pending", out.events[0].EventType)
	assert.Equal(t, event.AlertTypeWarning, out.events[0].AlertType)
	requireTag(t, out.events[0], "kueue_pending_reason:Inadmissible")
}

func TestKueueEvictedByPreemption(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)

	admitted := kueueWorkload(started.Add(-time.Hour), kueueCondition("Admitted", "True", "Admitted", after(-60)))
	admitted.ClusterQueueName = "team-a-gpu"
	kueue.set(admitted, true)

	evicted := kueueWorkload(started.Add(-time.Hour),
		workloadmeta.KueueWorkloadCondition{
			Type: "Evicted", Status: "True", Reason: "Preempted", LastTransitionTime: after(30),
			Message: "Preempted to accommodate a workload (UID: abc-123, JobUID: j) due to prioritization",
		},
		workloadmeta.KueueWorkloadCondition{Type: "Preempted", Status: "True", Reason: "InClusterQueue", LastTransitionTime: after(30)},
	)
	// The eviction clears the admission, so the queue comes from the state before.
	kueue.set(evicted, true)

	require.Len(t, out.events, 1)
	e := out.events[0]
	assert.Equal(t, "kueue.workload.evicted", e.EventType)
	assert.Contains(t, e.Text, "Eviction reason: Preempted.")
	assert.Contains(t, e.Text, "Preemption reason: InClusterQueue.")
	for _, tag := range []string{
		tags.SchedulerQueue + ":team-a-gpu", "kueue_cluster_queue:team-a-gpu", "kueue_eviction_reason:Preempted",
		"kueue_preemption_reason:InClusterQueue", "kueue_preempted_by:abc-123", "kueue_transition:evicted",
	} {
		requireTag(t, e, tag)
	}
}

func TestKueueEventTagsIncludeThoseOfTheTagger(t *testing.T) {
	tagger := fakeTagger{
		"kueue_workload://team-a/job-1":                    {"kueue_workload_tag:from-tagger"},
		"kubernetes_kueue_queue://clusterqueue//cq":        {"cluster_queue_tag:from-tagger"},
		"kubernetes_kueue_queue://localqueue/team-a/local": {"local_queue_tag:from-tagger"},
	}
	kueue, out, _ := newTestKueue(t, tagger)
	priority := int32(7)

	workload := kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10)))
	workload.ClusterQueueName = "cq"
	workload.Priority = &priority
	workload.PriorityClassName = "high"
	kueue.set(workload, true)

	require.NotEmpty(t, out.events)
	e := out.events[0]
	for _, tag := range []string{
		"kube_namespace:team-a", tags.Scheduler + ":kueue", tags.SchedulerWorkload + ":job-1", "kueue_workload:job-1", "kueue_workload_uid:uid-1", "kueue_local_queue:local",
		"kueue_workload_priority:7", "kueue_workload_priority_class:high",
		"workload_tag:from-tagger",
	} {
		if tag == "workload_tag:from-tagger" {
			tag = "kueue_workload_tag:from-tagger"
		}
		requireTag(t, e, tag)
	}
	requireTag(t, e, "cluster_queue_tag:from-tagger")
	requireTag(t, e, "local_queue_tag:from-tagger")
}

func TestKueueEventIndexCountsAndPersists(t *testing.T) {
	kueue, out, cache := newTestKueue(t, nil)

	kueue.set(kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10))), true)
	require.Len(t, out.events, 2)
	requireTag(t, out.events[0], "kueue_workload_event_index:"+strconv.Itoa(firstIndex))
	requireTag(t, out.events[1], "kueue_workload_event_index:"+strconv.Itoa(firstIndex+1))

	kueue.index.persist()
	assert.JSONEq(t, fmt.Sprintf(`{"uid-1": %d}`, firstIndex+2), cache["kueue"])

	// After a restart the count continues.
	restarted, out2, _ := newTestKueue(t, nil)
	restarted.index = newTestIndex(cache, "kueue")
	restarted.set(kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10))), true)
	requireTag(t, out2.events[0], "kueue_workload_event_index:"+strconv.Itoa(firstIndex+2))
}

func TestKueueUnsetForgetsTheWorkload(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)
	kueue.set(kueueWorkload(after(5)), true)
	require.Len(t, out.events, 1)

	kueue.unset("uid-1")

	assert.NotContains(t, kueue.states, "uid-1")
	assert.NotContains(t, kueue.index.counters, "uid-1")
}

func TestKueueWorkloadWithoutUIDIsSkipped(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)
	workload := kueueWorkload(after(5))
	workload.UID = ""

	kueue.set(workload, true)

	assert.Empty(t, out.events)
}

func TestEventIndexStartsFromTheClockWhenNothingIsPersisted(t *testing.T) {
	index := newTestIndex(memoryCache{}, "key")

	assert.Equal(t, firstIndex, index.next("uid-1"))
	assert.Equal(t, firstIndex+1, index.next("uid-1"))

	// A later leader without the count starts above everything the earlier one sent.
	later := newTestIndex(memoryCache{}, "key")
	later.now = func() time.Time { return started.Add(time.Hour) }
	assert.Greater(t, later.next("uid-1"), firstIndex+1)
}

func TestEventIndexContinuesFromThePersistedCount(t *testing.T) {
	cache := memoryCache{"key": `{"uid-1": 7}`}
	index := newTestIndex(cache, "key")

	assert.Equal(t, 7, index.next("uid-1"))
}

func TestKueueEventIsSentWithoutEntityTagsWhenTheTaggerFails(t *testing.T) {
	kueue, out, _ := newTestKueue(t, nil)
	kueue.tagger = failingTagger{err: errors.New("tagger not ready")}

	kueue.set(kueueWorkload(after(5)), true)

	require.Len(t, out.events, 1)
	requireTag(t, out.events[0], "kueue_workload:job-1")
	assert.Equal(t, "tagger not ready", kueue.lastTagErr)

	// Once the tagger works again the failure is forgotten, so that a new one is a warning.
	kueue.tagger = fakeTagger{}
	kueue.set(kueueWorkload(after(5), kueueCondition("Finished", "True", "Succeeded", after(30))), true)
	assert.Empty(t, kueue.lastTagErr)
}
