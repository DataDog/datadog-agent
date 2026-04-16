// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"fmt"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	workloadpatcher "github.com/DataDog/datadog-agent/pkg/clusteragent/patcher"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// controllerRevisionHashLabel is the label used by Kubernetes to track pod template revisions
	// for StatefulSets and DaemonSets. This is managed by Kubernetes itself and changes whenever
	// any part of the pod template changes.
	controllerRevisionHashLabel = "controller-revision-hash"

	// defaultResizePendingPeriod is the default delay for the resize pending period in seconds
	defaultResizePendingPeriod int32 = 600

	// defaultRolloutFallbackDelay is the default delay for the rollout fallback in seconds
	defaultRolloutFallbackDelay int32 = 1200
)

const inPlaceResizeSupportedCacheTTL = 15 * time.Minute

// isInPlaceResizeSupported checks whether the API server exposes the pods/resize
// subresource, which requires InPlacePodVerticalScaling to be enabled. The result
// is cached for 15 minutes.
func (u *verticalController) isInPlaceResizeSupported() bool {
	if u.inPlaceResizeSupported != nil && u.clock.Since(u.inPlaceResizeSupportedTime) < inPlaceResizeSupportedCacheTTL {
		return *u.inPlaceResizeSupported
	}
	if u.client == nil {
		return false
	}
	resources, err := u.client.Discovery().ServerResourcesForGroupVersion("v1")
	supported := err == nil && func() bool {
		for _, r := range resources.APIResources {
			if r.Name == "pods/resize" {
				return true
			}
		}
		return false
	}()
	u.inPlaceResizeSupported = &supported
	u.inPlaceResizeSupportedTime = u.clock.Now()
	return supported
}

// Pod resize condition types and reasons sourced directly from k8s.io/api/core/v1.
// See https://kubernetes.io/docs/tasks/configure-pod-container/resize-container-resources/#pod-resize-status
const (
	kubePodConditionResizePending                 = string(corev1.PodResizePending)
	kubePodConditionResizePendingReasonInfeasible = corev1.PodReasonInfeasible
	kubePodConditionResizePendingReasonDeferred   = corev1.PodReasonDeferred

	kubePodConditionResizeInProgress            = string(corev1.PodResizeInProgress)
	kubePodConditionResizeInProgressReasonError = corev1.PodReasonError
)

type PodResizeStatus int

const (
	PodResizeStatusNeedsPatch PodResizeStatus = iota
	PodResizeStatusCompleted
	PodResizeStatusInProgress
	PodResizeStatusError
	PodResizeStatusInfeasible
	PodResizeStatusDeferred
)

// classifiedPod pairs a pod with the LastTransitionTime of the condition that
// determined its resize status. For statuses with no relevant condition
// (NeedsPatch, Completed), LastTransitionTime is the zero value.
type classifiedPod struct {
	pod                *workloadmeta.KubernetesPod
	lastTransitionTime time.Time
}

// getVerticalPatchingStrategy applied policies to determine effective patching strategy.
// Return (strategy, reason). Reason is only returned when chosen strategy disables vertical patching.
func getVerticalPatchingStrategy(autoscalerInternal *model.PodAutoscalerInternal) (datadoghqcommon.DatadogPodAutoscalerUpdateStrategy, string) {
	// If we don't have spec, we cannot take decisions, should not happen.
	if autoscalerInternal.Spec() == nil {
		return datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy, "pod autoscaling hasn't been initialized yet"
	}

	// If we don't have a ScalingValue, we cannot take decisions, should not happen.
	if autoscalerInternal.ScalingValues().Vertical == nil {
		return datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy, "no scaling values available"
	}

	// By default, policy is to allow all
	if autoscalerInternal.Spec().ApplyPolicy == nil {
		return datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy, ""
	}

	// We do have policies, checking if they allow this source
	if !model.ApplyModeAllowSource(autoscalerInternal.Spec().ApplyPolicy.Mode, autoscalerInternal.ScalingValues().Vertical.Source) {
		return datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy, fmt.Sprintf("vertical scaling disabled due to applyMode: %s not allowing recommendations from source: %s", autoscalerInternal.Spec().ApplyPolicy.Mode, autoscalerInternal.ScalingValues().Vertical.Source)
	}

	if autoscalerInternal.Spec().ApplyPolicy.Update != nil {
		if autoscalerInternal.Spec().ApplyPolicy.Update.Strategy == datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy {
			return datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy, "vertical scaling disabled due to update strategy set to disabled"
		}

		return autoscalerInternal.Spec().ApplyPolicy.Update.Strategy, ""
	}

	// No update strategy defined, defaulting to auto
	return datadoghqcommon.DatadogPodAutoscalerAutoUpdateStrategy, ""
}

