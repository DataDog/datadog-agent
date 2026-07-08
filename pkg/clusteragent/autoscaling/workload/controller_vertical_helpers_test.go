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

	// Last action was a rollout for THIS recommendation
	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(time.Now().Add(-1 * time.Minute)),
		Version: recommendationID,
		Type:    datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
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

// TestShouldTriggerRollout_BurstableTransitionBypassesOngoingRollout verifies that
// switching from non-burstable to burstable mode triggers a new rollout even when
// one is already in progress. applyVerticalConstraints stamps removeLimitSentinel (-1) on
// the CPU limit when burstable=true, so hasLimitIncrease sees Sign() <= 0 ("no CPU limit")
// in the recommendation while the pod still has one → limit increase → bypass fires.
func TestShouldTriggerRollout_BurstableTransitionBypassesOngoingRollout(t *testing.T) {
	baseHash := "abc123"
	// After applyVerticalConstraints with burstable=true the hash changes; simulate
	// a new recommendation ID distinct from the old one.
	newRecommendationID := baseHash + "-new"
	cpuLimit := float64(50) // 500m

	// Pod is still on the non-burstable recommendation
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name:        "pod-1",
				Annotations: map[string]string{model.RecommendationIDAnnotation: baseHash},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{
					Name:      "app",
					Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit},
				},
			},
		},
	}
	podsPerRecommendationID := map[string]int32{baseHash: 1, newRecommendationID: 0}

	// applyVerticalConstraints has already stamped removeLimitSentinel (-1) on the CPU limit,
	// signalling "remove this limit from the pod" (burstable mode).
	recommendation := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{"cpu": resource.MustParse("250m")},
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("-1")}, // removeLimitSentinel
			},
		},
	}

	lastAction := &datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(time.Now().Add(-15 * time.Minute)),
		Version: baseHash,
		Type:    datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
	}

	decision := shouldTriggerRollout(
		newRecommendationID,
		pods,
		podsPerRecommendationID,
		lastAction,
		true, // rollout in progress
		recommendation,
		time.Now(),
		5*time.Minute,
		"test-autoscaler",
	)

	// Must trigger: removeLimitSentinel (-1) means CPU limit is removed (unlimited > 500m = limit increase)
	assert.Equal(t, rolloutDecisionTrigger, decision)
}

// TestHasLimitIncrease_BurstableRemovesCPULimit verifies that when applyVerticalConstraints
// stamps removeLimitSentinel (-1) on the CPU limit (burstable mode), hasLimitIncrease correctly
// detects a limit increase: the pod has a CPU limit but the recommendation removes it.
func TestHasLimitIncrease_BurstableRemovesCPULimit(t *testing.T) {
	cpuLimit := float64(50) // 500m
	pods := []*workloadmeta.KubernetesPod{
		{
			EntityMeta: workloadmeta.EntityMeta{
				Name: "pod-1", Namespace: "default",
				Annotations: map[string]string{model.RecommendationIDAnnotation: "hash"},
			},
			Containers: []workloadmeta.OrchestratorContainer{
				{Name: "app", Resources: workloadmeta.ContainerResources{CPULimit: &cpuLimit}},
			},
		},
	}

	// Without sentinel: same CPU limit as pod → no increase
	notBurstable := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "app",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("500m")},
			},
		},
	}
	assert.False(t, hasLimitIncrease(notBurstable, pods, "hash-v2"))

	// With removeLimitSentinel (-1) (set by applyVerticalConstraints in burstable mode):
	// CPU limit Sign() < 0 → treated as absent → pod has CPU limit → limit increase
	burstable := &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:   "app",
				Limits: corev1.ResourceList{"cpu": resource.MustParse("-1")}, // removeLimitSentinel
			},
		},
	}
	assert.True(t, hasLimitIncrease(burstable, pods, "hash-v3"))
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
	limitErr, err := applyVerticalConstraints(vertical, nil, false)
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// Empty container list
	limitErr, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{}, false)
	assert.NoError(t, err)
	assert.NoError(t, limitErr)

	// No matching constraint name
	limitErr, err = applyVerticalConstraints(vertical, &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{Name: "other-container", MinAllowed: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")}},
		},
	}, false)
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
	}, false)
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

	limitErr, err := applyVerticalConstraints(vertical, constraints, false)
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

