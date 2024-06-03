// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// UnsetGeneration is the value used to represent a .Spec value for which we have no generation (not created or not updated in-cluster yet)
const UnsetGeneration = -1

// PodAutoscalerInternal hols the necessary data to work with the `DatadogPodAutoscaler` CRD.
type PodAutoscalerInternal struct {
	// Namespace is the namespace of the PodAutoscaler
	Namespace string

	// Name is the name of the PodAutoscaler
	Name string

	// Generation is the received generation of the PodAutoscaler
	Generation int64

	// Keeping track of .Spec (configuration of the Autoscaling)
	Spec *datadoghq.DatadogPodAutoscalerSpec

	// SettingsTimestamp is the time when the settings were last updated
	// Version is stored in .Spec.RemoteVersion
	// (only if owner == remote)
	SettingsTimestamp time.Time

	// ScalingValues represents the current target scaling values (retrieved from RC)
	ScalingValues ScalingValues

	// HorizontalLastAction is the last horizontal action successfully taken
	HorizontalLastAction *datadoghq.DatadogPodAutoscalerHorizontalAction

	// HorizontalLastActionError is the last error encountered on horizontal scaling
	HorizontalLastActionError error

	// VerticalLastAction is the last action taken by the Vertical Pod Autoscaler
	VerticalLastAction *datadoghq.DatadogPodAutoscalerVerticalAction

	// VerticalLastActionError is the last error encountered on vertical scaling
	VerticalLastActionError error

	// CurrentReplicas is the current number of PODs for the targetRef
	CurrentReplicas *int32

	// ScaledReplicas is the current number of PODs for the targetRef matching the resources recommendations
	ScaledReplicas *int32

	// Error is the an error encountered by the controller not specific to a scaling action
	Error error

	// Deleted flags the PodAutoscaler as deleted (removal to be handled by the controller)
	// (only if owner == remote)
	Deleted bool

	//
	// Private fields
	//
	// targetGVK is the GroupVersionKind of the target resource
	// Parsed once from the .Spec.TargetRef
	targetGVK schema.GroupVersionKind
}

// NewPodAutoscalerInternal creates a new PodAutoscalerInternal from a Kubernetes CR
func NewPodAutoscalerInternal(podAutoscaler *datadoghq.DatadogPodAutoscaler) PodAutoscalerInternal {
	pai := PodAutoscalerInternal{
		Namespace: podAutoscaler.Namespace,
		Name:      podAutoscaler.Name,
	}
	pai.UpdateFromPodAutoscaler(podAutoscaler)
	pai.UpdateFromStatus(&podAutoscaler.Status)

	return pai
}

// NewPodAutoscalerFromSettings creates a new PodAutoscalerInternal from settings received through remote configuration
func NewPodAutoscalerFromSettings(ns, name string, podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsVersion uint64, settingsTimestamp time.Time) PodAutoscalerInternal {
	pda := PodAutoscalerInternal{
		Namespace: ns,
		Name:      name,
	}
	pda.UpdateFromSettings(podAutoscalerSpec, settingsVersion, settingsTimestamp)

	return pda
}

// ID returns the functional identifier of the PodAutoscaler
func (p *PodAutoscalerInternal) ID() string {
	return p.Namespace + "/" + p.Name
}

// GetTargetGVK resolves the GroupVersionKind if empty and returns it
func (p *PodAutoscalerInternal) GetTargetGVK() (schema.GroupVersionKind, error) {
	if !p.targetGVK.Empty() {
		return p.targetGVK, nil
	}

	gv, err := schema.ParseGroupVersion(p.Spec.TargetRef.APIVersion)
	if err != nil || gv.Group == "" || gv.Version == "" {
		return schema.GroupVersionKind{}, fmt.Errorf("failed to parse API version '%s', err: %w", p.Spec.TargetRef.APIVersion, err)
	}

	p.targetGVK = schema.GroupVersionKind{
		Group:   gv.Group,
		Version: gv.Version,
		Kind:    p.Spec.TargetRef.Kind,
	}
	return p.targetGVK, nil
}

