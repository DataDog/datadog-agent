// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	kubernetesresourceparsers "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util/kubernetes_resource_parsers"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func kueueWorkload(created time.Time, conditions ...workloadCondition) *kueueView {
	return &kueueView{
		KubernetesKueueWorkload: &workloadmeta.KubernetesKueueWorkload{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueWorkload, ID: "team-a/job-1"},
			EntityMeta: workloadmeta.EntityMeta{Name: "job-1", Namespace: "team-a", UID: "uid-1"},
			QueueName:  "local",
		},
		KueueWorkloadEvents: &kubernetesresourceparsers.KueueWorkloadEvents{
			CreationTimestamp: created,
			Conditions:        conditions,
		},
	}
}

func kueueCondition(conditionType, status, reason string, at time.Time) workloadCondition {
	return workloadCondition{Type: conditionType, Status: status, Reason: reason, LastTransitionTime: at}
}

// newTestKueue returns the events of Kueue Workloads with a mock tagger. The
// optional setup sets the tags the tagger has.
func newTestKueue(t *testing.T, setup func(taggermock.Mock)) (*kueueEvents, *collected) {
	t.Helper()
	out := &collected{}
	kueue := newKueueEvents(newTestTagger(t, setup), newTestIndex(), started, out.submit)
	kueue.now = func() time.Time { return after(1000) }
	return kueue, out
}

func TestKueueWorkloadThatExistedBeforeStartIsOnlyRecorded(t *testing.T) {
	kueue, out := newTestKueue(t, nil)

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
	kueue, out := newTestKueue(t, nil)

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
	kueue, out := newTestKueue(t, nil)
	workload := kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10)))

	kueue.set(workload, true)
	require.Len(t, out.events, 2)

	kueue.set(workload, true)
	assert.Len(t, out.events, 2)
}

func TestKueueFollowerRecordsStateWithoutEvents(t *testing.T) {
	kueue, out := newTestKueue(t, nil)

	kueue.set(kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10))), false)
	assert.Empty(t, out.events)

	// Once it is the leader, only what changed after is reported.
	kueue.set(kueueWorkload(after(5),
		kueueCondition("Admitted", "True", "Admitted", after(10)),
		kueueCondition("PodsReady", "True", "PodsReady", after(20)),
	), true)
	assert.Equal(t, []string{"kueue.workload.running"}, out.types())
}

// kueueWorkloadFromCluster is the Workload that Kueue v0.20.7 requeued after a
// preemption (/tmp/kind-validation/workloads-final.yaml of a real kind cluster).
func kueueWorkloadFromCluster(created time.Time) *kueueView {
	return kueueWorkload(created,
		workloadCondition{
			Type: "QuotaReserved", Status: "False", Reason: "WaitingForQuota", LastTransitionTime: after(40),
			Message: "couldn't assign flavors to pod set main: insufficient unused quota for cpu in flavor default-flavor, 1 more needed",
		},
		workloadCondition{Type: "Evicted", Status: "True", Reason: "Preempted", LastTransitionTime: after(39)},
		workloadCondition{Type: "Admitted", Status: "False", Reason: "NoReservation", LastTransitionTime: after(40)},
	)
}

func TestKueuePendingForAnyWaitingReason(t *testing.T) {
	// Reasons used by Kueue over its versions for QuotaReserved=False.
	for _, reason := range []string{"WaitingForQuota", "Inadmissible", "Pending", "NoMatchingFlavor", "ExceedsMaxQuota", "WaitingForPreemptedWorkloads", "Misconfigured"} {
		t.Run(reason, func(t *testing.T) {
			kueue, out := newTestKueue(t, nil)
			kueue.set(kueueWorkload(started.Add(-time.Hour)), true)
			kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", reason, after(6))), true)
			require.Equal(t, []string{"kueue.workload.pending"}, out.types())
			assert.Equal(t, event.AlertTypeWarning, out.events[0].AlertType)
			requireTag(t, out.events[0], "kueue_pending_reason:"+reason)
		})
	}
}

func TestKueueWorkloadNotEvaluatedYetIsNotPending(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(started.Add(-time.Hour)), true)
	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "PendingEvaluation", after(5))), true)
	assert.Empty(t, out.events)

	// Once evaluated, it waits and the event is sent once.
	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "WaitingForQuota", after(6))), true)
	assert.Equal(t, []string{"kueue.workload.pending"}, out.types())
}

func TestKueueWaitingWorkloadSendsOnePendingEvent(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(after(5), kueueCondition("QuotaReserved", "False", "Pending", after(6))), true)
	require.Equal(t, []string{"kueue.workload.created", "kueue.workload.pending"}, out.types())

	// Updates of the wait: a new reason, message and time, still one wait.
	kueue.set(kueueWorkload(after(5), kueueCondition("QuotaReserved", "False", "WaitingForQuota", after(8))), true)
	kueue.set(kueueWorkload(after(5), kueueCondition("QuotaReserved", "False", "NoMatchingFlavor", after(9))), true)
	kueue.set(kueueWorkload(after(5), kueueCondition("QuotaReserved", "False", "NoMatchingFlavor", after(9))), true)
	assert.Equal(t, []string{"kueue.workload.created", "kueue.workload.pending"}, out.types())
}

