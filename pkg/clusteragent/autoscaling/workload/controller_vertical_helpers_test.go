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
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

func TestHasLimitIncrease_Detected(t *testing.T) {
	cpuLimit1 := float64(50) // 500m
	memLimit1 := uint64(512 * 1024 * 1024)
	cpuLimit2 := float64(100) // 1000m

	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-1", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "container-1", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit1, MemoryLimit: &memLimit1}},
			},
		},
		{
			// Pod already on current recommendation - must be skipped
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-2", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "new-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "container-1", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit2}},
			},
		},
	}

	// Higher CPU limit + removed memory limit (no memory in Limits = unlimited)
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "container-1",
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("800m")},
				Requests: corev1.ResourceList{"cpu": resource.MustParse("250m"), "memory": resource.MustParse("256Mi")},
				// No memory in Limits -> pod-1's memory limit is being removed (= increase)
			},
		},
	}

	assert.True(t, hasLimitIncrease(recommendation, pods, "new-rec"))
}

func TestHasLimitIncrease_NotDetected(t *testing.T) {
	cpuLimit1 := float64(50)
	cpuLimit2 := float64(100) // 1000m
	memLimit2 := uint64(1024 * 1024 * 1024)
	cpuLimit3 := float64(100) // 1000m
	memLimit3 := uint64(1024 * 1024 * 1024)

	pods := []*workloadmeta.KubernetesPod{
		{
			// No recommendation annotation -> skipped
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-1", Namespace: "default",
				Annotations: map[string]string{},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "container-1", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit1}},
			},
		},
		{
			// Already on current recommendation -> skipped
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-2", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "current-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "container-1", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit2, MemoryLimit: &memLimit2}},
			},
		},
		{
			// Old recommendation, but limits are higher than the new recommendation
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-3", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "old-rec"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "container-1", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit3, MemoryLimit: &memLimit3}},
			},
		},
	}

	// Recommendation with lower limits than pod-3
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "container-1",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("500m"), "memory": resource.MustParse("512Mi")},
			},
		},
	}

	assert.False(t, hasLimitIncrease(recommendation, pods, "current-rec"))
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

// Tests for applyVerticalConstraints

func TestApplyVerticalConstraints_NoModification(t *testing.T) {
	vertical := &model.VerticalScalingValues{
		ResourcesHash: "original-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("256Mi")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1"), corev1.ResourceMemory: resource.MustParse("512Mi")},
			},
		},
	}

	// Nil constraints
	limitErr, err := applyVerticalConstraints(vertical, nil)
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// Empty container list
	limitErr, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{})
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// No matching constraint name
	limitErr, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{Name: "other-container", MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// Values within bounds (new top-level MinAllowed/MaxAllowed)
	limitErr, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{
				Name:       "app",
				MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
				MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("1Gi")},
			},
		},
	})
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// Hash unchanged through all scenarios
	assert.Equal(t, "original-hash", vertical.ResourcesHash)
}

