// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// longestScalingRulePeriodAllowed is the maximum period allowed for a scaling rule
	// increasing duration increase the number of events to keep in memory and to process for recommendations.
	longestScalingRulePeriodAllowed = 30 * time.Minute

	// statusRetainedActions is the number of horizontal actions kept in status
	statusRetainedActions = 5
)

// PodAutoscalerInternal holds the necessary data to work with the `DatadogPodAutoscaler` CRD.
type PodAutoscalerInternal struct {
	// namespace is the namespace of the PodAutoscaler
	namespace string

	// name is the name of the PodAutoscaler
	name string

	// generation is the received generation of the PodAutoscaler
	generation int64

	// keeping track of .Spec (configuration of the Autoscaling)
	spec *datadoghq.DatadogPodAutoscalerSpec

	// settingsTimestamp is the time when the settings were last updated
	// Version is stored in .Spec.RemoteVersion
	// (only if owner == remote)
	settingsTimestamp time.Time

	// creationTimestamp is the time when the kubernetes object was created
	// creationTimestamp is stored in .DatadogPodAutoscaler.CreationTimestamp
	creationTimestamp time.Time

	// scalingValues represents the current target scaling values (retrieved from RC)
	scalingValues ScalingValues

	// horizontalLastActions is the last horizontal action successfully taken
	horizontalLastActions []datadoghq.DatadogPodAutoscalerHorizontalAction

	// horizontalLastLimitReason is stored separately as we don't want to keep no-action events in `horizontalLastActions`
	// i.e. when targetReplicaCount after limits == currentReplicas but we want to surface the last limiting reason anyway.
	horizontalLastLimitReason string

	// horizontalLastActionError is the last error encountered on horizontal scaling
	horizontalLastActionError error

	// verticalLastAction is the last action taken by the Vertical Pod Autoscaler
	verticalLastAction *datadoghq.DatadogPodAutoscalerVerticalAction

	// verticalLastActionError is the last error encountered on vertical scaling
	verticalLastActionError error

	// currentReplicas is the current number of PODs for the targetRef
	currentReplicas *int32

	// scaledReplicas is the current number of PODs for the targetRef matching the resources recommendations
	scaledReplicas *int32

	// error is the an error encountered by the controller not specific to a scaling action
	error error

	// deleted flags the PodAutoscaler as deleted (removal to be handled by the controller)
	// (only if owner == remote)
	deleted bool

	//
	// Computed fields
	//
	// targetGVK is the GroupVersionKind of the target resource
	// Parsed once from the .Spec.TargetRef
	targetGVK schema.GroupVersionKind

	// horizontalEventsRetention is the time to keep horizontal events in memory
	// based on scale policies
	horizontalEventsRetention time.Duration
}

// NewPodAutoscalerInternal creates a new PodAutoscalerInternal from a Kubernetes CR
func NewPodAutoscalerInternal(podAutoscaler *datadoghq.DatadogPodAutoscaler) PodAutoscalerInternal {
	pai := PodAutoscalerInternal{
		namespace: podAutoscaler.Namespace,
		name:      podAutoscaler.Name,
	}
	pai.UpdateFromPodAutoscaler(podAutoscaler)
	pai.UpdateFromStatus(&podAutoscaler.Status)

	return pai
}

// NewPodAutoscalerFromSettings creates a new PodAutoscalerInternal from settings received through remote configuration
func NewPodAutoscalerFromSettings(ns, name string, podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsVersion uint64, settingsTimestamp time.Time) PodAutoscalerInternal {
	pda := PodAutoscalerInternal{
		namespace: ns,
		name:      name,
	}
	pda.UpdateFromSettings(podAutoscalerSpec, settingsVersion, settingsTimestamp)

	return pda
}

//
// Modifiers
//

// UpdateFromPodAutoscaler updates the PodAutoscalerInternal from a PodAutoscaler object inside K8S
func (p *PodAutoscalerInternal) UpdateFromPodAutoscaler(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	p.generation = podAutoscaler.Generation
	p.spec = podAutoscaler.Spec.DeepCopy()
	// Reset the target GVK as it might have changed
	// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
	p.targetGVK = schema.GroupVersionKind{}
	// Compute the horizontal events retention again in case .Spec.Policy has changed
	p.horizontalEventsRetention = getHorizontalEventsRetention(podAutoscaler.Spec.Policy, longestScalingRulePeriodAllowed)
	p.creationTimestamp = podAutoscaler.CreationTimestamp.Time
}

