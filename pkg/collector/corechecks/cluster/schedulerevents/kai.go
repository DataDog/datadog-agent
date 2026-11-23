// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package schedulerevents

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	kaiSourceTypeName = "kai"
	kaiSchedulerValue = "kai"

	// KAI never sets status.conditions on a PodGroup. It reports why a PodGroup
	// cannot be scheduled in one scheduling condition per node pool, removed
	// once the PodGroup is scheduled.
	kaiUnschedulableCondition = "UnschedulableOnNodePool"
	kaiNodePoolLabel          = "kai.scheduler/node-pool"
	kaiDefaultNodePool        = "default"

	// Annotations of the Evict Kubernetes event that KAI records on each evicted PodGroup.
	kaiEvictorActionAnnotation    = "evictor-action-type"
	kaiEvictorNameAnnotation      = "evictor-pod-group-name"
	kaiEvictorNamespaceAnnotation = "evictor-pod-group-namespace"

	// kaiEvictEventTolerance is the number of seconds between the eviction stamp
	// and the Evict event that KAI records for it.
	kaiEvictEventTolerance = 1

	kaiEvictListTimeout = 10 * time.Second
)

func kaiAlertType(transition string) event.AlertType {
	switch transition {
	case "pending", "evicted":
		return event.AlertTypeWarning
	default:
		return event.AlertTypeInfo
	}
}

// kaiState is what is remembered of a PodGroup to tell what changed in its next update.
type kaiState struct {
	pendingNodePools map[string]struct{}
	lastStart        time.Time
	lastEviction     time.Time
}

func newKAIState(pg *workloadmeta.KubernetesKAIPodGroup) kaiState {
	pools := map[string]struct{}{}
	for nodePool := range kaiPendingConditions(pg) {
		pools[nodePool] = struct{}{}
	}
	return kaiState{pendingNodePools: pools, lastStart: pg.LastStartTime, lastEviction: pg.LastEvictionTime}
}

// kaiPendingConditions returns, by node pool, the conditions that say the
// PodGroup cannot be scheduled.
func kaiPendingConditions(pg *workloadmeta.KubernetesKAIPodGroup) map[string]workloadmeta.KAISchedulingCondition {
	conditions := map[string]workloadmeta.KAISchedulingCondition{}
	for _, condition := range pg.SchedulingConditions {
		if condition.Type != kaiUnschedulableCondition || condition.Status != "True" {
			continue
		}
		nodePool := condition.NodePool
		if nodePool == "" {
			nodePool = kaiDefaultNodePool
		}
		conditions[nodePool] = condition
	}
	return conditions
}

type kaiTransition struct {
	name      string
	timestamp time.Time
	condition *workloadmeta.KAISchedulingCondition
}

// evictEventLister returns the Evict events that KAI recorded on the PodGroups of a namespace.
type evictEventLister func(namespace string) ([]corev1.Event, error)

// kaiEvents turns the updates of KAI PodGroups into events.
type kaiEvents struct {
	index     *eventIndex
	startedAt time.Time
	now       func() time.Time
	submit    func(event.Event)
	listEvict evictEventLister
	states    map[string]kaiState

	// evictEvents caches the Evict events of a namespace while a batch of
	// updates is handled, since it can hold many evictions of the same namespace.
	evictEvents map[string][]corev1.Event
}

func newKAIEvents(index *eventIndex, startedAt time.Time, submit func(event.Event), listEvict evictEventLister) *kaiEvents {
	return &kaiEvents{
		index:       index,
		startedAt:   startedAt.Truncate(time.Second),
		now:         time.Now,
		submit:      submit,
		listEvict:   listEvict,
		states:      map[string]kaiState{},
		evictEvents: map[string][]corev1.Event{},
	}
}

// endBatch drops what was cached while handling a batch of updates.
func (k *kaiEvents) endBatch() {
	k.evictEvents = map[string][]corev1.Event{}
}

