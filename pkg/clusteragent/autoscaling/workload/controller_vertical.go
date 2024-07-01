// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	k8sutil "github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	rolloutCheckRequeueDelay = 2 * time.Minute
)

// verticalController is responsible for updating targetRef objects with the vertical recommendations
type verticalController struct {
	clock         clock.Clock
	eventRecorder record.EventRecorder
	dynamicClient dynamic.Interface
	podWatcher    podWatcher
}

// newVerticalController creates a new *verticalController
func newVerticalController(clock clock.Clock, eventRecorder record.EventRecorder, cl dynamic.Interface, pw podWatcher) *verticalController {
	res := &verticalController{
		clock:         clock,
		eventRecorder: eventRecorder,
		dynamicClient: cl,
		podWatcher:    pw,
	}
	return res
}

func (u *verticalController) sync(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, autoscalerInternal *model.PodAutoscalerInternal) (autoscaling.ProcessResult, error) {
	// Check if the autoscaler has a vertical scaling recommendation
	if autoscalerInternal.ScalingValues.Vertical == nil || autoscalerInternal.ScalingValues.Vertical.ResourcesHash == "" {
		// Clearing live state if no recommendation is available
		autoscalerInternal.VerticalLastActionError = nil
		autoscalerInternal.ScaledReplicas = nil
		return autoscaling.NoRequeue, nil
	}

	recomendationID := autoscalerInternal.ScalingValues.Vertical.ResourcesHash
	targetGVK, err := autoscalerInternal.GetTargetGVK()
	if err != nil {
		return autoscaling.NoRequeue, err
	}

	// Get the pod owner from the workload
	target := NamespacedPodOwner{
		Namespace: autoscalerInternal.Namespace,
		Name:      autoscalerInternal.Spec.TargetRef.Name,
		Kind:      targetGVK.Kind,
	}

	// Get the pods for the pod owner
	pods := u.podWatcher.GetPodsForOwner(target)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		log.Debugf("No pods found for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec.TargetRef.Name)
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	}

	// Compute pods per resourceHash and per owner
	podsPerRecomendationID := make(map[string]int32)
	podsPerDirectOwner := make(map[string]int32)
	for _, pod := range pods {
		// PODs without any recommendation will be stored with "" key
		podsPerRecomendationID[pod.Annotations[model.RecommendationIDAnnotation]] = podsPerRecomendationID[pod.Annotations[model.RecommendationIDAnnotation]] + 1

		if len(pod.Owners) == 0 {
			// This condition should never happen since the pod watcher groups pods by owner
			log.Warnf("Pod %s/%s has no owner", pod.Namespace, pod.Name)
			continue
		}
		podsPerDirectOwner[pod.Owners[0].ID] = podsPerDirectOwner[pod.Owners[0].ID] + 1
	}

	// Update scaled replicas status
	autoscalerInternal.ScaledReplicas = pointer.Ptr(podsPerRecomendationID[recomendationID])

	// Check if we're allowed to rollout, we don't care about the source in this case, so passing most favorable source: manual
	updateStrategy, reason := getVerticalPatchingStrategy(autoscalerInternal)
	if updateStrategy == datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy {
		autoscalerInternal.VerticalLastActionError = errors.New(reason)
		return autoscaling.NoRequeue, nil
	}

	// Check if last action was done in the `rolloutCheckRequeueDelay` window
	if autoscalerInternal.VerticalLastAction != nil && autoscalerInternal.VerticalLastAction.Time.Add(rolloutCheckRequeueDelay).After(u.clock.Now()) {
		log.Debugf("Last action was done less than %s ago for autoscaler: %s, skipping", rolloutCheckRequeueDelay.String(), autoscalerInternal.ID())
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	}

	switch targetGVK.Kind {
	case k8sutil.DeploymentKind:
		return u.syncDeploymentKind(ctx, podAutoscaler, autoscalerInternal, updateStrategy, target, targetGVK, recomendationID, pods, podsPerRecomendationID, podsPerDirectOwner)
	default:
		autoscalerInternal.VerticalLastActionError = fmt.Errorf("automic rollout not available for target Kind: %s. Applying to existing PODs require manual trigger", targetGVK.Kind)
		return autoscaling.NoRequeue, nil
	}
}

