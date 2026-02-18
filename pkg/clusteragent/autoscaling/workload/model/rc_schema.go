// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	kubeAutoscaling "github.com/DataDog/agent-payload/v5/autoscaling/kubernetes"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha2 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

// ReccomendationError is an error encountered while computing a recommendation on Datadog side
type ReccomendationError kubeAutoscaling.Error

// Error returns the error message
func (e *ReccomendationError) Error() string {
	return e.Message
}

// Reason implements ConditionReason, providing a programmatic reason for conditions.
func (e *ReccomendationError) Reason() autoscaling.ConditionReasonType {
	return autoscaling.ConditionReasonRecommendationError
}

// AutoscalingSettingsList holds a list of AutoscalingSettings
type AutoscalingSettingsList struct {
	// Settings is a list of .Spec
	Settings []AutoscalingSettings `json:"settings"`
}

// AutoscalingSettings is the .Spec of a PodAutoscaler retrieved through remote config
type AutoscalingSettings struct {
	// Namespace is the namespace of the PodAutoscaler
	Namespace string `json:"namespace"`

	// Name is the name of the PodAutoscaler
	Name string `json:"name"`

	// Spec is the full spec of the PodAutoscaler
	// WARNING: Legacy field, to be removed
	Spec *datadoghqv1alpha1.DatadogPodAutoscalerSpec `json:"spec,omitempty"`

	// Specs contains one version of the PodAutoscaler spec
	// Has priority over the legacy field Spec
	Specs *AutoscalingSpecs `json:"specs,omitempty"`
}

// AutoscalingSpecs contains the different versions of the PodAutoscaler spec
type AutoscalingSpecs struct {
	V1Alpha1 *datadoghqv1alpha1.DatadogPodAutoscalerSpec `json:"v1alpha1,omitempty"`
	V1Alpha2 *datadoghqv1alpha2.DatadogPodAutoscalerSpec `json:"v1alpha2,omitempty"`
}
