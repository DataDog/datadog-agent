// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kube-state-metrics/v2/pkg/metric"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

func TestPodTimeToReadyGenerator_ReadyPod(t *testing.T) {
	scheduledTime := metav1.NewTime(time.Now().Add(-60 * time.Second))
	readyTime := metav1.NewTime(time.Now().Add(-30 * time.Second))

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "container-1",
					RestartCount: 0,
				},
			},
		},
	}

	family := podTimeToReadyGenerator(pod)

	require.Len(t, family.Metrics, 1)
	m := family.Metrics[0]
	// Time to ready should be approximately 30 seconds
	assert.InDelta(t, 30.0, m.Value, 1.0)
}

func TestPodTimeToReadyGenerator_NotReadyPod(t *testing.T) {
	scheduledTime := metav1.NewTime(time.Now().Add(-60 * time.Second))

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:   v1.PodReady,
					Status: v1.ConditionFalse, // Pod not ready
				},
			},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for pods that are not ready
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_NotScheduledPod(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:   v1.PodScheduled,
					Status: v1.ConditionFalse, // Pod not scheduled
				},
			},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for pods that are not scheduled
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_ContainerRestarted(t *testing.T) {
	scheduledTime := metav1.NewTime(time.Now().Add(-60 * time.Second))
	readyTime := metav1.NewTime(time.Now().Add(-30 * time.Second))

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "container-1",
					RestartCount: 1, // Container has restarted
				},
			},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for pods with container restarts
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_MultipleContainersOneRestarted(t *testing.T) {
	scheduledTime := metav1.NewTime(time.Now().Add(-60 * time.Second))
	readyTime := metav1.NewTime(time.Now().Add(-30 * time.Second))

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "container-1",
					RestartCount: 0,
				},
				{
					Name:         "container-2",
					RestartCount: 2, // This container has restarted
				},
			},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics even if only one container restarted
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_NegativeTimeToReady(t *testing.T) {
	// Edge case: ready time is before scheduled time (clock skew)
	scheduledTime := metav1.NewTime(time.Now())
	readyTime := metav1.NewTime(time.Now().Add(-60 * time.Second)) // Ready before scheduled

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for negative time to ready
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_ZeroTimeToReady(t *testing.T) {
	// Edge case: ready time equals scheduled time
	now := metav1.NewTime(time.Now())

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: now,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: now,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{},
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for zero time to ready
	assert.Len(t, family.Metrics, 0)
}

func TestPodTimeToReadyGenerator_NoConditions(t *testing.T) {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{}, // No conditions
		},
	}

	family := podTimeToReadyGenerator(pod)

	// Should return empty metrics for pods without conditions
	assert.Len(t, family.Metrics, 0)
}

func TestExtendedPodFactory_MetricFamilyGenerators_ContainsTimeToReady(t *testing.T) {
	factory := NewExtendedPodFactoryForKubelet()

	generators := factory.MetricFamilyGenerators()

	// Find the time to ready generator
	var found bool
	for _, gen := range generators {
		if gen.Name == "kube_pod_time_to_ready" {
			found = true
			assert.Equal(t, "Time in seconds from pod scheduled to ready.", gen.Help)
			assert.Equal(t, metric.Gauge, gen.Type)
			break
		}
	}
	assert.True(t, found, "kube_pod_time_to_ready metric should be registered")
}

func TestExtendedPodFactory_TimeToReady_WithWrapPodFunc(t *testing.T) {
	factory := NewExtendedPodFactoryForKubelet()
	generators := factory.MetricFamilyGenerators()

	// Find the time to ready generator
	var timeToReadyGenerator *generator.FamilyGenerator
	for i := range generators {
		if generators[i].Name == "kube_pod_time_to_ready" {
			timeToReadyGenerator = &generators[i]
			break
		}
	}
	require.NotNil(t, timeToReadyGenerator)

	scheduledTime := metav1.NewTime(time.Now().Add(-45 * time.Second))
	readyTime := metav1.NewTime(time.Now())

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-ns",
			UID:       "test-uid-123",
		},
		Status: v1.PodStatus{
			Conditions: []v1.PodCondition{
				{
					Type:               v1.PodScheduled,
					Status:             v1.ConditionTrue,
					LastTransitionTime: scheduledTime,
				},
				{
					Type:               v1.PodReady,
					Status:             v1.ConditionTrue,
					LastTransitionTime: readyTime,
				},
			},
			ContainerStatuses: []v1.ContainerStatus{
				{
					Name:         "app",
					RestartCount: 0,
				},
			},
		},
	}

	// Generate metrics using the wrapped function
	family := timeToReadyGenerator.Generate(pod)

	require.Len(t, family.Metrics, 1)
	m := family.Metrics[0]

	// Verify value is approximately 45 seconds
	assert.InDelta(t, 45.0, m.Value, 1.0)

	// Verify labels added by wrapPodFunc
	assert.Equal(t, []string{"namespace", "pod", "uid"}, m.LabelKeys)
	assert.Equal(t, []string{"test-ns", "test-pod", "test-uid-123"}, m.LabelValues)
}