// isRecommendationRolloutComplete checks if the current recommendation is entirely rolled out.
// Returns true if all pods have the given recommendation ID.
func isRecommendationRolloutComplete(recommendationID string, pods []*workloadmeta.KubernetesPod, podsPerRecommendationID map[string]int32) bool {
	// currently basic check with 100% match expected.
	// TODO: Refine the logic and add backoff for stuck PODs.
	return podsPerRecommendationID[recommendationID] == int32(len(pods))
}

// isStatefulSetRolloutInProgress checks if a StatefulSet rollout is currently in progress
// by examining the controller-revision-hash labels on pods. If pods have different revision
// hashes, it indicates that Kubernetes is in the process of rolling out a new pod template.
// This detects ANY ongoing rollout, not just ones triggered by us.
func isStatefulSetRolloutInProgress(pods []*workloadmeta.KubernetesPod) bool {
	if len(pods) <= 1 {
		return false
	}

	var firstRevision string
	for _, pod := range pods {
		revision := pod.Labels[controllerRevisionHashLabel]
		if revision == "" {
			// Pod doesn't have the label yet, might be initializing
			continue
		}
		if firstRevision == "" {
			firstRevision = revision
		} else if revision != firstRevision {
			// Pods have different revisions - rollout in progress
			return true
		}
	}
	return false
}

// rolloutDecision represents the decision on whether to trigger a rollout
type rolloutDecision int

const (
	// rolloutDecisionComplete indicates the rollout is complete (all pods have current recommendation)
	rolloutDecisionComplete rolloutDecision = iota
	// rolloutDecisionWait indicates we should wait (either already triggered or ongoing rollout without bypass)
	rolloutDecisionWait
	// rolloutDecisionTrigger indicates we should trigger a new rollout
	rolloutDecisionTrigger
)

// shouldTriggerRollout determines whether a rollout should be triggered based on current state.
// This function encapsulates the common decision logic used by all workload types:
// 1. If all pods have the current recommendation, rollout is complete
// 2. If we already triggered for this recommendation, wait for completion
// 3. If there's an ongoing rollout:
//   - Check if bypass is allowed (new recommendation increases limits)
//   - Check rate limiting for bypass
//
// 4. Otherwise, trigger the rollout
func shouldTriggerRollout(
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
	lastAction *datadoghqcommon.DatadogPodAutoscalerVerticalAction,
	rolloutInProgress bool,
	recommendation *model.VerticalScalingValues,
	opts recommendationOptions,
	currentTime time.Time,
	minDelayBetweenRollouts time.Duration,
	autoscalerID string,
) rolloutDecision {
	// Step 1: Check if rollout is complete for current recommendation
	if isRecommendationRolloutComplete(recommendationID, pods, podsPerRecommendationID) {
		return rolloutDecisionComplete
	}

	// Step 2: Check if we already triggered a rollout for THIS recommendation
	if lastAction != nil && lastAction.Type == datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType && lastAction.Version == recommendationID {
		log.Debugf("Rollout already triggered for recommendation %s on autoscaler %s, waiting for completion",
			recommendationID, autoscalerID)
		return rolloutDecisionWait
	}

	// Step 3: This is a NEW recommendation (different from what we last triggered)
	// Check if there's an ongoing rollout from a previous recommendation
	if rolloutInProgress {
		// Check if the new recommendation increases limits - if so, we may bypass the rollout check
		// to help recover from stuck rollouts caused by insufficient resources.
		if hasLimitIncrease(recommendation, pods, recommendationID, opts) {
			// Apply rate limiting to prevent rollout thrashing from rapid new recommendations
			if lastAction != nil && lastAction.Time.Add(minDelayBetweenRollouts).After(currentTime) {
				log.Debugf("Rollout in progress for autoscaler: %s with new recommendation increasing limits, "+
					"but last action was less than %s ago, waiting", autoscalerID, minDelayBetweenRollouts)
				return rolloutDecisionWait
			}
			log.Infof("Rollout in progress for autoscaler: %s, but new recommendation increases limits - bypassing check to help recovery",
				autoscalerID)
			// Fall through to trigger rollout
		} else {
			log.Debugf("Rollout already ongoing for autoscaler: %s, waiting for completion before applying new recommendation",
				autoscalerID)
			return rolloutDecisionWait
		}
	}

	// Step 4: No ongoing rollout (or bypassing due to limit increase) - trigger rollout
	return rolloutDecisionTrigger
}

