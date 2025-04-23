// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func (p *PodAutoscalerInternal) String(verbose bool) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler ID -----------")
	_, _ = fmt.Fprintln(&sb, p.ID())

	if !verbose {
		return sb.String()
	}

	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Meta -----------")
	_, _ = fmt.Fprintln(&sb, "Creation Timestamp:", p.CreationTimestamp())
	_, _ = fmt.Fprintln(&sb, "Generation:", p.Generation())
	_, _ = fmt.Fprintln(&sb, "Settings Timestamp:", p.SettingsTimestamp())
	if p.Spec() != nil {
		_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Spec -----------")
		_, _ = fmt.Fprintln(&sb, "Target Ref:", p.Spec().TargetRef)
		_, _ = fmt.Fprintln(&sb, "Owner:", p.Spec().Owner)
		if p.Spec().RemoteVersion != nil {
			_, _ = fmt.Fprintln(&sb, "Remote Version:", *p.Spec().RemoteVersion)
		}
		if p.Spec().ApplyPolicy != nil {
			_, _ = fmt.Fprint(&sb, formatPolicy(p.Spec().ApplyPolicy))
		}
		if p.Spec().Fallback != nil {
			_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Local Fallback -----------")
			_, _ = fmt.Fprint(&sb, formatFallback(p.Spec().Fallback))
		}
		if p.Spec().Constraints != nil {
			_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Constraints -----------")
			_, _ = fmt.Fprint(&sb, formatConstraints(p.Spec().Constraints))
		}
		_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Objectives -----------")
		for _, objective := range p.Spec().Objectives {
			_, _ = fmt.Fprintln(&sb, formatObjective(&objective))
		}
	}

	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Scaling Values -----------")
	_, _ = fmt.Fprintln(&sb, formatScalingValues(p.ScalingValues()))
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Main Scaling Values -----------")
	_, _ = fmt.Fprintln(&sb, formatScalingValues(p.MainScalingValues()))
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Fallback Scaling Values -----------")
	_, _ = fmt.Fprintln(&sb, formatScalingValues(p.FallbackScalingValues()))
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Status -----------")
	if p.CurrentReplicas() != nil {
		_, _ = fmt.Fprintln(&sb, "Current Replicas:", *p.CurrentReplicas())
	}
	if p.ScaledReplicas() != nil {
		_, _ = fmt.Fprintln(&sb, "Scaled Replicas:", *p.ScaledReplicas())
	}
	if p.Error() != nil {
		_, _ = fmt.Fprintln(&sb, "Error:", p.Error())
	}
	if p.Deleted() {
		_, _ = fmt.Fprintln(&sb, "Deleted:", p.Deleted())
	}
	_, _ = fmt.Fprintln(&sb, "--------------------------------")
	if p.HorizontalLastActionError() != nil {
		_, _ = fmt.Fprintln(&sb, "Horizontal Last Action Error:", p.HorizontalLastActionError())
	}
	if p.HorizontalLastActions() != nil {
		for _, action := range p.HorizontalLastActions() {
			_, _ = fmt.Fprintln(&sb, "Horizontal Last Action:", formatHorizontalAction(&action))
		}
	}
	_, _ = fmt.Fprintln(&sb, "--------------------------------")
	if p.VerticalLastActionError() != nil {
		_, _ = fmt.Fprintln(&sb, "Vertical Last Action Error:", p.VerticalLastActionError())
	}
	if p.VerticalLastAction() != nil {
		_, _ = fmt.Fprintln(&sb, "Vertical Last Action:", formatVerticalAction(p.VerticalLastAction()))
	}

	if p.CustomRecommenderConfiguration() != nil {
		_, _ = fmt.Fprintln(&sb, "----------- Custom Recommender -----------")
		_, _ = fmt.Fprintln(&sb, "Endpoint:", p.CustomRecommenderConfiguration().Endpoint)
		_, _ = fmt.Fprintln(&sb, "Settings:", p.CustomRecommenderConfiguration().Settings)
	}

	return sb.String()
}

