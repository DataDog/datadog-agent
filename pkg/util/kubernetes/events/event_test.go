// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func TestAlertType(t *testing.T) {
	tests := []struct {
		name    string
		k8sType string
		want    event.AlertType
	}{
		{name: "normal", k8sType: "Normal", want: event.AlertTypeInfo},
		{name: "warning", k8sType: "Warning", want: event.AlertTypeWarning},
		{name: "unknown", k8sType: "Unknown", want: event.AlertTypeInfo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AlertType(tt.k8sType))
		})
	}
}

func TestReadableKeyAndTitle(t *testing.T) {
	namespaced := Event{
		Reason:         "BackOff",
		InvolvedObject: ObjectReference{Kind: "Pod", Namespace: "default", Name: "nginx"},
	}
	assert.Equal(t, "Pod default/nginx", ReadableKey(namespaced.InvolvedObject))
	assert.Equal(t, "Pod default/nginx: BackOff", Title(namespaced))

	clusterScoped := Event{
		Reason:         "NodeNotReady",
		InvolvedObject: ObjectReference{Kind: "Node", Name: "node-1"},
	}
	assert.Equal(t, "Node node-1", ReadableKey(clusterScoped.InvolvedObject))
	assert.Equal(t, "Node node-1: NodeNotReady", Title(clusterScoped))
}

func TestAggregationKey(t *testing.T) {
	assert.Equal(t, "kubernetes_apiserver:abc-123", AggregationKey("abc-123"))
}

func TestNodeFromScheduledMessage(t *testing.T) {
	tests := []struct {
		name string
		ev   Event
		want string
	}{
		{
			name: "scheduler wording",
			ev:   Event{Reason: "Scheduled", Message: "Successfully assigned foo/my-pod to my-node-1"},
			want: "my-node-1",
		},
		{
			name: "trailing newline",
			ev:   Event{Reason: "Scheduled", Message: "Successfully assigned foo/my-pod to my-node-1\n"},
			want: "my-node-1",
		},
		{
			name: "other reason with a look-alike message",
			ev:   Event{Reason: "FailedScheduling", Message: "Successfully assigned foo/my-pod to my-node-1"},
			want: "",
		},
		{
			name: "unexpected wording",
			ev:   Event{Reason: "Scheduled", Message: "Pod foo/my-pod placed on my-node-1"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, NodeFromScheduledMessage(tt.ev))
		})
	}
}

func TestFromCoreV1(t *testing.T) {
	got := FromCoreV1(&v1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "nginx.17a"},
		InvolvedObject: v1.ObjectReference{
			Kind:       "Pod",
			Namespace:  "default",
			Name:       "nginx",
			UID:        types.UID("uid-1"),
			APIVersion: "v1",
		},
		Type:                "Warning",
		Reason:              "BackOff",
		Message:             "Back-off restarting failed container",
		ReportingController: "kubelet",
		Source:              v1.EventSource{Component: "kubelet", Host: "node-1"},
	})

	assert.Equal(t, Event{
		Type:    "Warning",
		Reason:  "BackOff",
		Message: "Back-off restarting failed container",
		InvolvedObject: ObjectReference{
			Kind:       "Pod",
			Namespace:  "default",
			Name:       "nginx",
			UID:        "uid-1",
			APIVersion: "v1",
		},
		ReportingController: "kubelet",
		SourceComponent:     "kubelet",
		SourceHost:          "node-1",
	}, got)
}