// UpdateFromPodAutoscaler updates the PodAutoscalerInternal from a PodAutoscaler object inside K8S
func (p *PodAutoscalerInternal) UpdateFromPodAutoscaler(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	p.Generation = podAutoscaler.Generation
	p.Spec = podAutoscaler.Spec.DeepCopy()
	// Reset the target GVK as it might have changed
	// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
	p.targetGVK = schema.GroupVersionKind{}
}

// UpdateFromValues updates the PodAutoscalerInternal from a new scaling values
func (p *PodAutoscalerInternal) UpdateFromValues(scalingValues ScalingValues) {
	p.ScalingValues = scalingValues
}

// RemoveValues clears autoscaling values data from the PodAutoscalerInternal as we stopped autoscaling
func (p *PodAutoscalerInternal) RemoveValues() {
	p.ScalingValues = ScalingValues{}
}

// UpdateFromSettings updates the PodAutoscalerInternal from a new settings
func (p *PodAutoscalerInternal) UpdateFromSettings(podAutoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, settingsVersion uint64, settingsTimestamp time.Time) {
	p.SettingsTimestamp = settingsTimestamp
	p.Spec = podAutoscalerSpec // From settings, we don't need to deep copy as the object is not stored anywhere else
	p.Spec.RemoteVersion = pointer.Ptr(settingsVersion)
	// Reset the target GVK as it might have changed
	// Resolving the target GVK is done in the controller sync to ensure proper sync and error handling
	p.targetGVK = schema.GroupVersionKind{}
}

// UpdateFromStatus updates the PodAutoscalerInternal from an existing status.
// It assumes the PodAutoscalerInternal is empty so it's not emptying existing data.
func (p *PodAutoscalerInternal) UpdateFromStatus(status *datadoghq.DatadogPodAutoscalerStatus) {
	if status.Horizontal != nil {
		if status.Horizontal.Target != nil {
			p.ScalingValues.Horizontal = &HorizontalScalingValues{
				Source:    status.Horizontal.Target.Source,
				Timestamp: status.Horizontal.Target.GeneratedAt.Time,
				Replicas:  status.Horizontal.Target.Replicas,
			}
		}

		p.HorizontalLastAction = status.Horizontal.LastAction
	}

	if status.Vertical != nil {
		if status.Vertical.Target != nil {
			p.ScalingValues.Vertical = &VerticalScalingValues{
				Source:             status.Vertical.Target.Source,
				Timestamp:          status.Vertical.Target.GeneratedAt.Time,
				ContainerResources: status.Vertical.Target.DesiredResources,
				ResourcesHash:      status.Vertical.Target.Version,
			}
		}

		p.VerticalLastAction = status.Vertical.LastAction
	}

	if status.CurrentReplicas != nil {
		p.CurrentReplicas = status.CurrentReplicas
	}

	// Reading potential errors from conditions. Resetting internal errors first.
	// We're only keeping error string, loosing type, but it's not important for what we do.
	for _, cond := range status.Conditions {
		switch {
		case cond.Type == datadoghq.DatadogPodAutoscalerErrorCondition && cond.Status == corev1.ConditionTrue:
			// Error condition could refer to a controller error or from a general Datadog error
			// We're restoring this to error as it's the most generic
			p.Error = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition && cond.Status == corev1.ConditionFalse:
			p.HorizontalLastActionError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerVerticalAbleToApply && cond.Status == corev1.ConditionFalse:
			p.VerticalLastActionError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.ScalingValues.HorizontalError = errors.New(cond.Reason)
		case cond.Type == datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition && cond.Status == corev1.ConditionFalse:
			p.ScalingValues.VerticalError = errors.New(cond.Reason)
		}
	}
}

