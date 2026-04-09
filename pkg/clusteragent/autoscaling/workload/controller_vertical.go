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

	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	k8sclient "k8s.io/client-go/kubernetes"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/evictor"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// rolloutCheckRequeueDelay is the delay between rollout status checks
	rolloutCheckRequeueDelay = 2 * time.Minute
	// minDelayBetweenRollouts is the minimum time between bypass rollout triggers to prevent thrashing
	minDelayBetweenRollouts = 10 * time.Minute
	// inplaceResizeRequeueDelay is the delay between in-place resize progress checks
	inplaceResizeRequeueDelay = 30 * time.Second
)

// verticalController is responsible for updating targetRef objects with the vertical recommendations
type verticalController struct {
	clock                      clock.Clock
	eventRecorder              record.EventRecorder
	client                     k8sclient.Interface
	isLeader                   func() bool
	patchClient                *workloadpatcher.Patcher
	podWatcher                 PodWatcher
	progressTracker            *rolloutProgressTracker
	inPlaceResizeSupported     *bool
	inPlaceResizeSupportedTime time.Time
}

// newVerticalController creates a new *verticalController
func newVerticalController(clock clock.Clock, eventRecorder record.EventRecorder, client k8sclient.Interface, isLeader func() bool, patchClient *workloadpatcher.Patcher, pw PodWatcher) *verticalController {
	return &verticalController{
		clock:           clock,
		eventRecorder:   eventRecorder,
		client:          client,
		isLeader:        isLeader,
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
	if autoscalerInternal.IsBurstable() {
		recommendationID += "-burstable"
	}

	// Get the pods for the pod owner
	pods := u.podWatcher.GetPodsForOwner(target)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		log.Debugf("No pods found for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec().TargetRef.Name)
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	}

	// Compute pods per resourceHash and per owner (needed by the rollout path).
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

	// Classify each non-terminating pod by resize status so we can set scaled replicas
	// (completed pods count) accurately. Pass this slice to syncInternal to avoid
	// a duplicate call to getPodResizeStatus.
	podsByResizeStatus := make(map[PodResizeStatus][]classifiedPod)
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		status, ltt := getPodResizeStatus(pod, recommendationID)
		podsByResizeStatus[status] = append(podsByResizeStatus[status], classifiedPod{pod: pod, lastTransitionTime: ltt})
	}
	autoscalerInternal.SetScaledReplicas(int32(len(podsByResizeStatus[PodResizeStatusCompleted])))

	// Check if we're allowed to rollout, we don't care about the source in this case, so passing most favorable source: manual
	updateStrategy, reason := getVerticalPatchingStrategy(autoscalerInternal)
	if updateStrategy == datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy {
		autoscalerInternal.UpdateFromVerticalAction(nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonPolicyRestricted, "%s", reason))
		return autoscaling.NoRequeue, nil
	}

	return u.syncInternal(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID, pods, podsPerRecommendationID, podsPerDirectOwner, podsByResizeStatus)
}