// set handles an update of a PodGroup, like kueueEvents.set does for Workloads,
// including the truncation of the start to the second of the creation timestamps.
func (k *kaiEvents) set(pg *workloadmeta.KubernetesKAIPodGroup, emit bool) {
	uid := pg.UID
	if uid == "" {
		log.Debugf("Skipping KAI PodGroup without uid: %s/%s", pg.Namespace, pg.Name)
		return
	}

	previous, known := k.states[uid]
	current := newKAIState(pg)
	k.states[uid] = current
	if !emit {
		return
	}

	var transitions []kaiTransition
	if !known {
		if pg.CreationTimestamp.Before(k.startedAt) {
			return
		}
		transitions = append(transitions, kaiTransition{name: "created", timestamp: pg.CreationTimestamp})
	}

	// KAI resets lastTransitionTime whenever the scheduling error message
	// changes, so a PodGroup only becomes pending on a node pool when the
	// condition appears there, not each time its message changes. KAI keeps a
	// condition while the pods of a new start bind or while only some of them
	// run, so after a new start a condition already seen is only new if it
	// changed after that start.
	started := !current.lastStart.IsZero() && !current.lastStart.Equal(previous.lastStart)
	pending := kaiPendingConditions(pg)
	nodePools := make([]string, 0, len(pending))
	for nodePool := range pending {
		nodePools = append(nodePools, nodePool)
	}
	sort.Strings(nodePools)
	for _, nodePool := range nodePools {
		condition := pending[nodePool]
		_, seen := previous.pendingNodePools[nodePool]
		recreated := started && !condition.LastTransitionTime.IsZero() && condition.LastTransitionTime.After(current.lastStart)
		if !seen || recreated {
			condition := condition
			transitions = append(transitions, kaiTransition{name: "pending", timestamp: condition.LastTransitionTime, condition: &condition})
		}
	}
	if started {
		transitions = append(transitions, kaiTransition{name: "admitted", timestamp: current.lastStart})
	}
	if !current.lastEviction.IsZero() && !current.lastEviction.Equal(previous.lastEviction) {
		transitions = append(transitions, kaiTransition{name: "evicted", timestamp: current.lastEviction})
	}

	// Several transitions can happen between two updates, such as an eviction
	// followed by a new start. Pending ones were added in node pool order and the
	// sort is stable, so events with the same time are always sent in the same order.
	sort.SliceStable(transitions, func(i, j int) bool {
		return transitionUnix(transitions[i]) < transitionUnix(transitions[j])
	})
	for _, transition := range transitions {
		k.send(pg, transition, previous)
	}
}

func transitionUnix(transition kaiTransition) int64 {
	if transition.timestamp.IsZero() {
		return 0
	}
	return transition.timestamp.Unix()
}

// unset forgets a deleted PodGroup.
func (k *kaiEvents) unset(uid string) {
	delete(k.states, uid)
	k.index.forget(uid)
}

func (k *kaiEvents) send(pg *workloadmeta.KubernetesKAIPodGroup, transition kaiTransition, previous kaiState) {
	text := []string{fmt.Sprintf("PodGroup %s/%s %s.", pg.Namespace, pg.Name, transition.name)}
	eventTags := kaiEventTags(pg, transition)

	switch transition.name {
	case "pending":
		var details []string
		details, eventTags = kaiPendingDetails(transition.condition, eventTags)
		text = append(text, details...)
	case "admitted":
		text = append(text, kaiAdmittedDetails(pg, transition, previous)...)
	case "evicted":
		var details []string
		details, eventTags = k.evictedDetails(pg, transition, eventTags)
		text = append(text, details...)
	}
	eventTags = append(eventTags, "kai_pod_group_event_index:"+strconv.Itoa(k.index.next(pg.UID)))

	timestamp := k.now().Unix()
	if !transition.timestamp.IsZero() {
		timestamp = transition.timestamp.Unix()
	}
	k.submit(event.Event{
		Title:          fmt.Sprintf("KAI PodGroup %s/%s %s", pg.Namespace, pg.Name, transition.name),
		Text:           strings.Join(text, " "),
		Ts:             timestamp,
		Tags:           eventTags,
		AlertType:      kaiAlertType(transition.name),
		AggregationKey: pg.UID,
		SourceTypeName: kaiSourceTypeName,
		EventType:      "kai.podgroup." + transition.name,
	})
}

func kaiEventTags(pg *workloadmeta.KubernetesKAIPodGroup, transition kaiTransition) []string {
	var nodePool string
	if transition.condition != nil {
		nodePool = transition.condition.NodePool
	} else {
		nodePool = pg.Labels[kaiNodePoolLabel]
	}
	if nodePool == "" {
		nodePool = kaiDefaultNodePool
	}

	var eventTags []string
	add := func(key, value string) {
		if value != "" {
			eventTags = append(eventTags, key+":"+value)
		}
	}
	add(tags.KubeNamespace, pg.Namespace)
	add(tags.Scheduler, kaiSchedulerValue)
	add(tags.SchedulerWorkload, pg.Name)
	add(tags.SchedulerQueue, pg.QueueName)
	add(tags.KAIPodGroup, pg.Name)
	add("kai_pod_group_uid", pg.UID)
	add(tags.KAIQueue, pg.QueueName)
	add("kai_nodepool", nodePool)
	add("kai_pod_group_priority_class", pg.PriorityClassName)
	add("kai_transition", transition.name)
	return eventTags
}

func kaiPendingDetails(condition *workloadmeta.KAISchedulingCondition, eventTags []string) ([]string, []string) {
	if condition == nil {
		return nil, eventTags
	}

	var reasons, messages []string
	for _, explanation := range condition.Explanations {
		if explanation.Reason != "" {
			reasons = append(reasons, explanation.Reason)
		}
		if explanation.Message != "" {
			messages = append(messages, explanation.Message)
		}
	}
	if len(reasons) == 0 && condition.Reason != "" {
		reasons = []string{condition.Reason}
	}
	for _, reason := range reasons {
		eventTags = append(eventTags, "kai_pending_reason:"+reason)
	}

	message := condition.Message
	if message == "" {
		message = strings.Join(messages, " ")
	}
	if message == "" {
		return nil, eventTags
	}
	return []string{message}, eventTags
}