// BuildStatus builds the status of the PodAutoscaler from the internal state
func (p *PodAutoscalerInternal) BuildStatus(currentTime metav1.Time, currentStatus *datadoghq.DatadogPodAutoscalerStatus) datadoghq.DatadogPodAutoscalerStatus {
	status := datadoghq.DatadogPodAutoscalerStatus{}

	// Syncing current replicas
	if p.CurrentReplicas != nil {
		status.CurrentReplicas = p.CurrentReplicas
	}

	// Produce Horizontal status only if we have a desired number of replicas
	if p.ScalingValues.Horizontal != nil {
		status.Horizontal = &datadoghq.DatadogPodAutoscalerHorizontalStatus{
			Target: &datadoghq.DatadogPodAutoscalerHorizontalTargetStatus{
				Source:      p.ScalingValues.Horizontal.Source,
				GeneratedAt: metav1.NewTime(p.ScalingValues.Horizontal.Timestamp),
				Replicas:    p.ScalingValues.Horizontal.Replicas,
			},
			LastAction: p.HorizontalLastAction,
		}
	}

	// Produce Vertical status only if we have a desired container resources
	if p.ScalingValues.Vertical != nil {
		cpuReqSum, memReqSum := p.ScalingValues.Vertical.SumCPUMemoryRequests()

		status.Vertical = &datadoghq.DatadogPodAutoscalerVerticalStatus{
			Target: &datadoghq.DatadogPodAutoscalerVerticalTargetStatus{
				Source:           p.ScalingValues.Vertical.Source,
				GeneratedAt:      metav1.NewTime(p.ScalingValues.Vertical.Timestamp),
				Version:          p.ScalingValues.Vertical.ResourcesHash,
				DesiredResources: p.ScalingValues.Vertical.ContainerResources,
				Scaled:           p.ScaledReplicas,
				PODCPURequest:    cpuReqSum,
				PODMemoryRequest: memReqSum,
			},
			LastAction: p.VerticalLastAction,
		}
	}

	// Building conditions
	existingConditions := map[datadoghq.DatadogPodAutoscalerConditionType]*datadoghq.DatadogPodAutoscalerCondition{
		datadoghq.DatadogPodAutoscalerErrorCondition:                     nil,
		datadoghq.DatadogPodAutoscalerActiveCondition:                    nil,
		datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition: nil,
		datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition:     nil,
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
	globalError := p.Error
	if p.Error == nil {
		globalError = p.ScalingValues.Error
	}
	status.Conditions = append(status.Conditions, newConditionFromError(true, currentTime, globalError, datadoghq.DatadogPodAutoscalerErrorCondition, existingConditions))

	// Building active condition
	// TODO: Implement, currently always true
	status.Conditions = append(status.Conditions, newCondition(corev1.ConditionTrue, "", currentTime, datadoghq.DatadogPodAutoscalerActiveCondition, existingConditions))

	// Building errors related to compute recommendations
	var horizontalAbleToRecommend datadoghq.DatadogPodAutoscalerCondition
	if p.ScalingValues.HorizontalError != nil || p.ScalingValues.Horizontal != nil {
		horizontalAbleToRecommend = newConditionFromError(false, currentTime, p.ScalingValues.HorizontalError, datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	} else {
		horizontalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", currentTime, datadoghq.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, horizontalAbleToRecommend)

	var verticalAbleToRecommend datadoghq.DatadogPodAutoscalerCondition
	if p.ScalingValues.VerticalError != nil || p.ScalingValues.Vertical != nil {
		verticalAbleToRecommend = newConditionFromError(false, currentTime, p.ScalingValues.VerticalError, datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	} else {
		verticalAbleToRecommend = newCondition(corev1.ConditionUnknown, "", currentTime, datadoghq.DatadogPodAutoscalerVerticalAbleToRecommendCondition, existingConditions)
	}
	status.Conditions = append(status.Conditions, verticalAbleToRecommend)

	// Building rollout errors
	var horizontalReason string
	horizontalStatus := corev1.ConditionUnknown
	if p.HorizontalLastActionError != nil {
		horizontalStatus = corev1.ConditionFalse
		horizontalReason = p.HorizontalLastActionError.Error()
	} else if p.HorizontalLastAction != nil {
		horizontalStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(horizontalStatus, horizontalReason, currentTime, datadoghq.DatadogPodAutoscalerHorizontalAbleToScaleCondition, existingConditions))

	var verticalReason string
	rolloutStatus := corev1.ConditionUnknown
	if p.VerticalLastActionError != nil {
		rolloutStatus = corev1.ConditionFalse
		verticalReason = p.VerticalLastActionError.Error()
	} else if p.VerticalLastAction != nil {
		rolloutStatus = corev1.ConditionTrue
	}
	status.Conditions = append(status.Conditions, newCondition(rolloutStatus, verticalReason, currentTime, datadoghq.DatadogPodAutoscalerVerticalAbleToApply, existingConditions))

	return status
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