// syncInternal is the internal implementation of the vertical controller.
func (u *verticalController) syncInternal(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
	podsPerDirectOwner map[string]int32,
	podsByResizeStatus map[PodResizeStatus][]classifiedPod,
) (autoscaling.ProcessResult, error) {

	// Fall back to rollout if in-place scaling is not enabled via config, if
	// TriggerRollout mode is explicitly set, or if the API server does not
	// support in-place resize (pods/resize subresource unavailable).
	if isRolloutRequired(autoscalerInternal) || !u.isInPlaceResizeSupported() {
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

	// In-place resize path.
	// If this is a new recommendation, record the action (regardless of whether the patches
	// below succeed) and reset the eviction counter for this cycle.
	var toEvictOnPatchFailure []classifiedPod
	var patchForbidden bool
	if needsPatch := podsByResizeStatus[PodResizeStatusNeedsPatch]; len(needsPatch) > 0 {
		lastAction := autoscalerInternal.VerticalLastAction()
		if lastAction == nil || lastAction.Version != recommendationID {
			autoscalerInternal.UpdateFromVerticalAction(&datadoghqcommon.DatadogPodAutoscalerVerticalAction{
				Time:    metav1.NewTime(u.clock.Now()),
				Version: recommendationID,
				Type:    datadoghqcommon.DatadogPodAutoscalerResizeTriggeredVerticalActionType,
			}, nil)
			autoscalerInternal.SetEvictedReplicas(0)
		}

		for _, cp := range needsPatch {
			if err := u.patchInPlace(ctx, autoscalerInternal, cp.pod, recommendationID); err != nil {
				if k8serrors.IsNotFound(err) {
					// Pod is already gone; the pod watcher hasn't caught up yet. Skip eviction.
					log.Debugf("pod %s/%s not found during resize patch, likely already evicted: %v", cp.pod.Namespace, cp.pod.Name, err)
					continue
				}
				if k8serrors.IsForbidden(err) {
					patchForbidden = true
				}
				log.Warnf("failed to patch pod %s/%s in place: %v", cp.pod.Namespace, cp.pod.Name, err)
				autoscalerInternal.InPlacePatchErrorInc()
				toEvictOnPatchFailure = append(toEvictOnPatchFailure, cp)
			} else {
				autoscalerInternal.InPlacePatchSuccessInc()
			}
		}
	}

	// Build the list of pods to evict: pods with unresolvable conditions, plus any
	// pods whose resize patch failed (the API rejected it — e.g. unsupported on this cluster).
	toEvict := make([]classifiedPod, 0, len(podsByResizeStatus[PodResizeStatusError])+len(podsByResizeStatus[PodResizeStatusInfeasible])+len(toEvictOnPatchFailure))
	toEvict = append(toEvict, podsByResizeStatus[PodResizeStatusError]...)
	toEvict = append(toEvict, podsByResizeStatus[PodResizeStatusInfeasible]...)
	toEvict = append(toEvict, toEvictOnPatchFailure...)
	now := u.clock.Now()
	for _, cp := range podsByResizeStatus[PodResizeStatusDeferred] {
		if shouldEvictDeferred(podAutoscaler, now) {
			toEvict = append(toEvict, cp)
		}
	}

	// If the resize patch was rejected as forbidden (e.g. RBAC missing for
	// pods/resize), or any pod has been stuck in an unresolvable state for longer
	// than RolloutFallbackDelay, escalate to a full rollout.
	if shouldFallbackToRollout(toEvict, podAutoscaler, now, patchForbidden) {
		if patchForbidden {
			log.Infof("In-place resize fallback: pods/resize patch forbidden, triggering rollout for autoscaler %s", autoscalerInternal.ID())
		} else {
			log.Infof("In-place resize fallback: pods stuck too long, triggering rollout for autoscaler %s", autoscalerInternal.ID())
		}
		autoscalerInternal.InPlaceRolloutFallbackInc()
		return u.triggerRollout(ctx, podAutoscaler, autoscalerInternal, target, targetGVK, recommendationID)
	}

	// Evict pods that cannot resize in-place, counting only successful evictions and
	// stopping on PDB rejection so we don't disrupt more pods than the budget allows.
	var evictedThisSync, failedEvictions int32
	var pdbBlocked bool
	for _, cp := range toEvict {
		result, err := u.evictPod(ctx, cp.pod)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				log.Debugf("pod %s/%s not found during eviction", cp.pod.Namespace, cp.pod.Name)
			} else {
				log.Warnf("error while evicting pod %s/%s: %v", cp.pod.Namespace, cp.pod.Name, err)
				failedEvictions++
				autoscalerInternal.InPlaceEvictionErrorInc()
				autoscalerInternal.UpdateFromVerticalAction(nil,
					autoscaling.NewConditionError(autoscaling.ConditionReasonFailedToEvict,
						fmt.Errorf("error while evicting pod %s/%s: %w", cp.pod.Namespace, cp.pod.Name, err)))
				autoscalerInternal.VerticalActionErrorInc()
			}
		}
		if result == evictor.Evicted {
			evictedThisSync++
			autoscalerInternal.InPlaceEvictionSuccessInc()
		}
		if result == evictor.PDBLockedOrThrottle || result == evictor.Skipped {
			pdbBlocked = true
			autoscalerInternal.InPlacePDBBlockedInc()
			break
		}
	}
	autoscalerInternal.AddEvictedReplicas(evictedThisSync)

	// Emit a summary event
	if evictedThisSync > 0 || failedEvictions > 0 || pdbBlocked {
		parts := make([]string, 0, 3)
		if evictedThisSync > 0 {
			parts = append(parts, fmt.Sprintf("%d evicted", evictedThisSync))
		}
		if failedEvictions > 0 {
			parts = append(parts, fmt.Sprintf("%d failed", failedEvictions))
		}
		if pdbBlocked {
			parts = append(parts, "PDB blocked further evictions")
		}
		eventType := corev1.EventTypeNormal
		reason := model.InPlaceEvictedEventReason
		if failedEvictions > 0 || pdbBlocked {
			eventType = corev1.EventTypeWarning
			reason = model.FailedToEvictEventReason
		}
		u.eventRecorder.Eventf(podAutoscaler, eventType, reason,
			"In-place resize eviction: %s (%d pods pending)", strings.Join(parts, ", "), len(toEvict))
	}

	// Terminating pods are excluded from podsByResizeStatus, so summing all bucket lengths
	// gives the total active pod count. If every active pod is complete, the resize cycle is done.
	var totalActive int
	for _, bucket := range podsByResizeStatus {
		totalActive += len(bucket)
	}
	if len(podsByResizeStatus[PodResizeStatusCompleted]) == totalActive {
		if lastAction := autoscalerInternal.VerticalLastAction(); lastAction != nil &&
			lastAction.Type == datadoghqcommon.DatadogPodAutoscalerResizeTriggeredVerticalActionType {
			u.eventRecorder.Eventf(podAutoscaler, corev1.EventTypeNormal, model.ResizeSuccessfulEventReason,
				"All %d pods resized successfully for autoscaler %s/%s", totalActive, podAutoscaler.Namespace, podAutoscaler.Name)
			autoscalerInternal.InPlaceResizeCompletedInc()
			autoscalerInternal.UpdateFromVerticalAction(&datadoghqcommon.DatadogPodAutoscalerVerticalAction{
				Time:    metav1.NewTime(u.clock.Now()),
				Version: recommendationID,
				Type:    datadoghqcommon.DatadogPodAutoscalerResizeCompletedVerticalActionType,
			}, nil)
		}
		return autoscaling.NoRequeue, nil
	}
	return autoscaling.ProcessResult{Requeue: true, RequeueAfter: inplaceResizeRequeueDelay}, nil
}

