// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	v2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
)

func TestBaseAutoscalerTags(t *testing.T) {
	tags := baseAutoscalerTags("test-ns", "test-target", "test-autoscaler")

	assert.Len(t, tags, 4)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, le.JoinLeaderLabel+":"+le.JoinLeaderValue)
}

func TestAutoscalerTagsWithSource(t *testing.T) {
	tags := autoscalerTagsWithSource("test-ns", "test-target", "test-autoscaler", "Autoscaling")

	assert.Len(t, tags, 5)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, "source:Autoscaling")
	assert.Contains(t, tags, le.JoinLeaderLabel+":"+le.JoinLeaderValue)
}

func TestAutoscalerTagsWithContainer(t *testing.T) {
	tags := autoscalerTagsWithContainer("test-ns", "test-target", "test-autoscaler", "Autoscaling", "app-container", "cpu")

	assert.Len(t, tags, 7)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, "source:Autoscaling")
	assert.Contains(t, tags, "container_name:app-container")
	assert.Contains(t, tags, "resource_name:cpu")
	assert.Contains(t, tags, le.JoinLeaderLabel+":"+le.JoinLeaderValue)
}

func TestConditionTags(t *testing.T) {
	tags := conditionTags("test-ns", "test-target", "test-autoscaler", "Active")

	assert.Len(t, tags, 5)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, "type:Active")
	assert.Contains(t, tags, le.JoinLeaderLabel+":"+le.JoinLeaderValue)
}

func TestGeneratePodAutoscalerMetrics(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func() *PodAutoscalerMetricsObject
		expectedCount  int
		validateMetric func(t *testing.T, metrics StructuredMetrics)
	}{
		{
			name: "horizontal scaling metrics",
			setupFunc: func() *PodAutoscalerMetricsObject {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					MainScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Replicas: 5,
							Source:   datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						},
					},
				}.Build()

				return &PodAutoscalerMetricsObject{
					CRD:      crd,
					Internal: &internal,
				}
			},
			expectedCount: 2, // horizontal_scaling_received_replicas + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics StructuredMetrics) {
				// Find horizontal scaling metric
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_received_replicas" {
						found = true
						assert.Equal(t, MetricTypeGauge, m.Type)
						assert.Equal(t, 5.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "target_name:test-deployment")
						assert.Contains(t, m.Tags, "autoscaler_name:test-dpa")
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
				}
				assert.True(t, found, "horizontal_scaling_received_replicas metric not found")
			},
		},
		{
			name: "vertical scaling metrics",
			setupFunc: func() *PodAutoscalerMetricsObject {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					MainScalingValues: model.ScalingValues{
						Vertical: &model.VerticalScalingValues{
							Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
							ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
								{
									Name: "app-container",
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("256Mi"),
									},
									Limits: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("200m"),
										corev1.ResourceMemory: resource.MustParse("512Mi"),
									},
								},
							},
						},
					},
				}.Build()

				return &PodAutoscalerMetricsObject{
					CRD:      crd,
					Internal: &internal,
				}
			},
			expectedCount: 5, // 2 requests + 2 limits + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics StructuredMetrics) {
				var requestsCount, limitsCount int
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_scaling_received_requests" {
						requestsCount++
						assert.Equal(t, MetricTypeGauge, m.Type)
						assert.Contains(t, m.Tags, "container_name:app-container")
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
					if m.Name == metricPrefix+".vertical_scaling_received_limits" {
						limitsCount++
						assert.Equal(t, MetricTypeGauge, m.Type)
						assert.Contains(t, m.Tags, "container_name:app-container")
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
				}
				assert.Equal(t, 2, requestsCount, "expected 2 request metrics (cpu + memory)")
				assert.Equal(t, 2, limitsCount, "expected 2 limit metrics (cpu + memory)")
			},
		},
		{
			name: "autoscaler conditions",
			setupFunc: func() *PodAutoscalerMetricsObject {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Status: datadoghqcommon.DatadogPodAutoscalerStatus{
						Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
							{
								Type:   "Active",
								Status: corev1.ConditionTrue,
							},
							{
								Type:   "Ready",
								Status: corev1.ConditionFalse,
							},
						},
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
				}.Build()

				return &PodAutoscalerMetricsObject{
					CRD:      crd,
					Internal: &internal,
				}
			},
			expectedCount: 3, // 2 conditions + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics StructuredMetrics) {
				var activeFound, readyFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".autoscaler_conditions" {
						if contains(m.Tags, "type:Active") {
							activeFound = true
							assert.Equal(t, 1.0, m.Value, "Active condition should be 1.0")
						}
						if contains(m.Tags, "type:Ready") {
							readyFound = true
							assert.Equal(t, 0.0, m.Value, "Ready condition should be 0.0")
						}
					}
				}
				assert.True(t, activeFound, "Active condition metric not found")
				assert.True(t, readyFound, "Ready condition metric not found")
			},
		},
		{
			name: "local fallback enabled",
			setupFunc: func() *PodAutoscalerMetricsObject {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					MainScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Replicas: 5,
							Source:   datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						},
					},
				}.Build()

				return &PodAutoscalerMetricsObject{
					CRD:      crd,
					Internal: &internal,
				}
			},
			expectedCount: 2, // horizontal_scaling_received_replicas + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".local_fallback_enabled" {
						found = true
						assert.Equal(t, 1.0, m.Value, "local fallback should be enabled (1.0)")
					}
				}
				assert.True(t, found, "local_fallback_enabled metric not found")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setupFunc()
			metrics := GeneratePodAutoscalerMetrics(obj)

			require.NotNil(t, metrics)
			assert.Equal(t, tt.expectedCount, len(metrics), "unexpected number of metrics")

			if tt.validateMetric != nil {
				tt.validateMetric(t, metrics)
			}
		})
	}
}

func TestGeneratePodAutoscalerMetrics_InvalidObject(t *testing.T) {
	// Pass wrong type
	metrics := GeneratePodAutoscalerMetrics("invalid object")
	assert.Nil(t, metrics)
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
