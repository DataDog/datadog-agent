// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"fmt"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// EventType is the event_type of the Datadog events built from Kubernetes events.
	// It is the name of the kubernetes_apiserver check, which historically emits them.
	EventType = "kubernetes_apiserver"

	// schedulerReason is the reason of the event the kube-scheduler emits when it assigns a pod to a node.
	schedulerReason = "Scheduled"
)

// ObjectReference identifies the object a Kubernetes event is about.
type ObjectReference struct {
	Kind       string
	Name       string
	Namespace  string
	UID        string
	APIVersion string
}

// Event holds the fields of a Kubernetes event that the mapping depends on.
//
// It does not tie the mapping to the core/v1 Event type, so that events that come from
// somewhere else, such as an OpenTelemetry log record, can be mapped as well.
type Event struct {
	// Type is the Kubernetes event type: Normal or Warning.
	Type    string
	Reason  string
	Message string

	InvolvedObject ObjectReference

	// ReportingController is set by emitters that use the events.k8s.io API.
	ReportingController string
	// SourceComponent is set by emitters that use the core API, which often leave
	// ReportingController empty.
	SourceComponent string
	// SourceHost is the name of the node that reported the event, when known.
	SourceHost string
}

// ObjectReferenceFromCoreV1 converts a core/v1 ObjectReference.
func ObjectReferenceFromCoreV1(obj v1.ObjectReference) ObjectReference {
	return ObjectReference{
		Kind:       obj.Kind,
		Name:       obj.Name,
		Namespace:  obj.Namespace,
		UID:        string(obj.UID),
		APIVersion: obj.APIVersion,
	}
}

// FromCoreV1 converts a core/v1 Event.
func FromCoreV1(ev *v1.Event) Event {
	return Event{
		Type:                ev.Type,
		Reason:              ev.Reason,
		Message:             ev.Message,
		InvolvedObject:      ObjectReferenceFromCoreV1(ev.InvolvedObject),
		ReportingController: ev.ReportingController,
		SourceComponent:     ev.Source.Component,
		SourceHost:          ev.Source.Host,
	}
}

// AlertType converts a Kubernetes event type into a Datadog alert type.
func AlertType(k8sType string) event.AlertType {
	switch k8sType {
	case v1.EventTypeNormal:
		return event.AlertTypeInfo
	case v1.EventTypeWarning:
		return event.AlertTypeWarning
	default:
		log.Debugf("Unknown event type '%s'", k8sType)
		return event.AlertTypeInfo
	}
}

// ReadableKey identifies an object in a title or a log: "<kind> <namespace>/<name>",
// or "<kind> <name>" for cluster-scoped objects.
func ReadableKey(obj ObjectReference) string {
	if obj.Namespace != "" {
		return fmt.Sprintf("%s %s/%s", obj.Kind, obj.Namespace, obj.Name)
	}

	return fmt.Sprintf("%s %s", obj.Kind, obj.Name)
}

// Title is the title of the Datadog event built from ev.
func Title(ev Event) string {
	return fmt.Sprintf("%s: %s", ReadableKey(ev.InvolvedObject), ev.Reason)
}

// AggregationKey is the aggregation key of the Datadog events about the object with the given UID,
// so that Datadog groups the events of one object together.
func AggregationKey(involvedObjectUID string) string {
	return EventType + ":" + involvedObjectUID
}

// scheduledMessageRe matches the message the kube-scheduler (and schedulers
// reusing its wording, e.g. GKE's high-throughput scheduler) attaches to a
// Pod's "Scheduled" event: "Successfully assigned <namespace>/<pod> to <node>".
var scheduledMessageRe = regexp.MustCompile(`^Successfully assigned \S+ to (\S+)$`)

// NodeFromScheduledMessage returns the node named in a Pod "Scheduled" event's
// message, or "" when the event is not such an event or its message does not
// have the scheduler's wording. Scheduler events carry no source host, and the
// message already says where the pod went, which saves a Pod GET.
func NodeFromScheduledMessage(ev Event) string {
	if ev.Reason != schedulerReason {
		return ""
	}
	m := scheduledMessageRe.FindStringSubmatch(strings.TrimSpace(ev.Message))
	if len(m) < 2 {
		return ""
	}
	return m[1]
}