func TestApplyVerticalConstraints_CPURequestsRemoveLimits(t *testing.T) {
	vertical := &model.VerticalScalingValues{
		ResourcesHash: "original-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{"cpu": resource.MustParse("300m"), "memory": resource.MustParse("256Mi")},
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("600m"), "memory": resource.MustParse("512Mi")},
			},
		},
	}
	constraints := &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{
				Name:             "app",
				ControlledValues: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerContainerControlledValuesCPURequestsRemoveLimitsMemoryRequestsAndLimits),
			},
		},
	}

	limitErr, err := applyVerticalConstraints(vertical, constraints, false)
	require.NoError(t, err)
	assert.Nil(t, limitErr)

	require.Len(t, vertical.ContainerResources, 1)
	app := vertical.ContainerResources[0]

	// CPU limit must carry the sentinel value so patchContainerResources removes it from the pod
	cpuLimit, exists := app.Limits[corev1.ResourceCPU]
	require.True(t, exists, "CPU key must be present in limits (sentinel)")
	assert.Equal(t, 0, cpuLimit.Cmp(removeLimitSentinel), "CPU limit must be the remove-limit sentinel value")
	// Memory limit must be preserved
	assert.Equal(t, resource.MustParse("512Mi"), app.Limits[corev1.ResourceMemory], "memory limit must be preserved")
	// CPU and memory requests must be preserved
	assert.Equal(t, resource.MustParse("300m"), app.Requests[corev1.ResourceCPU])
	assert.Equal(t, resource.MustParse("256Mi"), app.Requests[corev1.ResourceMemory])

	// Hash must be recomputed
	assert.NotEqual(t, "original-hash", vertical.ResourcesHash)
	expectedHash, err := autoscaling.ObjectHash(vertical.ContainerResources)
	require.NoError(t, err)
	assert.Equal(t, expectedHash, vertical.ResourcesHash)
}

// TestApplyVerticalConstraints_CPURequestsRemoveLimits_PerContainer verifies that the
// CPU-limit removal sentinel is applied per-container: with burstable=false, only the
// container whose ControlledValues is CPURequestsRemoveLimitsMemoryRequestsAndLimits gets
// its CPU limit stamped, while a sibling container with a plain constraint keeps its CPU
// limit untouched. This guards the per-container granularity against being collapsed into
// the autoscaler-wide burstable flag.
func TestApplyVerticalConstraints_CPURequestsRemoveLimits_PerContainer(t *testing.T) {
	vertical := &model.VerticalScalingValues{
		ResourcesHash: "original-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{"cpu": resource.MustParse("300m"), "memory": resource.MustParse("256Mi")},
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("600m"), "memory": resource.MustParse("512Mi")},
			},
			{
				Name:     "sidecar",
				Requests: corev1.ResourceList{"cpu": resource.MustParse("100m"), "memory": resource.MustParse("128Mi")},
				Limits:   corev1.ResourceList{"cpu": resource.MustParse("200m"), "memory": resource.MustParse("256Mi")},
			},
		},
	}
	constraints := &datadoghqcommon.DatadogPodAutoscalerConstraints{
		Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
			{
				Name:             "app",
				ControlledValues: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerContainerControlledValuesCPURequestsRemoveLimitsMemoryRequestsAndLimits),
			},
			{
				// Plain constraint, no CPU-limit removal requested.
				Name:       "sidecar",
				MaxAllowed: corev1.ResourceList{"cpu": resource.MustParse("2")},
			},
		},
	}

	limitErr, err := applyVerticalConstraints(vertical, constraints, false)
	require.NoError(t, err)
	assert.Nil(t, limitErr)

	require.Len(t, vertical.ContainerResources, 2)
	byName := map[string]datadoghqcommon.DatadogPodAutoscalerContainerResources{}
	for _, cr := range vertical.ContainerResources {
		byName[cr.Name] = cr
	}

	// "app" requested CPU-limit removal -> sentinel stamped.
	appCPULimit, exists := byName["app"].Limits[corev1.ResourceCPU]
	require.True(t, exists, "app CPU key must be present in limits (sentinel)")
	assert.Equal(t, 0, appCPULimit.Cmp(removeLimitSentinel), "app CPU limit must be the remove-limit sentinel value")

	// "sidecar" did not request removal -> CPU limit must be preserved verbatim, not a sentinel.
	sidecarCPULimit, exists := byName["sidecar"].Limits[corev1.ResourceCPU]
	require.True(t, exists, "sidecar CPU limit must be preserved")
	assert.Equal(t, 0, sidecarCPULimit.Cmp(resource.MustParse("200m")), "sidecar CPU limit must be unchanged")
	assert.Greater(t, sidecarCPULimit.Sign(), 0, "sidecar CPU limit must not carry the removal sentinel")
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
	}, false)
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
	}, false)
	require.Error(t, err)
	require.ErrorAs(t, err, &condErr)
	assert.Equal(t, autoscaling.ConditionReasonInvalidSpec, condErr.Reason())
	assert.Contains(t, err.Error(), "duplicate wildcard")

	// Vertical values should be untouched
	assert.Equal(t, "original-hash", vertical.ResourcesHash)
}