// patchInPlace applies the resource recommendation to a single pod via the resize subresource,
// then updates the pod's RecommendationIDAnnotation to record the applied recommendation.
func (u *verticalController) patchInPlace(ctx context.Context, autoscalerInternal *model.PodAutoscalerInternal, pod *workloadmeta.KubernetesPod, recommendationID string) error {
	patchTarget := workloadpatcher.PodTarget(pod.Namespace, pod.Name)

	// Patch spec.containers[*].resources via the pods/resize subresource.
	containersResourcePatches := fromAutoscalerToContainerResourcePatches(autoscalerInternal, pod)
	intent := workloadpatcher.NewPatchIntent(patchTarget).With(workloadpatcher.SetContainerResources(containersResourcePatches))
	_, err := u.patchClient.Apply(ctx, intent, workloadpatcher.PatchOptions{Caller: "vpa", Subresource: "resize", PatchType: types.StrategicMergePatchType})
	if err != nil {
		return fmt.Errorf("failed to patch pod %s/%s resources in place, will evict: %w", pod.Namespace, pod.Name, err)
	}

	// Record the applied recommendation ID on the pod's own metadata annotations.
	intent = workloadpatcher.NewPatchIntent(patchTarget).With(workloadpatcher.SetMetadataAnnotations(map[string]interface{}{
		model.RecommendationIDAnnotation: recommendationID,
	}))
	_, err = u.patchClient.Apply(ctx, intent, workloadpatcher.PatchOptions{Caller: "vpa"})
	if err != nil {
		return fmt.Errorf("failed to patch pod %s/%s annotations in place, will evict: %w", pod.Namespace, pod.Name, err)
	}

	return nil
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

func (u *verticalController) evictPod(ctx context.Context, pod *workloadmeta.KubernetesPod) (evictor.EvictResult, error) {
	evictorClient := evictor.NewClient(u.client, u.isLeader)
	result, err := evictorClient.Evict(ctx, pod.Namespace, pod.Name)
	if err != nil {
		return evictor.Error, err
	}
	if result == evictor.Evicted {
		log.Infof("Successfully evicted pod %s/%s", pod.Namespace, pod.Name)
	}
	return result, nil
}