func formatPolicy(policy *v1alpha2.DatadogPodAutoscalerApplyPolicy) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Apply Policy Mode:", policy.Mode)
	if policy.Update != nil {
		_, _ = fmt.Fprintln(&sb, "Update Policy:", policy.Update.Strategy)
	}
	if policy.ScaleUp != nil {
		if policy.ScaleUp.Strategy != nil {
			_, _ = fmt.Fprintln(&sb, "Scale Up Strategy:", *policy.ScaleUp.Strategy)
		}
		for _, rule := range policy.ScaleUp.Rules {
			_, _ = fmt.Fprintln(&sb, "Scale Up Rule Type:", rule.Type)
			_, _ = fmt.Fprintln(&sb, "Scale Up Rule Value:", rule.Value)
			_, _ = fmt.Fprintln(&sb, "Scale Up Rule Period:", rule.PeriodSeconds)
		}
		_, _ = fmt.Fprintln(&sb, "Scale Up Stabilization Window:", policy.ScaleUp.StabilizationWindowSeconds)
	}
	if policy.ScaleDown != nil {
		if policy.ScaleDown.Strategy != nil {
			_, _ = fmt.Fprintln(&sb, "Scale Down Strategy:", *policy.ScaleDown.Strategy)
		}
		for _, rule := range policy.ScaleDown.Rules {
			_, _ = fmt.Fprintln(&sb, "Scale Down Rule Type:", rule.Type)
			_, _ = fmt.Fprintln(&sb, "Scale Down Rule Value:", rule.Value)
			_, _ = fmt.Fprintln(&sb, "Scale Down Rule Period:", rule.PeriodSeconds)
		}
		_, _ = fmt.Fprintln(&sb, "Scale Down Stabilization Window:", policy.ScaleDown.StabilizationWindowSeconds)
	}
	return sb.String()
}

func formatFallback(fallback *v1alpha2.DatadogFallbackPolicy) string {
	var sb strings.Builder
	if fallback != nil {
		_, _ = fmt.Fprintln(&sb, "Horizontal Fallback Enabled:", fallback.Horizontal.Enabled)
		_, _ = fmt.Fprintln(&sb, "Horizontal Fallback Stale Recommendation Threshold:", fallback.Horizontal.Triggers.StaleRecommendationThresholdSeconds)
	}
	return sb.String()
}

func formatConstraints(constraints *datadoghqcommon.DatadogPodAutoscalerConstraints) string {
	var sb strings.Builder
	if constraints.MinReplicas != nil {
		_, _ = fmt.Fprintln(&sb, "Min Replicas:", *constraints.MinReplicas)
	}
	_, _ = fmt.Fprintln(&sb, "Max Replicas:", constraints.MaxReplicas)

	for _, container := range constraints.Containers {
		_, _ = fmt.Fprintln(&sb, "Container:", container.Name)
		if container.Enabled != nil {
			_, _ = fmt.Fprintln(&sb, "Enabled:", *container.Enabled)
		}
		if container.Requests != nil {
			_, _ = fmt.Fprintln(&sb, "Requests Min Allowed:", toResourceQuantityMap(container.Requests.MinAllowed))
			_, _ = fmt.Fprintln(&sb, "Requests Max Allowed:", toResourceQuantityMap(container.Requests.MaxAllowed))
		}
	}
	return sb.String()
}

func formatObjective(objective *datadoghqcommon.DatadogPodAutoscalerObjective) string {
	formatObjectiveValue := func(sb *strings.Builder, value *datadoghqcommon.DatadogPodAutoscalerObjectiveValue) {
		if value.Utilization != nil {
			_, _ = fmt.Fprintln(sb, "Utilization:", *value.Utilization)
		}
	}

	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Objective Type:", objective.Type)
	if objective.PodResource != nil {
		_, _ = fmt.Fprintln(&sb, "Resource Name:", objective.PodResource.Name)
		formatObjectiveValue(&sb, &objective.PodResource.Value)
	}
	if objective.ContainerResource != nil {
		_, _ = fmt.Fprintln(&sb, "Resource Name:", objective.ContainerResource.Name)
		_, _ = fmt.Fprintln(&sb, "Container Name:", objective.ContainerResource.Container)
		formatObjectiveValue(&sb, &objective.ContainerResource.Value)
	}
	return sb.String()
}

func formatScalingValues(scalingValues ScalingValues) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "[Horizontal]")
	_, _ = fmt.Fprintln(&sb, "Horizontal Error:", scalingValues.HorizontalError)
	if scalingValues.Horizontal != nil {
		_, _ = fmt.Fprintln(&sb, "Source:", scalingValues.Horizontal.Source)
		_, _ = fmt.Fprintln(&sb, "Timestamp:", scalingValues.Horizontal.Timestamp)
		_, _ = fmt.Fprintln(&sb, "Replicas:", scalingValues.Horizontal.Replicas)
	}
	_, _ = fmt.Fprintln(&sb, "--------------------------------")
	_, _ = fmt.Fprintln(&sb, "[Vertical]")
	_, _ = fmt.Fprintln(&sb, "Vertical Error:", scalingValues.VerticalError)
	if scalingValues.Vertical != nil {
		_, _ = fmt.Fprintln(&sb, "Source:", scalingValues.Vertical.Source)
		_, _ = fmt.Fprintln(&sb, "Timestamp:", scalingValues.Vertical.Timestamp)
		_, _ = fmt.Fprintln(&sb, "ResourcesHash:", scalingValues.Vertical.ResourcesHash)
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			_, _ = fmt.Fprintln(&sb, "Container Name:", containerResources.Name)
			_, _ = fmt.Fprintln(&sb, "Container Resources:", toResourceQuantityMap(containerResources.Requests))
			_, _ = fmt.Fprintln(&sb, "Container Limits:", toResourceQuantityMap(containerResources.Limits))
		}
	}
	_, _ = fmt.Fprintln(&sb, "--------------------------------")
	_, _ = fmt.Fprintln(&sb, "Error:", scalingValues.Error)
	return sb.String()
}