// recommendationOptions controls how hasLimitIncrease interprets the effective recommendation
// when comparing against currently running pods. Fields correspond to autoscaler modes that
// alter the recommendation at apply time (i.e. before the patch reaches the API server).
type recommendationOptions struct {
	// burstable indicates that the recommendation will have its CPU limit removed at apply time.
	// When true, the CPU limit is treated as absent even if the raw recommendation contains one,
	// so that a non-burstable → burstable transition is correctly identified as a limit increase
	// (removing a CPU limit is equivalent to setting it to unlimited).
	// See fromAutoscalerToContainerResourcePatches for where the removal happens.
	burstable bool
}

// hasLimitIncrease checks if the new recommendation increases any limit compared to existing patched pods.
// It only compares against pods that have an OLD RecommendationIDAnnotation set (i.e., pods that were
// previously patched but don't have the current recommendation). This is used to bypass the
// rollout-in-progress check when a new recommendation would increase limits, which could help fix
// a stuck rollout caused by insufficient resources.
// Returns true if ANY container has a limit increase for CPU or Memory.
// A limit is considered "increased" if:
// - The new limit is higher than the current limit
// - The pod has a limit but the recommendation removes it (no limit = unlimited)
//
// opts must reflect the current autoscaler state so that apply-time transformations (e.g. burstable
// CPU limit removal) are accounted for before comparing against pod limits.
//
// Performance: Uses early exit - returns as soon as any pod with lower limits is found.
// Only processes pods with old recommendations (not already on current recommendation).
func hasLimitIncrease(
	recommendation *model.VerticalScalingValues,
	pods []*workloadmeta.KubernetesPod,
	currentRecommendationID string,
	opts recommendationOptions,
) bool {
	if recommendation == nil || len(recommendation.ContainerResources) == 0 {
		return false
	}

	// recommendationLimits holds pre-computed limits from a recommendation for efficient comparison
	type recommendationLimits struct {
		cpuLimit    float64 // Percentage (100 = 1 core), 0 if not set
		memoryLimit uint64  // Bytes, 0 if not set
		hasCPU      bool    // true if recommendation specifies a CPU limit
		hasMemory   bool    // true if recommendation specifies a memory limit
	}

	// Pre-compute recommendation limits once
	// We store ALL containers from the recommendation, even those without limits,
	// so we can detect when a limit is being removed (pod has limit, reco doesn't)
	recoLimits := make(map[string]recommendationLimits, len(recommendation.ContainerResources))
	for _, recoContainer := range recommendation.ContainerResources {
		limits := recommendationLimits{}
		// In burstable mode the CPU limit is removed at apply time, so treat hasCPU as false
		// regardless of what the raw recommendation contains. This correctly identifies a
		// non-burstable → burstable transition as a limit increase (unlimited > any finite limit).
		if !opts.burstable {
			if cpuLimit := recoContainer.Limits.Cpu(); cpuLimit != nil && !cpuLimit.IsZero() {
				limits.cpuLimit = cpuLimit.AsApproximateFloat64() * 100 // Convert to percentage
				limits.hasCPU = true
			}
		}
		if memLimit := recoContainer.Limits.Memory(); memLimit != nil && !memLimit.IsZero() {
			limits.memoryLimit = uint64(memLimit.Value())
			limits.hasMemory = true
		}
		recoLimits[recoContainer.Name] = limits
	}

	// Check each pod - early exit as soon as we find any limit increase
	for _, pod := range pods {
		podRecoID := pod.Annotations[model.RecommendationIDAnnotation]
		// Skip pods without recommendation annotation (never patched)
		// Skip pods already on current recommendation (already have new limits)
		if podRecoID == "" || podRecoID == currentRecommendationID {
			continue
		}

		// Check each container in this pod
		for _, container := range pod.Containers {
			reco, ok := recoLimits[container.Name]
			if !ok {
				continue
			}

			// Case 1: Recommendation has higher CPU limit than pod
			if reco.hasCPU && container.Resources.CPULimit != nil && reco.cpuLimit > *container.Resources.CPULimit {
				return true
			}

			// Case 2: Recommendation removes CPU limit (pod has limit, reco doesn't)
			// No limit = unlimited, which is greater than any finite limit
			if !reco.hasCPU && container.Resources.CPULimit != nil {
				return true
			}

			// Case 3: Recommendation has higher Memory limit than pod
			if reco.hasMemory && container.Resources.MemoryLimit != nil && reco.memoryLimit > *container.Resources.MemoryLimit {
				return true
			}

			// Case 4: Recommendation removes Memory limit (pod has limit, reco doesn't)
			if !reco.hasMemory && container.Resources.MemoryLimit != nil {
				return true
			}
		}
	}

	return false
}

