// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"

	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ExtractPodDisruptionBudget returns the protobuf model corresponding to a Kubernetes
func ExtractPodDisruptionBudget(pdb *policyv1.PodDisruptionBudget) *model.PodDisruptionBudget {
	if pdb == nil {
		return nil
	}
	result := model.PodDisruptionBudget{
		Metadata: extractMetadata(&pdb.ObjectMeta),
		Spec:     extractPodDisruptionBudgetSpec(&pdb.Spec),
		Status:   extractPodDisruptionBudgetStatus(&pdb.Status),
	}
	result.Tags = append(result.Tags, transformers.RetrieveUnifiedServiceTags(pdb.ObjectMeta.Labels)...)
	return &result
}

func extractPodDisruptionBudgetSpec(spec *policyv1.PodDisruptionBudgetSpec) *model.PodDisruptionBudgetSpec {
	if spec == nil {
		return nil
	}
	result := model.PodDisruptionBudgetSpec{}
	result.MinAvailable = extractIntOrString(spec.MinAvailable)
	if spec.Selector != nil {
		result.Selector = extractLabelSelector(spec.Selector)
	}
	result.MaxUnavailable = extractIntOrString(spec.MaxUnavailable)
	if spec.UnhealthyPodEvictionPolicy != nil {
		result.UnhealthyPodEvictionPolicy = string(*spec.UnhealthyPodEvictionPolicy)
	}
	return &result
}

func extractIntOrString(source *intstr.IntOrString) *model.IntOrString {
	if source == nil {
		return nil
	}
	switch source.Type {
	case intstr.Int:
		return &model.IntOrString{
			Type:   model.IntOrString_Int,
			IntVal: source.IntVal,
		}
	case intstr.String:
		return &model.IntOrString{
			Type:   model.IntOrString_String,
			StrVal: source.StrVal,
		}
	}
	return nil
}

func extractPodDisruptionBudgetStatus(status *policyv1.PodDisruptionBudgetStatus) *model.PodDisruptionBudgetStatus {
	if status == nil {
		return nil
	}
	return &model.PodDisruptionBudgetStatus{
		DisruptedPods:      extractDisruptedPods(status.DisruptedPods),
		DisruptionsAllowed: status.DisruptionsAllowed,
		CurrentHealthy:     status.CurrentHealthy,
		DesiredHealthy:     status.DesiredHealthy,
		ExpectedPods:       status.ExpectedPods,
		Conditions:         extractPodDisruptionBudgetConditions(status.Conditions),
	}
}

func extractDisruptedPods(disruptedPodsmap map[string]metav1.Time) map[string]int64 {
	result := make(map[string]int64)
	for pod, t := range disruptedPodsmap {
		result[pod] = t.Time.Unix()
	}
	return result
}
func extractPodDisruptionBudgetConditions(conditions []metav1.Condition) []*model.Condition {
	result := make([]*model.Condition, 0)
	for _, condition := range conditions {
		result = append(result, &model.Condition{
			Type:               condition.Type,
			Status:             string(condition.Status),
			LastTransitionTime: condition.LastTransitionTime.Time.Unix(),
			Reason:             condition.Reason,
			Message:            condition.Message,
		})
	}
	return result
}