func formatHorizontalAction(action *datadoghqcommon.DatadogPodAutoscalerHorizontalAction) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Timestamp:", action.Time)
	_, _ = fmt.Fprintln(&sb, "From Replicas:", action.FromReplicas)
	_, _ = fmt.Fprintln(&sb, "To Replicas:", action.ToReplicas)
	if action.RecommendedReplicas != nil {
		_, _ = fmt.Fprintln(&sb, "Recommended Replicas:", *action.RecommendedReplicas)
	}
	if action.LimitedReason != nil {
		_, _ = fmt.Fprintln(&sb, "Limited Reason:", *action.LimitedReason)
	}
	return sb.String()
}

func formatVerticalAction(action *datadoghqcommon.DatadogPodAutoscalerVerticalAction) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Timestamp:", action.Time)
	_, _ = fmt.Fprintln(&sb, "Version:", action.Version)
	_, _ = fmt.Fprintln(&sb, "Type:", action.Type)
	return sb.String()
}

// TODO: move to common util, shared with orchestrator
func toResourceQuantityMap(resourceList corev1.ResourceList) map[string]string {
	quantities := make(map[string]string)
	for name, quantity := range resourceList {
		quantities[string(name)] = quantity.String()
	}
	return quantities
}

// MarshalJSON implements the json.Marshaler interface for PodAutoscalerInternal
func (p *PodAutoscalerInternal) MarshalJSON() ([]byte, error) {
	// Create a map with all the fields we want to include in the JSON
	return json.Marshal(map[string]interface{}{
		"namespace":          p.namespace,
		"name":               p.name,
		"creation_timestamp": p.creationTimestamp,
		"generation":         p.generation,
		"spec":               p.spec,
		"settings_timestamp": p.settingsTimestamp,
		"scaling_values": map[string]interface{}{
			"horizontal":       p.scalingValues.Horizontal,
			"vertical":         p.scalingValues.Vertical,
			"error":            errorToString(p.scalingValues.Error),
			"horizontal_error": errorToString(p.scalingValues.HorizontalError),
			"vertical_error":   errorToString(p.scalingValues.VerticalError),
		},

		"main_scaling_values": map[string]interface{}{
			"horizontal":       p.mainScalingValues.Horizontal,
			"vertical":         p.mainScalingValues.Vertical,
			"error":            errorToString(p.mainScalingValues.Error),
			"horizontal_error": errorToString(p.mainScalingValues.HorizontalError),
			"vertical_error":   errorToString(p.mainScalingValues.VerticalError),
		},

		"fallback_scaling_values": map[string]interface{}{
			"horizontal":       p.fallbackScalingValues.Horizontal,
			"vertical":         p.fallbackScalingValues.Vertical,
			"error":            errorToString(p.fallbackScalingValues.Error),
			"horizontal_error": errorToString(p.fallbackScalingValues.HorizontalError),
			"vertical_error":   errorToString(p.fallbackScalingValues.VerticalError),
		},
		"horizontal_last_actions":          p.horizontalLastActions,
		"horizontal_last_limit_reason":     p.horizontalLastLimitReason,
		"horizontal_last_action_error":     errorToString(p.horizontalLastActionError),
		"vertical_last_action":             p.verticalLastAction,
		"vertical_last_action_error":       errorToString(p.verticalLastActionError),
		"current_replicas":                 p.currentReplicas,
		"scaled_replicas":                  p.scaledReplicas,
		"error":                            errorToString(p.error),
		"deleted":                          p.deleted,
		"target_gvk":                       p.targetGVK,
		"horizontal_events_retention":      p.horizontalEventsRetention,
		"custom_recommender_configuration": p.customRecommenderConfiguration,
	})
}

// Helper function to handle potential nil errors
func errorToString(err error) interface{} {
	if err == nil {
		return nil
	}
	return err.Error()
}