// applyVerticalConstraints applies the container constraints from the PodAutoscaler spec to the recommendations
func applyVerticalConstraints(verticalRecs *model.VerticalScalingValues, constraints *datadoghqcommon.DatadogPodAutoscalerConstraints) (limitErr, err error) {
	if constraints == nil || len(constraints.Containers) == 0 || verticalRecs == nil {
		return nil, nil
	}

	// Build constraint lookup and validate uniqueness
	constraintsByName := make(map[string]*datadoghqcommon.DatadogPodAutoscalerContainerConstraints, len(constraints.Containers))
	var wildcardConstraint *datadoghqcommon.DatadogPodAutoscalerContainerConstraints
	for i := range constraints.Containers {
		c := &constraints.Containers[i]
		if c.Name == "*" {
			if wildcardConstraint != nil {
				return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec, "duplicate wildcard (*) constraint in containers list")
			}
			wildcardConstraint = c
		} else {
			if _, exists := constraintsByName[c.Name]; exists {
				return nil, autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec, "duplicate constraint for container %q", c.Name)
			}
			constraintsByName[c.Name] = c
		}
	}

	modified := false
	var clampedContainers []string
	kept := make([]datadoghqcommon.DatadogPodAutoscalerContainerResources, 0, len(verticalRecs.ContainerResources))

	for _, cr := range verticalRecs.ContainerResources {
		// Resolve constraint: specific name > wildcard > none
		constraint, found := constraintsByName[cr.Name]
		if !found {
			constraint = wildcardConstraint
		}
		if constraint == nil {
			kept = append(kept, cr)
			continue
		}

		// Enabled=false: drop this container's recommendations entirely
		if constraint.Enabled != nil && !*constraint.Enabled {
			modified = true
			continue
		}

		// Resolve which resources are controlled.
		// nil defaults to [cpu, memory]; empty list is equivalent to Enabled=false.
		controlled := constraint.ControlledResources
		if controlled == nil {
			controlled = []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
		}
		if len(controlled) == 0 {
			modified = true
			continue
		}

		// Remove resources not in the controlled list from requests and limits
		for name := range cr.Requests {
			if !slices.Contains(controlled, name) {
				delete(cr.Requests, name)
				modified = true
			}
		}
		for name := range cr.Limits {
			if !slices.Contains(controlled, name) {
				delete(cr.Limits, name)
				modified = true
			}
		}

		// ControlledValues=RequestsOnly: strip all limits
		if constraint.ControlledValues != nil && *constraint.ControlledValues == datadoghqcommon.DatadogPodAutoscalerContainerControlledValuesRequestsOnly {
			if len(cr.Limits) > 0 {
				cr.Limits = nil
				modified = true
			}
		}

		// Resolve min/max bounds for clamping.
		// New top-level MinAllowed/MaxAllowed apply to both requests and limits.
		// Deprecated Requests.MinAllowed/MaxAllowed apply to requests only.
		reqMin, reqMax, limMin, limMax := resolveMinMaxBounds(constraint)

		// Clamp existing requests and limits to their respective bounds.
		// Track which containers were clamped for the VerticalScalingLimited condition.
		requestsClamped := clampResourceList(cr.Requests, reqMin, reqMax)
		limitsClamped := clampResourceList(cr.Limits, limMin, limMax)
		if requestsClamped || limitsClamped {
			clampedContainers = append(clampedContainers, cr.Name)
			modified = true
		}

		// Maintain invariant: limits >= requests for all resources where both exist
		for resourceName, reqQty := range cr.Requests {
			if limQty, hasLimit := cr.Limits[resourceName]; hasLimit && limQty.Cmp(reqQty) < 0 {
				cr.Limits[resourceName] = reqQty.DeepCopy()
				modified = true
			}
		}

		kept = append(kept, cr)
	}

	verticalRecs.ContainerResources = kept

	if modified {
		newHash, hashErr := autoscaling.ObjectHash(verticalRecs.ContainerResources)
		if hashErr != nil {
			return nil, autoscaling.NewConditionError(autoscaling.ConditionReasonRecommendationError,
				fmt.Errorf("failed to recompute resources hash after applying constraints: %w", hashErr))
		}
		verticalRecs.ResourcesHash = newHash
	}

	if len(clampedContainers) > 0 {
		limitErr = autoscaling.NewConditionErrorf(autoscaling.ConditionReasonLimitedByConstraint,
			"recommendation clamped to min/max bounds for containers: %s", strings.Join(clampedContainers, ", "))
	}

	return limitErr, nil
}

