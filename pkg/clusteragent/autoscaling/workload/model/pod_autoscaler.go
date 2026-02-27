// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// longestScalingRulePeriodAllowed is the maximum period allowed for a scaling rule
	// increasing duration increase the number of events to keep in memory and to process for recommendations.
	longestScalingRulePeriodAllowed = 60 * time.Minute

	// longestStabilizationWindowAllowed is the maximum period allowed for a stabilization window
	// increasing duration increase the number of recommendations to keep in memory.
	longestStabilizationWindowAllowed = 60 * time.Minute

	// statusRetainedActions is the number of horizontal actions kept in status
	statusRetainedActions = 10

	// statusRetainedRecommendations is the maximum number of horizontal recommendations kept in status
	statusRetainedRecommendations = 60

	// CustomRecommenderAnnotationKey is the key used to store custom recommender configuration in annotations
	CustomRecommenderAnnotationKey = "autoscaling.datadoghq.com/custom-recommender"
)

// PodAutoscalerInternal holds the necessary data to work with the `DatadogPodAutoscaler` CRD.
type PodAutoscalerInternal struct {
	// namespace is the namespace of the PodAutoscaler
	namespace string

	// name is the name of the PodAutoscaler
	name string

	// upstreamCR keeping track of the upstream DPA CR.
	// For local-owner DPAs this is always the K8s object from the informer.
	// For remote-owner DPAs it is initially a minimal shell populated from RC settings,
	// and later replaced by the real K8s object once the CRD is reconciled.
	upstreamCR *datadoghq.DatadogPodAutoscaler

	// creationTimestamp is the time when the kubernetes object was created
	// creationTimestamp is stored in .DatadogPodAutoscaler.CreationTimestamp
	creationTimestamp time.Time

	// generation is the received generation of the PodAutoscaler
	generation int64

	// settingsTimestamp is the time when the settings were last updated
	// Version is stored in .Spec.RemoteVersion
	// (only if owner == remote)
	settingsTimestamp time.Time

	// scalingValues represents the active scaling values that should be used
	scalingValues ScalingValues

	// mainScalingValues represents the scaling values retrieved from the main recommender (product, optionally a custom endpoint)
	mainScalingValues ScalingValues

	// mainScalingValuesVersion is the remote config version of the last received main scaling values (0 if not set)
	mainScalingValuesVersion uint64

	// fallbackScalingValues represents the scaling values retrieved from the fallback
	fallbackScalingValues ScalingValues

	// horizontalLastActions is the last horizontal action successfully taken
	horizontalLastActions []datadoghqcommon.DatadogPodAutoscalerHorizontalAction

	// horizontalLastRecommendations is the history of recommendations selected by the horizontal controller
	horizontalLastRecommendations []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation

	// horizontalLastLimitReason is stored separately as we don't want to keep no-action events in `horizontalLastActions`
	// i.e. when targetReplicaCount after limits == currentReplicas but we want to surface the last limiting reason anyway.
	horizontalLastLimitReason string

	// horizontalLastActionError is the last error encountered on horizontal scaling
	horizontalLastActionError error

	// horizontalActionErrorCount is the number of horizontal actions that triggered an error
	horizontalActionErrorCount uint

	// horizontalActionSuccessCount is the number of successful horizontal actions
	horizontalActionSuccessCount uint

	// verticalLastAction is the last action taken by the Vertical Pod Autoscaler
	verticalLastAction *datadoghqcommon.DatadogPodAutoscalerVerticalAction

	// verticalLastActionError is the last error encountered on vertical scaling
	verticalLastActionError error

	// verticalActionErrorCount is the number of vertical actions that triggered an error
	verticalActionErrorCount uint

	// verticalActionSuccessCount is the number of successful vertical actions
	verticalActionSuccessCount uint

	// verticalLastLimitReason is the reason vertical scaling was limited by min/max constraints.
	// When non-nil, it carries a ConditionError so that both Reason and Message are preserved.
	verticalLastLimitReason error

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

	// horizontalEventsRetention is the time to keep horizontal events in memory based on scale policies
	horizontalEventsRetention time.Duration

	// horizontalRecommendationsRetention is the time to keep horizontal recommendations in memory
	horizontalRecommendationsRetention time.Duration

	// customRecommenderConfiguration holds the configuration for custom recommenders,
	// Parsed from annotations on the autoscaler
	customRecommenderConfiguration *RecommenderConfiguration
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
func NewPodAutoscalerFromSettings(ns, name string, podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsTimestamp time.Time) PodAutoscalerInternal {
	pda := PodAutoscalerInternal{
		namespace: ns,
		name:      name,
	}
	pda.UpdateFromSettings(podAutoscalerSpec, settingsTimestamp)

	return pda
}

//
// Modifiers
//

// UpdateFromPodAutoscaler updates the PodAutoscalerInternal from a PodAutoscaler object inside K8S
func (p *PodAutoscalerInternal) UpdateFromPodAutoscaler(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	p.upstreamCR = podAutoscaler
	p.creationTimestamp = podAutoscaler.CreationTimestamp.Time
	p.generation = podAutoscaler.Generation
	// Reset the target GVK as it might have changed
	// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
	p.targetGVK = schema.GroupVersionKind{}
	// Compute the horizontal events retention again in case .Spec.ApplyPolicy has changed
	p.horizontalEventsRetention, p.horizontalRecommendationsRetention = getHorizontalRetentionValues(podAutoscaler.Spec.ApplyPolicy)
	// Compute recommender configuration again in case .Annotations has changed
	p.updateCustomRecommenderConfiguration(podAutoscaler.Annotations)
}

// UpdateFromSettings updates the PodAutoscalerInternal from a new settings
func (p *PodAutoscalerInternal) UpdateFromSettings(podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsTimestamp time.Time) {
	currentSpec := p.Spec()
	if currentSpec == nil || currentSpec.RemoteVersion == nil || *currentSpec.RemoteVersion != *podAutoscalerSpec.RemoteVersion {
		// Reset the target GVK as it might have changed
		// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
		p.targetGVK = schema.GroupVersionKind{}
		// Compute the horizontal events retention again in case .Spec.ApplyPolicy has changed
		p.horizontalEventsRetention, p.horizontalRecommendationsRetention = getHorizontalRetentionValues(podAutoscalerSpec.ApplyPolicy)
	}
	// For remote-owner DPAs the K8s CRD may not exist yet; create a minimal shell so that
	// Spec() is always accessible without a separate spec field.
	if p.upstreamCR == nil {
		p.upstreamCR = &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: p.namespace,
				Name:      p.name,
			},
		}
	}
	p.upstreamCR.Spec = *podAutoscalerSpec
	p.settingsTimestamp = settingsTimestamp
}

