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
	_, _ = fmt.Fprintln(&sb)

	if !verbose {
		return sb.String()
	}

	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Meta -----------")
	_, _ = fmt.Fprintln(&sb, "Creation Timestamp:", p.CreationTimestamp())
	_, _ = fmt.Fprintln(&sb, "Generation:", p.Generation())
	_, _ = fmt.Fprintln(&sb, "Settings Timestamp:", p.SettingsTimestamp())
	_, _ = fmt.Fprintln(&sb)

	if p.Spec() != nil {
		_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Spec -----------")
		_, _ = fmt.Fprintln(&sb, "Target Ref:", p.Spec().TargetRef)
		_, _ = fmt.Fprintln(&sb, "Owner:", p.Spec().Owner)
		if p.Spec().RemoteVersion != nil {
			_, _ = fmt.Fprintln(&sb, "Remote Version:", *p.Spec().RemoteVersion)
		}
		if p.Spec().ApplyPolicy != nil {
			_, _ = fmt.Fprintln(&sb, formatPolicy(p.Spec().ApplyPolicy))
		}
		if p.Spec().Fallback != nil {
			_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Local Fallback -----------")
			_, _ = fmt.Fprintln(&sb, formatFallback(p.Spec().Fallback))
		}
		if p.Spec().Constraints != nil {
			_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Constraints -----------")
			_, _ = fmt.Fprintln(&sb, formatConstraints(p.Spec().Constraints))
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
		_, _ = fmt.Fprintln(&sb, "--------------------------------")
	}
	if p.HorizontalLastRecommendations() != nil {
		for _, recommendation := range p.HorizontalLastRecommendations() {
			_, _ = fmt.Fprintln(&sb, "Horizontal Last Recommendation:", formatHorizontalRecommendation(&recommendation))
		}
		_, _ = fmt.Fprintln(&sb, "--------------------------------")
	}
	if p.VerticalLastActionError() != nil {
		_, _ = fmt.Fprintln(&sb, "Vertical Last Action Error:", p.VerticalLastActionError())
	}
	if p.VerticalLastAction() != nil {
		_, _ = fmt.Fprintln(&sb, "Vertical Last Action:", formatVerticalAction(p.VerticalLastAction()))
	}
	_, _ = fmt.Fprintln(&sb)

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
		_, _ = fmt.Fprintln(&sb, "Horizontal Fallback Scaling Direction:", fallback.Horizontal.Direction)
	}
	return sb.String()
}

func formatConstraints(constraints *datadoghqcommon.DatadogPodAutoscalerConstraints) string {
	var sb strings.Builder
	if constraints.MinReplicas != nil {
		_, _ = fmt.Fprintln(&sb, "Min Replicas:", *constraints.MinReplicas)
	}
	if constraints.MaxReplicas != nil {
		_, _ = fmt.Fprintln(&sb, "Max Replicas:", *constraints.MaxReplicas)
	}

	for _, container := range constraints.Containers {
		_, _ = fmt.Fprintln(&sb, "Container:", container.Name)
		if container.Enabled != nil {
			_, _ = fmt.Fprintln(&sb, "Enabled:", *container.Enabled)
		}
		if container.Requests != nil {
			_, _ = fmt.Fprintln(&sb, "Requests Min Allowed:", printResourceList(container.Requests.MinAllowed))
			_, _ = fmt.Fprintln(&sb, "Requests Max Allowed:", printResourceList(container.Requests.MaxAllowed))
		}
	}
	return sb.String()
}

