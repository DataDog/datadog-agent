// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/local"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

func TestUnbundledEventsTransform(t *testing.T) {
	ts := metav1.Time{Time: time.Date(2024, 5, 29, 6, 0, 51, 0, time.UTC)}

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
			Type:    "Noarmal",
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

	tests := []struct {
		name                   string
		collectedEventTypes    []collectedEventType
		bundleUnspecifedEvents bool
		expected               []event.Event
	}{
		{
			name: "unbundled events by Kind:Pod",
			collectedEventTypes: []collectedEventType{
				{Kind: "Pod", Source: "", Reasons: []string{}},
			},
			bundleUnspecifedEvents: true,
			expected: []event.Event{
				{
					Title: "Events from the PodDisruptionBudget default/otel-demo-opensearch-pdb",
					Text: `%%% 
1 **CalculateExpectedPodCountFailed**: Failed to calculate the number of expected pods: found no controllers for pod "otel-demo-opensearch-0"
 
 _Events emitted by the kubelet seen at 2024-05-29 06:00:51 +0000 UTC since 2024-05-29 06:00:51 +0000 UTC_ 

 %%%`,
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:PodDisruptionBudget",
						"kube_name:otel-demo-opensearch-pdb",
						"kubernetes_kind:PodDisruptionBudget",
						"name:otel-demo-opensearch-pdb",
						"kube_namespace:default",
						"namespace:default",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:b63ccea1-89bd-403c-8a06-d189bb01deff",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the ReplicaSet default/blastoise-759fd559f7",
					Text: `%%% 
1 **Killing**: Stopping container blastoise
 1 **SuccessfulDelete**: Deleted pod: blastoise-759fd559f7-5wtqr
 
 _Events emitted by the kubelet seen at 2024-05-29 06:00:51 +0000 UTC since 2024-05-29 06:00:51 +0000 UTC_ 

 %%%`,
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
						"source_component:kubelet",
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
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
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
			bundleUnspecifedEvents: false,
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
					},
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
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
			bundleUnspecifedEvents: true,
			expected: []event.Event{
				{
					Title: "Events from the Pod default/squirtle-8fff95dbb-tsc7v",
					Text: `%%% 
1 **Pulled**: Successfully pulled image "pokemon/squirtle:latest" in 1.263s (1.263s including waiting)
 
 _Events emitted by the kubelet seen at 2024-05-29 06:00:51 +0000 UTC since 2024-05-29 06:00:51 +0000 UTC_ 

 %%%`,
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
					},
					Host:           "test-host-test-cluster",
					AlertType:      event.AlertTypeInfo,
					AggregationKey: "kubernetes_apiserver:43b7e0d3-9212-4355-a957-4ac15ce3a7f7",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the Pod default/wartortle-8fff95dbb-tsc7v",
					Text: `%%% 
1 **Failed**: All containers terminated
 
 _Events emitted by the kubelet seen at 2024-05-29 06:00:51 +0000 UTC since 2024-05-29 06:00:51 +0000 UTC_ 

 %%%`,
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
					},
					Host:           "test-host-test-cluster",
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:17f2bab8-d051-4861-bc87-db3ba75dd6f6",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
				{
					Title: "Events from the PodDisruptionBudget default/otel-demo-opensearch-pdb",
					Text: `%%% 
1 **CalculateExpectedPodCountFailed**: Failed to calculate the number of expected pods: found no controllers for pod "otel-demo-opensearch-0"
 
 _Events emitted by the kubelet seen at 2024-05-29 06:00:51 +0000 UTC since 2024-05-29 06:00:51 +0000 UTC_ 

 %%%`,
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Tags: []string{
						"kube_kind:PodDisruptionBudget",
						"kube_name:otel-demo-opensearch-pdb",
						"kubernetes_kind:PodDisruptionBudget",
						"name:otel-demo-opensearch-pdb",
						"kube_namespace:default",
						"namespace:default",
						"source_component:kubelet",
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
			transformer := newUnbundledTransformer("test-cluster", local.NewFakeTagger(), tt.collectedEventTypes, tt.bundleUnspecifedEvents)

			events, errors := transformer.Transform(incomingEvents)

			// Sort events by title for easier comparison
			sort.Slice(events, func(i, j int) bool {
				return events[i].Title < events[j].Title
			})

			assert.Empty(t, errors)
			assert.Equal(t, tt.expected, events)
		})
	}
}

func TestGetTagsFromTagger(t *testing.T) {
	taggerInstance := local.NewFakeTagger()
	taggerInstance.SetTags("kubernetes_pod_uid://nginx", "workloadmeta-kubernetes_pod", nil, []string{"pod_name:nginx"}, nil, nil)
	taggerInstance.SetGlobalTags([]string{"global:here"}, nil, nil, nil)

	tests := []struct {
		name         string
		obj          v1.ObjectReference
		expectedTags *tagset.HashlessTagsAccumulator
	}{
		{
			name: "accumulates basic pod tags",
			obj: v1.ObjectReference{
				UID:       "redis",
				Kind:      "Pod",
				Namespace: "default",
				Name:      "redis",
			},
			expectedTags: tagset.NewHashlessTagsAccumulatorFromSlice([]string{"global:here"}),
		},
		{
			name: "add tagger pod tags",
			obj: v1.ObjectReference{
				UID:       "nginx",
				Kind:      "Pod",
				Namespace: "default",
				Name:      "nginx",
			},
			expectedTags: tagset.NewHashlessTagsAccumulatorFromSlice([]string{"global:here", "pod_name:nginx"}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collectedTypes := []collectedEventType{
				{Kind: "Pod", Reasons: []string{}},
			}
			transformer := newUnbundledTransformer("test-cluster", taggerInstance, collectedTypes, false)
			accumulator := tagset.NewHashlessTagsAccumulator()
			transformer.(*unbundledTransformer).getTagsFromTagger(tt.obj, accumulator)
			assert.Equal(t, tt.expectedTags, accumulator)
		})
	}
}

func TestUnbundledEventsShouldCollect(t *testing.T) {
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

			transformer := newUnbundledTransformer("test-cluster", local.NewFakeTagger(), collectedTypes, false)
			got := transformer.(*unbundledTransformer).shouldCollect(tt.event)
			assert.Equal(t, tt.expected, got)
		})
	}
}