// SetActiveScalingValues updates the PodAutoscalerInternal scaling values based on the desired source of recommendations
func (p *PodAutoscalerInternal) SetActiveScalingValues(currentTime time.Time, horizontalActiveSource, verticalActiveSource *datadoghqcommon.DatadogPodAutoscalerValueSource) {
	// Helper function to select scaling values based on the source
	selectScalingValues := func(source *datadoghqcommon.DatadogPodAutoscalerValueSource) ScalingValues {
		switch {
		case source == nil:
			return p.scalingValues
		case *source == datadoghqcommon.DatadogPodAutoscalerLocalValueSource:
			return p.fallbackScalingValues
		default:
			return p.mainScalingValues
		}
	}

	// Update scaling values
	p.scalingValues.Horizontal = selectScalingValues(horizontalActiveSource).Horizontal
	p.scalingValues.Vertical = selectScalingValues(verticalActiveSource).Vertical

	// Update error states based on main product recommendations
	p.scalingValues.HorizontalError = p.mainScalingValues.HorizontalError
	p.scalingValues.VerticalError = p.mainScalingValues.VerticalError
	p.scalingValues.Error = p.mainScalingValues.Error

	// Store the recommendation in history if newer
	if p.scalingValues.Horizontal != nil &&
		(len(p.horizontalLastRecommendations) == 0 ||
			p.horizontalLastRecommendations[len(p.horizontalLastRecommendations)-1].GeneratedAt.Before(pointer.Ptr(metav1.NewTime(p.scalingValues.Horizontal.Timestamp)))) {
		p.horizontalLastRecommendations = addRecommendationToHistory(currentTime, p.horizontalRecommendationsRetention, p.horizontalLastRecommendations, datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
			GeneratedAt: metav1.NewTime(p.scalingValues.Horizontal.Timestamp),
			Replicas:    p.scalingValues.Horizontal.Replicas,
		})
	}
}

