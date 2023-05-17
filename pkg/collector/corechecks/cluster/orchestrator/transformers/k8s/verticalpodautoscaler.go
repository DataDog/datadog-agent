// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
)

// ExtractVerticalPodAutoscaler returns the protobuf model corresponding to a Kubernetes Vertical Pod Autoscaler resource.
func ExtractVerticalPodAutoscaler(v *v1.VerticalPodAutoscaler) *model.VerticalPodAutoscaler {
	if v == nil {
		return &model.VerticalPodAutoscaler{}
	}

	m := &model.VerticalPodAutoscaler{
		Metadata: extractMetadata(&v.ObjectMeta),
		Spec:     extractVerticalPodAutoscalerSpec(&v.Spec),
		Status:   extractVerticalPodAutoscalerStatus(&v.Status),
	}
	m.Tags = append(m.Tags, transformers.RetrieveUnifiedServiceTags(v.ObjectMeta.Labels)...)
	return m
}

// extractVerticalPodAutoscalerSpec converts the Kubernetes spec to our custom one
// https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1/types.go#L73
func extractVerticalPodAutoscalerSpec(s *v1.VerticalPodAutoscalerSpec) *model.VerticalPodAutoscalerSpec {
	spec := &model.VerticalPodAutoscalerSpec{
		Target: &model.VerticalPodAutoscalerTarget{
			Kind: s.TargetRef.Kind,
			Name: s.TargetRef.Name,
		},
		ResourcePolicies: extractContainerResourcePolicies(s.ResourcePolicy),
	}
	if s.UpdatePolicy != nil && s.UpdatePolicy.UpdateMode != nil {
		spec.UpdateMode = string(*s.UpdatePolicy.UpdateMode)
	}
	return spec
}

// extractContainerResourcePolicy pulls the ContainerResourcePolicy out of PodResourcePolicy
// and converts it to our protobuf model
// https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1/types.go#L149
func extractContainerResourcePolicies(p *v1.PodResourcePolicy) []*model.ContainerResourcePolicy {
	if p == nil {
		return []*model.ContainerResourcePolicy{}
	}

	policies := []*model.ContainerResourcePolicy{}
	if p == nil {
		return policies
	}

	for _, policy := range p.ContainerPolicies {
		m := model.ContainerResourcePolicy{
			ContainerName:      policy.ContainerName,
			MinAllowed:         extractResourceList(&policy.MinAllowed),
			MaxAllowed:         extractResourceList(&policy.MaxAllowed),
			ControlledResource: extractControlledResources(policy.ControlledResources),
		}
		if policy.Mode != nil {
			m.Mode = string(*policy.Mode)
		}
		if policy.ControlledValues != nil {
			m.ControlledValues = string(*policy.ControlledValues)
		}
		policies = append(policies, &m)
	}
	return policies
}

// extractResourceList converts Kuberentes ResourceLists to our protobuf model
// https://github.com/kubernetes/api/blob/v0.23.8/core/v1/types.go#L5176
func extractResourceList(rl *corev1.ResourceList) *model.ResourceList {
	if rl == nil {
		return &model.ResourceList{}
	}

	mv := map[string]float64{}
	for name, quantity := range *rl {
		value, valid := quantity.AsInt64()
		if valid {
			mv[string(name)] = float64(value)
		} else {
			mv[string(name)] = quantity.ToDec().AsApproximateFloat64()
		}
	}
	return &model.ResourceList{
		MetricValues: mv,
	}
}

// extractControlledResources converts typed Kuberentes ResourceNames to a slice of strings
// https://github.com/kubernetes/api/blob/v0.23.8/core/v1/types.go#L5147
func extractControlledResources(rn *[]corev1.ResourceName) []string {
	if rn == nil {
		return []string{}
	}

	names := []string{}
	for _, name := range *rn {
		names = append(names, string(name))
	}
	return names
}

// extractVerticalPodAutoscalerStatus converts Kubernetes PodAutoscalerStatus to our protobuf model
// https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1/types.go#L218
func extractVerticalPodAutoscalerStatus(s *v1.VerticalPodAutoscalerStatus) *model.VerticalPodAutoscalerStatus {
	if s == nil {
		return &model.VerticalPodAutoscalerStatus{}
	}

	status := model.VerticalPodAutoscalerStatus{}
	for _, condition := range s.Conditions {
		if condition.Type == v1.RecommendationProvided &&
			condition.Status == corev1.ConditionTrue {
			status.LastRecommendedDate = condition.LastTransitionTime.Unix()
		}
	}
	if s.Recommendation != nil {
		status.Recommendations = extractContainerRecommendations(s.Recommendation.ContainerRecommendations)
	}
	status.Conditions = extractContainerConditions(s.Conditions)
	return &status
}

// extractContainerRecommendations converts Kuberentes Recommendations to our protobuf model
// https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1/types.go#L245
func extractContainerRecommendations(cr []v1.RecommendedContainerResources) []*model.ContainerRecommendation {
	if cr == nil {
		return []*model.ContainerRecommendation{}
	}

	recs := []*model.ContainerRecommendation{}
	for _, r := range cr {
		rec := model.ContainerRecommendation{
			ContainerName:  r.ContainerName,
			Target:         extractResourceList(&r.Target),
			LowerBound:     extractResourceList(&r.LowerBound),
			UpperBound:     extractResourceList(&r.UpperBound),
			UncappedTarget: extractResourceList(&r.UncappedTarget),
		}
		recs = append(recs, &rec)
	}
	return recs
}

// extractContainerConditions converts Kuberentes Conditions to our protobuf model
// https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1/types.go#L295
func extractContainerConditions(cr []v1.VerticalPodAutoscalerCondition) []*model.VPACondition {
	con := []*model.VPACondition{}
	for _, r := range cr {
		rec := model.VPACondition{
			ConditionType:      string(r.Type),
			ConditionStatus:    string(r.Status),
			LastTransitionTime: r.LastTransitionTime.Unix(),
			Reason:             r.Reason,
			Message:            r.Message,
		}
		con = append(con, &rec)
	}
	return con
}
