// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

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
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Spec -----------")
	_, _ = fmt.Fprintln(&sb, "Target Ref:", p.Spec().TargetRef)
	_, _ = fmt.Fprintln(&sb, "Owner:", p.Spec().Owner)
	_, _ = fmt.Fprintln(&sb, "Remote Version:", p.Spec().RemoteVersion)
	if p.Spec().ApplyPolicy != nil {
		_, _ = fmt.Fprint(&sb, formatPolicy(p.Spec().ApplyPolicy))
	}
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Local Fallback -----------")
	_, _ = fmt.Fprint(&sb, formatFallback(p.Spec().Fallback))
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Constraints -----------")
	_, _ = fmt.Fprint(&sb, formatConstraints(p.Spec().Constraints))
	_, _ = fmt.Fprintln(&sb, "----------- PodAutoscaler Objectives -----------")
	for _, objective := range p.Spec().Objectives {
		_, _ = fmt.Fprintln(&sb, formatObjective(&objective))
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
		_, _ = fmt.Fprintln(&sb, "Scale Up Policy:", policy.ScaleUp)
	}
	if policy.ScaleDown != nil {
		_, _ = fmt.Fprintln(&sb, "Scale Down Policy:", policy.ScaleDown)
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