func TestFromAutoscalerToContainerResourcePatches_PreservesPodOrder(t *testing.T) {
	sv := &model.VerticalScalingValues{
		ResourcesHash: "r1",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{Name: "c3", Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("300m")}},
			{Name: "c1", Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")}},
			{Name: "c2", Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")}},
		},
	}
	ai := (&model.FakePodAutoscalerInternal{
		Namespace:     "default",
		Name:          "ai",
		ScalingValues: model.ScalingValues{Vertical: sv},
	}).Build()

	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{ID: "pod1"},
		// Pod defines containers in a specific order that differs from the recommendation.
		Containers: []workloadmeta.OrchestratorContainer{
			{Name: "c1"},
			{Name: "c2"},
			{Name: "c3"},
		},
	}

	patches := fromAutoscalerToContainerResourcePatches(&ai, pod)

	require.Len(t, patches, 3)
	assert.Equal(t, "c1", patches[0].Name, "patch order must follow pod container order")
	assert.Equal(t, "c2", patches[1].Name)
	assert.Equal(t, "c3", patches[2].Name)
}

func TestFromAutoscalerToContainerResourcePatches_Burstable(t *testing.T) {
	sv := &model.VerticalScalingValues{
		ResourcesHash: "r1",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{
				Name:     "app",
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")},
			},
		},
	}

	pod := &workloadmeta.KubernetesPod{
		EntityID:   workloadmeta.EntityID{ID: "pod1"},
		Containers: []workloadmeta.OrchestratorContainer{{Name: "app"}},
	}

	t.Run("burstable=true: cpu removed from limits, LimitsToDelete set", func(t *testing.T) {
		ai := (&model.FakePodAutoscalerInternal{
			Namespace:            "default",
			Name:                 "ai",
			ScalingValues:        model.ScalingValues{Vertical: sv},
			PreviewAnnotationKey: `{"burstable":true}`,
		}).Build()

		patches := fromAutoscalerToContainerResourcePatches(&ai, pod)

		require.Len(t, patches, 1)
		p := patches[0]
		assert.Equal(t, "app", p.Name)
		assert.NotContains(t, p.Limits, "cpu", "cpu must not be set in limits when burstable")
		assert.Equal(t, "512Mi", p.Limits["memory"], "memory limit must be unchanged")
		assert.Equal(t, []string{"cpu"}, p.LimitsToDelete, "cpu must be listed for deletion")
	})

	t.Run("burstable=false: cpu limit set normally, LimitsToDelete empty", func(t *testing.T) {
		ai := (&model.FakePodAutoscalerInternal{
			Namespace:     "default",
			Name:          "ai",
			ScalingValues: model.ScalingValues{Vertical: sv},
		}).Build()

		patches := fromAutoscalerToContainerResourcePatches(&ai, pod)

		require.Len(t, patches, 1)
		p := patches[0]
		assert.Equal(t, "500m", p.Limits["cpu"], "cpu limit must be set when not burstable")
		assert.Empty(t, p.LimitsToDelete, "LimitsToDelete must be empty when not burstable")
	})
}