// UpdateFromSettings updates the PodAutoscalerInternal from a new settings
func (p *PodAutoscalerInternal) UpdateFromSettings(podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsVersion uint64, settingsTimestamp time.Time) {
	p.settingsTimestamp = settingsTimestamp
	p.spec = podAutoscalerSpec // From settings, we don't need to deep copy as the object is not stored anywhere else
	p.spec.RemoteVersion = pointer.Ptr(settingsVersion)
	// Reset the target GVK as it might have changed
	// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
	p.targetGVK = schema.GroupVersionKind{}
	// Compute the horizontal events retention again in case .Spec.Policy has changed
	p.horizontalEventsRetention = getHorizontalEventsRetention(podAutoscalerSpec.Policy, longestScalingRulePeriodAllowed)
}

// UpdateFromValues updates the PodAutoscalerInternal from a new scaling values
func (p *PodAutoscalerInternal) UpdateFromValues(scalingValues ScalingValues) {
	p.scalingValues = scalingValues
}

// RemoveValues clears autoscaling values data from the PodAutoscalerInternal as we stopped autoscaling
func (p *PodAutoscalerInternal) RemoveValues() {
	p.scalingValues = ScalingValues{}
}

// UpdateFromHorizontalAction updates the PodAutoscalerInternal from a new horizontal action
func (p *PodAutoscalerInternal) UpdateFromHorizontalAction(action *datadoghq.DatadogPodAutoscalerHorizontalAction, err error) {
	if err != nil {
		p.horizontalLastActionError = err
		p.horizontalLastLimitReason = ""
	} else if action != nil {
		p.horizontalLastActionError = nil
	}

	if action != nil {
		replicasChanged := false
		if action.ToReplicas != action.FromReplicas {
			p.horizontalLastActions = addHorizontalAction(action.Time.Time, p.horizontalEventsRetention, p.horizontalLastActions, action)
			replicasChanged = true
		}

		if action.LimitedReason != nil {
			p.horizontalLastLimitReason = *action.LimitedReason
		} else if replicasChanged {
			p.horizontalLastLimitReason = ""
		}
	}
}

// UpdateFromVerticalAction updates the PodAutoscalerInternal from a new vertical action
func (p *PodAutoscalerInternal) UpdateFromVerticalAction(action *datadoghq.DatadogPodAutoscalerVerticalAction, err error) {
	if err != nil {
		p.verticalLastActionError = err
	} else if action != nil {
		p.verticalLastActionError = nil
	}

	if action != nil {
		p.verticalLastAction = action
	}
}

// SetGeneration sets the generation of the PodAutoscaler
func (p *PodAutoscalerInternal) SetGeneration(generation int64) {
	p.generation = generation
}

// SetScaledReplicas sets the current number of replicas for the targetRef matching the resources recommendations
func (p *PodAutoscalerInternal) SetScaledReplicas(replicas int32) {
	p.scaledReplicas = &replicas
}

// SetCurrentReplicas sets the current number of replicas for the targetRef
func (p *PodAutoscalerInternal) SetCurrentReplicas(replicas int32) {
	p.currentReplicas = &replicas
}

// SetError sets an error encountered by the controller not specific to a scaling action
func (p *PodAutoscalerInternal) SetError(err error) {
	p.error = err
}

// SetDeleted flags the PodAutoscaler as deleted
func (p *PodAutoscalerInternal) SetDeleted() {
	p.deleted = true
}

// UpdateFromStatus updates the PodAutoscalerInternal from an existing status.
// It assumes the PodAutoscalerInternal is empty so it's not emptying existing data.
func (p *PodAutoscalerInternal) UpdateFromStatus(status *datadoghq.DatadogPodAutoscalerStatus) {
	if status.Horizontal != nil {
		if status.Horizontal.Target != nil {
			p.scalingValues.Horizontal = &HorizontalScalingValues{
				Source:    status.Horizontal.Target.Source,
				Timestamp: status.Horizontal.Target.GeneratedAt.Time,
				Replicas:  status.Horizontal.Target.Replicas,
			}
		}

		if len(status.Horizontal.LastActions) > 0 {
			p.horizontalLastActions = status.Horizontal.LastActions
		}
	}

	if status.Vertical != nil {
		if status.Vertical.Target != nil {
			p.scalingValues.Vertical = &VerticalScalingValues{
				Source:             status.Vertical.Target.Source,
				Timestamp:          status.Vertical.Target.GeneratedAt.Time,
				ContainerResources: status.Vertical.Target.DesiredResources,
				ResourcesHash:      status.Vertical.Target.Version,
			}
		}

		p.verticalLastAction = status.Vertical.LastAction
	}

	if status.CurrentReplicas != nil {
		p.currentReplicas = status.CurrentReplicas
	}

	// Reading potential errors from conditions. Resetting internal errors first.
	// We're only keeping error string, loosing type, but it's not important for what we do.
	for _, cond := range status.Conditions {
		switch {
		case cond.Type == datadoghq.DatadogPodAutoscalerErrorCondition && cond.Status == corev1.ConditionTrue:
			// Error condition could refer to a controller error or from a general Datadog error
			// We're restoring this to error as it's the most generic
			p.error = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.scalingValues.HorizontalError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition && cond.Status == corev1.ConditionFalse:
			p.horizontalLastActionError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition && cond.Status == corev1.ConditionTrue:
			p.horizontalLastLimitReason = cond.Reason
		case cond.Type == datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.scalingValues.VerticalError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerVerticalAbleToApply && cond.Status == corev1.ConditionFalse:
			p.verticalLastActionError = errors.New(cond.Reason)
		}
	}
}