// resolveMinMaxBounds returns the effective min/max bounds for requests and limits.
// New top-level MinAllowed/MaxAllowed apply to both; deprecated Requests field applies to requests only.
func resolveMinMaxBounds(c *datadoghqcommon.DatadogPodAutoscalerContainerConstraints) (reqMin, reqMax, limMin, limMax corev1.ResourceList) {
	if len(c.MinAllowed) > 0 {
		reqMin = c.MinAllowed
		limMin = c.MinAllowed
	} else if c.Requests != nil {
		reqMin = c.Requests.MinAllowed
	}

	if len(c.MaxAllowed) > 0 {
		reqMax = c.MaxAllowed
		limMax = c.MaxAllowed
	} else if c.Requests != nil {
		reqMax = c.Requests.MaxAllowed
	}

	return
}

// clampResourceList clamps each resource quantity in the list to [min, max].
// Returns true if any values were modified.
func clampResourceList(rl corev1.ResourceList, minAllowed, maxAllowed corev1.ResourceList) bool {
	if rl == nil {
		return false
	}
	modified := false
	for name, qty := range rl {
		clamped := false
		if minQty, ok := minAllowed[name]; ok && qty.Cmp(minQty) < 0 {
			qty = minQty.DeepCopy()
			clamped = true
		}
		if maxQty, ok := maxAllowed[name]; ok && qty.Cmp(maxQty) > 0 {
			qty = maxQty.DeepCopy()
			clamped = true
		}
		if clamped {
			rl[name] = qty
			modified = true
		}
	}
	return modified
}

// shouldEvictDeferred returns true if the time since the last action is greater than the resize pending period, false otherwise
func shouldEvictDeferred(podAutoscaler *datadoghq.DatadogPodAutoscaler, now time.Time) bool {
	period := defaultResizePendingPeriod
	// If the DPA has a configured resize pending period, use that instead of the default
	if podAutoscaler.Spec.ApplyPolicy != nil && podAutoscaler.Spec.ApplyPolicy.Update != nil && podAutoscaler.Spec.ApplyPolicy.Update.ResizePendingPeriod > 0 {
		period = podAutoscaler.Spec.ApplyPolicy.Update.ResizePendingPeriod
	}

	if podAutoscaler.Status.Vertical == nil || podAutoscaler.Status.Vertical.LastAction == nil {
		return false
	}

	return now.Sub(podAutoscaler.Status.Vertical.LastAction.Time.Time) > time.Duration(period)*time.Second
}

// shouldFallbackToRollout returns true if a rollout should be triggered instead
// of continuing to attempt in-place resizing
func shouldFallbackToRollout(toEvict []classifiedPod, podAutoscaler *datadoghq.DatadogPodAutoscaler, now time.Time, patchForbidden bool) bool {
	if patchForbidden {
		return true
	}

	delay := defaultRolloutFallbackDelay
	// If the DPA has a configured rollout fallback delay, use that instead of the default
	if podAutoscaler.Spec.ApplyPolicy != nil && podAutoscaler.Spec.ApplyPolicy.Update != nil && podAutoscaler.Spec.ApplyPolicy.Update.RolloutFallbackDelay > 0 {
		delay = podAutoscaler.Spec.ApplyPolicy.Update.RolloutFallbackDelay
	}

	threshold := time.Duration(delay) * time.Second
	for _, cp := range toEvict {
		if !cp.lastTransitionTime.IsZero() && now.Sub(cp.lastTransitionTime) > threshold {
			return true
		}
	}
	return false
}