func TestKueuePreemptedWorkloadThatIsRequeuedIsPending(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(after(5),
		kueueCondition("QuotaReserved", "True", "QuotaReserved", after(6)),
		kueueCondition("Admitted", "True", "Admitted", after(6)),
	), true)
	require.Equal(t, []string{"kueue.workload.created", "kueue.workload.quota_reserved", "kueue.workload.admitted"}, out.types())
	out.events = nil

	kueue.set(kueueWorkloadFromCluster(after(5)), true)
	assert.Equal(t, []string{"kueue.workload.evicted", "kueue.workload.pending"}, out.types())
	requireTag(t, out.events[1], "kueue_pending_reason:WaitingForQuota")

	// Quota is reserved again, and the Workload waits again later: a new wait.
	out.events = nil
	kueue.set(kueueWorkload(after(5), kueueCondition("QuotaReserved", "True", "QuotaReserved", after(50))), true)
	assert.Equal(t, []string{"kueue.workload.quota_reserved"}, out.types())
	out.events = nil
	kueue.set(kueueWorkloadFromCluster(after(5)), true)
	assert.Contains(t, out.types(), "kueue.workload.pending")
}

func TestKueueFollowerDoesNotRepeatThePendingOfTheLeader(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(started.Add(-time.Hour)), false)
	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "WaitingForQuota", after(6))), false)
	kueue.set(kueueWorkload(started.Add(-time.Hour), kueueCondition("QuotaReserved", "False", "WaitingForQuota", after(7))), true)
	assert.Empty(t, out.events)
}

// TestKueueRunningIsNotRepeatedWhenTheWorkloadFinishes uses what Kueue v0.20.7
// does when a Job finishes (recorded with a watch on a real cluster): PodsReady
// goes True/Started, False/WaitForRecovery and True/Recovered, then Finished.
func TestKueueRunningIsNotRepeatedWhenTheWorkloadFinishes(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	admitted := []workloadCondition{
		kueueCondition("QuotaReserved", "True", "QuotaReserved", after(6)),
		kueueCondition("Admitted", "True", "Admitted", after(6)),
	}
	with := func(extra ...workloadCondition) *kueueView {
		return kueueWorkload(after(5), append(append([]workloadCondition{}, admitted...), extra...)...)
	}
	kueue.set(with(kueueCondition("PodsReady", "False", "WaitForStart", after(6))), true)
	kueue.set(with(kueueCondition("PodsReady", "True", "Started", after(7))), true)
	require.Equal(t, []string{
		"kueue.workload.created", "kueue.workload.quota_reserved", "kueue.workload.admitted", "kueue.workload.running",
	}, out.types())
	out.events = nil

	kueue.set(with(kueueCondition("PodsReady", "False", "WaitForRecovery", after(23))), true)
	kueue.set(with(kueueCondition("PodsReady", "True", "Recovered", after(24))), true)
	assert.Empty(t, out.events)
	kueue.set(with(kueueCondition("PodsReady", "True", "Recovered", after(24)), kueueCondition("Finished", "True", "Succeeded", after(24))), true)
	assert.Equal(t, []string{"kueue.workload.finished"}, out.types())
}

func TestKueueRunningAgainAfterTheWorkloadIsEvictedAndRestarted(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(after(5), kueueCondition("PodsReady", "True", "Started", after(10))), true)
	kueue.set(kueueWorkload(after(5), kueueCondition("PodsReady", "False", "WaitForRecovery", after(15))), true)
	kueue.set(kueueWorkload(after(5), kueueCondition("PodsReady", "True", "Recovered", after(16))), true)
	require.Equal(t, 1, countType(out.types(), "kueue.workload.running"))
	out.events = nil

	// Evicted, requeued and admitted again: the pods start again.
	kueue.set(kueueWorkload(after(5), kueueCondition("PodsReady", "False", "WaitForStart", after(20))), true)
	assert.Empty(t, out.events)
	kueue.set(kueueWorkload(after(5), kueueCondition("PodsReady", "True", "Started", after(30))), true)
	assert.Equal(t, []string{"kueue.workload.running"}, out.types())
}

func countType(types []string, want string) int {
	n := 0
	for _, t := range types {
		if t == want {
			n++
		}
	}
	return n
}