//
// Getters
//

// Namespace returns the namespace of the PodAutoscaler
func (p *PodAutoscalerInternal) Namespace() string {
	return p.namespace
}

// Name returns the name of the PodAutoscaler
func (p *PodAutoscalerInternal) Name() string {
	return p.name
}

// ID returns the functional identifier of the PodAutoscaler
func (p *PodAutoscalerInternal) ID() string {
	return p.namespace + "/" + p.name
}

// Generation returns the generation of the PodAutoscaler
func (p *PodAutoscalerInternal) Generation() int64 {
	return p.generation
}

// Spec returns the spec of the PodAutoscaler
func (p *PodAutoscalerInternal) Spec() *datadoghq.DatadogPodAutoscalerSpec {
	return p.spec
}

// SettingsTimestamp returns the timestamp of the last settings update
func (p *PodAutoscalerInternal) SettingsTimestamp() time.Time {
	return p.settingsTimestamp
}

// CreationTimestamp returns the timestamp the kubernetes object was created
func (p *PodAutoscalerInternal) CreationTimestamp() time.Time {
	return p.creationTimestamp
}

// ScalingValues returns the scaling values of the PodAutoscaler
func (p *PodAutoscalerInternal) ScalingValues() ScalingValues {
	return p.scalingValues
}

// HorizontalLastActions returns the last horizontal actions taken
func (p *PodAutoscalerInternal) HorizontalLastActions() []datadoghq.DatadogPodAutoscalerHorizontalAction {
	return p.horizontalLastActions
}

// HorizontalLastActionError returns the last error encountered on horizontal scaling
func (p *PodAutoscalerInternal) HorizontalLastActionError() error {
	return p.horizontalLastActionError
}

// VerticalLastAction returns the last action taken by the Vertical Pod Autoscaler
func (p *PodAutoscalerInternal) VerticalLastAction() *datadoghq.DatadogPodAutoscalerVerticalAction {
	return p.verticalLastAction
}

// VerticalLastActionError returns the last error encountered on vertical scaling
func (p *PodAutoscalerInternal) VerticalLastActionError() error {
	return p.verticalLastActionError
}

// CurrentReplicas returns the current number of PODs for the targetRef
func (p *PodAutoscalerInternal) CurrentReplicas() *int32 {
	return p.currentReplicas
}

// ScaledReplicas returns the current number of PODs for the targetRef matching the resources recommendations
func (p *PodAutoscalerInternal) ScaledReplicas() *int32 {
	return p.scaledReplicas
}

// Error returns the an error encountered by the controller not specific to a scaling action
func (p *PodAutoscalerInternal) Error() error {
	return p.error
}

// Deleted returns the deletion status of the PodAutoscaler
func (p *PodAutoscalerInternal) Deleted() bool {
	return p.deleted
}

// TargetGVK resolves the GroupVersionKind if empty and returns it
func (p *PodAutoscalerInternal) TargetGVK() (schema.GroupVersionKind, error) {
	if !p.targetGVK.Empty() {
		return p.targetGVK, nil
	}

	gv, err := schema.ParseGroupVersion(p.spec.TargetRef.APIVersion)
	if err != nil || gv.Group == "" || gv.Version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to parse API version '%s', err: %w", p.spec.TargetRef.APIVersion, err)
	}

	p.targetGVK = schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    p.spec.TargetRef.Kind,
	}
	return p.targetGVK, nil
}

//
// Helpers
//

