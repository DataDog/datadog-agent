// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/common"
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kueueSourceTypeName = "kueue"
	kueueSchedulerValue = "kueue"

	kueueTransitionCreated = "created"
	kueueConditionPreempt  = "Preempted"
)

// kueueTransition is a change of a Workload condition that is reported as an event.
type kueueTransition struct {
	name          string
	conditionType string
	status        string
	label         string
	alertType     event.AlertType
}

// kueueTransitions is in the order in which events of a Workload are sent when
// several conditions change at once.
var kueueTransitions = []kueueTransition{
	{"quota_reserved", "QuotaReserved", "True", "quota reserved", event.AlertTypeInfo},
	{"admitted", "Admitted", "True", "admitted", event.AlertTypeInfo},
	{"running", "PodsReady", "True", "running", event.AlertTypeInfo},
	{"evicted", "Evicted", "True", "evicted", event.AlertTypeWarning},
	{"finished", "Finished", "True", "finished", event.AlertTypeInfo},
	{"pending", "QuotaReserved", "False", "pending", event.AlertTypeWarning},
}

// kueueNotEvaluatedReasons are the reasons of an unreserved quota that mean the
// scheduler has not looked at the Workload yet ("PendingEvaluation" in recent
// Kueue versions), so it is not waiting for anything and the "created" event
// covers it.
//
// Any other reason is a Workload that is waiting. Kueue has used several over
// its versions, and keeps adding some: "Inadmissible" (queue missing or inactive),
// "Pending" (deprecated, also used when a Workload is requeued), "WaitingForQuota"
// (v0.20.7), "NoMatchingFlavor", "ExceedsMaxQuota", "WaitingForPreemptedWorkloads",
// "Misconfigured", "Suspended", "WaitingForPodsReady", and others. Listing them
// would silently drop the next one, so the check does not.
var kueueNotEvaluatedReasons = map[string]struct{}{"PendingEvaluation": {}}

var kueuePreemptingWorkloadUID = regexp.MustCompile(`(?i)\bworkload \(UID: ([^),\s]+)`)

// kueueState is what is remembered of a Workload to tell what changed in its
// next update.
type kueueState struct {
	conditions   map[string]workloadCondition
	clusterQueue string
	// pendingReported is true while the Workload is waiting for quota and its
	// "pending" event was sent (or would have been, on a follower). The reason of
	// the wait changes while it lasts, and it must not send one event for each.
	pendingReported bool
}

func newKueueState(w *kueueView) kueueState {
	state := kueueState{
		conditions:   make(map[string]workloadCondition, len(w.Conditions)),
		clusterQueue: w.ClusterQueueName,
	}
	for _, condition := range w.Conditions {
		state.conditions[condition.Type] = condition
	}
	return state
}

// tagLookup is the part of the tagger the events need.
type tagLookup interface {
	Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error)
}

// kueueEvents turns the updates of Kueue Workloads into events.
type kueueEvents struct {
	tagger    tagLookup
	index     *eventIndex
	startedAt time.Time
	now       func() time.Time
	submit    func(event.Event)
	states    map[string]kueueState
}

func newKueueEvents(tagger tagLookup, index *eventIndex, startedAt time.Time, submit func(event.Event)) *kueueEvents {
	return &kueueEvents{
		tagger:    tagger,
		index:     index,
		startedAt: startedAt.Truncate(time.Second),
		now:       time.Now,
		submit:    submit,
		states:    map[string]kueueState{},
	}
}

