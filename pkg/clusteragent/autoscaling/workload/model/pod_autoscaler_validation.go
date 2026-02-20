// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"fmt"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
)

// ValidateAutoscalerSpec validates the spec of a DatadogPodAutoscaler.
func ValidateAutoscalerSpec(spec *datadoghq.DatadogPodAutoscalerSpec) error {
	if err := validateConstraints(spec.Constraints); err != nil {
		return err
	}
	if err := validateObjectives(spec.Objectives); err != nil {
		return err
	}
	if err := validateFallback(spec.Fallback); err != nil {
		return err
	}
	return nil
}

func validateConstraints(constraints *datadoghqcommon.DatadogPodAutoscalerConstraints) error {
	if constraints == nil {
		return nil
	}

	if constraints.MinReplicas != nil && constraints.MaxReplicas != nil && *constraints.MaxReplicas < *constraints.MinReplicas {
		return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
			"constraints.maxReplicas (%d) must be greater than or equal to constraints.minReplicas (%d)",
			*constraints.MaxReplicas, *constraints.MinReplicas)
	}

	seen := make(map[string]struct{}, len(constraints.Containers))
	for _, container := range constraints.Containers {
		if _, exists := seen[container.Name]; exists {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"duplicate container name %q in constraints", container.Name)
		}
		seen[container.Name] = struct{}{}

		if err := validateContainerConstraints(container); err != nil {
			return err
		}
	}

	return nil
}

func validateObjectives(objectives []datadoghqcommon.DatadogPodAutoscalerObjective) error {
	for _, objective := range objectives {
		if err := validateSingleObjective(objective); err != nil {
			return err
		}
	}
	return nil
}

func validateFallback(fallback *datadoghq.DatadogFallbackPolicy) error {
	if fallback == nil {
		return nil
	}

	for _, objective := range fallback.Horizontal.Objectives {
		if objective.Type == datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"Autoscaler fallback cannot be based on custom query objective")
		}
		if err := validateSingleObjective(objective); err != nil {
			return fmt.Errorf("fallback: %w", err)
		}
	}

	return nil
}

func validateContainerConstraints(container datadoghqcommon.DatadogPodAutoscalerContainerConstraints) error {
	if err := validateResourceBounds(container.Name, container.MinAllowed, container.MaxAllowed); err != nil {
		return err
	}

	if container.Requests != nil {
		if err := validateResourceBounds(container.Name, container.Requests.MinAllowed, container.Requests.MaxAllowed); err != nil {
			return err
		}
	}

	return nil
}

func validateResourceBounds(containerName string, minAllowed, maxAllowed corev1.ResourceList) error {
	for resource, minVal := range minAllowed {
		maxVal, hasMax := maxAllowed[resource]
		if hasMax && minVal.Cmp(maxVal) > 0 {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"container %q: minAllowed %s (%s) must be less than or equal to maxAllowed (%s)",
				containerName, resource, minVal.String(), maxVal.String())
		}
	}
	return nil
}

func validateSingleObjective(objective datadoghqcommon.DatadogPodAutoscalerObjective) error {
	switch objective.Type {
	case datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType:
		if objective.CustomQuery == nil {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"Autoscaler objective type is custom query but customQueryObjective is nil")
		}
	case datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType:
		if objective.PodResource == nil {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"autoscaler objective type is %s but podResource is nil", objective.Type)
		}
	case datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType:
		if objective.ContainerResource == nil {
			return autoscaling.NewConditionErrorf(autoscaling.ConditionReasonInvalidSpec,
				"autoscaler objective type is %s but containerResource is nil", objective.Type)
		}
	}
	return nil
}
