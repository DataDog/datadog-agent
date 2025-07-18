// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"

	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
)

func TestHorizontalPodAutoscalerCollector(t *testing.T) {
	exampleTime := metav1.NewTime(CreateTestTime().Time)
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

	horizontalPodAutoscaler := &v2.HorizontalPodAutoscaler{
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
			Finalizers:      []string{"final", "izers"},
			ResourceVersion: "1210",
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
	}

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewHorizontalPodAutoscalerCollector(metadataAsTags)

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{horizontalPodAutoscaler},
		ExpectedMetadataType:       &model.CollectorHorizontalPodAutoscaler{},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		ExpectedMetadataMessages:   1,
		ExpectedManifestMessages:   1,
	}

	RunCollectorTest(t, config, collector)
}
