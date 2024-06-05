// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesapiserver

import (
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
	ts := metav1.Time{Time: time.Now()}
	pod := v1.ObjectReference{
		UID:       "foobar",
		Kind:      "Pod",
		Namespace: "default",
		Name:      "redis",
	}

	tests := []struct {
		name     string
		event    *v1.Event
		expected []event.Event
	}{
		{
			name: "event is filtered out",
			event: &v1.Event{
				InvolvedObject: pod,
				Type:           "Info",
				Reason:         "SandboxChanged",
				Message:        "Pod sandbox changed, it will be killed and re-created.",
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "test-host",
				},
				FirstTimestamp: ts,
				LastTimestamp:  ts,
				Count:          1,
			},
			expected: nil,
		},
		{
			name: "event is collected",
			event: &v1.Event{
				InvolvedObject: pod,
				Type:           "Warning",
				Reason:         "Failed",
				Message:        "All containers terminated",
				Source: v1.EventSource{
					Component: "kubelet",
					Host:      "test-host",
				},
				FirstTimestamp: ts,
				LastTimestamp:  ts,
				Count:          1,
			},
			expected: []event.Event{
				{
					Title:    "Pod default/redis: Failed",
					Text:     "All containers terminated",
					Ts:       ts.Time.Unix(),
					Priority: event.PriorityNormal,
					Host:     "test-host-test-cluster",
					Tags: []string{
						"event_reason:Failed",
						"kube_kind:Pod",
						"kube_name:redis",
						"kube_namespace:default",
						"kubernetes_kind:Pod",
						"name:redis",
						"namespace:default",
						"pod_name:redis",
						"source_component:kubelet",
					},
					AlertType:      event.AlertTypeWarning,
					AggregationKey: "kubernetes_apiserver:foobar",
					SourceTypeName: "kubernetes",
					EventType:      "kubernetes_apiserver",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collectedTypes := []collectedEventType{
				{Kind: "Pod", Reasons: []string{"Failed"}},
			}
			transformer := newUnbundledTransformer("test-cluster", taggerimpl.SetupFakeTagger(t), collectedTypes)

			events, errors := transformer.Transform([]*v1.Event{tt.event})

			assert.Empty(t, errors)
			assert.Equal(t, tt.expected, events)
		})
	}
}

func TestGetTagsFromTagger(t *testing.T) {
	taggerInstance := taggerimpl.SetupFakeTagger(t)
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
			transformer := newUnbundledTransformer("test-cluster", taggerInstance, collectedTypes)
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

			transformer := newUnbundledTransformer("test-cluster", taggerimpl.SetupFakeTagger(t), collectedTypes)
			got := transformer.(*unbundledTransformer).shouldCollect(tt.event)
			assert.Equal(t, tt.expected, got)
		})
	}
}