// stringToError converts a string or nil into an error
func stringToError(value interface{}) error {
	if value == nil {
		return nil
	}

	str, ok := value.(string)
	if !ok {
		return nil // If it's not a string, return nil
	}

	if str == "" {
		return nil // Empty string becomes nil error
	}

	return errors.New(str)
}

// UnmarshalJSON implements the json.Unmarshaler interface for PodAutoscalerInternal
func (p *PodAutoscalerInternal) UnmarshalJSON(data []byte) error {
	// Create a temporary struct to unmarshal into
	var temp struct {
		Namespace                      string                                                 `json:"namespace"`
		Name                           string                                                 `json:"name"`
		CreationTimestamp              time.Time                                              `json:"creation_timestamp"`
		Generation                     int64                                                  `json:"generation"`
		Spec                           *v1alpha2.DatadogPodAutoscalerSpec                     `json:"spec"`
		SettingsTimestamp              time.Time                                              `json:"settings_timestamp"`
		ScalingValues                  map[string]interface{}                                 `json:"scaling_values"`
		MainScalingValues              map[string]interface{}                                 `json:"main_scaling_values"`
		FallbackScalingValues          map[string]interface{}                                 `json:"fallback_scaling_values"`
		HorizontalLastActions          []datadoghqcommon.DatadogPodAutoscalerHorizontalAction `json:"horizontal_last_actions"`
		HorizontalLastLimitReason      string                                                 `json:"horizontal_last_limit_reason"`
		HorizontalLastActionError      interface{}                                            `json:"horizontal_last_action_error"`
		VerticalLastAction             *datadoghqcommon.DatadogPodAutoscalerVerticalAction    `json:"vertical_last_action"`
		VerticalLastActionError        interface{}                                            `json:"vertical_last_action_error"`
		CurrentReplicas                *int32                                                 `json:"current_replicas"`
		ScaledReplicas                 *int32                                                 `json:"scaled_replicas"`
		Error                          interface{}                                            `json:"error"`
		Deleted                        bool                                                   `json:"deleted"`
		TargetGVK                      schema.GroupVersionKind                                `json:"target_gvk"`
		HorizontalEventsRetention      time.Duration                                          `json:"horizontal_events_retention"`
		CustomRecommenderConfiguration *RecommenderConfiguration                              `json:"custom_recommender_configuration"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	// Copy the values to our PodAutoscalerInternal
	p.namespace = temp.Namespace
	p.name = temp.Name
	p.generation = temp.Generation
	p.creationTimestamp = temp.CreationTimestamp
	p.settingsTimestamp = temp.SettingsTimestamp
	p.spec = temp.Spec
	p.deleted = temp.Deleted
	p.currentReplicas = temp.CurrentReplicas
	p.scaledReplicas = temp.ScaledReplicas
	p.horizontalLastActions = temp.HorizontalLastActions
	p.horizontalLastLimitReason = temp.HorizontalLastLimitReason
	p.horizontalLastActionError = stringToError(temp.HorizontalLastActionError)
	p.verticalLastAction = temp.VerticalLastAction
	p.verticalLastActionError = stringToError(temp.VerticalLastActionError)
	p.targetGVK = temp.TargetGVK
	p.horizontalEventsRetention = temp.HorizontalEventsRetention
	p.customRecommenderConfiguration = temp.CustomRecommenderConfiguration
	p.error = stringToError(temp.Error)

	if temp.ScalingValues != nil {
		p.scalingValues = unmarshalScalingValues(temp.ScalingValues)
	}
	if temp.MainScalingValues != nil {
		p.mainScalingValues = unmarshalScalingValues(temp.MainScalingValues)
	}
	if temp.FallbackScalingValues != nil {
		p.fallbackScalingValues = unmarshalScalingValues(temp.FallbackScalingValues)
	}

	return nil
}

func unmarshalScalingValues(data map[string]interface{}) ScalingValues {
	result := ScalingValues{}

	// Handle the horizontal field
	if h, ok := data["horizontal"]; ok {
		if hBytes, err := json.Marshal(h); err == nil {
			var horizontal HorizontalScalingValues
			if err := json.Unmarshal(hBytes, &horizontal); err == nil {
				result.Horizontal = &horizontal
			}
		}
	}

	// Handle vertical similarly
	if v, ok := data["vertical"]; ok {
		if vBytes, err := json.Marshal(v); err == nil {
			var vertical VerticalScalingValues
			if err := json.Unmarshal(vBytes, &vertical); err == nil {
				result.Vertical = &vertical
			}
		}
	}

	// Handle error fields
	result.Error = stringToError(data["error"])
	result.HorizontalError = stringToError(data["horizontal_error"])
	result.VerticalError = stringToError(data["vertical_error"])

	return result
}
