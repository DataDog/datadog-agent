// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/metricsstore"
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
		setupFunc      func() *model.PodAutoscalerInternal
		expectedCount  int
		validateMetric func(t *testing.T, metrics metricsstore.StructuredMetrics)
	}{
		{
			name: "horizontal scaling metrics",
			setupFunc: func() *model.PodAutoscalerInternal {
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
				return &internal
			},
			expectedCount: 2, // horizontal_scaling_received_replicas + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_received_replicas" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
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
			setupFunc: func() *model.PodAutoscalerInternal {
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
				return &internal
			},
			expectedCount: 5, // 2 requests + 2 limits + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var requestsCount, limitsCount int
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_scaling_received_requests" {
						requestsCount++
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Contains(t, m.Tags, "container_name:app-container")
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
					if m.Name == metricPrefix+".vertical_scaling_received_limits" {
						limitsCount++
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
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
			setupFunc: func() *model.PodAutoscalerInternal {
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
					UpstreamCR: crd,
				}.Build()
				return &internal
			},
			expectedCount: 3, // 2 conditions + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var activeFound, readyFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".autoscaler_conditions" {
						if slices.Contains(m.Tags, "type:Active") {
							activeFound = true
							assert.Equal(t, 1.0, m.Value, "Active condition should be 1.0")
						}
						if slices.Contains(m.Tags, "type:Ready") {
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
			name: "horizontal scaling action success only",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					ScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
						},
					},
					HorizontalLastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
						{FromReplicas: 3, ToReplicas: 5, Time: metav1.Now()},
					},
					HorizontalActionSuccessCount: 4,
				}.Build()
				return &internal
			},
			expectedCount: 3, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(ok) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var appliedFound, actionsFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_applied_replicas" {
						appliedFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 5.0, m.Value)
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
					if m.Name == metricPrefix+".horizontal_scaling_actions" {
						actionsFound = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 4.0, m.Value)
						assert.Contains(t, m.Tags, "status:ok")
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
				}
				assert.True(t, appliedFound, "horizontal_scaling_applied_replicas metric not found")
				assert.True(t, actionsFound, "horizontal_scaling_actions metric not found")
			},
		},
		{
			name: "horizontal scaling action error only",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					HorizontalActionErrorCount: 2,
				}.Build()
				return &internal
			},
			expectedCount: 2, // horizontal_scaling_actions(error) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var actionsFound bool
				for _, m := range metrics {
					assert.NotEqual(t, metricPrefix+".horizontal_scaling_applied_replicas", m.Name,
						"horizontal_scaling_applied_replicas should not be emitted with no actions list")
					if m.Name == metricPrefix+".horizontal_scaling_actions" {
						actionsFound = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 2.0, m.Value)
						assert.Contains(t, m.Tags, "status:error")
					}
				}
				assert.True(t, actionsFound, "horizontal_scaling_actions metric not found")
			},
		},
		{
			name: "horizontal scaling applied_replicas uses last action when multiple actions exist",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					HorizontalLastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
						{FromReplicas: 1, ToReplicas: 3, Time: metav1.Now()},
						{FromReplicas: 3, ToReplicas: 7, Time: metav1.Now()},
					},
					HorizontalActionSuccessCount: 2,
				}.Build()
				return &internal
			},
			expectedCount: 3, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(ok) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_applied_replicas" {
						assert.Equal(t, 7.0, m.Value, "should use ToReplicas from last action")
						assert.Contains(t, m.Tags, "source:")
					}
				}
			},
		},
		{
			name: "horizontal scaling both success and error actions",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					HorizontalLastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
						{FromReplicas: 3, ToReplicas: 5, Time: metav1.Now()},
					},
					HorizontalActionSuccessCount: 6,
					HorizontalActionErrorCount:   1,
				}.Build()
				return &internal
			},
			expectedCount: 4, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error) + horizontal_scaling_actions(ok) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var appliedFound, okFound, errorFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_applied_replicas" {
						appliedFound = true
						assert.Equal(t, 5.0, m.Value)
					}
					if m.Name == metricPrefix+".horizontal_scaling_actions" {
						if slices.Contains(m.Tags, "status:ok") {
							okFound = true
							assert.Equal(t, 6.0, m.Value)
						}
						if slices.Contains(m.Tags, "status:error") {
							errorFound = true
							assert.Equal(t, 1.0, m.Value)
						}
					}
				}
				assert.True(t, appliedFound, "horizontal_scaling_applied_replicas metric not found")
				assert.True(t, okFound, "horizontal_scaling_actions status:ok metric not found")
				assert.True(t, errorFound, "horizontal_scaling_actions status:error metric not found")
			},
		},
		{
			name: "vertical rollout triggered success only",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					VerticalActionSuccessCount: 3,
				}.Build()
				return &internal
			},
			expectedCount: 2, // vertical_rollout_triggered(ok) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_rollout_triggered" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 3.0, m.Value)
						assert.Contains(t, m.Tags, "status:ok")
					}
				}
				assert.True(t, found, "vertical_rollout_triggered metric not found")
			},
		},
		{
			name: "vertical rollout triggered error only",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					VerticalActionErrorCount: 2,
				}.Build()
				return &internal
			},
			expectedCount: 2, // vertical_rollout_triggered(error) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_rollout_triggered" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 2.0, m.Value)
						assert.Contains(t, m.Tags, "status:error")
					}
				}
				assert.True(t, found, "vertical_rollout_triggered metric not found")
			},
		},
		{
			name: "vertical rollout triggered both success and error",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					VerticalActionSuccessCount: 5,
					VerticalActionErrorCount:   1,
				}.Build()
				return &internal
			},
			expectedCount: 3, // vertical_rollout_triggered(error) + vertical_rollout_triggered(ok) + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var foundOk, foundError bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_rollout_triggered" {
						if slices.Contains(m.Tags, "status:ok") {
							foundOk = true
							assert.Equal(t, 5.0, m.Value)
						}
						if slices.Contains(m.Tags, "status:error") {
							foundError = true
							assert.Equal(t, 1.0, m.Value)
						}
					}
				}
				assert.True(t, foundOk, "vertical_rollout_triggered status:ok metric not found")
				assert.True(t, foundError, "vertical_rollout_triggered status:error metric not found")
			},
		},
		{
			name: "local fallback enabled",
			setupFunc: func() *model.PodAutoscalerInternal {
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
					// ScalingValues holds the active source; LocalValueSource here means fallback is active
					ScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Replicas: 5,
							Source:   datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 2, // horizontal_scaling_received_replicas + local_fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
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
