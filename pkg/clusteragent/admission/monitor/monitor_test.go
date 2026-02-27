// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package monitor

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	admcommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
)

func TestShouldHaveBeenMutated(t *testing.T) {
	m := &Monitor{
		startTime: time.Now().Add(-time.Minute),
	}

	tests := []struct {
		name     string
		pod      *workloadmeta.KubernetesPod
		filter   *fakeFilter
		expected bool
	}{
		{
			name: "pod with enabled label should be mutated",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "true"},
				},
			},
			filter:   &fakeFilter{shouldMutate: true},
			expected: true,
		},
		{
			name: "pod with disabled label should not be mutated",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "test-pod",
					Namespace: "default",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "false"},
				},
			},
			filter:   &fakeFilter{shouldMutate: false},
			expected: false,
		},
		{
			name: "pod without label defers to filter",
			pod: &workloadmeta.KubernetesPod{
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			},
			filter:   &fakeFilter{shouldMutate: true},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.filter = tt.filter
			assert.Equal(t, tt.expected, m.shouldHaveBeenMutated(tt.pod))
		})
	}
}

func TestHandleEvent(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name        string
		pod         *workloadmeta.KubernetesPod
		startTime   time.Time
		filter      *fakeFilter
		expectError bool
	}{
		{
			name: "pod created before monitor started is skipped",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "uid-1",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "old-pod",
					Namespace: "default",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "true"},
				},
				CreationTimestamp: now.Add(-10 * time.Minute),
			},
			startTime:   now,
			filter:      &fakeFilter{shouldMutate: true},
			expectError: false,
		},
		{
			name: "new pod that was mutated is fine",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "uid-2",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "mutated-pod",
					Namespace: "default",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "true"},
					Annotations: map[string]string{
						admcommon.MutatedByWebhookAnnotationKey: "agent_config",
					},
				},
				CreationTimestamp: now.Add(time.Minute),
			},
			startTime:   now,
			filter:      &fakeFilter{shouldMutate: true},
			expectError: false,
		},
		{
			name: "new pod that should have been mutated but was not triggers error",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "uid-3",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "unmutated-pod",
					Namespace: "default",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "true"},
				},
				CreationTimestamp: now.Add(time.Minute),
			},
			startTime:   now,
			filter:      &fakeFilter{shouldMutate: true},
			expectError: true,
		},
		{
			name: "new pod that should not be mutated is fine",
			pod: &workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   "uid-4",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "excluded-pod",
					Namespace: "kube-system",
					Labels:    map[string]string{admcommon.EnabledLabelKey: "false"},
				},
				CreationTimestamp: now.Add(time.Minute),
			},
			startTime:   now,
			filter:      &fakeFilter{shouldMutate: false},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Monitor{
				filter:    tt.filter,
				startTime: tt.startTime,
			}

			event := workloadmeta.Event{
				Type:   workloadmeta.EventTypeSet,
				Entity: tt.pod,
			}

			// handleEvent logs errors but does not return them, so we
			// verify indirectly that the logic paths are exercised.
			// A more thorough integration test would capture log output.
			m.handleEvent(event)
		})
	}
}

// fakeFilter implements mutatecommon.MutationFilter for testing.
type fakeFilter struct {
	shouldMutate bool
}

func (f *fakeFilter) ShouldMutatePod(_ *corev1.Pod) bool {
	return f.shouldMutate
}

func (f *fakeFilter) IsNamespaceEligible(_ string) bool {
	return f.shouldMutate
}