func TestApplyVerticalConstraints_AllFeatures(t *testing.T) {
	// A comprehensive test with 5 containers exercising every constraint feature:
	//
	// "app" (specific constraint, new MinAllowed/MaxAllowed applying to BOTH requests and limits):
	//   - CPU request  50m  -> clamped UP to min 100m
	//   - CPU limit   80m   -> clamped UP to min 100m (new fields apply to limits too)
	//   - Mem request  8Gi  -> clamped DOWN to max 4Gi
	//   - Mem limit   16Gi  -> clamped DOWN to max 4Gi (new fields apply to limits too)
	//
	// "sidecar" (wildcard, ControlledValues=RequestsOnly):
	//   - Limits stripped entirely
	//   - CPU request 10m -> clamped UP to wildcard min 50m
	//   - Memory request absent -> added from wildcard minAllowed 64Mi
	//
	// "disabled" (Enabled=false):
	//   - Entire container removed from recommendations
	//
	// "filtered" (ControlledResources=[cpu] only):
	//   - Memory removed from both requests and limits (not controlled)
	//   - CPU stays, ephemeral-storage removed
	//
	// "empty-controlled" (ControlledResources=[] empty list):
	//   - Entire container removed (equivalent to Enabled=false)
	//
	// "limit-bump" (specific constraint, MinAllowed pushes request above existing limit):
	//   - CPU request 50m, limit 80m
	//   - MinAllowed CPU 200m -> request clamped to 200m, limit (80m) < request (200m) -> raised to 200m

	vertical := &model.VerticalScalingValues{
		ResourcesHash: "original-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m"), corev1.ResourceMemory: resource.MustParse("8Gi")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("80m"), corev1.ResourceMemory: resource.MustParse("16Gi")},
			},
			{
				Name:     "sidecar",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("10m")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
			{
				Name:     "disabled",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			},
			{
				Name: "filtered",
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:              resource.MustParse("200m"),
					corev1.ResourceMemory:           resource.MustParse("128Mi"),
					corev1.ResourceEphemeralStorage: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("256Mi"),
				},
			},
			{
				Name:     "empty-controlled",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
			},
			{
				Name:     "limit-bump",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("80m")},
			},
		},
	}

	constraints := &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{
				// Wildcard: applies to "sidecar" (no specific constraint)
				Name: "*",
				MinAllowed: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("50m"),
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
				ControlledValues: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerContainerControlledValuesRequestsOnly),
			},
			{
				// Specific for "app": new top-level MinAllowed/MaxAllowed (apply to both requests and limits)
				Name:       "app",
				MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
				MaxAllowed: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
			},
			{
				Name:    "disabled",
				Enabled: pointer.Ptr(false),
			},
			{
				Name:                "filtered",
				ControlledResources: []corev1.ResourceName{corev1.ResourceCPU},
			},
			{
				Name:                "empty-controlled",
				ControlledResources: []corev1.ResourceName{}, // empty = disabled
			},
			{
				Name:       "limit-bump",
				MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
			},
		},
	}

	limitErr, err := applyVerticalConstraints(vertical, constraints)
	require.NoError(t, err)

	// "disabled" and "empty-controlled" should be removed -> 4 containers left
	require.Len(t, vertical.ContainerResources, 4)

	// --- "app" (index 0): specific constraint, min/max applied to BOTH requests and limits ---
	app := vertical.ContainerResources[0]
	assert.Equal(t, "app", app.Name)
	assert.True(t, app.Requests[corev1.ResourceCPU].Equal(resource.MustParse("100m")))   // 50m -> min 100m
	assert.True(t, app.Requests[corev1.ResourceMemory].Equal(resource.MustParse("4Gi"))) // 8Gi -> max 4Gi
	assert.True(t, app.Limits[corev1.ResourceCPU].Equal(resource.MustParse("100m")))     // 80m -> min 100m (limits clamped too)
	assert.True(t, app.Limits[corev1.ResourceMemory].Equal(resource.MustParse("4Gi")))   // 16Gi -> max 4Gi (limits clamped too)

	// --- "sidecar" (index 1): wildcard, RequestsOnly + min clamping ---
	sidecar := vertical.ContainerResources[1]
	assert.Equal(t, "sidecar", sidecar.Name)
	assert.True(t, sidecar.Requests[corev1.ResourceCPU].Equal(resource.MustParse("50m"))) // 10m -> min 50m
	_, hasSidecarMem := sidecar.Requests[corev1.ResourceMemory]
	assert.False(t, hasSidecarMem, "memory not in recommendation -> should not be injected")
	assert.Nil(t, sidecar.Limits, "limits should be stripped by RequestsOnly")

	// --- "filtered" (index 2): ControlledResources=[cpu] only ---
	filtered := vertical.ContainerResources[2]
	assert.Equal(t, "filtered", filtered.Name)
	assert.True(t, filtered.Requests[corev1.ResourceCPU].Equal(resource.MustParse("200m"))) // unchanged
	_, hasMemReq := filtered.Requests[corev1.ResourceMemory]
	_, hasEphReq := filtered.Requests[corev1.ResourceEphemeralStorage]
	assert.False(t, hasMemReq, "memory should be removed (not controlled)")
	assert.False(t, hasEphReq, "ephemeral-storage should be removed (not controlled)")
	assert.True(t, filtered.Limits[corev1.ResourceCPU].Equal(resource.MustParse("500m"))) // unchanged
	_, hasMemLim := filtered.Limits[corev1.ResourceMemory]
	assert.False(t, hasMemLim, "memory limit should be removed (not controlled)")

	// --- "limit-bump" (index 3): MinAllowed pushes request above limit, limit raised to match ---
	limitBump := vertical.ContainerResources[3]
	assert.Equal(t, "limit-bump", limitBump.Name)
	assert.True(t, limitBump.Requests[corev1.ResourceCPU].Equal(resource.MustParse("200m")), "request clamped up to min")
	assert.True(t, limitBump.Limits[corev1.ResourceCPU].Equal(resource.MustParse("200m")), "limit raised to match request")

	// --- Limit reason: "app", "sidecar", "limit-bump" were clamped; "filtered" was not ---
	require.Error(t, limitErr)
	var condErr autoscaling.ConditionReason
	require.ErrorAs(t, limitErr, &condErr)
	assert.Equal(t, autoscaling.ConditionReasonLimitedByConstraint, condErr.Reason())
	assert.Contains(t, limitErr.Error(), "app")
	assert.Contains(t, limitErr.Error(), "sidecar")
	assert.Contains(t, limitErr.Error(), "limit-bump")
	assert.NotContains(t, limitErr.Error(), "filtered")

	// --- Hash should be recomputed and valid ---
	assert.NotEqual(t, "original-hash", vertical.ResourcesHash)
	expectedHash, err := autoscaling.ObjectHash(vertical.ContainerResources)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, vertical.ResourcesHash)
}

func TestApplyVerticalConstraints_ValidationErrors(t *testing.T) {
	vertical := &model.VerticalScalingValues{
		ResourcesHash: "original-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{Name: "app", Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m")}},
		},
	}

	// Duplicate container name
	_, err := applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{Name: "app", MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "app", MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}},
		},
	})
	require.Error(t, err)
	var condErr autoscaling.ConditionReason
	require.ErrorAs(t, err, &condErr)
	assert.Equal(t, autoscaling.ConditionReasonInvalidSpec, condErr.Reason())
	assert.Contains(t, err.Error(), "duplicate constraint for container")

	// Duplicate wildcard
	_, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{Name: "*", MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("50m")}},
			{Name: "*", MaxAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("4")}},
		},
	})
	require.Error(t, err)
	require.ErrorAs(t, err, &condErr)
	assert.Equal(t, autoscaling.ConditionReasonInvalidSpec, condErr.Reason())
	assert.Contains(t, err.Error(), "duplicate wildcard")

	// Vertical values should be untouched
	assert.Equal(t, "original-hash", vertical.ResourcesHash)
}