// UpdateFromMainValues updates the PodAutoscalerInternal from new main scaling values
func (p *PodAutoscalerInternal) UpdateFromMainValues(mainScalingValues ScalingValues, version uint64) {
	p.mainScalingValues = mainScalingValues
	p.mainScalingValuesVersion = version
}

// UpdateFromLocalValues updates the PodAutoscalerInternal from new local scaling values
func (p *PodAutoscalerInternal) UpdateFromLocalValues(fallbackScalingValues ScalingValues) {
	p.fallbackScalingValues = fallbackScalingValues
}

// RemoveValues clears autoscaling values data from the PodAutoscalerInternal as we stopped autoscaling
func (p *PodAutoscalerInternal) RemoveValues() {
	p.scalingValues = ScalingValues{}
}

// RemoveMainValues clears main autoscaling values data from the PodAutoscalerInternal as we stopped autoscaling
func (p *PodAutoscalerInternal) RemoveMainValues() {
	p.mainScalingValues = ScalingValues{}
	p.mainScalingValuesVersion = 0
}

// RemoveLocalValues clears local autoscaling values data from the PodAutoscalerInternal as we stopped autoscaling
func (p *PodAutoscalerInternal) RemoveLocalValues() {
	p.fallbackScalingValues = ScalingValues{}
}