// isRolloutRequired checks if a rollout is required for the podAutoscaler.
// A rollout is required when:
//
//	a) The global config flag (autoscaling.workload.in_place_vertical_scaling.enabled) is disabled, or
//	b) The DPA explicitly sets Strategy: TriggerRollout
func isRolloutRequired(autoscalerInternal *model.PodAutoscalerInternal) bool {
	if !pkgconfigsetup.Datadog().GetBool("autoscaling.workload.in_place_vertical_scaling.enabled") {
		return true
	}
	spec := autoscalerInternal.Spec()
	if spec == nil || spec.ApplyPolicy == nil || spec.ApplyPolicy.Update == nil {
		return false
	}
	return spec.ApplyPolicy.Update.Strategy == datadoghqcommon.DatadogPodAutoscalerTriggerRolloutUpdateStrategy
}

// getPodResizeStatus returns the resize status of pod and the LastTransitionTime
// of the condition that produced that status (zero if not condition-based).
func getPodResizeStatus(pod *workloadmeta.KubernetesPod, recommendationID string) (PodResizeStatus, time.Time) {
	if pod.Annotations[model.RecommendationIDAnnotation] != recommendationID {
		return PodResizeStatusNeedsPatch, time.Time{}
	}

	// A pod having the conditions PodResizePending and PodResizeInProgress are not mutually exclusive,
	// but usually a condition of PodResizeInProgress without an error reason means the pod is actively being resized.
	// Therefore we check for the PodResizeInProgress condition first, and if it's not present, we check for the PodResizePending condition.
	for _, condition := range pod.Conditions {
		if condition.Type == kubePodConditionResizeInProgress {
			if condition.Reason == kubePodConditionResizeInProgressReasonError {
				return PodResizeStatusError, condition.LastTransitionTime
			}
			return PodResizeStatusInProgress, condition.LastTransitionTime
		}
	}

	for _, condition := range pod.Conditions {
		if condition.Type == kubePodConditionResizePending {
			if condition.Reason == kubePodConditionResizePendingReasonInfeasible {
				return PodResizeStatusInfeasible, condition.LastTransitionTime
			}
			// If the reason is not Infeasible, it must be Deferred (ref: https://github.com/kubernetes/kubernetes/blob/42eb93b12fa6e9fd0e0da852cc01f13850ac5258/pkg/kubelet/status/status_manager.go#L289-L291)
			return PodResizeStatusDeferred, condition.LastTransitionTime
		}
	}

	return PodResizeStatusCompleted, time.Time{}
}

func fromAutoscalerToContainerResourcePatches(autoscalerInternal *model.PodAutoscalerInternal, pod *workloadmeta.KubernetesPod) []workloadpatcher.ContainerResourcePatch {
	containersResources := autoscalerInternal.ScalingValues().Vertical.ContainerResources

	// Build a map from container name to container resources.
	recoByName := make(map[string]datadoghqcommon.DatadogPodAutoscalerContainerResources, len(containersResources))
	for _, cr := range containersResources {
		recoByName[cr.Name] = cr
	}

	burstable := autoscalerInternal.IsBurstable()

	// Build the list of patches ordered to API server pod container order.
	patches := make([]workloadpatcher.ContainerResourcePatch, 0, len(containersResources))
	for _, c := range pod.Containers {
		cr, ok := recoByName[c.Name]
		if !ok {
			continue
		}
		patch := workloadpatcher.ContainerResourcePatch{
			Name:     cr.Name,
			Requests: resourceListToStringMap(cr.Requests),
			Limits:   resourceListToStringMap(cr.Limits),
		}
		if burstable {
			delete(patch.Limits, string(corev1.ResourceCPU)) // don't re-set CPU limit
			patch.LimitsToDelete = []string{string(corev1.ResourceCPU)}
		}
		patches = append(patches, patch)
	}
	return patches
}

// resourceListToStringMap converts a corev1.ResourceList to the string map expected by
// ContainerResourcePatch, including only resources that are actually set.
func resourceListToStringMap(rl corev1.ResourceList) map[string]string {
	if len(rl) == 0 {
		return nil
	}
	m := make(map[string]string, len(rl))
	for name, qty := range rl {
		m[string(name)] = qty.String()
	}
	return m
}
