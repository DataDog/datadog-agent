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
	"k8s.io/apimachinery/pkg/api/resource"

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

// removeLimitSentinel is a sentinel quantity stored in ContainerResources.Limits to
// signal that an existing limit must be actively deleted from the live pod. Negative
// quantities are never valid Kubernetes resource values, making the intent unambiguous.
var removeLimitSentinel = resource.MustParse("-1")

// isInPlaceResizeSupported checks whether the API server exposes the pods/resize
// subresource, which requires InPlacePodVerticalScaling to be enabled. The result
// is cached for 15 minutes.
func (u *verticalController) isInPlaceResizeSupported() bool {
	u.inPlaceResizeMu.Lock()
	defer u.inPlaceResizeMu.Unlock()

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
	// PodResizeStatusEvicting marks pods with an accepted eviction pending termination.
	PodResizeStatusEvicting
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

	if autoscalerInternal.IsPaused() {
		return datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy, "vertical scaling disabled: autoscaling locally paused by the " + model.PauseAnnotationKey + " annotation"
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
//  1. If all pods have the current recommendation, rollout is complete
//  2. If we already triggered for this recommendation, wait for completion
//  3. If there's an ongoing rollout:
//     - Check if bypass is allowed (new recommendation increases limits, or resources are forced
//     by annotation)
//     - Check rate limiting for bypass
//
// 4. Otherwise, trigger the rollout
func shouldTriggerRollout(
	recommendationID string,
	pods []*workloadmeta.KubernetesPod,
	podsPerRecommendationID map[string]int32,
	lastAction *datadoghqcommon.DatadogPodAutoscalerVerticalAction,
	rolloutInProgress bool,
	recommendation *model.VerticalScalingValues,
	currentTime time.Time,
	minDelayBetweenRollouts time.Duration,
	autoscalerID string,
	forcedResources bool,
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
		// Check if the new recommendation increases limits, or carries resources forced by annotation
		// (a break-glass change) - if so, we may bypass the rollout check to help recover from stuck
		// rollouts caused by insufficient resources.
		if forcedResources || hasLimitIncrease(recommendation, pods, recommendationID) {
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
) bool {
	if recommendation == nil || len(recommendation.ContainerResources) == 0 {
		return false
	}

	// recommendationLimits holds pre-computed limits from a recommendation for efficient comparison
	type recommendationLimits struct {
		cpuLimit    float64 // Percentage (100 = 1 core), 0 if not set
		memoryLimit uint64  // Bytes, 0 if not set
		hasCPU      bool    // true if recommendation specifies a non-zero CPU limit
		hasMemory   bool    // true if recommendation specifies a memory limit
	}

	// Pre-compute recommendation limits once.
	// In burstable mode applyVerticalConstraints sets the CPU limit to removeLimitSentinel (-1),
	// so cpuLimit.Sign() < 0 and hasCPU stays false.
	// Case 2 below then detects the transition (pod has CPU limit, reco removes it).
	recoLimits := make(map[string]recommendationLimits, len(recommendation.ContainerResources))
	for _, recoContainer := range recommendation.ContainerResources {
		limits := recommendationLimits{}
		if cpuLimit := recoContainer.Limits.Cpu(); cpuLimit != nil && cpuLimit.Sign() > 0 {
			limits.cpuLimit = cpuLimit.AsApproximateFloat64() * 100 // Convert to percentage
			limits.hasCPU = true
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

// applyVerticalConstraints applies the container constraints from the PodAutoscaler spec to the
// recommendations. When the CPU limit must be removed from the live pod — autoscaler-wide in
// burstable mode, or per-container when ControlledValues is CPURequestsRemoveLimitsMemoryRequestsAndLimits —
// it stores removeLimitSentinel (-1) on that container's CPU limit so that:
//   - the ResourcesHash changes when the removal is toggled (triggering pod re-patches)
//   - hasLimitIncrease sees cpuLimit.Sign() <= 0 and correctly identifies the transition as a limit increase
//   - patchContainerResources removes the CPU limit from the pod when it encounters the sentinel
func applyVerticalConstraints(verticalRecs *model.VerticalScalingValues, constraints *datadoghqcommon.DatadogPodAutoscalerConstraints, burstable bool) (limitErr, err error) {
	if verticalRecs == nil {
		return nil, nil
	}
	hasConstraints := constraints != nil && len(constraints.Containers) > 0
	if !hasConstraints && !burstable {
		return nil, nil
	}

	// Build constraint lookup and validate uniqueness (may be empty when constraints == nil).
	constraintsByName := make(map[string]*datadoghqcommon.DatadogPodAutoscalerContainerConstraints)
	var wildcardConstraint *datadoghqcommon.DatadogPodAutoscalerContainerConstraints
	if constraints != nil {
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

		// removeCPULimit is true when the CPU limit must be deleted from the live pod.
		// It applies autoscaler-wide in burstable mode, and per-container when the
		// constraint's ControlledValues requests CPU-limit removal (resolved below).
		// The actual stamping happens once, after constraint processing.
		removeCPULimit := burstable

		if constraint != nil {
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

			// ControlledValues=CPURequestsRemoveLimitsMemoryRequestsAndLimits: like burstable,
			// the CPU limit must be removed from the live pod (handled by the shared stamping below).
			if constraint.ControlledValues != nil &&
				*constraint.ControlledValues == datadoghqcommon.DatadogPodAutoscalerContainerControlledValuesCPURequestsRemoveLimitsMemoryRequestsAndLimits {
				removeCPULimit = true
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

			// Maintain invariant: limits >= requests for all resources where both exist.
			// Skip sentinel limits (negative) — they are internal markers, not values.
			for resourceName, reqQty := range cr.Requests {
				if limQty, hasLimit := cr.Limits[resourceName]; hasLimit && limQty.Sign() >= 0 && limQty.Cmp(reqQty) < 0 {
					cr.Limits[resourceName] = reqQty.DeepCopy()
					modified = true
				}
			}
		}

		// Stamp the CPU limit with removeLimitSentinel (-1) when it must be removed from the
		// live pod. Done after clamping/invariant so the sentinel value is never altered (both
		// skip negative quantities). This has three effects:
		//   1. The ResourcesHash changes when removal is toggled (pods get re-patched).
		//   2. hasLimitIncrease sees cpuLimit.Sign() <= 0, correctly identifying the transition
		//      to "no limit" as a limit increase (unlimited > any finite limit).
		//   3. patchContainerResources sees Sign() < 0 and removes the CPU limit from the running
		//      pod instead of setting it (an absent entry would mean "leave untouched").
		if removeCPULimit {
			if cr.Limits == nil {
				cr.Limits = corev1.ResourceList{}
			}
			cr.Limits[corev1.ResourceCPU] = removeLimitSentinel.DeepCopy()
			modified = true
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

// applyForcedResources overlays the resources forced by the force-resources annotation on vertical
// values, before the constraints are applied: forced values are bounded by them like recommended ones.
// Only the forced requests and limits change; a forced container without a recommendation is added
// with its forced values only. The annotation is not validated when parsed, so only the usable values
// are overlaid: named containers, cpu and memory, strictly positive quantities. When a forced value
// conflicts with a recommended one, the forced value wins and the recommended one is adjusted so that
// the request never exceeds the limit. It returns the VerticalScalingLimited reason when something is
// forced.
func applyForcedResources(verticalRecs *model.VerticalScalingValues, forced []datadoghqcommon.DatadogPodAutoscalerContainerResources) (limitErr, err error) {
	if verticalRecs == nil {
		return nil, nil
	}

	var overridden []string
	for _, entry := range forced {
		forcedContainer, usable := usableForcedResources(entry)
		if !usable {
			continue
		}
		index := slices.IndexFunc(verticalRecs.ContainerResources, func(cr datadoghqcommon.DatadogPodAutoscalerContainerResources) bool {
			return cr.Name == forcedContainer.Name
		})
		if index < 0 {
			verticalRecs.ContainerResources = append(verticalRecs.ContainerResources, datadoghqcommon.DatadogPodAutoscalerContainerResources{Name: forcedContainer.Name})
			index = len(verticalRecs.ContainerResources) - 1
		}
		overlayResourceList(&verticalRecs.ContainerResources[index], forcedContainer)
		overridden = append(overridden, forcedContainer.Name)
	}
	if len(overridden) == 0 {
		return nil, nil
	}

	newHash, hashErr := autoscaling.ObjectHash(verticalRecs.ContainerResources)
	if hashErr != nil {
		return nil, autoscaling.NewConditionError(autoscaling.ConditionReasonRecommendationError,
			fmt.Errorf("failed to recompute resources hash after applying forced resources: %w", hashErr))
	}
	verticalRecs.ResourcesHash = newHash

	return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonForcedByAnnotation,
		"resources overridden for containers %s by the %s annotation", strings.Join(overridden, ", "), model.ForceResourcesAnnotationKey), nil
}

// usableForcedResources keeps the forced values of one entry that can be overlaid: cpu and memory with
// a strictly positive quantity (a negative limit is the remove-limit sentinel). It reports false for an
// entry without a name or without any usable value.
func usableForcedResources(entry datadoghqcommon.DatadogPodAutoscalerContainerResources) (datadoghqcommon.DatadogPodAutoscalerContainerResources, bool) {
	usable := datadoghqcommon.DatadogPodAutoscalerContainerResources{Name: entry.Name}
	keep := func(list corev1.ResourceList) corev1.ResourceList {
		var kept corev1.ResourceList
		for name, qty := range list {
			if (name == corev1.ResourceCPU || name == corev1.ResourceMemory) && qty.Sign() > 0 {
				if kept == nil {
					kept = corev1.ResourceList{}
				}
				kept[name] = qty
			}
		}
		return kept
	}
	usable.Requests = keep(entry.Requests)
	usable.Limits = keep(entry.Limits)
	return usable, usable.Name != "" && (len(usable.Requests) > 0 || len(usable.Limits) > 0)
}

// overlayResourceList sets the forced requests and limits of one container on it, keeping the request
// below the limit: the forced side wins, the recommended side is adjusted.
func overlayResourceList(cr *datadoghqcommon.DatadogPodAutoscalerContainerResources, forced datadoghqcommon.DatadogPodAutoscalerContainerResources) {
	for name, qty := range forced.Requests {
		if cr.Requests == nil {
			cr.Requests = corev1.ResourceList{}
		}
		cr.Requests[name] = qty.DeepCopy()
	}
	for name, qty := range forced.Limits {
		if cr.Limits == nil {
			cr.Limits = corev1.ResourceList{}
		}
		cr.Limits[name] = qty.DeepCopy()
	}

	for name, req := range cr.Requests {
		lim, hasLimit := cr.Limits[name]
		// A negative limit is the remove-limit sentinel, meaning no limit at all.
		if !hasLimit || lim.Sign() < 0 || req.Cmp(lim) <= 0 {
			continue
		}
		if _, limitForced := forced.Limits[name]; limitForced {
			cr.Requests[name] = lim.DeepCopy()
		} else if _, requestForced := forced.Requests[name]; requestForced {
			cr.Limits[name] = req.DeepCopy()
		}
	}
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
// removeLimitSentinel entries are skipped: they are internal markers for downstream
// consumers (signalling "delete this limit from the pod") and must survive clamping.
func clampResourceList(rl corev1.ResourceList, minAllowed, maxAllowed corev1.ResourceList) bool {
	if rl == nil {
		return false
	}
	modified := false
	for name, qty := range rl {
		if qty.Cmp(removeLimitSentinel) == 0 { // preserve the remove-limit sentinel as-is
			continue
		}
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
// of continuing to attempt in-place resizing.
func shouldFallbackToRollout(toEvict []classifiedPod, hasInfeasible bool, podAutoscaler *datadoghq.DatadogPodAutoscaler, now time.Time, patchForbidden bool) bool {
	if patchForbidden || hasInfeasible {
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
//	b) The DPA explicitly sets Strategy: TriggerRollout, or
//	c) Any runtime value is recommended for a container but not yet applied to all running pods
//
// pods is the current live pod list for the workload, used to check whether runtime values are already
// applied so we avoid triggering unnecessary rollouts (e.g. when only CPU changed).
func isRolloutRequired(autoscalerInternal *model.PodAutoscalerInternal, pods []*workloadmeta.KubernetesPod) bool {
	if !pkgconfigsetup.Datadog().GetBool("autoscaling.workload.in_place_vertical_scaling.enabled") {
		return true
	}
	// Runtime values are env vars that can only be applied to new pods via the
	// admission webhook — they cannot be updated on a running container via pods/resize.
	// Force the rollout path only when the recommended value differs from what is already on the pods.
	if sv := autoscalerInternal.ScalingValues(); sv.Vertical != nil {
		if hash, hasRuntime := computeRuntimeRecommendationID(sv.Vertical.ContainerResources); hasRuntime {
			if !runtimeRecommendationIDApplied(hash, pods) {
				return true
			}
		}
	}
	spec := autoscalerInternal.Spec()
	if spec == nil || spec.ApplyPolicy == nil || spec.ApplyPolicy.Update == nil {
		return false
	}
	return spec.ApplyPolicy.Update.Strategy == datadoghqcommon.DatadogPodAutoscalerTriggerRolloutUpdateStrategy
}

// computeRuntimeRecommendationID collects all non-nil Runtime values across container resources
// and returns a deterministic hash of the combined map, suitable for use as the runtime-rec-id
// annotation. Returns ("", false) when no container has runtime values.
func computeRuntimeRecommendationID(containerResources []datadoghqcommon.DatadogPodAutoscalerContainerResources) (string, bool) {
	runtimeValues := make(map[string]datadoghqcommon.DatadogPodAutoscalerContainerRuntimeValues)
	for _, cr := range containerResources {
		if cr.Runtime != nil {
			runtimeValues[cr.Name] = *cr.Runtime
		}
	}
	if len(runtimeValues) == 0 {
		return "", false
	}
	hash, err := autoscaling.ObjectHash(runtimeValues)
	if err != nil {
		log.Debugf("Failed to compute runtime recommendation ID hash: %v", err)
		return "", false
	}
	return hash, true
}

// runtimeRecommendationIDApplied returns true if all non-terminating pods already carry the expected
// runtime-rec-id annotation. Returns false if any pod is missing the annotation or has a different hash.
func runtimeRecommendationIDApplied(expectedHash string, pods []*workloadmeta.KubernetesPod) bool {
	if len(pods) == 0 {
		return false
	}
	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if pod.Annotations[model.RuntimeRecommendationIDAnnotation] != expectedHash {
			return false
		}
	}
	return true
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

// isDisruptiveResize reports whether the recommendation changes a resource whose RestartContainer
// policy would restart a container. Other resizes happen in place and are never throttled.
func isDisruptiveResize(pod *workloadmeta.KubernetesPod, recommendation *model.VerticalScalingValues) bool {
	if recommendation == nil {
		return false
	}
	recoByName := resourcesByName(recommendation)
	for _, c := range pod.Containers {
		cr, ok := recoByName[c.Name]
		if !ok {
			continue
		}
		if restartsOnChange(c, func(name corev1.ResourceName) bool { return resourceChanging(c.Resources, cr, name) }) {
			return true
		}
	}
	return false
}

// restartsOnChange reports whether changing a resource of container c restarts it: its resize policy
// for that resource is RestartContainer, and changes reports a change of it.
func restartsOnChange(c workloadmeta.OrchestratorContainer, changes func(corev1.ResourceName) bool) bool {
	return (c.ResizePolicy.CPURestartPolicy == string(corev1.RestartContainer) && changes(corev1.ResourceCPU)) ||
		(c.ResizePolicy.MemoryRestartPolicy == string(corev1.RestartContainer) && changes(corev1.ResourceMemory))
}

// resourcesByName returns the container resources of a vertical target by container name; empty for a
// nil target.
func resourcesByName(target *model.VerticalScalingValues) map[string]datadoghqcommon.DatadogPodAutoscalerContainerResources {
	if target == nil {
		return nil
	}
	byName := make(map[string]datadoghqcommon.DatadogPodAutoscalerContainerResources, len(target.ContainerResources))
	for _, cr := range target.ContainerResources {
		byName[cr.Name] = cr
	}
	return byName
}

// resourceChanging reports whether reco changes the container's request or limit of a resource.
func resourceChanging(current workloadmeta.ContainerResources, reco datadoghqcommon.DatadogPodAutoscalerContainerResources, name corev1.ResourceName) bool {
	if q, ok := reco.Requests[name]; ok && !currentResourceEquals(current, false, name, q) {
		return true
	}
	q, ok := reco.Limits[name]
	return ok && !currentResourceEquals(current, true, name, q)
}

// countDisruptedPods counts pods that are unavailable or mid-resize. In-flight resizes count even
// while Ready, since a RestartContainer resize stays Ready while Deferred/InProgress.
func countDisruptedPods(podsByResizeStatus map[PodResizeStatus][]classifiedPod) int {
	disrupted := 0
	for status, cps := range podsByResizeStatus {
		for _, cp := range cps {
			if (status == PodResizeStatusNeedsPatch || status == PodResizeStatusCompleted) && cp.pod.Ready {
				continue
			}
			disrupted++
		}
	}
	return disrupted
}

// allowedDisruptions returns how many more pods may be disrupted this sync to stay within budget.
func allowedDisruptions(configured int, alreadyDisrupted int) int {
	pct := pkgconfigsetup.Datadog().GetInt("autoscaling.workload.in_place_vertical_scaling.disruption_tolerance_percent")
	tolerance := int(float64(configured) * float64(pct) / 100.0)
	// Allow one disruption when a healthy workload's tolerance truncates to 0, so low-replica
	// workloads can still make progress.
	if tolerance == 0 && alreadyDisrupted == 0 && configured > 0 {
		return 1
	}
	return max(0, tolerance-alreadyDisrupted)
}

// fromAutoscalerToContainerResourcePatches builds the in-place resize patch of pod: the resources of
// the vertical target, plus the resets returned by recordedResets, which may concern containers that
// are not in the target.
func fromAutoscalerToContainerResourcePatches(autoscalerInternal *model.PodAutoscalerInternal, pod *workloadmeta.KubernetesPod, resets map[string]resourceReset) []workloadpatcher.ContainerResourcePatch {
	target := autoscalerInternal.ScalingValues().Vertical
	recoByName := resourcesByName(target)

	burstable := autoscalerInternal.IsBurstable()

	// Build the list of patches ordered to API server pod container order.
	patches := make([]workloadpatcher.ContainerResourcePatch, 0, len(recoByName))
	for _, c := range pod.Containers {
		cr, inTarget := recoByName[c.Name]
		reset, hasReset := resets[c.Name]
		if !inTarget && !hasReset {
			continue
		}
		patch := workloadpatcher.ContainerResourcePatch{
			Name:     c.Name,
			Requests: resourceListToStringMap(cr.Requests),
			Limits:   resourceListToStringMap(cr.Limits),
		}
		if burstable && inTarget {
			delete(patch.Limits, string(corev1.ResourceCPU)) // don't re-set CPU limit
			patch.LimitsToDelete = []string{string(corev1.ResourceCPU)}
		}
		patch.Requests = addResourcesToStringMap(patch.Requests, reset.requests)
		patch.Limits = addResourcesToStringMap(patch.Limits, reset.limits)
		for _, name := range reset.deleteLimits {
			if !slices.Contains(patch.LimitsToDelete, string(name)) {
				patch.LimitsToDelete = append(patch.LimitsToDelete, string(name))
			}
		}
		patches = append(patches, patch)
	}
	return patches
}

// resourceReset holds the requests and limits of a container to set back to their original value,
// and the limits to delete because they were absent originally.
type resourceReset struct {
	requests     corev1.ResourceList
	limits       corev1.ResourceList
	deleteLimits []corev1.ResourceName
}

// recordedResets returns, by container name, the cpu and memory requests and limits of pod to set back
// to the original value recorded in model.OriginalResourcesAnnotation: the ones that the vertical
// target does not control and that differ from it on the pod. They were changed by an earlier target,
// e.g. a value forced by the force-resources annotation and since removed, or a resource that the
// constraints no longer control. A recreated pod gets the original value for them, so a pod resized in
// place must get it too. A limit that was absent originally is deleted; a request that was absent is
// left as is, as a request cannot be removed in place. A limit is not set back below the request the
// pod keeps: the resize would be rejected. Only regular containers are reset, as the resize patch
// only covers spec.containers: the originals the webhook records for sidecar containers are not used
// here.
func recordedResets(pod *workloadmeta.KubernetesPod, target *model.VerticalScalingValues, original originalResources) map[string]resourceReset {
	if len(original) == 0 {
		return nil
	}
	targetByName := resourcesByName(target)

	var resets map[string]resourceReset
	for _, c := range pod.Containers {
		recorded := original[c.Name]
		if recorded == nil {
			continue
		}
		cr := targetByName[c.Name]
		var reset resourceReset
		for name, value := range recorded.Requests {
			if _, controlled := cr.Requests[name]; controlled || value == nil {
				continue
			}
			if !currentResourceEquals(c.Resources, false, name, *value) {
				reset.requests = setResource(reset.requests, name, *value)
			}
		}
		for name, value := range recorded.Limits {
			// A limit the target removes (removeLimitSentinel, e.g. the burstable cpu limit) is controlled.
			if _, controlled := cr.Limits[name]; controlled {
				continue
			}
			switch {
			case value == nil:
				if currentResource(c.Resources, true, name) != nil {
					reset.deleteLimits = append(reset.deleteLimits, name)
				}
			case currentResourceEquals(c.Resources, true, name, *value):
			case belowRequest(*value, keptRequest(c.Resources, cr, reset, name)):
				log.Debugf("Not setting the %s limit of container %s of pod %s/%s back to %s: below its request", name, c.Name, pod.Namespace, pod.Name, value.String())
			default:
				reset.limits = setResource(reset.limits, name, *value)
			}
		}
		if len(reset.requests) > 0 || len(reset.limits) > 0 || len(reset.deleteLimits) > 0 {
			if resets == nil {
				resets = map[string]resourceReset{}
			}
			slices.Sort(reset.deleteLimits)
			resets[c.Name] = reset
		}
	}
	return resets
}

// keptRequest returns the request of a resource that a container keeps after its resize: the target
// value, else the reset value, else the current one. nil when there is none.
func keptRequest(current workloadmeta.ContainerResources, target datadoghqcommon.DatadogPodAutoscalerContainerResources, reset resourceReset, name corev1.ResourceName) *resource.Quantity {
	if q, found := target.Requests[name]; found {
		return &q
	}
	if q, found := reset.requests[name]; found {
		return &q
	}
	return currentResource(current, false, name)
}

func belowRequest(limit resource.Quantity, request *resource.Quantity) bool {
	return request != nil && limit.Cmp(*request) < 0
}

// isDisruptiveReset reports whether resetting resources to their original value restarts a
// container. resets only holds values that change, as returned by recordedResets.
func isDisruptiveReset(pod *workloadmeta.KubernetesPod, resets map[string]resourceReset) bool {
	for _, c := range pod.Containers {
		reset, found := resets[c.Name]
		if !found {
			continue
		}
		if restartsOnChange(c, func(name corev1.ResourceName) bool {
			_, request := reset.requests[name]
			_, limit := reset.limits[name]
			return request || limit || slices.Contains(reset.deleteLimits, name)
		}) {
			return true
		}
	}
	return false
}

// recordInPlaceChanges records in original the current value of the cpu and memory requests and
// limits that patches change on pod, before the resize changes them. It reports whether the record
// changed.
func recordInPlaceChanges(original originalResources, pod *workloadmeta.KubernetesPod, patches []workloadpatcher.ContainerResourcePatch) bool {
	containers := make(map[string]workloadmeta.ContainerResources, len(pod.Containers))
	for _, c := range pod.Containers {
		containers[c.Name] = c.Resources
	}

	changed := false
	// currentResourceEquals is true and currentResource nil for other resources than cpu and memory:
	// they are never recorded.
	recordIfChanging := func(container string, current workloadmeta.ContainerResources, limit bool, name corev1.ResourceName, value string) {
		q, err := resource.ParseQuantity(value)
		if err != nil || currentResourceEquals(current, limit, name, q) {
			return
		}
		changed = original.record(container, limit, name, currentResource(current, limit, name)) || changed
	}
	for _, patch := range patches {
		current, found := containers[patch.Name]
		if !found {
			continue
		}
		for name, value := range patch.Requests {
			recordIfChanging(patch.Name, current, false, corev1.ResourceName(name), value)
		}
		for name, value := range patch.Limits {
			recordIfChanging(patch.Name, current, true, corev1.ResourceName(name), value)
		}
		for _, name := range patch.LimitsToDelete {
			if value := currentResource(current, true, corev1.ResourceName(name)); value != nil {
				changed = original.record(patch.Name, true, corev1.ResourceName(name), value) || changed
			}
		}
	}
	return changed
}

// currentResource returns the current cpu or memory request (limit=false) or limit of a container, or
// nil when it is not set.
func currentResource(current workloadmeta.ContainerResources, limit bool, name corev1.ResourceName) *resource.Quantity {
	switch name {
	case corev1.ResourceCPU:
		value := current.CPURequest
		if limit {
			value = current.CPULimit
		}
		if value == nil {
			return nil
		}
		return resource.NewMilliQuantity(cpuPercentToMillis(*value), resource.DecimalSI)
	case corev1.ResourceMemory:
		value := current.MemoryRequest
		if limit {
			value = current.MemoryLimit
		}
		if value == nil {
			return nil
		}
		return resource.NewQuantity(int64(*value), resource.BinarySI)
	}
	return nil
}

// currentResourceEquals reports whether the current cpu or memory request (limit=false) or limit of a
// container equals q: CPU in millicores, memory in bytes, the precision of the workloadmeta values.
// Other resources are not tracked and always compare equal.
func currentResourceEquals(current workloadmeta.ContainerResources, limit bool, name corev1.ResourceName, q resource.Quantity) bool {
	value := currentResource(current, limit, name)
	if value == nil {
		return !slices.Contains(recordedResourceNames, name)
	}
	if name == corev1.ResourceCPU {
		return value.MilliValue() == q.MilliValue()
	}
	return value.Value() == q.Value()
}

// cpuPercentToMillis converts a workloadmeta CPURequest/CPULimit, stored as percentage of 1 CPU
// (0–100*numCPU), to millicores, rounded to avoid float equality across code paths.
func cpuPercentToMillis(percent float64) int64 {
	return int64(percent*10 + 0.5)
}

func setResource(rl corev1.ResourceList, name corev1.ResourceName, q resource.Quantity) corev1.ResourceList {
	if rl == nil {
		rl = corev1.ResourceList{}
	}
	rl[name] = q
	return rl
}

// addResourcesToStringMap adds rl to m, in the string format of resourceListToStringMap.
func addResourcesToStringMap(m map[string]string, rl corev1.ResourceList) map[string]string {
	if len(rl) == 0 {
		return m
	}
	if m == nil {
		m = make(map[string]string, len(rl))
	}
	for name, qty := range rl {
		m[string(name)] = qty.String()
	}
	return m
}

// resourceListToStringMap converts a corev1.ResourceList to the string map expected by
// ContainerResourcePatch, including only resources that are actually set.
func resourceListToStringMap(rl corev1.ResourceList) map[string]string {
	return addResourcesToStringMap(nil, rl)
}