// UpdateFromHorizontalAction updates the PodAutoscalerInternal from a new horizontal action
func (p *PodAutoscalerInternal) UpdateFromHorizontalAction(action *datadoghqcommon.DatadogPodAutoscalerHorizontalAction, err error) {
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
func (p *PodAutoscalerInternal) UpdateFromVerticalAction(action *datadoghqcommon.DatadogPodAutoscalerVerticalAction, err error) {
	if err != nil {
		p.verticalLastActionError = err
	} else if action != nil {
		p.verticalLastActionError = nil
	}

	if action != nil {
		p.verticalLastAction = action
	}
}

// ClearHorizontalState clears horizontal scaling status data when horizontal scaling is disabled.
func (p *PodAutoscalerInternal) ClearHorizontalState() {
	p.horizontalLastActions = nil
	p.horizontalLastRecommendations = nil
	p.horizontalLastLimitReason = ""
	p.horizontalLastActionError = nil
}

// SetConstrainedVerticalScaling replaces the vertical scaling values and the limit error
func (p *PodAutoscalerInternal) SetConstrainedVerticalScaling(v *VerticalScalingValues, limitErr error) {
	if v == nil {
		p.verticalLastLimitReason = nil
		return
	}
	p.scalingValues.Vertical = v
	p.verticalLastLimitReason = limitErr
}

// ClearVerticalState clears vertical scaling status data when vertical scaling is disabled.
func (p *PodAutoscalerInternal) ClearVerticalState() {
	p.verticalLastAction = nil
	p.verticalLastActionError = nil
	p.verticalLastLimitReason = nil
	p.scaledReplicas = nil
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

// ClearCurrentReplicas clears the tracked replica count (e.g. when the target no longer exists).
func (p *PodAutoscalerInternal) ClearCurrentReplicas() {
	p.currentReplicas = nil
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
func (p *PodAutoscalerInternal) UpdateFromStatus(status *datadoghqcommon.DatadogPodAutoscalerStatus) {
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

			for _, recommendation := range status.Horizontal.LastRecommendations {
				if recommendation.Replicas != 0 {
					p.horizontalLastRecommendations = addRecommendationToHistory(recommendation.GeneratedAt.Time, p.horizontalRecommendationsRetention, p.horizontalLastRecommendations, datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
						GeneratedAt: recommendation.GeneratedAt,
						Replicas:    recommendation.Replicas,
					})
				}
			}
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

	// Reading potential errors from conditions.
	// We restore the error with its programmatic reason when available.
	for _, cond := range status.Conditions {
		switch {
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerErrorCondition && cond.Status == corev1.ConditionTrue:
			// Error condition could refer to a controller error or from a general Datadog error
			// We're restoring this to error as it's the most generic
			p.error = errorFromCondition(cond)
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.scalingValues.HorizontalError = errorFromCondition(cond)
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition && cond.Status == corev1.ConditionFalse:
			p.horizontalLastActionError = errorFromCondition(cond)
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition && cond.Status == corev1.ConditionTrue:
			p.horizontalLastLimitReason = cond.Message
			// Backward compatibility: if Message is empty, fall back to Reason
			if p.horizontalLastLimitReason == "" {
				p.horizontalLastLimitReason = cond.Reason
			}
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.scalingValues.VerticalError = errorFromCondition(cond)
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply && cond.Status == corev1.ConditionFalse:
			p.verticalLastActionError = errorFromCondition(cond)
		case cond.Type == datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition && cond.Status == corev1.ConditionTrue:
			p.verticalLastLimitReason = errorFromCondition(cond)
		}
	}
}

// UpdateCreationTimestamp updates the timestamp the kubernetes object was created
func (p *PodAutoscalerInternal) UpdateCreationTimestamp(creationTimestamp time.Time) {
	p.creationTimestamp = creationTimestamp
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

// UpstreamCR returns the upstream DatadogPodAutoscaler CR tracked by this internal object
func (p *PodAutoscalerInternal) UpstreamCR() *datadoghq.DatadogPodAutoscaler {
	return p.upstreamCR
}

// ID returns the functional identifier of the PodAutoscaler
func (p *PodAutoscalerInternal) ID() string {
	return p.namespace + "/" + p.name
}

// Generation returns the generation of the PodAutoscaler
func (p *PodAutoscalerInternal) Generation() int64 {
	return p.generation
}

// Spec returns the spec of the PodAutoscaler, sourced from the upstream CR.
// Returns nil if no upstream CR has been set yet.
func (p *PodAutoscalerInternal) Spec() *datadoghq.DatadogPodAutoscalerSpec {
	if p.upstreamCR == nil {
		return nil
	}
	return &p.upstreamCR.Spec
}

func (p *PodAutoscalerInternal) IsHorizontalScalingEnabled() bool {
	spec := p.Spec()
	if spec == nil || spec.ApplyPolicy == nil {
		return true
	}

	scaleUpDisabled := spec.ApplyPolicy.ScaleUp != nil &&
		spec.ApplyPolicy.ScaleUp.Strategy != nil &&
		*spec.ApplyPolicy.ScaleUp.Strategy == datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect

	scaleDownDisabled := spec.ApplyPolicy.ScaleDown != nil &&
		spec.ApplyPolicy.ScaleDown.Strategy != nil &&
		*spec.ApplyPolicy.ScaleDown.Strategy == datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect

	return !(scaleUpDisabled && scaleDownDisabled)
}

func (p *PodAutoscalerInternal) IsVerticalScalingEnabled() bool {
	spec := p.Spec()
	if spec == nil || spec.ApplyPolicy == nil {
		return true
	}

	if spec.ApplyPolicy.Update == nil {
		return true
	}

	return spec.ApplyPolicy.Update.Strategy != datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy
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

// MainScalingValues returns the main scaling values of the PodAutoscaler
func (p *PodAutoscalerInternal) MainScalingValues() ScalingValues {
	return p.mainScalingValues
}

// MainScalingValuesVersion returns the remote config version of the last received main scaling values (0 if not set)
func (p *PodAutoscalerInternal) MainScalingValuesVersion() uint64 {
	return p.mainScalingValuesVersion
}

// FallbackScalingValues returns the fallback scaling values of the PodAutoscaler
func (p *PodAutoscalerInternal) FallbackScalingValues() ScalingValues {
	return p.fallbackScalingValues
}

// HorizontalLastActions returns the last horizontal actions taken
func (p *PodAutoscalerInternal) HorizontalLastActions() []datadoghqcommon.DatadogPodAutoscalerHorizontalAction {
	return p.horizontalLastActions
}

// HorizontalLastRecommendations returns the last recommendations
func (p *PodAutoscalerInternal) HorizontalLastRecommendations() []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation {
	return p.horizontalLastRecommendations
}

// HorizontalLastActionError returns the last error encountered on horizontal scaling
func (p *PodAutoscalerInternal) HorizontalLastActionError() error {
	return p.horizontalLastActionError
}

// HorizontalActionErrorCount returns the number of horizontal actions that triggered an error
func (p *PodAutoscalerInternal) HorizontalActionErrorCount() uint {
	return p.horizontalActionErrorCount
}

// HorizontalActionErrorInc increment the number of horizontal actions that triggered an error
func (p *PodAutoscalerInternal) HorizontalActionErrorInc() {
	p.horizontalActionErrorCount++
}

// HorizontalActionSuccessCount returns the number of successful horizontal actions
func (p *PodAutoscalerInternal) HorizontalActionSuccessCount() uint {
	return p.horizontalActionSuccessCount
}

// HorizontalActionSuccessInc increment the number of horizontal actions that triggered a success
func (p *PodAutoscalerInternal) HorizontalActionSuccessInc() {
	p.horizontalActionSuccessCount++
}

// VerticalLastAction returns the last action taken by the Vertical Pod Autoscaler
func (p *PodAutoscalerInternal) VerticalLastAction() *datadoghqcommon.DatadogPodAutoscalerVerticalAction {
	return p.verticalLastAction
}

// VerticalLastActionError returns the last error encountered on vertical scaling
func (p *PodAutoscalerInternal) VerticalLastActionError() error {
	return p.verticalLastActionError
}

// VerticalActionErrorCount returns the number of vertical actions that triggered an error
func (p *PodAutoscalerInternal) VerticalActionErrorCount() uint {
	return p.verticalActionErrorCount
}

// VerticalActionErrorInc increment the number of horizontal actions that triggered an error
func (p *PodAutoscalerInternal) VerticalActionErrorInc() {
	p.verticalActionErrorCount++
}

// VerticalActionSuccessCount returns the number of successful vertical actions
func (p *PodAutoscalerInternal) VerticalActionSuccessCount() uint {
	return p.verticalActionSuccessCount
}

// VerticalActionSuccessInc increment the number of vertical actions that triggered as success
func (p *PodAutoscalerInternal) VerticalActionSuccessInc() {
	p.verticalActionSuccessCount++
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

	spec := p.Spec()
	if spec == nil {
		return schema.GroupVersionKind{}, autoscaling.NewConditionError(autoscaling.ConditionReasonInvalidTargetRef, errors.New("spec is not set"))
	}

	gv, err := schema.ParseGroupVersion(spec.TargetRef.APIVersion)
	if err != nil || gv.Group == "" || gv.Version == "" {
		return schema.GroupVersionKind{}, autoscaling.NewConditionError(autoscaling.ConditionReasonInvalidTargetRef, fmt.Errorf("failed to parse API version '%s', err: %w", spec.TargetRef.APIVersion, err))
	}

	p.targetGVK = schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    spec.TargetRef.Kind,
	}
	return p.targetGVK, nil
}

// CustomRecommenderConfiguration returns the configuration set on the autoscaler for a customer recommender
func (p *PodAutoscalerInternal) CustomRecommenderConfiguration() *RecommenderConfiguration {
	return p.customRecommenderConfiguration
}

//
// Helpers
//

// BuildStatus builds the status of the PodAutoscaler from the internal state
func (p *PodAutoscalerInternal) BuildStatus(currentTime metav1.Time, currentStatus *datadoghqcommon.DatadogPodAutoscalerStatus) datadoghqcommon.DatadogPodAutoscalerStatus {
	status := datadoghqcommon.DatadogPodAutoscalerStatus{}

	verticalEnabled := p.IsVerticalScalingEnabled()
	horizontalEnabled := p.IsHorizontalScalingEnabled()

	// Syncing current replicas
	if p.currentReplicas != nil {
		status.CurrentReplicas = p.currentReplicas
	}

	// Produce Horizontal status only if we have a desired number of replicas
	if horizontalEnabled && p.scalingValues.Horizontal != nil {
		status.Horizontal = &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
			Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				Source:      p.scalingValues.Horizontal.Source,
				GeneratedAt: metav1.NewTime(p.scalingValues.Horizontal.Timestamp),
				Replicas:    p.scalingValues.Horizontal.Replicas,
			},
		}

		if lenActions := len(p.horizontalLastActions); lenActions > 0 {
			firstIndex := max(lenActions-statusRetainedActions, 0)

			status.Horizontal.LastActions = slices.Clone(p.horizontalLastActions[firstIndex:lenActions])
		}

		if lenRecommendations := len(p.horizontalLastRecommendations); lenRecommendations > 0 {
			firstIndex := max(lenRecommendations-statusRetainedRecommendations, 0)

			status.Horizontal.LastRecommendations = slices.Clone(p.horizontalLastRecommendations[firstIndex:lenRecommendations])
		}
	}

	// Produce Vertical status only if we have a desired container resources
	if verticalEnabled && p.scalingValues.Vertical != nil {
		cpuReqSum, memReqSum := p.scalingValues.Vertical.SumCPUMemoryRequests()

		status.Vertical = &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
			Target: &datadoghqcommon.DatadogPodAutoscalerVerticalTargetStatus{
				Source:           p.scalingValues.Vertical.Source,
				GeneratedAt:      metav1.NewTime(p.scalingValues.Vertical.Timestamp),
				Version:          p.scalingValues.Vertical.ResourcesHash,
				DesiredResources: p.scalingValues.Vertical.ContainerResources,
				Scaled:           p.scaledReplicas,
				PodCPURequest:    cpuReqSum,
				PodMemoryRequest: memReqSum,
			},
			LastAction: p.verticalLastAction,
		}
	}

	// Building conditions
	existingConditions := map[datadoghqcommon.DatadogPodAutoscalerConditionType]*datadoghqcommon.DatadogPodAutoscalerCondition{
		datadoghqcommon.DatadogPodAutoscalerErrorCondition:                     nil,
		datadoghqcommon.DatadogPodAutoscalerActiveCondition:                    nil,
		datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition: nil,
		datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition:     nil,
		datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition:  nil,
		datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition:   nil,
		datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply:                nil,
		datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition:    nil,
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
	status.Conditions = append(status.Conditions, newConditionFromError(true, currentTime, globalError, datadoghqcommon.DatadogPodAutoscalerErrorCondition, existingConditions))

	// Building active condition, should handle multiple reasons, currently only disabled if target replicas = 0
	if p.currentReplicas != nil && *p.currentReplicas == 0 {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionFalse, "", "Target has been scaled to 0 replicas", currentTime, datadoghqcommon.DatadogPodAutoscalerActiveCondition, existingConditions))
	} else {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionTrue, "", "", currentTime, datadoghqcommon.DatadogPodAutoscalerActiveCondition, existingConditions))
	}

	// Building errors related to compute recommendations
	var horizontalAbleToRecommend datadoghqcommon.DatadogPodAutoscalerCondition
	if horizontalEnabled && (p.scalingValues.HorizontalError != nil || p.scalingValues.Horizontal != nil) {
		horizontalAbleToRecommend = newConditionFromError(false, currentTime, p.scalingValues.HorizontalError, datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	} else {
		horizontalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", "", currentTime, datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, horizontalAbleToRecommend)

	var verticalAbleToRecommend datadoghqcommon.DatadogPodAutoscalerCondition
	if verticalEnabled && (p.scalingValues.VerticalError != nil || p.scalingValues.Vertical != nil) {
		verticalAbleToRecommend = newConditionFromError(false, currentTime, p.scalingValues.VerticalError, datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	} else {
		verticalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", "", currentTime, datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, verticalAbleToRecommend)

	// Horizontal: handle scaling limited condition
	if horizontalEnabled && p.horizontalLastLimitReason != "" {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionTrue, "", p.horizontalLastLimitReason, currentTime, datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, existingConditions))
	} else {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionFalse, "", "", currentTime, datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, existingConditions))
	}

	// Vertical: handle scaling limited condition
	if verticalEnabled && p.verticalLastLimitReason != nil {
		status.Conditions = append(status.Conditions, newConditionFromError(true, currentTime, p.verticalLastLimitReason, datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, existingConditions))
	} else {
		status.Conditions = append(status.Conditions, newCondition(corev1.ConditionFalse, "", "", currentTime, datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, existingConditions))
	}

	// Building rollout errors
	var horizontalReason, horizontalMessage string
	horizontalStatus := corev1.ConditionUnknown
	if p.horizontalLastActionError != nil {
		horizontalStatus = corev1.ConditionFalse
		horizontalReason, horizontalMessage = reasonAndMessageFromError(p.horizontalLastActionError)
	} else if len(p.horizontalLastActions) > 0 {
		horizontalStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(horizontalStatus, horizontalReason, horizontalMessage, currentTime, datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, existingConditions))

	var verticalReason, verticalMessage string
	rolloutStatus := corev1.ConditionUnknown
	if p.verticalLastActionError != nil {
		rolloutStatus = corev1.ConditionFalse
		verticalReason, verticalMessage = reasonAndMessageFromError(p.verticalLastActionError)
	} else if p.verticalLastAction != nil {
		rolloutStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(rolloutStatus, verticalReason, verticalMessage, currentTime, datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, existingConditions))

	return status
}

