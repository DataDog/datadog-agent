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
	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"

	model "github.com/DataDog/agent-payload/v5/process"
)

func TestExtractVerticalPodAutoscaler(t *testing.T) {
	exampleTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	mode := v1.ContainerScalingModeAuto
	updateMode := v1.UpdateModeOff
	controlledValues := v1.ContainerControlledValuesRequestsAndLimits

	tests := map[string]struct {
		input    v1.VerticalPodAutoscaler
		expected model.VerticalPodAutoscaler
	}{
		"standard": {
			input: v1.VerticalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "VPATest",
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
				Spec: v1.VerticalPodAutoscalerSpec{
					TargetRef: &autoscaling.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "My Service",
					},
					UpdatePolicy: &v1.PodUpdatePolicy{
						UpdateMode: &updateMode,
					},
					ResourcePolicy: &v1.PodResourcePolicy{
						ContainerPolicies: []v1.ContainerResourcePolicy{
							{
								ContainerName: "TestContainer",
								Mode:          &mode,
								MinAllowed: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: resource.MustParse("12345"),
								},
								MaxAllowed: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceMemory: resource.MustParse("6789"),
								},
								ControlledResources: &[]corev1.ResourceName{
									corev1.ResourceCPU,
								},
								ControlledValues: &controlledValues,
							},
						},
					},
					Recommenders: []*v1.VerticalPodAutoscalerRecommenderSelector{
						{
							Name: "Test",
						},
					},
				},
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: &v1.RecommendedPodResources{
						ContainerRecommendations: []v1.RecommendedContainerResources{
							{
								ContainerName: "TestContainer",
								Target: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: resource.MustParse("2"),
								},
								LowerBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: resource.MustParse("1"),
								},
								UpperBound: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: resource.MustParse("3"),
								},
								UncappedTarget: map[corev1.ResourceName]resource.Quantity{
									corev1.ResourceCPU: resource.MustParse("4"),
								},
							},
						},
					},
					Conditions: []v1.VerticalPodAutoscalerCondition{
						{
							Type:               v1.RecommendationProvided,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
						},
						{
							Type:               v1.NoPodsMatched,
							Status:             corev1.ConditionTrue,
							LastTransitionTime: exampleTime,
							Reason:             "NoPodsMatched",
							Message:            "No pods match this VPA object",
						},
					},
				},
			},
			expected: model.VerticalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "VPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.VerticalPodAutoscalerSpec{
					Target: &model.VerticalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "My Service",
					},
					UpdateMode: string(updateMode),
					ResourcePolicies: []*model.ContainerResourcePolicy{
						{
							ContainerName: "TestContainer",
							Mode:          string(mode),
							MinAllowed: &model.ResourceList{
								MetricValues: map[string]float64{
									"cpu": float64(12345),
								},
							},
							MaxAllowed: &model.ResourceList{
								MetricValues: map[string]float64{
									"memory": float64(6789),
								},
							},
							ControlledResource: []string{"cpu"},
							ControlledValues:   string(controlledValues),
						},
					},
				},
				Status: &model.VerticalPodAutoscalerStatus{
					LastRecommendedDate: exampleTime.Unix(),
					Recommendations: []*model.ContainerRecommendation{
						{
							ContainerName: "TestContainer",
							Target: &model.ResourceList{
								MetricValues: map[string]float64{
									"cpu": float64(2),
								},
							},
							LowerBound: &model.ResourceList{
								MetricValues: map[string]float64{
									"cpu": float64(1),
								},
							},
							UpperBound: &model.ResourceList{
								MetricValues: map[string]float64{
									"cpu": float64(3),
								},
							},
							UncappedTarget: &model.ResourceList{
								MetricValues: map[string]float64{
									"cpu": float64(4),
								},
							},
						},
					},
					Conditions: []*model.VPACondition{
						{
							ConditionType:      "RecommendationProvided",
							ConditionStatus:    "True",
							LastTransitionTime: exampleTime.Unix(),
						},
						{
							ConditionType:      "NoPodsMatched",
							ConditionStatus:    "True",
							LastTransitionTime: exampleTime.Unix(),
							Reason:             "NoPodsMatched",
							Message:            "No pods match this VPA object",
						},
					},
				},
			},
		},
		"minimum-required": {
			input: v1.VerticalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "VPATest",
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
				Spec: v1.VerticalPodAutoscalerSpec{
					TargetRef: &autoscaling.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "My Service",
					},
					UpdatePolicy:   nil, // +optional
					ResourcePolicy: nil, // +optional expanded in second unit test case
					Recommenders:   nil, // +optional
				},
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: nil, // +optional
				},
			},
			expected: model.VerticalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "VPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.VerticalPodAutoscalerSpec{
					Target: &model.VerticalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "My Service",
					},
					ResourcePolicies: []*model.ContainerResourcePolicy{},
				},
				Status: &model.VerticalPodAutoscalerStatus{
					Conditions: []*model.VPACondition{},
				},
			},
		},
		"minimum-required-inner-resourcepolicy": {
			input: v1.VerticalPodAutoscaler{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "VPATest",
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
				Spec: v1.VerticalPodAutoscalerSpec{
					TargetRef: &autoscaling.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "My Service",
					},
					UpdatePolicy: nil, // +optional
					ResourcePolicy: &v1.PodResourcePolicy{
						ContainerPolicies: []v1.ContainerResourcePolicy{
							{
								ContainerName: "TestContainer",
								Mode:          nil, // +optional
								MinAllowed:    nil, // +optional
								MaxAllowed:    nil, // +optional
								ControlledResources: &[]corev1.ResourceName{
									corev1.ResourceCPU,
								},
								ControlledValues: nil, // +optional
							},
						},
					},
					Recommenders: nil, // +optional
				},
				Status: v1.VerticalPodAutoscalerStatus{
					Recommendation: nil, // +optional
				},
			},
			expected: model.VerticalPodAutoscaler{
				Metadata: &model.Metadata{
					Name:              "VPATest",
					Namespace:         "Namespace",
					Uid:               "326331f4-77e2-11ed-a1eb-0242ac120002",
					CreationTimestamp: exampleTime.Unix(),
					DeletionTimestamp: exampleTime.Unix(),
					Labels:            []string{"app:my-app"},
					Annotations:       []string{"annotation:my-annotation"},
					Finalizers:        []string{"final", "izers"},
				},
				Spec: &model.VerticalPodAutoscalerSpec{
					Target: &model.VerticalPodAutoscalerTarget{
						Kind: "Deployment",
						Name: "My Service",
					},
					ResourcePolicies: []*model.ContainerResourcePolicy{
						{
							ContainerName: "TestContainer",
							MinAllowed: &model.ResourceList{
								MetricValues: map[string]float64{},
							},
							MaxAllowed: &model.ResourceList{
								MetricValues: map[string]float64{},
							},
							ControlledResource: []string{"cpu"},
						},
					},
				},
				Status: &model.VerticalPodAutoscalerStatus{
					Recommendations: nil,
					Conditions:      []*model.VPACondition{},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, &tc.expected, ExtractVerticalPodAutoscaler(&tc.input))
		})
	}
}
