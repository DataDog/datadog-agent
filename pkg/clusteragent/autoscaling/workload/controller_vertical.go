// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// rolloutCheckRequeueDelay is the delay between rollout status checks
	rolloutCheckRequeueDelay = 2 * time.Minute
	// minDelayBetweenRollouts is the minimum time between bypass rollout triggers to prevent thrashing
	minDelayBetweenRollouts = 10 * time.Minute
)

// verticalController is responsible for updating targetRef objects with the vertical recommendations
type verticalController struct {
	clock           clock.Clock
	eventRecorder   record.EventRecorder
	patchClient     *workloadpatcher.Patcher
	podWatcher      PodWatcher
	progressTracker *rolloutProgressTracker
}

// newVerticalController creates a new *verticalController
func newVerticalController(clock clock.Clock, eventRecorder record.EventRecorder, patchClient *workloadpatcher.Patcher, pw PodWatcher) *verticalController {
	return &verticalController{
		clock:           clock,
		eventRecorder:   eventRecorder,
		patchClient:     patchClient,
		podWatcher:      pw,
		progressTracker: newRolloutProgressTracker(),
	}
}

func (u *verticalController) sync(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, autoscalerInternal *model.PodAutoscalerInternal, targetGVK schema.GroupVersionKind, target NamespacedPodOwner) (autoscaling.ProcessResult, error) {
	// If vertical scaling is disabled, clear vertical state and exit.
	if !autoscalerInternal.IsVerticalScalingEnabled() {
		autoscalerInternal.ClearVerticalState()
		return autoscaling.NoRequeue, nil
	}

	scalingValues := autoscalerInternal.ScalingValues()

	// Check if the autoscaler has a vertical scaling recommendation
	if scalingValues.Vertical == nil || scalingValues.Vertical.ResourcesHash == "" {
		// Clearing live state if no recommendation is available
		autoscalerInternal.UpdateFromVerticalAction(nil, nil)
		return autoscaling.NoRequeue, nil
	}

	// Deep-copy to avoid mutating the original recommendation stored in mainScalingValues/fallbackScalingValues.
	// Without this, clamped values would persist and the VerticalScalingLimited condition would be
	// cleared on the next sync since constraints re-applied to already-clamped values are no-ops.
	constrainedVertical := scalingValues.Vertical.DeepCopy()
	limitErr, err := applyVerticalConstraints(constrainedVertical, autoscalerInternal.Spec().Constraints)
	if err != nil {
		autoscalerInternal.SetConstrainedVerticalScaling(nil, nil)
		autoscalerInternal.UpdateFromVerticalAction(nil, err)
		return autoscaling.NoRequeue, err
	}
	autoscalerInternal.SetConstrainedVerticalScaling(constrainedVertical, limitErr)

	recommendationID := constrainedVertical.ResourcesHash

	// Get the pods for the pod owner
	pods := u.podWatcher.GetPodsForOwner(target)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		log.Debugf("No pods found for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec().TargetRef.Name)
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	}

	// Compute pods per resourceHash and per owner
	podsPerRecommendationID := make(map[string]int32)
	podsPerDirectOwner := make(map[string]int32)
	for _, pod := range pods {
		// PODs without any recommendation will be stored with "" key
		podsPerRecommendationID[pod.Annotations[model.RecommendationIDAnnotation]] = podsPerRecommendationID[pod.Annotations[model.RecommendationIDAnnotation]] + 1

		if len(pod.Owners) == 0 {
			// This condition should never happen since the pod watcher groups pods by owner
			log.Warnf("Pod %s/%s has no owner", pod.Namespace, pod.Name)
			continue
		}
		podsPerDirectOwner[pod.Owners[0].ID] = podsPerDirectOwner[pod.Owners[0].ID] + 1
	}

	// Update scaled replicas status
	podsOnRecommendation := podsPerRecommendationID[recommendationID]
	autoscalerInternal.SetScaledReplicas(podsOnRecommendation)

	// Check if we're allowed to rollout, we don't care about the source in this case, so passing most favorable source: manual
	updateStrategy, reason := getVerticalPatchingStrategy(autoscalerInternal)
	if updateStrategy == datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy {
		autoscalerInternal.UpdateFromVerticalAction(nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonPolicyRestricted, "%s", reason))
		return autoscaling.NoRequeue, nil
	}

	switch targetGVK.Kind {
	case k8sutil.DeploymentKind:
		return u.syncDeploymentKind(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, pods, podsPerRecommendationID, podsPerDirectOwner)
	case k8sutil.RolloutKind:
		return u.syncRolloutKind(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, pods, podsPerRecommendationID, podsPerDirectOwner)
	case k8sutil.StatefulSetKind:
		return u.syncStatefulSetKind(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, pods, podsPerRecommendationID)
	default:
		autoscalerInternal.UpdateFromVerticalAction(nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonUnsupportedTargetKind, "automatic rollout not available for target Kind: %s. Applying to existing PODs require manual trigger", targetGVK.Kind))
		return autoscaling.NoRequeue, nil
	}
}