// set handles an update of a Workload. The state is always recorded, but
// events are only sent when emit is true, so that a follower keeps up with the
// Workloads and does not report again what the leader reported.
//
// A Workload seen for the first time is only reported if it was not created
// before the second the check started in. Creation timestamps have a precision
// of one second, so the start is truncated to the second: a Workload created in
// the startup second is reported, and a rare duplicate is better than a miss. The
// ones that already existed are the state the check starts from.
func (k *kueueEvents) set(w *kueueView, emit bool) {
	uid := w.UID
	if uid == "" {
		log.Debugf("Skipping Kueue Workload without uid: %s/%s", w.Namespace, w.Name)
		return
	}

	previous, known := k.states[uid]
	current := newKueueState(w)
	pending := false
	if quota, found := current.conditions["QuotaReserved"]; found && quota.Status == "False" {
		_, notEvaluated := kueueNotEvaluatedReasons[quota.Reason]
		pending = !notEvaluated && !previous.pendingReported
		current.pendingReported = pending || previous.pendingReported
	}
	k.states[uid] = current
	if !emit {
		return
	}

	if !known {
		if w.CreationTimestamp.Before(k.startedAt) {
			return
		}
		k.send(kueueTransitionCreated, kueueTransitionCreated, event.AlertTypeInfo, w, nil, kueueState{})
	}

	for _, transition := range kueueTransitions {
		condition, found := current.conditions[transition.conditionType]
		if !found || condition.Status != transition.status {
			continue
		}
		previousCondition, had := previous.conditions[transition.conditionType]
		switch transition.name {
		case "pending":
			// One event per wait, whatever its reason does.
			if !pending {
				continue
			}
		case "running":
			// Kueue changes the reason and time of PodsReady while it stays True
			// ("Started" to "Recovered" when the Job finishes): not a new start.
			if had && previousCondition.Status == condition.Status {
				continue
			}
		default:
			if had && !conditionChanged(condition, previousCondition) {
				continue
			}
		}
		k.send(transition.name, transition.label, transition.alertType, w, &condition, previous)
	}
}

// unset forgets a deleted Workload.
func (k *kueueEvents) unset(uid string) {
	delete(k.states, uid)
	k.index.forget(uid)
}

func conditionChanged(condition, previous workloadCondition) bool {
	return condition.Status != previous.Status ||
		condition.Reason != previous.Reason ||
		!condition.LastTransitionTime.Equal(previous.LastTransitionTime)
}

func (k *kueueEvents) send(
	transition, label string,
	alertType event.AlertType,
	w *kueueView,
	condition *workloadCondition,
	previous kueueState,
) {
	key := w.UID
	eventTags := dedupeTags(k.eventTags(transition, w, condition, previous))
	eventTags = append(eventTags, "kueue_workload_event_index:"+strconv.Itoa(k.index.next(key)))

	k.submit(event.Event{
		Title:          fmt.Sprintf("Kueue workload %s/%s %s", w.Namespace, w.Name, label),
		Text:           k.eventText(transition, w, condition),
		Ts:             k.eventTimestamp(w, condition),
		Tags:           eventTags,
		AlertType:      alertType,
		AggregationKey: key,
		SourceTypeName: kueueSourceTypeName,
		EventType:      "kueue.workload." + transition,
	})
}

func (k *kueueEvents) eventTimestamp(w *kueueView, condition *workloadCondition) int64 {
	if condition != nil && !condition.LastTransitionTime.IsZero() {
		return condition.LastTransitionTime.Unix()
	}
	if !w.CreationTimestamp.IsZero() {
		return w.CreationTimestamp.Unix()
	}
	return k.now().Unix()
}

func (k *kueueEvents) eventText(transition string, w *kueueView, condition *workloadCondition) string {
	text := fmt.Sprintf("Workload %s/%s %s.", w.Namespace, w.Name, humanTransition(transition))
	if condition == nil {
		return text
	}

	if condition.Message != "" {
		text += " " + condition.Message
	}

	switch transition {
	case "admitted":
		if !w.CreationTimestamp.IsZero() && !condition.LastTransitionTime.IsZero() {
			waited := condition.LastTransitionTime.Sub(w.CreationTimestamp)
			text += fmt.Sprintf(" Queued wait time was %.0fs.", waited.Seconds())
		}
	case "evicted":
		if condition.Reason != "" {
			text += fmt.Sprintf(" Eviction reason: %s.", condition.Reason)
		}
		if preempted := findKueueCondition(w, kueueConditionPreempt); condition.Reason == "Preempted" && preempted != nil && preempted.Reason != "" {
			text += fmt.Sprintf(" Preemption reason: %s.", preempted.Reason)
		}
	case "finished":
		if condition.Reason != "" {
			text += fmt.Sprintf(" Finished reason: %s.", condition.Reason)
		}
	}
	return text
}

