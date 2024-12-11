// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestExtractHorizontalPodAutoscaler(t *testing.T) {
	exampleTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	minReplicas := new(int32)
	*minReplicas = 1
	resourceQuantity := resource.MustParse("5332m")
	window := new(int32)
	*window = 10
	selectPolicy := v2.MaxChangePolicySelect
	oberservedGeneration := new(int64)
	*oberservedGeneration = 1
	averageUtilization := new(int32)
	*averageUtilization = 60

	tests := map[string]struct {
		input    v2.HorizontalPodAutoscaler
		expected model.HorizontalPodAutoscaler
	}{
		"standard": {
			input: v2.HorizontalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "HPATest",
					Namespace:         "Namespace",
					UID:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime,
					DeletionTimestamp: &exampleTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					Finalizers: []string{"final", "izers"},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "agent",
					},
					MinReplicas: minReplicas,
					MaxReplicas: 3,
					Metrics: []v2.MetricSpec{
						{
							Type: "Object",
							Object: &v2.ObjectMetricSource{
								DescribedObject: v2.CrossVersionObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									APIVersion: "v1",
								},
								Target: v2.MetricTarget{
									Type:  v2.ValueMetricType,
									Value: &resourceQuantity,
								},
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &v2.PodsMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Target: v2.MetricTarget{
									Type:               v2.UtilizationMetricType,
									AverageUtilization: averageUtilization,
								},
							},
						},
						{
							Type: "Resource",
							Resource: &v2.ResourceMetricSource{
								Name: "CPU",
								Target: v2.MetricTarget{
									Type:               v2.UtilizationMetricType,
									AverageUtilization: averageUtilization,
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricSource{
								Name: "CPU",
								Target: v2.MetricTarget{
									Type:               v2.UtilizationMetricType,
									AverageUtilization: averageUtilization,
								},
								Container: "agent",
							},
						},
						{
							Type: "External",
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Target: v2.MetricTarget{
									Type:               v2.UtilizationMetricType,
									AverageUtilization: averageUtilization,
								},
							},
						},
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &v2.HPAScalingRules{
							StabilizationWindowSeconds: window,
							SelectPolicy:               &selectPolicy,
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PodsScalingPolicy,
									Value:         4,
									PeriodSeconds: 60,
								},
							},
						},
					},
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					ObservedGeneration: oberservedGeneration,
					LastScaleTime:      &exampleTime,
					CurrentReplicas:    1,
					DesiredReplicas:    2,
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "Object",
							Object: &v2.ObjectMetricStatus{
								DescribedObject: v2.CrossVersionObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									APIVersion: "v1",
								},
								Current: v2.MetricValueStatus{
									Value: &resourceQuantity,
								},
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &v2.PodsMetricStatus{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Current: v2.MetricValueStatus{
									AverageValue: &resourceQuantity,
								},
							},
						},
						{
							Type: "Resource",
							Resource: &v2.ResourceMetricStatus{
								Name: "CPU",
								Current: v2.MetricValueStatus{
									AverageUtilization: averageUtilization,
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &v2.ContainerResourceMetricStatus{
								Name: "CPU",
								Current: v2.MetricValueStatus{
									AverageUtilization: averageUtilization,
								},
								Container: "agent",
							},
						},
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Current: v2.MetricValueStatus{
									Value: &resourceQuantity,
								},
							},
						},
					},
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.AbleToScale,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
							Reason:             "ReadyForNewScale",
							Message:            "recommended size matches current size",
						},
						{
							Type:               v2.ScalingActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
							Reason:             "ValidMetricFound",
							Message:            "the HPA was able to successfully calculate a replica count from external metric",
						},
						{
							Type:               v2.ScalingLimited,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: exampleTime,
							Reason:             "DesiredWithinRange",
							Message:            "the desired count is within the acceptable range",
						},
					},
				},
			},
			expected: model.HorizontalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "HPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.HorizontalPodAutoscalerSpec{
					Target: &model.HorizontalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "agent",
					},
					MinReplicas: 1,
					MaxReplicas: 3,
					Metrics: []*model.HorizontalPodAutoscalerMetricSpec{
						{
							Type: "Object",
							Object: &model.ObjectMetricSource{
								DescribedObject: &model.ObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									ApiVersion: "v1",
								},
								Target: &model.MetricTarget{
									Type:  "Value",
									Value: 5332,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &model.PodsMetricSource{
								Target: &model.MetricTarget{
									Type:  "Utilization",
									Value: 60,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Resource",
							Resource: &model.ResourceMetricSource{
								ResourceName: "CPU",
								Target: &model.MetricTarget{
									Type:  "Utilization",
									Value: 60,
								},
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &model.ContainerResourceMetricSource{
								ResourceName: "CPU",
								Target: &model.MetricTarget{
									Type:  "Utilization",
									Value: 60,
								},
								Container: "agent",
							},
						},
						{
							Type: "External",
							External: &model.ExternalMetricSource{
								Target: &model.MetricTarget{
									Type:  "Utilization",
									Value: 60,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
					},
					Behavior: &model.HorizontalPodAutoscalerBehavior{
						ScaleUp: &model.HPAScalingRules{
							StabilizationWindowSeconds: 10,
							SelectPolicy:               "Max",
							Policies: []*model.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 60,
								},
							},
						},
					},
				},
				Status: &model.HorizontalPodAutoscalerStatus{
					ObservedGeneration: 1,
					LastScaleTime:      exampleTime.Unix(),
					CurrentReplicas:    1,
					DesiredReplicas:    2,
					CurrentMetrics: []*model.HorizontalPodAutoscalerMetricStatus{
						{
							Type: "Object",
							Object: &model.ObjectMetricStatus{
								DescribedObject: &model.ObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									ApiVersion: "v1",
								},
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &model.PodsMetricStatus{
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Resource",
							Resource: &model.ResourceMetricStatus{
								ResourceName: "CPU",
								Current:      60,
							},
						},
						{
							Type: "ContainerResource",
							ContainerResource: &model.ContainerResourceMetricStatus{
								ResourceName: "CPU",
								Current:      60,
								Container:    "agent",
							},
						},
						{
							Type: "External",
							External: &model.ExternalMetricStatus{
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
					},
				},
				Conditions: []*model.HorizontalPodAutoscalerCondition{
					{
						ConditionType:      "AbleToScale",
						ConditionStatus:    "True",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "ReadyForNewScale",
						Message:            "recommended size matches current size",
					},
					{
						ConditionType:      "ScalingActive",
						ConditionStatus:    "True",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "ValidMetricFound",
						Message:            "the HPA was able to successfully calculate a replica count from external metric",
					},
					{
						ConditionType:      "ScalingLimited",
						ConditionStatus:    "False",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "DesiredWithinRange",
						Message:            "the desired count is within the acceptable range",
					},
				},
				Tags: []string{
					"kube_condition_abletoscale:true",
					"kube_condition_scalingactive:true",
					"kube_condition_scalinglimited:false",
				},
			},
		},
		"minimum-required": {
			input: v2.HorizontalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "HPATest",
					Namespace:         "Namespace",
					UID:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime,
					DeletionTimestamp: &exampleTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					Finalizers: []string{"final", "izers"},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "agent",
					},
					MinReplicas: nil,
					Metrics: []v2.MetricSpec{
						{
							Type: "External",
							External: &v2.ExternalMetricSource{
								Metric: v2.MetricIdentifier{
									Name:     "CPU",
									Selector: nil,
								},
								Target: v2.MetricTarget{
									Type:               v2.UtilizationMetricType,
									AverageUtilization: averageUtilization,
								},
							},
						},
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleUp:   nil,
						ScaleDown: nil,
					},
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					ObservedGeneration: nil,
					LastScaleTime:      nil,
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "External",
							External: &v2.ExternalMetricStatus{
								Metric: v2.MetricIdentifier{
									Name:     "CPU",
									Selector: nil,
								},
								Current: v2.MetricValueStatus{
									AverageValue: &resourceQuantity,
								},
							},
						},
					},
					Conditions: []v2.HorizontalPodAutoscalerCondition{},
				},
			},
			expected: model.HorizontalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "HPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.HorizontalPodAutoscalerSpec{
					Target: &model.HorizontalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "agent",
					},
					Metrics: []*model.HorizontalPodAutoscalerMetricSpec{
						{
							Type: "External",
							External: &model.ExternalMetricSource{
								Target: &model.MetricTarget{
									Type:  "Utilization",
									Value: 60,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
								},
							},
						},
					},
					Behavior: &model.HorizontalPodAutoscalerBehavior{
						ScaleUp:   nil,
						ScaleDown: nil,
					},
				},
				Status: &model.HorizontalPodAutoscalerStatus{
					CurrentMetrics: []*model.HorizontalPodAutoscalerMetricStatus{
						{
							Type: "External",
							External: &model.ExternalMetricStatus{
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
								},
							},
						},
					},
				},
				Conditions: nil,
			},
		},
		"mixed": {
			input: v2.HorizontalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "HPATest",
					Namespace:         "Namespace",
					UID:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime,
					DeletionTimestamp: &exampleTime,
					Labels: map[string]string{
						"app": "my-app",
					},
					Annotations: map[string]string{
						"annotation": "my-annotation",
					},
					Finalizers: []string{"final", "izers"},
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: v2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "agent",
					},
					MinReplicas: minReplicas,
					MaxReplicas: 3,
					Metrics: []v2.MetricSpec{
						{
							Type: "Object",
							Object: &v2.ObjectMetricSource{
								DescribedObject: v2.CrossVersionObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									APIVersion: "v1",
								},
								Target: v2.MetricTarget{
									Type:         v2.AverageValueMetricType,
									AverageValue: &resourceQuantity,
								},
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &v2.PodsMetricSource{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Target: v2.MetricTarget{
									Type:         v2.AverageValueMetricType,
									AverageValue: &resourceQuantity,
								},
							},
						},
					},
					Behavior: &v2.HorizontalPodAutoscalerBehavior{
						ScaleUp: &v2.HPAScalingRules{
							StabilizationWindowSeconds: window,
							SelectPolicy:               nil,
							Policies: []v2.HPAScalingPolicy{
								{
									Type:          v2.PodsScalingPolicy,
									Value:         4,
									PeriodSeconds: 60,
								},
							},
						},
					},
				},
				Status: v2.HorizontalPodAutoscalerStatus{
					ObservedGeneration: nil,
					LastScaleTime:      &exampleTime,
					CurrentReplicas:    1,
					DesiredReplicas:    2,
					CurrentMetrics: []v2.MetricStatus{
						{
							Type: "Object",
							Object: &v2.ObjectMetricStatus{
								DescribedObject: v2.CrossVersionObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									APIVersion: "v1",
								},
								Current: v2.MetricValueStatus{
									Value: &resourceQuantity,
								},
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &v2.PodsMetricStatus{
								Metric: v2.MetricIdentifier{
									Name: "CPU",
									Selector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"service": "datadog",
										},
									},
								},
								Current: v2.MetricValueStatus{
									AverageValue: &resourceQuantity,
								},
							},
						},
					},
					Conditions: []v2.HorizontalPodAutoscalerCondition{
						{
							Type:               v2.AbleToScale,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
							Reason:             "ReadyForNewScale",
							Message:            "recommended size matches current size",
						},
						{
							Type:               v2.ScalingActive,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
							Reason:             "ValidMetricFound",
							Message:            "the HPA was able to successfully calculate a replica count from external metric",
						},
						{
							Type:               v2.ScalingLimited,
							Status:             corev1.ConditionFalse,
							LastTransitionTime: exampleTime,
							Reason:             "DesiredWithinRange",
							Message:            "the desired count is within the acceptable range",
						},
					},
				},
			},
			expected: model.HorizontalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "HPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.HorizontalPodAutoscalerSpec{
					Target: &model.HorizontalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "agent",
					},
					MinReplicas: 1,
					MaxReplicas: 3,
					Metrics: []*model.HorizontalPodAutoscalerMetricSpec{
						{
							Type: "Object",
							Object: &model.ObjectMetricSource{
								DescribedObject: &model.ObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									ApiVersion: "v1",
								},
								Target: &model.MetricTarget{
									Type:  "AverageValue",
									Value: 5332,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &model.PodsMetricSource{
								Target: &model.MetricTarget{
									Type:  "AverageValue",
									Value: 5332,
								},
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
					},
					Behavior: &model.HorizontalPodAutoscalerBehavior{
						ScaleUp: &model.HPAScalingRules{
							StabilizationWindowSeconds: 10,
							SelectPolicy:               "",
							Policies: []*model.HPAScalingPolicy{
								{
									Type:          "Pods",
									Value:         4,
									PeriodSeconds: 60,
								},
							},
						},
					},
				},
				Status: &model.HorizontalPodAutoscalerStatus{
					LastScaleTime:   exampleTime.Unix(),
					CurrentReplicas: 1,
					DesiredReplicas: 2,
					CurrentMetrics: []*model.HorizontalPodAutoscalerMetricStatus{
						{
							Type: "Object",
							Object: &model.ObjectMetricStatus{
								DescribedObject: &model.ObjectReference{
									Kind:       "Pod",
									Name:       "agent",
									ApiVersion: "v1",
								},
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
						{
							Type: "Pods",
							Pods: &model.PodsMetricStatus{
								Current: 5332,
								Metric: &model.MetricIdentifier{
									Name: "CPU",
									LabelSelector: []*model.LabelSelectorRequirement{
										{
											Key:      "service",
											Operator: "In",
											Values:   []string{"datadog"},
										},
									},
								},
							},
						},
					},
				},
				Conditions: []*model.HorizontalPodAutoscalerCondition{
					{
						ConditionType:      "AbleToScale",
						ConditionStatus:    "True",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "ReadyForNewScale",
						Message:            "recommended size matches current size",
					},
					{
						ConditionType:      "ScalingActive",
						ConditionStatus:    "True",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "ValidMetricFound",
						Message:            "the HPA was able to successfully calculate a replica count from external metric",
					},
					{
						ConditionType:      "ScalingLimited",
						ConditionStatus:    "False",
						LastTransitionTime: exampleTime.Unix(),
						Reason:             "DesiredWithinRange",
						Message:            "the desired count is within the acceptable range",
					},
				},
				Tags: []string{
					"kube_condition_abletoscale:true",
					"kube_condition_scalingactive:true",
					"kube_condition_scalinglimited:false",
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractHorizontalPodAutoscaler(&tc.input))
		})
	}
}