const restartContainer = string(corev1.RestartContainer)

func disruptionReco(name, cpu, mem string) *model.VerticalScalingValues {
	requests := corev1.ResourceList{}
	if cpu != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(cpu)
	}
	if mem != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(mem)
	}
	return &model.VerticalScalingValues{
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
			{Name: name, Requests: requests},
		},
	}
}

func TestIsDisruptiveResize(t *testing.T) {
	cpu500m := float64(50)
	mem512 := uint64(512 * 1024 * 1024)

	container := func(policy workloadmeta.ContainerResizePolicy) workloadmeta.OrchestratorContainer {
		return workloadmeta.OrchestratorContainer{
			Name:         "app",
			Resources:    workloadmeta.ContainerResources{CPURequest: &cpu500m, MemoryRequest: &mem512},
			ResizePolicy: policy,
		}
	}
	podWith := func(c workloadmeta.OrchestratorContainer) *workloadmeta.KubernetesPod {
		return &workloadmeta.KubernetesPod{Containers: []workloadmeta.OrchestratorContainer{c}}
	}

	tests := []struct {
		name string
		pod  *workloadmeta.KubernetesPod
		reco *model.VerticalScalingValues
		want bool
	}{
		{
			name: "no resize policy + cpu changing",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{})),
			reco: disruptionReco("app", "1000m", "512Mi"),
			want: false,
		},
		{
			name: "cpu RestartContainer + cpu changing",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{CPURestartPolicy: restartContainer})),
			reco: disruptionReco("app", "1000m", "512Mi"),
			want: true,
		},
		{
			name: "memory RestartContainer + only cpu changing",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{MemoryRestartPolicy: restartContainer})),
			reco: disruptionReco("app", "1000m", "512Mi"),
			want: false,
		},
		{
			name: "cpu RestartContainer + cpu unchanged",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{CPURestartPolicy: restartContainer})),
			reco: disruptionReco("app", "500m", "512Mi"),
			want: false,
		},
		{
			name: "memory RestartContainer + memory changing",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{MemoryRestartPolicy: restartContainer})),
			reco: disruptionReco("app", "500m", "1Gi"),
			want: true,
		},
		{
			name: "recommendation container absent from pod",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{CPURestartPolicy: restartContainer})),
			reco: disruptionReco("other", "1000m", "1Gi"),
			want: false,
		},
		{
			name: "nil recommendation",
			pod:  podWith(container(workloadmeta.ContainerResizePolicy{CPURestartPolicy: restartContainer})),
			reco: nil,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isDisruptiveResize(tc.pod, tc.reco))
		})
	}
}

func TestAllowedDisruptions(t *testing.T) {
	tests := []struct {
		name             string
		configured       int
		alreadyDisrupted int
		want             int
	}{
		{name: "no replicas", configured: 0, alreadyDisrupted: 0, want: 0},
		{name: "single replica, healthy", configured: 1, alreadyDisrupted: 0, want: 1},
		{name: "single replica, already disrupted", configured: 1, alreadyDisrupted: 1, want: 0},
		{name: "even fleet, nothing disrupted", configured: 20, alreadyDisrupted: 0, want: 3},
		{name: "even fleet, partially consumed", configured: 20, alreadyDisrupted: 1, want: 2},
		{name: "even fleet, budget exhausted", configured: 20, alreadyDisrupted: 3, want: 0},
		{name: "even fleet, over budget", configured: 20, alreadyDisrupted: 5, want: 0},
		{name: "small fleet truncates tolerance", configured: 3, alreadyDisrupted: 0, want: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, allowedDisruptions(tc.configured, tc.alreadyDisrupted))
		})
	}
}

