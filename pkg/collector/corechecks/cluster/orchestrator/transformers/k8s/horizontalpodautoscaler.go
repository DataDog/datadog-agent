// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	v2 "k8s.io/api/autoscaling/v2"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers"
)

// ExtractHorizontalPodAutoscaler returns the protobuf model corresponding to a Kubernetes Horizontal Pod Autoscaler resource.
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L33
func ExtractHorizontalPodAutoscaler(v *v2.HorizontalPodAutoscaler) *model.HorizontalPodAutoscaler {
	if v == nil {
		return &model.HorizontalPodAutoscaler{}
	}

	m := &model.HorizontalPodAutoscaler{
		Metadata: extractMetadata(&v.ObjectMeta),
		Spec:     extractHorizontalPodAutoscalerSpec(&v.Spec),
		Status:   extractHorizontalPodAutoscalerStatus(&v.Status),
	}

	if len(v.Status.Conditions) > 0 {
		hpaConditions, conditionTags := extractHorizontalPodAutoscalerConditions(v)
		m.Conditions = hpaConditions
		m.Tags = append(m.Tags, conditionTags...)
	}

	m.Tags = append(m.Tags, transformers.RetrieveUnifiedServiceTags(v.ObjectMeta.Labels)...)
	return m
}

// extractHorizontalPodAutoscalerSpec converts Kubernetes HorizontalPodAutoscalerSpec to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L51
func extractHorizontalPodAutoscalerSpec(s *v2.HorizontalPodAutoscalerSpec) *model.HorizontalPodAutoscalerSpec {
	spec := model.HorizontalPodAutoscalerSpec{
		Target: &model.HorizontalPodAutoscalerTarget{
			Kind: s.ScaleTargetRef.Kind,
			Name: s.ScaleTargetRef.Name,
		},
		MaxReplicas: s.MaxReplicas,
		Metrics:     extractMetricSpec(s.Metrics),
		Behavior:    extractHorizontalPodAutoscalerBehavior(s.Behavior),
	}
	if s.MinReplicas != nil {
		spec.MinReplicas = *s.MinReplicas
	}
	return &spec
}

// extractMetricSpec converts Kubernetes MetricSpec to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L97
func extractMetricSpec(ms []v2.MetricSpec) []*model.HorizontalPodAutoscalerMetricSpec {
	if ms == nil {
		return []*model.HorizontalPodAutoscalerMetricSpec{}
	}

	metrics := []*model.HorizontalPodAutoscalerMetricSpec{}
	for _, m := range ms {
		metric := &model.HorizontalPodAutoscalerMetricSpec{
			Type: string(m.Type),
		}
		switch m.Type {
		case v2.ObjectMetricSourceType:
			metric.Object = extractObjectMetric(m.Object)
		case v2.PodsMetricSourceType:
			metric.Pods = extractPodsMetric(m.Pods)
		case v2.ResourceMetricSourceType:
			metric.Resource = extractResourceMetric(m.Resource)
		case v2.ContainerResourceMetricSourceType:
			metric.ContainerResource = extractContainerResourceMetric(m.ContainerResource)
		case v2.ExternalMetricSourceType:
			metric.External = extractExternalMetric(m.External)
		}
		metrics = append(metrics, metric)
	}
	return metrics
}

// extractMetricIdentifier converts Kubernetes MetricIdentifier to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L310
func extractMetricIdentifier(m v2.MetricIdentifier) *model.MetricIdentifier {
	mi := model.MetricIdentifier{
		Name: m.Name,
	}
	if m.Selector != nil {
		mi.LabelSelector = extractLabelSelector(m.Selector)
	}
	return &mi
}

// extractMetricTarget converts Kubernetes MetricTarget to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L321
func extractMetricTarget(m v2.MetricTarget) *model.MetricTarget {
	mt := model.MetricTarget{
		Type: string(m.Type),
	}

	switch m.Type {
	case v2.UtilizationMetricType:
		if m.AverageUtilization != nil {
			mt.Value = int64(*m.AverageUtilization)
		}
	case v2.ValueMetricType:
		if m.Value != nil {
			mt.Value = m.Value.ToDec().MilliValue()
		}
	case v2.AverageValueMetricType:
		if m.AverageValue != nil {
			mt.Value = m.AverageValue.ToDec().MilliValue()
		}
	}
	return &mt
}

// extractObjectMetric converts Kubernetes ObjectMetricSource to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L249
func extractObjectMetric(s *v2.ObjectMetricSource) *model.ObjectMetricSource {
	if s == nil {
		return &model.ObjectMetricSource{}
	}
	m := model.ObjectMetricSource{
		DescribedObject: &model.ObjectReference{
			Kind:       s.DescribedObject.Kind,
			Name:       s.DescribedObject.Name,
			ApiVersion: s.DescribedObject.APIVersion,
		},
		Metric: extractMetricIdentifier(s.Metric),
		Target: extractMetricTarget(s.Target),
	}
	return &m
}

