// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

func TestHasLimitIncrease_CPUIncrease(t *testing.T) {
	// Pod with current CPU limit of 500m (50% in workloadmeta format)
	cpuLimit := float64(50)
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit: &cpuLimit,
					},
				},
			},
		},
	}

	// Recommendation with higher CPU limit (800m)
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("800m")},
			},
		},
	}

	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_MemoryIncrease(t *testing.T) {
	// Pod with current memory limit of 512Mi
	memLimit := uint64(512 * 1024 * 1024)
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						MemoryLimit: &memLimit,
					},
				},
			},
		},
	}

	// Recommendation with higher memory limit (1Gi)
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"memory": resource.MustParse("1Gi")},
			},
		},
	}

	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_NoIncrease(t *testing.T) {
	// Pod with current CPU limit of 1000m (100%)
	cpuLimit := float64(100)
	memLimit := uint64(1024 * 1024 * 1024) // 1Gi
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit:    &cpuLimit,
						MemoryLimit: &memLimit,
					},
				},
			},
		},
	}

	// Recommendation with lower limits
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("500m"), "memory": resource.MustParse("512Mi")},
			},
		},
	}

	assert.False(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_NoPatchedPods(t *testing.T) {
	// Pods without RecommendationIDAnnotation should be ignored
	cpuLimit := float64(50)
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Namespace:   "default",
				Annotations: map[string]string{}, // No RecommendationIDAnnotation
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit: &cpuLimit,
					},
				},
			},
		},
	}

	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("800m")},
			},
		},
	}

	// Should return false since there are no patched pods to compare with
	assert.False(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_SkipsPodsWithCurrentRecommendation(t *testing.T) {
	// Pod-1 has old recommendation with low limit - should be checked
	// Pod-2 has current recommendation - should be skipped
	cpuLimit1 := float64(50)  // 500m
	cpuLimit2 := float64(100) // 1000m
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit: &cpuLimit1,
					},
				},
			},
		},
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-2",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "new-rec-456", // Already on current recommendation
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit: &cpuLimit2,
					},
				},
			},
		},
	}

	// Recommendation with 700m - higher than pod-1's 500m limit
	// Pod-2 is skipped because it already has current recommendation
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("700m")},
			},
		},
	}

	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_AllPodsOnCurrentRecommendation(t *testing.T) {
	// All pods already have current recommendation - should return false
	cpuLimit := float64(50) // 500m
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "current-rec",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit: &cpuLimit,
					},
				},
			},
		},
	}

	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("800m")},
			},
		},
	}

	// Should return false since all pods are already on current recommendation
	assert.False(t, hasLimitIncrease(recommendation, pods, "current-rec"))
}

func TestHasLimitIncrease_LimitRemovedIsIncrease(t *testing.T) {
	// Pod has a limit, but recommendation removes it (no limit = unlimited)
	// This should be treated as an increase
	cpuLimit := float64(50) // 500m
	memLimit := uint64(512 * 1024 * 1024)
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit:    &cpuLimit,
						MemoryLimit: &memLimit,
					},
				},
			},
		},
	}

	// Recommendation with only requests, no limits
	// This means limits are being removed (unlimited)
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "container-1",
				Requests: corev1.ResourceList{"cpu": resource.MustParse("250m"), "memory": resource.MustParse("256Mi")},
				// No Limits specified - this removes the limit
			},
		},
	}

	// Should return true since removing a limit is effectively increasing it to unlimited
	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

func TestHasLimitIncrease_OnlyCPULimitRemoved(t *testing.T) {
	// Pod has CPU limit, recommendation removes only CPU limit but keeps memory limit
	cpuLimit := float64(50) // 500m
	memLimit := uint64(512 * 1024 * 1024)
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:      "pod-1",
				Namespace: "default",
				Annotations: map[string]string{
					model.RecommendationIDAnnotation: "old-rec-123",
				},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name: "container-1",
					Resources: workloadmeta.ContainerResources{
						CPULimit:    &cpuLimit,
						MemoryLimit: &memLimit,
					},
				},
			},
		},
	}

	// Recommendation keeps memory limit but removes CPU limit
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"memory": resource.MustParse("512Mi")}, // Only memory, no CPU limit
			},
		},
	}

	// Should return true since CPU limit is being removed
	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec-456"))
}

// Tests for shouldTriggerRollout

func TestShouldTriggerRollout_Complete(t *testing.T) {
	recommendationID := "rec-123"
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: recommendationID},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{recommendationID: 1}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		nil,   // no last action
		false, // no rollout in progress
		nil,   // no recommendation needed for complete check
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionComplete, decision)
}

func TestShouldTriggerRollout_AlreadyTriggered(t *testing.T) {
	recommendationID := "rec-123"
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"old-rec": 1, recommendationID: 0}

	// Last action was for THIS recommendation
	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		Version: recommendationID,
	}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		lastAction,
		false, // no rollout in progress
		nil,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionWait, decision)
}

func TestShouldTriggerRollout_NewRecommendationNoRollout(t *testing.T) {
	recommendationID := "rec-new"
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"old-rec": 1, recommendationID: 0}

	// Last action was for a DIFFERENT recommendation
	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		Version: "old-rec",
	}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		lastAction,
		false, // no rollout in progress
		nil,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionTrigger, decision)
}

func TestShouldTriggerRollout_OngoingRolloutNoBypass(t *testing.T) {
	recommendationID := "rec-new"
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"old-rec": 1, recommendationID: 0}

	// No limit increase (empty recommendation)
	recommendation := &model.VerticalScalingValues{}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		nil,
		true, // rollout in progress
		recommendation,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionWait, decision)
}

func TestShouldTriggerRollout_BypassAllowed(t *testing.T) {
	recommendationID := "rec-new"
	cpuLimit := float64(25) // 250m
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name:      "container-1",
					Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit},
				},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"old-rec": 1, recommendationID: 0}

	// Recommendation with higher CPU limit
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")},
			},
		},
	}

	// Last action was 10 minutes ago (outside rate limit)
	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(time.Now().Add(-10 * time.Minute)),
		Version: "old-rec",
	}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		lastAction,
		true, // rollout in progress
		recommendation,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionTrigger, decision)
}

func TestShouldTriggerRollout_BypassRateLimited(t *testing.T) {
	recommendationID := "rec-new"
	cpuLimit := float64(25) // 250m
	currentTime := time.Now()
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name:      "container-1",
					Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit},
				},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"old-rec": 1, recommendationID: 0}

	// Recommendation with higher CPU limit
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")},
			},
		},
	}

	// Last action was 2 minutes ago (within rate limit of 5 mins)
	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(currentTime.Add(-2 * time.Minute)),
		Version: "old-rec",
	}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		lastAction,
		true, // rollout in progress
		recommendation,
		currentTime,
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionWait, decision)
}

func TestShouldTriggerRollout_FirstTriggerNoLastAction(t *testing.T) {
	recommendationID := "rec-new"
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{}, // no recommendation annotation
			},
		},
	}
	podsPerRecommendationID := map[string]int32{"": 1, recommendationID: 0}

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		nil,   // no last action (first time)
		false, // no rollout in progress
		nil,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	assert.Equal(t, rolloutDecisionTrigger, decision)
}