func TestCountDisruptedPods(t *testing.T) {
	ready := &workloadmeta.KubernetesPod{Ready: true}
	notReady := &workloadmeta.KubernetesPod{Ready: false}
	m := map[PodResizeStatus][]classifiedPod{
		PodResizeStatusNeedsPatch: {{pod: ready}, {pod: notReady}}, // NotReady counts, Ready does not
		PodResizeStatusCompleted:  {{pod: ready}},                  // on target + Ready: not disrupted
		PodResizeStatusInProgress: {{pod: ready}},                  // in-flight counts even while Ready
		PodResizeStatusDeferred:   {{pod: ready}},                  // in-flight counts even while Ready
		PodResizeStatusEvicting:   {{pod: notReady}},               // being evicted: counts
	}
	assert.Equal(t, 4, countDisruptedPods(m))
	assert.Equal(t, 0, countDisruptedPods(nil))
}

// TestApplyVerticalConstraints_BurstableHashChange verifies that enabling and disabling
// burstable mode produces distinct ResourcesHash values.  This hash difference is what
// the vertical controller uses as the recommendationID: when pods carry the old hash and
// the new recommendationID differs, a rollout is triggered.  Concretely:
//   - burstable=false with no constraints is a no-op (hash unchanged from backend value)
//   - burstable=true stamps removeLimitSentinel on every CPU limit and recomputes the hash
//
// The two hashes must never be equal, otherwise unsetting spec.options.burstable would not
// trigger a rollout to restore CPU limits.
func TestApplyVerticalConstraints_BurstableHashChange(t *testing.T) {
	backendHash := "backend-hash-v1"
	baseRec := func() *model.VerticalScalingValues {
		return &model.VerticalScalingValues{
			ResourcesHash: backendHash,
			ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name:     "app",
					Requests: corev1.ResourceList{"cpu": resource.MustParse("200m")},
					Limits:   corev1.ResourceList{"cpu": resource.MustParse("400m")},
				},
			},
		}
	}

	t.Run("burstable=false with no constraints leaves hash unchanged", func(t *testing.T) {
		rec := baseRec()
		_, err := applyVerticalConstraints(rec, nil, false)
		require.NoError(t, err)
		assert.Equal(t, backendHash, rec.ResourcesHash,
			"burstable=false with no constraints must not modify the hash")
	})

	t.Run("burstable=true stamps sentinel and recomputes hash", func(t *testing.T) {
		rec := baseRec()
		_, err := applyVerticalConstraints(rec, nil, true)
		require.NoError(t, err)
		assert.NotEqual(t, backendHash, rec.ResourcesHash,
			"burstable=true must recompute the hash after stamping the CPU-limit sentinel")
		cpuLimit := rec.ContainerResources[0].Limits[corev1.ResourceCPU]
		assert.Equal(t, removeLimitSentinel, cpuLimit,
			"burstable=true must stamp removeLimitSentinel on each CPU limit")
	})

	t.Run("burstable hash differs from non-burstable hash — rollout is triggered on toggle", func(t *testing.T) {
		withBurstable := baseRec()
		_, err := applyVerticalConstraints(withBurstable, nil, true)
		require.NoError(t, err)

		withoutBurstable := baseRec()
		// burstable=false with no constraints is a no-op; hash stays as backendHash.
		_, err = applyVerticalConstraints(withoutBurstable, nil, false)
		require.NoError(t, err)

		assert.NotEqual(t, withBurstable.ResourcesHash, withoutBurstable.ResourcesHash,
			"toggling burstable must change the recommendationID so the vertical controller "+
				"detects that pods carry a stale hash and triggers a rollout to restore CPU limits")
	})
}