// extractPodsMetric converts Kubernetes PodsMetricSource to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L262
func extractPodsMetric(s *v2.PodsMetricSource) *model.PodsMetricSource {
	if s == nil {
		return &model.PodsMetricSource{}
	}
	m := model.PodsMetricSource{
		Metric: extractMetricIdentifier(s.Metric),
		Target: extractMetricTarget(s.Target),
	}
	return &m
}

// extractResourceMetric converts Kubernetes ResourceMetricSource to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L276
func extractResourceMetric(s *v2.ResourceMetricSource) *model.ResourceMetricSource {
	if s == nil {
		return &model.ResourceMetricSource{}
	}
	m := model.ResourceMetricSource{
		ResourceName: s.Name.String(),
		Target:       extractMetricTarget(s.Target),
	}
	return &m
}

// extractContainerResourceMetric converts Kubernetes ContainerResourceMetricSource to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L290
func extractContainerResourceMetric(s *v2.ContainerResourceMetricSource) *model.ContainerResourceMetricSource {
	if s == nil {
		return &model.ContainerResourceMetricSource{}
	}
	m := model.ContainerResourceMetricSource{
		ResourceName: s.Name.String(),
		Target:       extractMetricTarget(s.Target),
		Container:    s.Container,
	}
	return &m
}

// extractExternalMetric converts Kubernetes ExternalMetricSource to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L302
func extractExternalMetric(s *v2.ExternalMetricSource) *model.ExternalMetricSource {
	if s == nil {
		return &model.ExternalMetricSource{}
	}
	m := model.ExternalMetricSource{
		Metric: extractMetricIdentifier(s.Metric),
		Target: extractMetricTarget(s.Target),
	}
	return &m
}

// extractHorizontalPodAutoscalerBehavior converts Kubernetes HorizontalPodAutoscalerBehavior to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L139
func extractHorizontalPodAutoscalerBehavior(v *v2.HorizontalPodAutoscalerBehavior) *model.HorizontalPodAutoscalerBehavior {
	if v == nil {
		return &model.HorizontalPodAutoscalerBehavior{}
	}

	b := model.HorizontalPodAutoscalerBehavior{}
	if v.ScaleUp != nil {
		b.ScaleUp = extractScalingRules(v.ScaleUp)
	}
	if v.ScaleDown != nil {
		b.ScaleDown = extractScalingRules(v.ScaleDown)
	}

	return &b
}

// extractScalingRules converts Kubernetes HPAScalingRules to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L173
func extractScalingRules(sr *v2.HPAScalingRules) *model.HPAScalingRules {
	if sr == nil {
		return &model.HPAScalingRules{}
	}

	r := model.HPAScalingRules{}
	if sr.StabilizationWindowSeconds != nil {
		r.StabilizationWindowSeconds = *sr.StabilizationWindowSeconds
	}
	if sr.SelectPolicy != nil {
		r.SelectPolicy = string(*sr.SelectPolicy)
	}
	policies := []*model.HPAScalingPolicy{}
	for _, p := range sr.Policies {
		p2 := &model.HPAScalingPolicy{
			Type:          string(p.Type),
			Value:         p.Value,
			PeriodSeconds: p.PeriodSeconds,
		}
		policies = append(policies, p2)
	}
	r.Policies = policies
	return &r
}

// extractHorizontalPodAutoscalerStatus converts Kubernetes HorizontalPodAutoscalerStatus to our protobuf model
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L353
func extractHorizontalPodAutoscalerStatus(s *v2.HorizontalPodAutoscalerStatus) *model.HorizontalPodAutoscalerStatus {
	if s == nil {
		return &model.HorizontalPodAutoscalerStatus{}
	}

	status := model.HorizontalPodAutoscalerStatus{
		CurrentReplicas: s.CurrentReplicas,
		DesiredReplicas: s.DesiredReplicas,
		CurrentMetrics:  extractMetricStatus(s.CurrentMetrics),
	}
	if s.ObservedGeneration != nil {
		status.ObservedGeneration = *s.ObservedGeneration
	}
	if s.LastScaleTime != nil {
		status.LastScaleTime = s.LastScaleTime.Unix()
	}

	return &status
}

// extractMetricStatus converts Kubernetes MetricStatus to our custom HorizontalPodAutoscalerMetricStatus model
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L519
func extractMetricStatus(ms []v2.MetricStatus) []*model.HorizontalPodAutoscalerMetricStatus {
	if ms == nil {
		return []*model.HorizontalPodAutoscalerMetricStatus{}
	}

	metrics := []*model.HorizontalPodAutoscalerMetricStatus{}
	for _, m := range ms {
		metric := &model.HorizontalPodAutoscalerMetricStatus{
			Type: string(m.Type),
		}
		switch m.Type {
		case v2.ObjectMetricSourceType:
			metric.Object = extractObjectMetricStatus(m.Object)
		case v2.PodsMetricSourceType:
			metric.Pods = extractPodsMetricStatus(m.Pods)
		case v2.ResourceMetricSourceType:
			metric.Resource = extractResourceMetricStatus(m.Resource)
		case v2.ContainerResourceMetricSourceType:
			metric.ContainerResource = extractContainerResourceMetricStatus(m.ContainerResource)
		case v2.ExternalMetricSourceType:
			metric.External = extractExternalMetricStatus(m.External)
		}
		metrics = append(metrics, metric)
	}
	return metrics
}