func (u *verticalController) syncDeploymentKind(
	ctx context.Context,
	podAutoscaler *datadoghq.DatadogPodAutoscaler,
	autoscalerInternal *model.PodAutoscalerInternal,
	_ datadoghq.DatadogPodAutoscalerUpdateStrategy,
	target NamespacedPodOwner,
	targetGVK schema.GroupVersionKind,
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecomendationID map[string]int32,
	podsPerDirectOwner map[string]int32,
) (autoscaling.ProcessResult, error) {
	// Check if we need to rollout, currently basic check with 100% match expected.
	// TODO: Refine the logic and add backoff for stuck PODs.
	if podsPerRecomendationID[recommendationID] == int32(len(pods)) {
		autoscalerInternal.VerticalLastActionError = nil
		return autoscaling.NoRequeue, nil
	}

	// Check if a rollout is already ongoing
	// TODO: Refine the logic and add backoff for stuck PODs.
	if len(podsPerDirectOwner) > 1 {
		log.Debugf("Rollout already ongoing for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec.TargetRef.Name)
		return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
	}

	// Normally we should check updateStrategy here, we currently only support one way, so not required for now.

	// Generate the patch request which adds the scaling hash annotation to the pod template
	gvr := targetGVK.GroupVersion().WithResource(fmt.Sprintf("%ss", strings.ToLower(targetGVK.Kind)))
	patchTime := u.clock.Now()
	patchData, err := json.Marshal(map[string]interface{}{
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{
						model.RolloutTimestampAnnotation: patchTime.Format(time.RFC3339),
						model.RecommendationIDAnnotation: recommendationID,
					},
				},
			},
		},
	})
	if err != nil {
		autoscalerInternal.VerticalLastActionError = fmt.Errorf("Unable to produce JSONPatch : %v", err)
		return autoscaling.Requeue, err
	}

	// Apply patch to trigger rollout
	_, err = u.dynamicClient.Resource(gvr).Namespace(target.Namespace).Patch(ctx, target.Name, types.StrategicMergePatchType, patchData, metav1.PatchOptions{})
	if err != nil {
		err = fmt.Errorf("failed to trigger rollout for gvk: %s, name: %s, err: %v", targetGVK.String(), autoscalerInternal.Spec.TargetRef.Name, err)
		rolloutTriggered.Inc(target.Kind, target.Name, target.Namespace, "error")
		autoscalerInternal.VerticalLastActionError = err
		u.eventRecorder.Event(podAutoscaler, corev1.EventTypeWarning, model.FailedTriggerRolloutEventReason, err.Error())

		return autoscaling.Requeue, err
	}

	// Propagating information about the rollout
	log.Infof("Successfully triggered rollout for autoscaler: %s, gvk: %s, name: %s", autoscalerInternal.ID(), targetGVK.String(), autoscalerInternal.Spec.TargetRef.Name)
	rolloutTriggered.Inc(target.Kind, target.Name, target.Namespace, "success")
	u.eventRecorder.Eventf(podAutoscaler, corev1.EventTypeNormal, model.SuccessfulTriggerRolloutEventReason, "Successfully triggered rollout on target:%s/%s", targetGVK.String(), autoscalerInternal.Spec.TargetRef.Name)

	autoscalerInternal.VerticalLastActionError = nil
	autoscalerInternal.VerticalLastAction = &datadoghq.DatadogPodAutoscalerVerticalAction{
		Time:    metav1.NewTime(patchTime),
		Version: recommendationID,
		Type:    datadoghq.DatadogPodAutoscalerRolloutTriggeredVerticalActionType,
	}

	// Requeue regularly to check for rollout completion
	return autoscaling.ProcessResult{Requeue: true, RequeueAfter: rolloutCheckRequeueDelay}, nil
}

// getVerticalPatchingStrategy applied policies to determine effective patching strategy.
// Return (strategy, reason). Reason is only returned when chosen strategy disables vertical patching.
func getVerticalPatchingStrategy(autoscalerInternal *model.PodAutoscalerInternal) (datadoghq.DatadogPodAutoscalerUpdateStrategy, string) {
	// If we don't have spec, we cannot take decisions, should not happen.
	if autoscalerInternal.Spec == nil {
		return datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy, "pod autoscaling hasn't been initialized yet"
	}

	// If we don't have a ScalingValue, we cannot take decisions, should not happen.
	if autoscalerInternal.ScalingValues.Vertical == nil {
		return datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy, "no scaling values available"
	}

	// By default, policy is to allow all
	if autoscalerInternal.Spec.Policy == nil {
		return datadoghq.DatadogPodAutoscalerAutoUpdateStrategy, ""
	}

	// We do have policies, checking if they allow this source
	if !model.ApplyModeAllowSource(autoscalerInternal.Spec.Policy.ApplyMode, autoscalerInternal.ScalingValues.Vertical.Source) {
		return datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy, fmt.Sprintf("vertical scaling disabled due to applyMode: %s not allowing recommendations from source: %s", autoscalerInternal.Spec.Policy.ApplyMode, autoscalerInternal.ScalingValues.Vertical.Source)
	}

	if autoscalerInternal.Spec.Policy.Update != nil {
		if autoscalerInternal.Spec.Policy.Update.Strategy == datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy {
			return datadoghq.DatadogPodAutoscalerDisabledUpdateStrategy, "vertical scaling disabled due to update strategy set to disabled"
		}

		return autoscalerInternal.Spec.Policy.Update.Strategy, ""
	}

	// No update strategy defined, defaulting to auto
	return datadoghq.DatadogPodAutoscalerAutoUpdateStrategy, ""
}