// BuildStatus builds the status of the PodAutoscaler from the internal state
func (p *PodAutoscalerInternal) BuildStatus(currentTime metav1.Time, currentStatus *datadoghq.DatadogPodAutoscalerStatus) datadoghq.DatadogPodAutoscalerStatus {
	status := datadoghq.DatadogPodAutoscalerStatus{}

	// Syncing current replicas
	if p.currentReplicas != nil {
		status.CurrentReplicas = p.currentReplicas
	}

	// Produce Horizontal status only if we have a desired number of replicas
	if p.scalingValues.Horizontal != nil {
		status.Horizontal = &datadoghq.DatadogPodAutoscalerHorizontalStatus{
			Target: &datadoghq.DatadogPodAutoscalerHorizontalTargetStatus{
				Source:      p.scalingValues.Horizontal.Source,
				GeneratedAt: metav1.NewTime(p.scalingValues.Horizontal.Timestamp),
				Replicas:    p.scalingValues.Horizontal.Replicas,
			},
		}

		if lenActions := len(p.horizontalLastActions); lenActions > 0 {
			firstIndex := lenActions - statusRetainedActions
			if firstIndex < 0 {
				firstIndex = 0
			}

			status.Horizontal.LastActions = slices.Clone(p.horizontalLastActions[firstIndex:lenActions])
		}
	}

	// Produce Vertical status only if we have a desired container resources
	if p.scalingValues.Vertical != nil {
		cpuReqSum, memReqSum := p.scalingValues.Vertical.SumCPUMemoryRequests()

		status.Vertical = &datadoghq.DatadogPodAutoscalerVerticalStatus{
			Target: &datadoghq.DatadogPodAutoscalerVerticalTargetStatus{
				Source:           p.scalingValues.Vertical.Source,
				GeneratedAt:      metav1.NewTime(p.scalingValues.Vertical.Timestamp),
				Version:          p.scalingValues.Vertical.ResourcesHash,
				DesiredResources: p.scalingValues.Vertical.ContainerResources,
				Scaled:           p.scaledReplicas,
				PODCPURequest:    cpuReqSum,
				PODMemoryRequest: memReqSum,
			},
			LastAction: p.verticalLastAction,
		}
	}

	// Building conditions
	existingConditions := map[datadoghq.DatadogPodAutoscalerConditionType]*datadoghq.DatadogPodAutoscalerCondition{
		datadoghq.DatadogPodAutoscalerErrorCondition:                     nil,
		datadoghq.DatadogPodAutoscalerActiveCondition:                    nil,
		datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition: nil,
		datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition:     nil,
		datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition:  nil,
		datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition:   nil,
		datadoghq.DatadogPodAutoscalerVerticalAbleToApply:                nil,
	}

	if currentStatus != nil {
		for i := range currentStatus.Conditions {
			condition := &currentStatus.Conditions[i]
			if _, ok := existingConditions[condition.Type]; ok {
				existingConditions[condition.Type] = condition
			}
		}
	}

	// Building global error condition
	globalError := p.error
	if p.error == nil {
		globalError = p.scalingValues.Error
	}
	status.Conditions = append(status.Conditions, newConditionFromError(true, currentTime, globalError, datadoghq.DatadogPodAutoscalerErrorCondition, existingConditions))

	// Building active condition, should handle multiple reasons, currently only disabled if target replicas = 0
	if p.currentReplicas != nil && *p.currentReplicas == 0 {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionFalse, "Target has been scaled to 0 replicas", currentTime, datadoghq.DatadogPodAutoscalerActiveCondition, existingConditions))
	} else {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionTrue, "", currentTime, datadoghq.DatadogPodAutoscalerActiveCondition, existingConditions))
	}

	// Building errors related to compute recommendations
	var horizontalAbleToRecommend datadoghq.DatadogPodAutoscalerCondition
	if p.scalingValues.HorizontalError != nil || p.scalingValues.Horizontal != nil {
		horizontalAbleToRecommend = newConditionFromError(false, currentTime, p.scalingValues.HorizontalError, datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	} else {
		horizontalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", currentTime, datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, horizontalAbleToRecommend)

	var verticalAbleToRecommend datadoghq.DatadogPodAutoscalerCondition
	if p.scalingValues.VerticalError != nil || p.scalingValues.Vertical != nil {
		verticalAbleToRecommend = newConditionFromError(false, currentTime, p.scalingValues.VerticalError, datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	} else {
		verticalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", currentTime, datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, verticalAbleToRecommend)

	// Horizontal: handle scaling limited condition
	if p.horizontalLastLimitReason != "" {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionTrue, p.horizontalLastLimitReason, currentTime, datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition, existingConditions))
	} else {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionFalse, "", currentTime, datadoghq.DatadogPodAutoscalerHorizontalScalingLimitedCondition, existingConditions))
	}

	// Building rollout errors
	var horizontalReason string
	horizontalStatus := corev1.ConditionUnknown
	if p.horizontalLastActionError != nil {
		horizontalStatus = corev1.ConditionFalse
		horizontalReason = p.horizontalLastActionError.Error()
	} else if len(p.horizontalLastActions) > 0 {
		horizontalStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(horizontalStatus, horizontalReason, currentTime, datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition, existingConditions))

	var verticalReason string
	rolloutStatus := corev1.ConditionUnknown
	if p.verticalLastActionError != nil {
		rolloutStatus = corev1.ConditionFalse
		verticalReason = p.verticalLastActionError.Error()
	} else if p.verticalLastAction != nil {
		rolloutStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(rolloutStatus, verticalReason, currentTime, datadoghq.DatadogPodAutoscalerVerticalAbleToApply, existingConditions))

	return status
}

