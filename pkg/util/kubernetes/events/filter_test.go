// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldCollect(t *testing.T) {
	podBackOff := Event{
		Reason:          "BackOff",
		SourceComponent: "kubelet",
		InvolvedObject:  ObjectReference{Kind: "Pod", Name: "my-pod-1"},
	}

	tests := []struct {
		name           string
		ev             Event
		collectedTypes []CollectedType
		want           bool
	}{
		{
			name:           "kind matches, no reason listed",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Kind: "Pod"}},
			want:           true,
		},
		{
			name:           "kind does not match",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Kind: "Node"}},
			want:           false,
		},
		{
			name:           "source matches",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Source: "kubelet"}},
			want:           true,
		},
		{
			name:           "source does not match",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Source: "karpenter"}},
			want:           false,
		},
		{
			name:           "reason is listed",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Kind: "Pod", Reasons: []string{"Failed", "BackOff"}}},
			want:           true,
		},
		{
			name:           "reason is not listed",
			ev:             podBackOff,
			collectedTypes: []CollectedType{{Kind: "Pod", Reasons: []string{"Failed"}}},
			want:           false,
		},
		{
			name: "any matching entry is enough",
			ev:   podBackOff,
			collectedTypes: []CollectedType{
				{Kind: "Node"},
				{Kind: "Pod", Reasons: []string{"BackOff"}},
			},
			want: true,
		},
		{
			name: "no entry",
			ev:   podBackOff,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldCollect(tt.ev, tt.collectedTypes))
		})
	}
}

func TestShouldCollectByDefault(t *testing.T) {
	tests := []struct {
		name             string
		ev               Event
		detectionEnabled bool
		want             bool
	}{
		{
			name: "BackOff is collected for any emitter",
			ev: Event{
				Reason:          "BackOff",
				SourceComponent: "unknown",
				InvolvedObject:  ObjectReference{Kind: "Pod"},
			},
			detectionEnabled: true,
			want:             true,
		},
		{
			name: "a pod event of the kubernetes integration",
			ev: Event{
				Reason:              "FailedScheduling",
				ReportingController: "default-scheduler",
				InvolvedObject:      ObjectReference{Kind: "Pod"},
			},
			detectionEnabled: true,
			want:             true,
		},
		{
			name: "a normal event is not collected",
			ev: Event{
				Reason:              "Scheduled",
				ReportingController: "default-scheduler",
				InvolvedObject:      ObjectReference{Kind: "Pod"},
			},
			detectionEnabled: true,
			want:             false,
		},
		{
			name: "every event of a whole integration is collected",
			ev: Event{
				Reason:              "Reconciled",
				ReportingController: "source-controller",
				SourceComponent:     "fluxcd",
				InvolvedObject:      ObjectReference{Kind: "GitRepository"},
			},
			detectionEnabled: true,
			want:             true,
		},
		{
			name: "with detection disabled, integrations are not told apart",
			ev: Event{
				Reason:              "Reconciled",
				ReportingController: "source-controller",
				SourceComponent:     "fluxcd",
				InvolvedObject:      ObjectReference{Kind: "GitRepository"},
			},
			detectionEnabled: false,
			want:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ShouldCollectByDefault(tt.ev, tt.detectionEnabled))
		})
	}
}