func formatObjective(objective *datadoghqcommon.DatadogPodAutoscalerObjective) string {
	formatObjectiveValue := func(sb *strings.Builder, value *datadoghqcommon.DatadogPodAutoscalerObjectiveValue) {
		if value.Utilization != nil {
			_, _ = fmt.Fprintln(sb, "Utilization:", *value.Utilization)
		}
		if value.AbsoluteValue != nil {
			_, _ = fmt.Fprintln(sb, "Average Value:", value.AbsoluteValue.String())
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
	if objective.CustomQuery != nil {
		formatObjectiveValue(&sb, &objective.CustomQuery.Value)
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
			_, _ = fmt.Fprintln(&sb, "Container Resources:", printResourceList(containerResources.Requests))
			_, _ = fmt.Fprintln(&sb, "Container Limits:", printResourceList(containerResources.Limits))
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
	return strings.TrimRight(sb.String(), "\n")
}

func formatVerticalAction(action *datadoghqcommon.DatadogPodAutoscalerVerticalAction) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Timestamp:", action.Time)
	_, _ = fmt.Fprintln(&sb, "Version:", action.Version)
	_, _ = fmt.Fprintln(&sb, "Type:", action.Type)
	return strings.TrimRight(sb.String(), "\n")
}

func formatHorizontalRecommendation(recommendation *datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation) string {
	var sb strings.Builder
	_, _ = fmt.Fprintln(&sb, "Source:", recommendation.Source)
	_, _ = fmt.Fprintln(&sb, "GeneratedAt:", recommendation.GeneratedAt)
	_, _ = fmt.Fprintln(&sb, "Replicas:", recommendation.Replicas)
	return strings.TrimRight(sb.String(), "\n")
}

func printResourceList(resourceList corev1.ResourceList) string {
	var sb strings.Builder
	if cpuQuantity, exists := resourceList[corev1.ResourceCPU]; exists {
		_, _ = fmt.Fprintf(&sb, "[cpu:%s]", cpuQuantity.String())
	}
	if memoryQuantity, exists := resourceList[corev1.ResourceMemory]; exists {
		_, _ = fmt.Fprintf(&sb, "[memory:%s]", memoryQuantity.String())
	}
	return sb.String()
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

// MarshalJSON implements the json.Marshaler interface for PodAutoscalerInternal
func (p *PodAutoscalerInternal) MarshalJSON() ([]byte, error) {
	// Create a map with all the fields we want to include in the JSON
	return json.Marshal(map[string]interface{}{
		"namespace":                                p.namespace,
		"name":                                     p.name,
		"creation_timestamp":                       p.creationTimestamp,
		"generation":                               p.generation,
		"spec":                                     p.spec,
		"settings_timestamp":                       p.settingsTimestamp,
		"scaling_values":                           p.scalingValues,
		"scaling_values_error":                     errorToString(p.scalingValues.Error),
		"scaling_values_horizontal_error":          errorToString(p.scalingValues.HorizontalError),
		"scaling_values_vertical_error":            errorToString(p.scalingValues.VerticalError),
		"main_scaling_values":                      p.mainScalingValues,
		"main_scaling_values_error":                errorToString(p.mainScalingValues.Error),
		"main_scaling_values_horizontal_error":     errorToString(p.mainScalingValues.HorizontalError),
		"main_scaling_values_vertical_error":       errorToString(p.mainScalingValues.VerticalError),
		"fallback_scaling_values":                  p.fallbackScalingValues,
		"fallback_scaling_values_error":            errorToString(p.fallbackScalingValues.Error),
		"fallback_scaling_values_horizontal_error": errorToString(p.fallbackScalingValues.HorizontalError),
		"fallback_scaling_values_vertical_error":   errorToString(p.fallbackScalingValues.VerticalError),
		"horizontal_last_actions":                  p.horizontalLastActions,
		"horizontal_last_recommendations":          p.horizontalLastRecommendations,
		"horizontal_last_limit_reason":             p.horizontalLastLimitReason,
		"horizontal_last_action_error":             errorToString(p.horizontalLastActionError),
		"vertical_last_action":                     p.verticalLastAction,
		"vertical_last_action_error":               errorToString(p.verticalLastActionError),
		"current_replicas":                         p.currentReplicas,
		"scaled_replicas":                          p.scaledReplicas,
		"error":                                    errorToString(p.error),
		"deleted":                                  p.deleted,
		"target_gvk":                               p.targetGVK,
		"horizontal_events_retention":              p.horizontalEventsRetention,
		"horizontal_recommendations_retention":     p.horizontalRecommendationsRetention,
		"custom_recommender_configuration":         p.customRecommenderConfiguration,
	})
}

// UnmarshalJSON implements the json.Unmarshaler interface for PodAutoscalerInternal
func (p *PodAutoscalerInternal) UnmarshalJSON(data []byte) error {
	// Create a temporary struct to unmarshal into
	var temp struct {
		Namespace                            string                                                         `json:"namespace"`
		Name                                 string                                                         `json:"name"`
		CreationTimestamp                    time.Time                                                      `json:"creation_timestamp"`
		Generation                           int64                                                          `json:"generation"`
		Spec                                 *v1alpha2.DatadogPodAutoscalerSpec                             `json:"spec"`
		SettingsTimestamp                    time.Time                                                      `json:"settings_timestamp"`
		ScalingValues                        ScalingValues                                                  `json:"scaling_values"`
		ScalingValuesError                   interface{}                                                    `json:"scaling_values_error"`
		ScalingValuesHorizontalError         interface{}                                                    `json:"scaling_values_horizontal_error"`
		ScalingValuesVerticalError           interface{}                                                    `json:"scaling_values_vertical_error"`
		MainScalingValues                    ScalingValues                                                  `json:"main_scaling_values"`
		MainScalingValuesError               interface{}                                                    `json:"main_scaling_values_error"`
		MainScalingValuesHorizontalError     interface{}                                                    `json:"main_scaling_values_horizontal_error"`
		MainScalingValuesVerticalError       interface{}                                                    `json:"main_scaling_values_vertical_error"`
		FallbackScalingValues                ScalingValues                                                  `json:"fallback_scaling_values"`
		FallbackScalingValuesError           interface{}                                                    `json:"fallback_scaling_values_error"`
		FallbackScalingValuesHorizontalError interface{}                                                    `json:"fallback_scaling_values_horizontal_error"`
		FallbackScalingValuesVerticalError   interface{}                                                    `json:"fallback_scaling_values_vertical_error"`
		HorizontalLastActions                []datadoghqcommon.DatadogPodAutoscalerHorizontalAction         `json:"horizontal_last_actions"`
		HorizontalLastRecommendations        []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation `json:"horizontal_last_recommendations"`
		HorizontalLastLimitReason            string                                                         `json:"horizontal_last_limit_reason"`
		HorizontalLastActionError            interface{}                                                    `json:"horizontal_last_action_error"`
		VerticalLastAction                   *datadoghqcommon.DatadogPodAutoscalerVerticalAction            `json:"vertical_last_action"`
		VerticalLastActionError              interface{}                                                    `json:"vertical_last_action_error"`
		CurrentReplicas                      *int32                                                         `json:"current_replicas"`
		ScaledReplicas                       *int32                                                         `json:"scaled_replicas"`
		Error                                interface{}                                                    `json:"error"`
		Deleted                              bool                                                           `json:"deleted"`
		TargetGVK                            schema.GroupVersionKind                                        `json:"target_gvk"`
		HorizontalEventsRetention            time.Duration                                                  `json:"horizontal_events_retention"`
		HorizontalRecommendationsRetention   time.Duration                                                  `json:"horizontal_recommendations_retention"`
		CustomRecommenderConfiguration       *RecommenderConfiguration                                      `json:"custom_recommender_configuration"`
	}

	if err := json.Unmarshal(data, &temp); err != nil {
		// If the error is not related to unmarshaling into error type, return the error
		if !(strings.Contains(err.Error(), "cannot unmarshal object into Go struct field") &&
			strings.Contains(err.Error(), "of type error")) {
			return err
		}
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
	p.horizontalLastRecommendations = temp.HorizontalLastRecommendations
	p.horizontalLastLimitReason = temp.HorizontalLastLimitReason
	p.horizontalLastActionError = stringToError(temp.HorizontalLastActionError)
	p.verticalLastAction = temp.VerticalLastAction
	p.verticalLastActionError = stringToError(temp.VerticalLastActionError)
	p.targetGVK = temp.TargetGVK
	p.horizontalEventsRetention = temp.HorizontalEventsRetention
	p.horizontalRecommendationsRetention = temp.HorizontalRecommendationsRetention
	p.customRecommenderConfiguration = temp.CustomRecommenderConfiguration
	p.error = stringToError(temp.Error)
	p.scalingValues = temp.ScalingValues
	p.scalingValues.Error = stringToError(temp.ScalingValuesError)
	p.scalingValues.HorizontalError = stringToError(temp.ScalingValuesHorizontalError)
	p.scalingValues.VerticalError = stringToError(temp.ScalingValuesVerticalError)
	p.mainScalingValues = temp.MainScalingValues
	p.mainScalingValues.Error = stringToError(temp.MainScalingValuesError)
	p.mainScalingValues.HorizontalError = stringToError(temp.MainScalingValuesHorizontalError)
	p.mainScalingValues.VerticalError = stringToError(temp.MainScalingValuesVerticalError)
	p.fallbackScalingValues = temp.FallbackScalingValues
	p.fallbackScalingValues.Error = stringToError(temp.FallbackScalingValuesError)
	p.fallbackScalingValues.HorizontalError = stringToError(temp.FallbackScalingValuesHorizontalError)
	p.fallbackScalingValues.VerticalError = stringToError(temp.FallbackScalingValuesVerticalError)

	// convert metav1.Time to UTC
	for i := range p.horizontalLastActions {
		p.horizontalLastActions[i].Time.Time = p.horizontalLastActions[i].Time.Time.UTC()
	}
	if p.verticalLastAction != nil {
		p.verticalLastAction.Time.Time = p.verticalLastAction.Time.Time.UTC()
	}
	for i := range p.horizontalLastRecommendations {
		p.horizontalLastRecommendations[i].GeneratedAt.Time = p.horizontalLastRecommendations[i].GeneratedAt.Time.UTC()
	}

	return nil
}