// triggerRollout patches the target workload's pod template to trigger a rollout.
// This is shared logic used by all workload types that support vertical scaling.
func (u *verticalController) triggerRollout(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
) (autoscaling.ProcessResult, error) {
	// Generate the patch request which adds the scaling hash annotation to the pod template
	gvr := targetGVK.GroupVersion().WithResource(strings.ToLower(targetGVK.Kind) + "s")
	patchTime := u.clock.Now()
	patchTarget := workloadpatcher.Target{GVR: gvr, Namespace: target.Namespace, Name: target.Name}
	intent := workloadpatcher.NewPatchIntent(patchTarget).
		With(workloadpatcher.SetPodTemplateAnnotations(map[string]interface{}{
			model.RolloutTimestampAnnotation: patchTime.Format(time.RFC3339),
			model.RecommendationIDAnnotation: recommendationID,
		}))

	// Apply patch to trigger rollout
	_, err := u.patchClient.Apply(ctx, intent, workloadpatcher.PatchOptions{Caller: "vpa"})
	if err != nil {
		err = autoscaling.NewConditionError(autoscaling.ConditionReasonRolloutFailed, fmt.Errorf("failed to trigger rollout for gvk: %s, name: %s, err: %v", targetGVK.String(), autoscalerInternal.Spec().TargetRef.Name, err))
		autoscalerInternal.UpdateFromVerticalAction(nil, err)
		autoscalerInternal.VerticalActionErrorInc()
		u.eventRecorder.Event(podAutoscaler, corev1.EventTypeWarning, model.FailedTriggerRolloutEventReason, err.Error())
		return autoscaling.Requeue, err
	}

	// Propagating information about the rollout
	log.Infof("Successfully triggered rollout for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec().TargetRef.Name)
	u.eventRecorder.Eventf(podAutoscaler, corev1.EventTypeNormal, model.SuccessfulTriggerRolloutEventReason, "Successfully triggered rollout on target:%s/%s", targetGVK.String(), autoscalerInternal.Spec().TargetRef.Name)
	autoscalerInternal.VerticalActionSuccessInc()
	autoscalerInternal.UpdateFromVerticalAction(&datadoghqcommon.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(patchTime),
		Version: recommendationID,
		Type:    datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
	}, nil)
	// Requeue regularly to check for rollout completion
	return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
}

func (u *verticalController) syncDeploymentKind(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
	podsPerDirectOwner map[string]int32,
) (autoscaling.ProcessResult, error) {
	// For Deployments/Rollouts, multiple direct owners (ReplicaSets) indicate an ongoing rollout
	rolloutInProgress := len(podsPerDirectOwner) > 1

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		autoscalerInternal.VerticalLastAction(),
		rolloutInProgress,
		autoscalerInternal.ScalingValues().Vertical,
		u.clock.Now(),
		minDelayBetweenRollouts,
		autoscalerInternal.ID(),
	)

	return u.handleRolloutDecision(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, podsPerRecommendationID, decision)
}

func (u *verticalController) syncRolloutKind(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
	podsPerDirectOwner map[string]int32,
) (autoscaling.ProcessResult, error) {
	// Argo Rollouts use the same pod template structure as Deployments,
	// so we can reuse the same rollout logic
	return u.syncDeploymentKind(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, pods, podsPerRecommendationID, podsPerDirectOwner)
}

func (u *verticalController) syncStatefulSetKind(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
) (autoscaling.ProcessResult, error) {
	// For StatefulSets, different controller-revision-hash labels indicate an ongoing rollout
	rolloutInProgress := isStatefulSetRolloutInProgress(pods)

	decision := shouldTriggerRollout(
		recommendationID,
		pods,
		podsPerRecommendationID,
		autoscalerInternal.VerticalLastAction(),
		rolloutInProgress,
		autoscalerInternal.ScalingValues().Vertical,
		u.clock.Now(),
		minDelayBetweenRollouts,
		autoscalerInternal.ID(),
	)

	return u.handleRolloutDecision(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, podsPerRecommendationID, decision)
}

// handleRolloutDecision processes the rollout decision and takes appropriate action.
// This is shared logic used by all workload types.
func (u *verticalController) handleRolloutDecision(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	podsPerRecommendationID map[string]int32,
	decision rolloutDecision,
) (autoscaling.ProcessResult, error) {
	switch decision {
	case rolloutDecisionComplete:
		u.progressTracker.Clear(autoscalerInternal.ID())
		autoscalerInternal.UpdateFromVerticalAction(nil, nil)
		return autoscaling.NoRequeue, nil
	case rolloutDecisionWait:
		// Check if the rollout is stalled (no pod movement for too long)
		isStalled := u.progressTracker.Update(autoscalerInternal.ID(), recommendationID, podsPerRecommendationID[recommendationID], u.clock.Now())
		if isStalled {
			log.Infof("Rollout stalled for autoscaler %s (no pod movement for %s), re-triggering rollout", autoscalerInternal.ID(), rolloutStallTimeout)
			return u.triggerRollout(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID)
		}
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	case rolloutDecisionTrigger:
		return u.triggerRollout(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID)
	}

	// This should never happen if all rolloutDecision values are handled above
	log.Errorf("Unknown rollout decision %d for autoscaler %s", decision, autoscalerInternal.ID())
	return autoscaling.NoRequeue, nil
}