// Private helpers
func addHorizontalAction(currentTime time.Time, retention time.Duration, actions []datadoghq.DatadogPodAutoscalerHorizontalAction, action *datadoghq.DatadogPodAutoscalerHorizontalAction) []datadoghq.DatadogPodAutoscalerHorizontalAction {
	if retention == 0 {
		actions = actions[:0]
		actions = append(actions, *action)
		return actions
	}

	// Find oldest event index to keep
	cutoffTime := currentTime.Add(-retention)
	cutoffIndex := 0
	for i, action := range actions {
		// The first event after the cutoff time is the oldest event to keep
		if action.Time.Time.After(cutoffTime) {
			cutoffIndex = i
			break
		}
	}

	// We are basically removing space from the array until we reallocate
	actions = actions[cutoffIndex:]
	actions = append(actions, *action)
	return actions
}

func newConditionFromError(trueOnError bool, currentTime metav1.Time, err error, conditionType datadoghq.DatadogPodAutoscalerConditionType, existingConditions map[datadoghq.DatadogPodAutoscalerConditionType]*datadoghq.DatadogPodAutoscalerCondition) datadoghq.DatadogPodAutoscalerCondition {
	var condition corev1.ConditionStatus

	var reason string
	if err != nil {
		reason = err.Error()
		if trueOnError {
			condition = corev1.ConditionTrue
		} else {
			condition = corev1.ConditionFalse
		}
	} else {
		if trueOnError {
			condition = corev1.ConditionFalse
		} else {
			condition = corev1.ConditionTrue
		}
	}

	return newCondition(condition, reason, currentTime, conditionType, existingConditions)
}

func newCondition(status corev1.ConditionStatus, reason string, currentTime metav1.Time, conditionType datadoghq.DatadogPodAutoscalerConditionType, existingConditions map[datadoghq.DatadogPodAutoscalerConditionType]*datadoghq.DatadogPodAutoscalerCondition) datadoghq.DatadogPodAutoscalerCondition {
	condition := datadoghq.DatadogPodAutoscalerCondition{
		Type:   conditionType,
		Status: status,
		Reason: reason,
	}

	prevCondition := existingConditions[conditionType]
	if prevCondition == nil || (prevCondition.Status != condition.Status) {
		condition.LastTransitionTime = currentTime
	} else {
		condition.LastTransitionTime = prevCondition.LastTransitionTime
	}

	return condition
}

func getHorizontalEventsRetention(policy *datadoghq.DatadogPodAutoscalerPolicy, longestLookbackAllowed time.Duration) time.Duration {
	var longestRetention time.Duration
	if policy == nil {
		return 0
	}

	if policy.Upscale != nil {
		upscaleRetention := getLongestScalingRulesPeriod(policy.Upscale.Rules)
		if upscaleRetention > longestRetention {
			longestRetention = upscaleRetention
		}
	}

	if policy.Downscale != nil {
		downscaleRetention := getLongestScalingRulesPeriod(policy.Downscale.Rules)
		if downscaleRetention > longestRetention {
			longestRetention = downscaleRetention
		}
	}

	if longestRetention > longestLookbackAllowed {
		return longestLookbackAllowed
	}
	return longestRetention
}

func getLongestScalingRulesPeriod(rules []datadoghq.DatadogPodAutoscalerScalingRule) time.Duration {
	var longest time.Duration
	for _, rule := range rules {
		periodDuration := time.Second * time.Duration(rule.PeriodSeconds)
		if periodDuration > longest {
			longest = periodDuration
		}
	}

	return longest
}
