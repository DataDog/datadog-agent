// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestUnbundledEventsTransform(t *testing.T) {
	ts := metav1.Time{Time: time.Date(2024, 5, 29, 6, 0, 51, 0, time.Now().Location())}
	oldTs := metav1.Time{Time: ts.Add(-3 * time.Hour)}

	incomingEvents := []*v1.Event{
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "wartortle-8fff95dbb-tsc7v",
				UID:       "17f2bab8-d051-4861-bc87-db3ba75dd6f6",
			},
			Type:    "Warning",
			Reason:  "Failed",
			Message: "All containers terminated",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "squirtle-8fff95dbb-tsc7v",
				UID:       "43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
			},
			Type:    "Normal",
			Reason:  "Pulled",
			Message: "Successfully pulled image \"pokemon/squirtle:latest\" in 1.263s (1.263s including waiting)",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: oldTs,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "squirtle-8fff95dbb-tsc7v",
				UID:       "43b7e0d3-9212-4355-a957-4ac15ce3a263",
			},
			Type:    "Normal",
			Reason:  "Scheduled",
			Message: "Successfully assigned default/squirtle-8fff95dbb-tsc7v to test-host",
			Source: v1.EventSource{
				Host: "test-host",
			},
			ReportingController: "default-scheduler",
			EventTime:           metav1.NewMicroTime(ts.Time),
			Count:               1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "ReplicaSet",
				Namespace: "default",
				Name:      "blastoise-759fd559f7",
				UID:       "b96b5c25-6282-4e6f-a2fb-010196a284d9",
			},
			Type:    "Normal",
			Reason:  "Killing",
			Message: "Stopping container blastoise",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp:      ts,
			LastTimestamp:       ts,
			Count:               1,
			ReportingController: "disruption-budget-manager",
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "ReplicaSet",
				Namespace: "default",
				Name:      "blastoise-759fd559f7",
				UID:       "b96b5c25-6282-4e6f-a2fb-010196a284d9",
			},
			Type:    "Normal",
			Reason:  "SuccessfulDelete",
			Message: "Deleted pod: blastoise-759fd559f7-5wtqr",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "PodDisruptionBudget",
				Namespace: "default",
				Name:      "otel-demo-opensearch-pdb",
				UID:       "b63ccea1-89bd-403c-8a06-d189bb01deff",
			},
			Type:    "Warning",
			Reason:  "CalculateExpectedPodCountFailed",
			Message: "Failed to calculate the number of expected pods: found no controllers for pod \"otel-demo-opensearch-0\"",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
	}

	taggerInstance := taggerimpl.SetupFakeTagger(t)

	tests := []struct {
		name                    string
		collectedEventTypes     []collectedEventType
		bundleUnspecifiedEvents bool
		expected                []event.Event
	}{
		{
			name: "unbundled events by Kind:Pod",
			collectedEventTypes: []collectedEventType{
				{Kind: "Pod", Source: "", Reasons: []string{}},
			},
			bundleUnspecifiedEvents: true,
			expected: []event.Event{
				{
					Title: "Events from the PodDisruptionBudget default/otel-demo-opensearch-pdb",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **CalculateExpectedPodCountFailed**: Failed to calculate the number of expected pods: found no controllers for pod "otel-demo-opensearch-0"
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:PodDisruptionBudget",
						"kube_name:otel-demo-opensearch-pdb",
						"kubernetes_kind:PodDisruptionBudget",
						"name:otel-demo-opensearch-pdb",
						"kube_namespace:default",
						"namespace:default",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:b63ccea1-89bd-403c-8a06-d189bb01deff",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the ReplicaSet default/blastoise-759fd559f7",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Killing**: Stopping container blastoise
 1 **SuccessfulDelete**: Deleted pod: blastoise-759fd559f7-5wtqr
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "",
					Tags: []string{
						"kube_kind:ReplicaSet",
						"kube_name:blastoise-759fd559f7",
						"kubernetes_kind:ReplicaSet",
						"name:blastoise-759fd559f7",
						"kube_namespace:default",
						"namespace:default",
						"kube_replica_set:blastoise-759fd559f7",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:disruption-budget-manager",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:b96b5c25-6282-4e6f-a2fb-010196a284d9",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/squirtle-8fff95dbb-tsc7v: Pulled",
					Text:     "Successfully pulled image \"pokemon/squirtle:latest\" in 1.263s (1.263s including waiting)",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Pulled",
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"source_component:kubelet",
						"orchestrator:kubernetes",
						"reporting_controller:",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/squirtle-8fff95dbb-tsc7v: Scheduled",
					Text:     "Successfully assigned default/squirtle-8fff95dbb-tsc7v to test-host",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Scheduled",
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"reporting_controller:default-scheduler",
						"orchestrator:kubernetes",
						"source_component:",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a263",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},

		{
			name: "unbundled events by Kind:Pod, don't bundle unspecified events",
			collectedEventTypes: []collectedEventType{
				{Kind: "Pod", Source: "", Reasons: []string{}},
			},
			bundleUnspecifiedEvents: false,
			expected: []event.Event{
				{
					Title:    "Pod default/squirtle-8fff95dbb-tsc7v: Pulled",
					Text:     "Successfully pulled image \"pokemon/squirtle:latest\" in 1.263s (1.263s including waiting)",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Pulled",
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"source_component:kubelet",
						"orchestrator:kubernetes",
						"reporting_controller:",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/squirtle-8fff95dbb-tsc7v: Scheduled",
					Text:     "Successfully assigned default/squirtle-8fff95dbb-tsc7v to test-host",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Scheduled",
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"reporting_controller:default-scheduler",
						"orchestrator:kubernetes",
						"source_component:",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a263",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},

		{
			name: "unbundled events by Kind:ReplicaSet and Reason:Killing,SuccessfulDelete",
			collectedEventTypes: []collectedEventType{
				{Kind: "ReplicaSet", Source: "", Reasons: []string{"Killing", "SuccessfulDelete"}},
			},
			bundleUnspecifiedEvents: true,
			expected: []event.Event{
				{
					Title: "Events from the Pod default/squirtle-8fff95dbb-tsc7v",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Pulled**: Successfully pulled image "pokemon/squirtle:latest" in 1.263s (1.263s including waiting)
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[3]s_%[1]s

 %%%%%%`, " ", ts.String(), oldTs.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"source_component:kubelet",
						"orchestrator:kubernetes",
						"reporting_controller:",
					},
					Host:           "test-host-test-cluster",
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the Pod default/squirtle-8fff95dbb-tsc7v",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Scheduled**: Successfully assigned default/squirtle-8fff95dbb-tsc7v to test-host
%[1]s
 _Events emitted by the  seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"namespace:default",
						"pod_name:squirtle-8fff95dbb-tsc7v",
						"source_component:",
						"orchestrator:kubernetes",
						"reporting_controller:default-scheduler",
					},
					Host:           "test-host-test-cluster",
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a263",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the Pod default/wartortle-8fff95dbb-tsc7v",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Failed**: All containers terminated
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"source_component:kubelet",
						"orchestrator:kubernetes",
						"reporting_controller:",
					},
					Host:           "test-host-test-cluster",
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the PodDisruptionBudget default/otel-demo-opensearch-pdb",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **CalculateExpectedPodCountFailed**: Failed to calculate the number of expected pods: found no controllers for pod "otel-demo-opensearch-0"
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:PodDisruptionBudget",
						"kube_name:otel-demo-opensearch-pdb",
						"kubernetes_kind:PodDisruptionBudget",
						"name:otel-demo-opensearch-pdb",
						"kube_namespace:default",
						"namespace:default",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:b63ccea1-89bd-403c-8a06-d189bb01deff",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "ReplicaSet default/blastoise-759fd559f7: Killing",
					Text:     "Stopping container blastoise",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"event_reason:Killing",
						"kube_kind:ReplicaSet",
						"kube_name:blastoise-759fd559f7",
						"kube_namespace:default",
						"kube_replica_set:blastoise-759fd559f7",
						"kubernetes_kind:ReplicaSet",
						"name:blastoise-759fd559f7",
						"namespace:default",
						"orchestrator:kubernetes",
						"reporting_controller:disruption-budget-manager",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:b96b5c25-6282-4e6f-a2fb-010196a284d9",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "ReplicaSet default/blastoise-759fd559f7: SuccessfulDelete",
					Text:     "Deleted pod: blastoise-759fd559f7-5wtqr",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"event_reason:SuccessfulDelete",
						"kube_kind:ReplicaSet",
						"kube_name:blastoise-759fd559f7",
						"kube_namespace:default",
						"kube_replica_set:blastoise-759fd559f7",
						"kubernetes_kind:ReplicaSet",
						"name:blastoise-759fd559f7",
						"namespace:default",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:b96b5c25-6282-4e6f-a2fb-010196a284d9",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := newUnbundledTransformer("test-cluster", taggerInstance, tt.collectedEventTypes, tt.bundleUnspecifiedEvents, false)

			events, errors := transformer.Transform(incomingEvents)

			// Sort events by title and description for easier comparison
			sort.SliceStable(events, func(i, j int) bool {
				return events[i].Title+events[i].Text < events[j].Title+events[j].Text
			})

			assert.Empty(t, errors)
			for i := range events {
				assert.Equal(t, tt.expected[i].Ts, events[i].Ts)
				assert.Equal(t, tt.expected[i].Title, events[i].Title)
				assert.Equal(t, tt.expected[i].Text, events[i].Text)
				assert.Equal(t, tt.expected[i].Priority, events[i].Priority)
				assert.Equal(t, tt.expected[i].Host, events[i].Host)
				assert.Equal(t, tt.expected[i].AlertType, events[i].AlertType)
				assert.Equal(t, tt.expected[i].AggregationKey, events[i].AggregationKey)
				assert.Equal(t, tt.expected[i].SourceTypeName, events[i].SourceTypeName)
				assert.Equal(t, tt.expected[i].EventType, events[i].EventType)
				assert.ElementsMatch(t, tt.expected[i].Tags, events[i].Tags)
			}
		})
	}
}

func TestUnbundledEventsTransformFiltering(t *testing.T) {
	ts := metav1.Time{Time: time.Date(2024, 5, 29, 6, 0, 51, 0, time.Now().Location())}

	incomingEvents := []*v1.Event{
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "wartortle-8fff95dbb-tsc7v",
				UID:       "17f2bab8-d051-4861-bc87-db3ba75dd6f6",
			},
			Type:    "Warning",
			Reason:  "Failed",
			Message: "All containers terminated",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "Pod",
				Namespace: "default",
				Name:      "squirtle-8fff95dbb-tsc7v",
				UID:       "43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
			},
			Type:    "Normal",
			Reason:  "Pulled",
			Message: "Successfully pulled image \"pokemon/squirtle:latest\" in 1.263s (1.263s including waiting)",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "ReplicaSet",
				Namespace: "default",
				Name:      "blastoise-759fd559f7",
				UID:       "b96b5c25-6282-4e6f-a2fb-010196a284d9",
			},
			Type:    "Normal",
			Reason:  "Killing",
			Message: "Stopping container blastoise",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp:      ts,
			LastTimestamp:       ts,
			Count:               1,
			ReportingController: "disruption-budget-manager",
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "ReplicaSet",
				Namespace: "default",
				Name:      "blastoise-759fd559f7",
				UID:       "b96b5c25-6282-4e6f-a2fb-010196a284d9",
			},
			Type:    "Normal",
			Reason:  "SuccessfulDelete",
			Message: "Deleted pod: blastoise-759fd559f7-5wtqr",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
		{
			InvolvedObject: v1.ObjectReference{
				Kind:      "PodDisruptionBudget",
				Namespace: "default",
				Name:      "otel-demo-opensearch-pdb",
				UID:       "b63ccea1-89bd-403c-8a06-d189bb01deff",
			},
			Type:    "Warning",
			Reason:  "CalculateExpectedPodCountFailed",
			Message: "Failed to calculate the number of expected pods: found no controllers for pod \"otel-demo-opensearch-0\"",
			Source: v1.EventSource{
				Component: "kubelet",
				Host:      "test-host",
			},
			FirstTimestamp: ts,
			LastTimestamp:  ts,
			Count:          1,
		},
	}

	taggerInstance := taggerimpl.SetupFakeTagger(t)

	tests := []struct {
		name                   string
		bundleUnspecifedEvents bool
		filteringEnabled       bool
		customFilter           []collectedEventType
		expected               []event.Event
	}{
		{
			name:                   "default filtering enabled, bundle unspecified events disabled",
			bundleUnspecifedEvents: false,
			filteringEnabled:       true,
			expected: []event.Event{
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},
		{
			name:                   "default filtering enabled with custom filter, bundle unspecified events disabled",
			bundleUnspecifedEvents: false,
			filteringEnabled:       true,
			customFilter: []collectedEventType{
				{
					Kind:    "Pod",
					Source:  "kubelet",
					Reasons: []string{"Pulled"},
				},
			},
			expected: []event.Event{
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Pulled",
					Text:     "Successfully pulled image \"pokemon/squirtle:latest\" in 1.263s (1.263s including waiting)",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Pulled",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes_custom",
					EventType:      "kubernetes_apiserver",
				},
			},
		},
		{
			name:                   "default filtering and bundle unspecified events enabled",
			bundleUnspecifedEvents: true,
			filteringEnabled:       true,
			expected: []event.Event{
				{
					Title: "Events from the Pod default/squirtle-8fff95dbb-tsc7v",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Pulled**: Successfully pulled image "pokemon/squirtle:latest" in 1.263s (1.263s including waiting)
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:Pod",
						"kube_name:squirtle-8fff95dbb-tsc7v",
						"kubernetes_kind:Pod",
						"name:squirtle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"namespace:default",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:",
						"pod_name:squirtle-8fff95dbb-tsc7v",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
					Host:           "test-host-test-cluster",
				},
				{
					Title: "Events from the PodDisruptionBudget default/otel-demo-opensearch-pdb",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **CalculateExpectedPodCountFailed**: Failed to calculate the number of expected pods: found no controllers for pod "otel-demo-opensearch-0"
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:PodDisruptionBudget",
						"kube_name:otel-demo-opensearch-pdb",
						"kubernetes_kind:PodDisruptionBudget",
						"name:otel-demo-opensearch-pdb",
						"kube_namespace:default",
						"namespace:default",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:b63ccea1-89bd-403c-8a06-d189bb01deff",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the ReplicaSet default/blastoise-759fd559f7",
					Text: fmt.Sprintf(`%%%%%%%[1]s
1 **Killing**: Stopping container blastoise
 1 **SuccessfulDelete**: Deleted pod: blastoise-759fd559f7-5wtqr
%[1]s
 _Events emitted by the kubelet seen at %[2]s since %[2]s_%[1]s

 %%%%%%`, " ", ts.String()),
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "",
					Tags: []string{
						"kube_kind:ReplicaSet",
						"kube_name:blastoise-759fd559f7",
						"kubernetes_kind:ReplicaSet",
						"name:blastoise-759fd559f7",
						"kube_namespace:default",
						"namespace:default",
						"kube_replica_set:blastoise-759fd559f7",
						"orchestrator:kubernetes",
						"source_component:kubelet",
						"reporting_controller:disruption-budget-manager",
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:b96b5c25-6282-4e6f-a2fb-010196a284d9",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title:    "Pod default/wartortle-8fff95dbb-tsc7v: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:wartortle-8fff95dbb-tsc7v",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:wartortle-8fff95dbb-tsc7v",
						"namespace:default",
						"pod_name:wartortle-8fff95dbb-tsc7v",
						"orchestrator:kubernetes",
						"reporting_controller:",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transformer := newUnbundledTransformer("test-cluster", taggerInstance, []collectedEventType{}, tt.bundleUnspecifedEvents, tt.filteringEnabled)

			events, errors := transformer.Transform(incomingEvents)

			// Sort events by title for easier comparison
			sort.Slice(events, func(i, j int) bool {
				return events[i].Title < events[j].Title
			})

			assert.Empty(t, errors)
			for i := range events {
				assert.Equal(t, tt.expected[i].Ts, events[i].Ts)
				assert.Equal(t, tt.expected[i].Title, events[i].Title)
				assert.Equal(t, tt.expected[i].Text, events[i].Text)
				assert.Equal(t, tt.expected[i].Priority, events[i].Priority)
				assert.Equal(t, tt.expected[i].Host, events[i].Host)
				assert.Equal(t, tt.expected[i].AlertType, events[i].AlertType)
				assert.Equal(t, tt.expected[i].AggregationKey, events[i].AggregationKey)
				assert.Equal(t, tt.expected[i].SourceTypeName, events[i].SourceTypeName)
				assert.Equal(t, tt.expected[i].EventType, events[i].EventType)
				assert.ElementsMatch(t, tt.expected[i].Tags, events[i].Tags)
			}
		})
	}
}

func TestGetTagsFromTagger(t *testing.T) {
	taggerInstance := taggerimpl.SetupFakeTagger(t)
	taggerInstance.SetGlobalTags([]string{"global:here"}, nil, nil, nil)

	tests := []struct {
		name         string
		obj          v1.ObjectReference
		expectedTags *tagset.HashlessTagsAccumulator
	}{
		{
			name: "accumulates global tags",
			obj: v1.ObjectReference{
				UID:       "redis",
				Kind:      "Pod",
				Namespace: "default",
				Name:      "redis",
			},
			expectedTags: tagset.NewHashlessTagsAccumulatorFromSlice([]string{"global:here"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collectedTypes := []collectedEventType{
				{Kind: "Pod", Reasons: []string{}},
			}
			transformer := newUnbundledTransformer("test-cluster", taggerInstance, collectedTypes, false, false)
			accumulator := tagset.NewHashlessTagsAccumulator()
			transformer.(*unbundledTransformer).getTagsFromTagger(accumulator)
			assert.Equal(t, tt.expectedTags, accumulator)
		})
	}
}

func TestUnbundledEventsShouldCollect(t *testing.T) {
	taggerInstance := taggerimpl.SetupFakeTagger(t)

	tests := []struct {
		name     string
		event    *v1.Event
		expected bool
	}{
		{
			name: "matches kind and reason",
			event: &v1.Event{
				InvolvedObject: v1.ObjectReference{Kind: "Pod"},
				Reason:         "Failed",
				Source:         v1.EventSource{Component: "kubelet"},
			},
			expected: true,
		},
		{
			name: "matches source and reason",
			event: &v1.Event{
				InvolvedObject: v1.ObjectReference{Kind: "NotPod"},
				Reason:         "SomeReason",
				Source:         v1.EventSource{Component: "some-component"},
			},
			expected: true,
		},
		{
			name: "matches source",
			event: &v1.Event{
				InvolvedObject: v1.ObjectReference{Kind: "NotPod"},
				Reason:         "AnyReason",
				Source:         v1.EventSource{Component: "a-component"},
			},
			expected: true,
		},
		{
			name: "matches kind",
			event: &v1.Event{
				InvolvedObject: v1.ObjectReference{Kind: "AnyKind"},
				Reason:         "AnyReason",
				Source:         v1.EventSource{Component: "other-component"},
			},
			expected: true,
		},
		{
			name: "matches none",
			event: &v1.Event{
				InvolvedObject: v1.ObjectReference{Kind: "Pod"},
				Reason:         "AnyReason",
				Source:         v1.EventSource{Component: "other-component"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collectedTypes := []collectedEventType{
				{
					Kind:    "Pod",
					Reasons: []string{"Failed"},
				},
				{
					Source:  "some-component",
					Reasons: []string{"SomeReason"},
				},
				{
					Kind: "AnyKind",
				},
				{
					Source: "a-component",
				},
			}

			transformer := newUnbundledTransformer("test-cluster", taggerInstance, collectedTypes, false, false)
			got := transformer.(*unbundledTransformer).shouldCollect(tt.event)
			assert.Equal(t, tt.expected, got)
		})
	}
}