// Private helpers
func (p *PodAutoscalerInternal) updateCustomRecommenderConfiguration(annotations map[string]string) {
	annotation, err := parseCustomConfigurationAnnotation(annotations)
	if err != nil {
		p.error = err
		return
	}
	p.customRecommenderConfiguration = annotation
}

func addHorizontalAction(currentTime time.Time, retention time.Duration, actions []datadoghqcommon.DatadogPodAutoscalerHorizontalAction, action *datadoghqcommon.DatadogPodAutoscalerHorizontalAction) []datadoghqcommon.DatadogPodAutoscalerHorizontalAction {
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

// TODO: We should be able to have a single function with generics
func addRecommendationToHistory(currentTime time.Time, retention time.Duration, history []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation, value datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation) []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation {
	if retention == 0 {
		history = history[:0]
		history = append(history, value)
		return history
	}

	// Find oldest event index to keep
	cutoffTime := currentTime.Add(-retention)
	cutoffIndex := 0
	for i, h := range history {
		// The first event after the cutoff time is the oldest event to keep
		if h.GeneratedAt.After(cutoffTime) {
			cutoffIndex = i
			break
		}
	}

	// We are basically removing space from the array until we reallocate
	history = history[cutoffIndex:]
	history = append(history, value)
	return history
}

// errorFromCondition restores an error from a Kubernetes condition.
// If the condition has a programmatic Reason, the error is wrapped with it.
// For backward compatibility, if Message is empty, Reason is used as the error message
// (matching the old behavior where err.Error() was stored in the Reason field).
func errorFromCondition(cond datadoghqcommon.DatadogPodAutoscalerCondition) error {
	message := cond.Message
	// Backward compatibility: if Message is empty, fall back to Reason as error message
	if message == "" {
		message = cond.Reason
		if message == "" {
			return errors.New("unknown error")
		}
		return errors.New(message)
	}

	if cond.Reason != "" {
		return autoscaling.NewConditionError(autoscaling.ConditionReasonType(cond.Reason), errors.New(message))
	}
	return errors.New(message)
}

// reasonAndMessageFromError extracts a programmatic reason and human-readable message from an error.
// If the error implements autoscaling.ConditionReason, the reason is extracted from it.
// The message is always the error's Error() string.
func reasonAndMessageFromError(err error) (reason, message string) {
	if err == nil {
		return "", ""
	}

	message = err.Error()
	var cr autoscaling.ConditionReason
	if errors.As(err, &cr) {
		reason = string(cr.Reason())
	}
	return reason, message
}

func newConditionFromError(trueOnError bool, currentTime metav1.Time, err error, conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, existingConditions map[datadoghqcommon.DatadogPodAutoscalerConditionType]*datadoghqcommon.DatadogPodAutoscalerCondition) datadoghqcommon.DatadogPodAutoscalerCondition {
	var status corev1.ConditionStatus

	var reason, message string
	if err != nil {
		reason, message = reasonAndMessageFromError(err)
		if trueOnError {
			status = corev1.ConditionTrue
		} else {
			status = corev1.ConditionFalse
		}
	} else {
		if trueOnError {
			status = corev1.ConditionFalse
		} else {
			status = corev1.ConditionTrue
		}
	}

	return newCondition(status, reason, message, currentTime, conditionType, existingConditions)
}

func newCondition(status corev1.ConditionStatus, reason, message string, currentTime metav1.Time, conditionType datadoghqcommon.DatadogPodAutoscalerConditionType, existingConditions map[datadoghqcommon.DatadogPodAutoscalerConditionType]*datadoghqcommon.DatadogPodAutoscalerCondition) datadoghqcommon.DatadogPodAutoscalerCondition {
	condition := datadoghqcommon.DatadogPodAutoscalerCondition{
		Type:    conditionType,
		Status:  status,
		Reason:  reason,
		Message: message,
	}

	prevCondition := existingConditions[conditionType]
	if prevCondition == nil || (prevCondition.Status != condition.Status) {
		condition.LastTransitionTime = currentTime
	} else {
		condition.LastTransitionTime = prevCondition.LastTransitionTime
	}

	return condition
}

func getHorizontalRetentionValues(policy *datadoghq.DatadogPodAutoscalerApplyPolicy) (eventsRetention time.Duration, recommendationRetention time.Duration) {
	if policy == nil {
		return 0, 0
	}

	if policy.ScaleUp != nil {
		scaleUpRetention := getLongestScalingRulesPeriod(policy.ScaleUp.Rules)
		eventsRetention = max(eventsRetention, scaleUpRetention)

		scaleUpStabilizationWindow := time.Second * time.Duration(policy.ScaleUp.StabilizationWindowSeconds)
		recommendationRetention = max(recommendationRetention, scaleUpStabilizationWindow)
	}

	if policy.ScaleDown != nil {
		scaleDownRetention := getLongestScalingRulesPeriod(policy.ScaleDown.Rules)
		eventsRetention = max(eventsRetention, scaleDownRetention)

		scaleDownStabilizationWindow := time.Second * time.Duration(policy.ScaleDown.StabilizationWindowSeconds)
		recommendationRetention = max(recommendationRetention, scaleDownStabilizationWindow)
	}

	eventsRetention = min(eventsRetention, longestScalingRulePeriodAllowed)
	recommendationRetention = min(recommendationRetention, longestStabilizationWindowAllowed)

	return eventsRetention, recommendationRetention
}

func getLongestScalingRulesPeriod(rules []datadoghqcommon.DatadogPodAutoscalerScalingRule) time.Duration {
	var longest time.Duration
	for _, rule := range rules {
		periodDuration := time.Second * time.Duration(rule.PeriodSeconds)
		if periodDuration > longest {
			longest = periodDuration
		}
	}

	return longest
}

func parseCustomConfigurationAnnotation(annotations map[string]string) (*RecommenderConfiguration, error) {
	annotation, ok := annotations[CustomRecommenderAnnotationKey]

	if !ok { // No annotation set
		return nil, nil
	}

	customConfiguration := RecommenderConfiguration{}

	if err := json.Unmarshal([]byte(annotation), &customConfiguration); err != nil {
		return nil, fmt.Errorf("Failed to parse annotations for custom recommender configuration: %v", err)
	}

	return &customConfiguration, nil
}