func humanTransition(transition string) string {
	return strings.ReplaceAll(transition, "_", " ")
}

// dedupeTags drops repeated tags, keeping the order of the first occurrences.
// The tags added by hand and the ones of the tagger can be the same.
func dedupeTags(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := in[:0:0]
	for _, tag := range in {
		if _, found := seen[tag]; found {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func findKueueCondition(w *kueueView, conditionType string) *workloadCondition {
	for i := range w.Conditions {
		if w.Conditions[i].Type == conditionType {
			return &w.Conditions[i]
		}
	}
	return nil
}

func (k *kueueEvents) eventTags(
	transition string,
	w *kueueView,
	condition *workloadCondition,
	previous kueueState,
) []string {
	var eventTags []string
	add := func(key, value string) {
		if value != "" {
			eventTags = append(eventTags, key+":"+value)
		}
	}

	add(tags.KubeNamespace, w.Namespace)
	add(tags.Scheduler, kueueSchedulerValue)
	add(tags.SchedulerWorkload, w.Name)
	add(tags.KueueWorkload, w.Name)
	add(tags.KueueWorkloadUID, w.UID)
	add(tags.KueueLocalQueue, w.QueueName)
	add("kueue_transition", transition)

	if w.Priority != nil {
		add("kueue_workload_priority", strconv.Itoa(int(*w.Priority)))
	}
	add("kueue_workload_priority_class", w.PriorityClassName)

	// An evicted Workload has no admission anymore, so its queue comes from the
	// state it had before.
	clusterQueue := w.ClusterQueueName
	if transition == "evicted" && clusterQueue == "" {
		clusterQueue = previous.clusterQueue
	}

	if clusterQueue != "" {
		// The ClusterQueue holds the quota, so it is the scheduler queue, as on the pods the Agent tags.
		add(tags.SchedulerQueue, clusterQueue)
		add(tags.KueueClusterQueue, clusterQueue)
		if id, err := workloadmeta.GenerateKueueQueueEntityID(workloadmeta.KueueClusterQueue, "", clusterQueue); err == nil {
			eventTags = append(eventTags, k.entityTags(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueQueue, ID: id})...)
		}
	}

	eventTags = append(eventTags, k.entityTags(w.EntityID)...)

	if w.Namespace != "" && w.QueueName != "" {
		if id, err := workloadmeta.GenerateKueueQueueEntityID(workloadmeta.KueueLocalQueue, w.Namespace, w.QueueName); err == nil {
			eventTags = append(eventTags, k.entityTags(workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesKueueQueue, ID: id})...)
		}
	}

	if condition == nil {
		return eventTags
	}

	switch transition {
	case "evicted":
		add("kueue_eviction_reason", condition.Reason)
		if condition.Reason == "Preempted" {
			preempted := findKueueCondition(w, kueueConditionPreempt)
			if preempted != nil {
				add("kueue_preemption_reason", preempted.Reason)
			}
			add("kueue_preempted_by", preemptingWorkloadUID(condition, preempted))
		}
	case "pending":
		add("kueue_pending_reason", condition.Reason)
	}
	return eventTags
}

// entityTags returns the tags the Agent gives to an entity, so that the events
// carry the same tags as the pods of the Workload.
func (k *kueueEvents) entityTags(id workloadmeta.EntityID) []string {
	entityTags, err := k.tagger.Tag(common.BuildTaggerEntityID(id), types.OrchestratorCardinality)
	if err != nil {
		log.Warnf("Cannot get the tags of %s %s for a Kueue event, it is sent without them: %v", id.Kind, id.ID, err)
		return nil
	}
	return entityTags
}

// preemptingWorkloadUID finds the UID of the Workload that preempted another
// one in the message of the Evicted or Preempted condition.
func preemptingWorkloadUID(condition *workloadCondition, preempted *workloadCondition) string {
	for _, candidate := range []*workloadCondition{condition, preempted} {
		if candidate == nil {
			continue
		}
		if match := kueuePreemptingWorkloadUID.FindStringSubmatch(candidate.Message); match != nil {
			return match[1]
		}
	}
	return ""
}