func TestKueueEvictedByPreemption(t *testing.T) {
	kueue, out := newTestKueue(t, nil)

	admitted := kueueWorkload(started.Add(-time.Hour), kueueCondition("Admitted", "True", "Admitted", after(-60)))
	admitted.ClusterQueueName = "team-a-gpu"
	kueue.set(admitted, true)

	evicted := kueueWorkload(started.Add(-time.Hour),
		workloadCondition{
			Type: "Evicted", Status: "True", Reason: "Preempted", LastTransitionTime: after(30),
			Message: "Preempted to accommodate a workload (UID: abc-123, JobUID: j) due to prioritization",
		},
		workloadCondition{Type: "Preempted", Status: "True", Reason: "InClusterQueue", LastTransitionTime: after(30)},
	)
	// The eviction clears the admission, so the queue comes from the state before.
	kueue.set(evicted, true)

	require.Len(t, out.events, 1)
	e := out.events[0]
	assert.Equal(t, "kueue.workload.evicted", e.EventType)
	assert.Contains(t, e.Text, "due to prioritization. Eviction reason: Preempted.")
	assert.Contains(t, e.Text, "Preemption reason: InClusterQueue.")
	for _, tag := range []string{
		tags.SchedulerQueue + ":team-a-gpu", "kueue_cluster_queue:team-a-gpu", "kueue_eviction_reason:Preempted",
		"kueue_preemption_reason:InClusterQueue", "kueue_preempted_by:abc-123", "kueue_transition:evicted",
	} {
		requireTag(t, e, tag)
	}
}

func TestKueueEventTagsIncludeThoseOfTheTagger(t *testing.T) {
	kueue, out := newTestKueue(t, func(tagger taggermock.Mock) {
		tagger.SetTags(types.NewEntityID(types.KueueWorkload, "team-a/job-1"), "test", []string{"kueue_workload_tag:from-tagger"}, nil, nil, nil)
		tagger.SetTags(types.NewEntityID(types.KubernetesKueueQueue, "clusterqueue//cq"), "test", []string{"cluster_queue_tag:from-tagger"}, nil, nil, nil)
		tagger.SetTags(types.NewEntityID(types.KubernetesKueueQueue, "localqueue/team-a/local"), "test", []string{"local_queue_tag:from-tagger"}, nil, nil, nil)
	})
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

func TestKueueEventIndexCountsPerWorkload(t *testing.T) {
	kueue, out := newTestKueue(t, nil)

	kueue.set(kueueWorkload(after(5), kueueCondition("Admitted", "True", "Admitted", after(10))), true)
	require.Len(t, out.events, 2)
	requireTag(t, out.events[0], "kueue_workload_event_index:"+strconv.Itoa(firstIndex))
	requireTag(t, out.events[1], "kueue_workload_event_index:"+strconv.Itoa(firstIndex+1))
}

func TestKueueWorkloadCreatedInTheStartSecondIsReported(t *testing.T) {
	// Creation timestamps have a precision of one second, so a Workload created
	// after the check started in the same second looks as old as the start.
	out := &collected{}
	startedInTheSecond := started.Add(700 * time.Millisecond)
	kueue := newKueueEvents(newTestTagger(t, nil), newTestIndex(), startedInTheSecond, out.submit)
	kueue.now = func() time.Time { return after(1000) }

	kueue.set(kueueWorkload(started), true)
	require.Equal(t, []string{"kueue.workload.created"}, out.types())

	// One created in the second before the start still counts as existing.
	before := kueueWorkload(started.Add(-time.Second))
	before.UID = "uid-2"
	kueue.set(before, true)
	assert.Len(t, out.events, 1)
}

func TestKueueUnsetForgetsTheWorkload(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.set(kueueWorkload(after(5)), true)
	require.Len(t, out.events, 1)

	kueue.unset("uid-1")

	assert.NotContains(t, kueue.states, "uid-1")
	assert.NotContains(t, kueue.index.last, "uid-1")
}

func TestKueueWorkloadWithoutUIDIsSkipped(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	workload := kueueWorkload(after(5))
	workload.UID = ""

	kueue.set(workload, true)

	assert.Empty(t, out.events)
}

func TestEventIndexStartsFromTheClock(t *testing.T) {
	index := newTestIndex()

	assert.Equal(t, firstIndex, index.next("uid-1"))
	assert.Equal(t, firstIndex+1, index.next("uid-1"))
	assert.Equal(t, firstIndex, index.next("uid-2"))
}

func TestEventIndexFollowsTheClockAfterAGap(t *testing.T) {
	index := newTestIndex()
	assert.Equal(t, firstIndex, index.next("uid-1"))

	// A later event is above anything sent earlier, also by another leader.
	index.now = func() time.Time { return started.Add(time.Hour) }
	assert.Equal(t, firstIndex+3600, index.next("uid-1"))
}

func TestEventIndexForgetStartsTheWorkloadAgain(t *testing.T) {
	index := newTestIndex()
	index.next("uid-1")
	index.next("uid-1")

	index.forget("uid-1")

	assert.Equal(t, firstIndex, index.next("uid-1"))
}

func TestKueueEventIsSentWithoutEntityTagsWhenTheTaggerFails(t *testing.T) {
	kueue, out := newTestKueue(t, nil)
	kueue.tagger = failingTagger{err: errors.New("tagger not ready")}

	kueue.set(kueueWorkload(after(5)), true)

	require.Len(t, out.events, 1)
	requireTag(t, out.events[0], "kueue_workload:job-1")
}