// kaiAdmittedDetails says how long the PodGroup waited. KAI also stamps a new
// start when a PodGroup without allocated pods gets new ones, such as the next
// pod of a Job, so the wait counts from the latest eviction if it is between
// the previous start and this one, from the creation of a PodGroup that had not
// started, and is unknown otherwise. The annotations do not tell a partial
// eviction from a full one, so after a partial eviction the wait can include
// time the PodGroup spent running.
func kaiAdmittedDetails(pg *workloadmeta.KubernetesKAIPodGroup, transition kaiTransition, previous kaiState) []string {
	start := transition.timestamp
	created := pg.CreationTimestamp

	reference := created
	if !previous.lastStart.IsZero() {
		reference = previous.lastStart
	}
	eviction := pg.LastEvictionTime

	var queuedSince time.Time
	switch {
	case !start.IsZero() && !reference.IsZero() && !eviction.IsZero() && reference.Unix() <= eviction.Unix() && eviction.Unix() <= start.Unix():
		queuedSince = eviction
	case previous.lastStart.IsZero():
		queuedSince = created
	}
	if start.IsZero() || queuedSince.IsZero() {
		return nil
	}
	return []string{fmt.Sprintf("Queued wait time was %ds.", start.Unix()-queuedSince.Unix())}
}

// evictedDetails adds the eviction action and the evicting PodGroup, which only
// the Evict event that KAI records on the PodGroup has.
func (k *kaiEvents) evictedDetails(pg *workloadmeta.KubernetesKAIPodGroup, transition kaiTransition, eventTags []string) ([]string, []string) {
	eviction := transition.timestamp
	if eviction.IsZero() {
		return nil, eventTags
	}

	// The scheduler records the event and stamps the eviction at most a second
	// apart. An older event belongs to an earlier eviction, and a later one to an
	// eviction that happened after the PodGroups were received.
	var matched *corev1.Event
	events := k.namespaceEvictEvents(pg.Namespace)
	for i := range events {
		candidate := &events[i]
		if string(candidate.InvolvedObject.UID) != pg.UID {
			continue
		}
		// The Kubernetes event recorder combines bursts of similar events, such
		// as one per pod of a large PodGroup, into events without KAI's annotations.
		if _, annotated := candidate.Annotations[kaiEvictorActionAnnotation]; !annotated {
			continue
		}
		at := evictEventUnix(candidate)
		if at < eviction.Unix() || at > eviction.Unix()+kaiEvictEventTolerance {
			continue
		}
		if matched == nil || at < evictEventUnix(matched) {
			matched = candidate
		}
	}
	if matched == nil {
		return nil, eventTags
	}

	action := matched.Annotations[kaiEvictorActionAnnotation]
	eventTags = append(eventTags, "kai_eviction_action:"+action)

	var details []string
	// KAI ends some eviction messages with a period and others without.
	if matched.Message != "" {
		details = append(details, strings.TrimRight(matched.Message, ".")+".")
	}
	details = append(details, fmt.Sprintf("Eviction action: %s.", action))

	evictorName := matched.Annotations[kaiEvictorNameAnnotation]
	evictorNamespace := matched.Annotations[kaiEvictorNamespaceAnnotation]
	if evictorName != "" && evictorNamespace != "" {
		eventTags = append(eventTags, fmt.Sprintf("kai_evicted_by:%s/%s", evictorNamespace, evictorName))
	}
	return details, eventTags
}

// evictEventUnix returns when the event last happened. The event recorder
// merges identical events into one whose lastTimestamp is the latest occurrence.
func evictEventUnix(e *corev1.Event) int64 {
	switch {
	case !e.LastTimestamp.IsZero():
		return e.LastTimestamp.Unix()
	case !e.EventTime.IsZero():
		return e.EventTime.Unix()
	case !e.CreationTimestamp.IsZero():
		return e.CreationTimestamp.Unix()
	default:
		return 0
	}
}

func (k *kaiEvents) namespaceEvictEvents(namespace string) []corev1.Event {
	if events, cached := k.evictEvents[namespace]; cached {
		return events
	}

	events, err := k.listEvict(namespace)
	if err != nil {
		log.Warnf("Cannot list the Evict events of namespace %s to add eviction details: %v", namespace, err)
		events = nil
	}
	k.evictEvents[namespace] = events
	return events
}

// listEvictEventsFromAPIServer lists the Evict events that KAI recorded on the PodGroups of a namespace.
func listEvictEventsFromAPIServer(namespace string) ([]corev1.Event, error) {
	client, err := apiserver.GetAPIClient()
	if err != nil {
		return nil, fmt.Errorf("getting the API server client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), kaiEvictListTimeout)
	defer cancel()
	list, err := client.Cl.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.kind=PodGroup,reason=Evict",
	})
	if err != nil {
		return nil, fmt.Errorf("listing the Evict events: %w", err)
	}
	return list.Items, nil
}