// extractObjectMetricStatus converts Kubernetes ObjectMetricStatus to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L465
func extractObjectMetricStatus(s *v2.ObjectMetricStatus) *model.ObjectMetricStatus {
	if s == nil {
		return &model.ObjectMetricStatus{}
	}
	m := model.ObjectMetricStatus{
		DescribedObject: &model.ObjectReference{
			Kind:       s.DescribedObject.Kind,
			Name:       s.DescribedObject.Name,
			ApiVersion: s.DescribedObject.APIVersion,
		},
		Metric: extractMetricIdentifier(s.Metric),
	}
	// ObjectMetric only supports value and AverageValue
	if s.Current.Value != nil {
		m.Current = s.Current.Value.ToDec().MilliValue()
	}
	if s.Current.AverageValue != nil {
		m.Current = s.Current.AverageValue.ToDec().MilliValue()
	}
	return &m
}

// extractPodsMetricStatus converts Kubernetes PodsMetricStatus to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L476
func extractPodsMetricStatus(s *v2.PodsMetricStatus) *model.PodsMetricStatus {
	if s == nil {
		return &model.PodsMetricStatus{}
	}
	m := model.PodsMetricStatus{
		Metric: extractMetricIdentifier(s.Metric),
	}

	// Only AverageValue is supported for PodsMetric
	if s.Current.AverageValue != nil {
		m.Current = s.Current.AverageValue.ToDec().MilliValue()
	}
	return &m
}

// extractResourceMetricStatus converts Kubernetes ResourceMetricStatus to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L488
func extractResourceMetricStatus(s *v2.ResourceMetricStatus) *model.ResourceMetricStatus {
	if s == nil {
		return &model.ResourceMetricStatus{}
	}
	m := model.ResourceMetricStatus{
		ResourceName: s.Name.String(),
	}
	// Only AverageValue and AverageUtilization is supported for ResourceMetric
	if s.Current.AverageValue != nil {
		m.Current = s.Current.AverageValue.ToDec().MilliValue()
	}
	if s.Current.AverageUtilization != nil {
		m.Current = int64(*s.Current.AverageUtilization)
	}
	return &m
}

// extractContainerResourceMetricStatus converts Kubernetes ContainerResourceMetricStatus to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L500
func extractContainerResourceMetricStatus(s *v2.ContainerResourceMetricStatus) *model.ContainerResourceMetricStatus {
	if s == nil {
		return &model.ContainerResourceMetricStatus{}
	}
	m := model.ContainerResourceMetricStatus{
		ResourceName: s.Name.String(),
		Container:    s.Container,
	}
	if s.Current.AverageUtilization != nil {
		m.Current = int64(*s.Current.AverageUtilization)
	}
	return &m
}

// extractExternalMetricStatus converts Kubernetes ExternalMetricStatus to our custom one
// https://github.com/kubernetes/api/blob/v0.23.15/autoscaling/v2/types.go#L511
func extractExternalMetricStatus(s *v2.ExternalMetricStatus) *model.ExternalMetricStatus {
	if s == nil {
		return &model.ExternalMetricStatus{}
	}
	m := model.ExternalMetricStatus{
		Metric: extractMetricIdentifier(s.Metric),
	}
	// Only Value and AverageValue is supported for ResourceMetric
	if s.Current.Value != nil {
		m.Current = s.Current.Value.ToDec().MilliValue()
	}
	if s.Current.AverageValue != nil {
		m.Current = s.Current.AverageValue.ToDec().MilliValue()
	}
	return &m
}

// extractHorizontalPodAutoscalerConditions iterates over hpa conditions and returns:
// - the payload representation of those conditions
// - the list of tags that will enable pod filtering by condition
func extractHorizontalPodAutoscalerConditions(p *v2.HorizontalPodAutoscaler) ([]*model.HorizontalPodAutoscalerCondition, []string) {
	conditions := make([]*model.HorizontalPodAutoscalerCondition, 0, len(p.Status.Conditions))
	conditionTags := make([]string, 0, len(p.Status.Conditions))

	for _, condition := range p.Status.Conditions {
		c := &model.HorizontalPodAutoscalerCondition{
			Message:         condition.Message,
			Reason:          condition.Reason,
			ConditionStatus: string(condition.Status),
			ConditionType:   string(condition.Type),
		}
		if !condition.LastTransitionTime.IsZero() {
			c.LastTransitionTime = condition.LastTransitionTime.Unix()
		}

		conditions = append(conditions, c)

		conditionTag := createConditionTag(string(condition.Type), string(condition.Status))
		conditionTags = append(conditionTags, conditionTag)
	}

	return conditions, conditionTags
}
