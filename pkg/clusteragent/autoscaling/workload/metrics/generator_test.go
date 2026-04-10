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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestBaseAutoscalerTags(t *testing.T) {
	internal := model.FakePodAutoscalerInternal{
		Namespace: "test-ns",
		Name:      "test-autoscaler",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name: "test-target",
				Kind: "Deployment",
			},
		},
	}.Build()

	tags := baseAutoscalerTags(&internal)

	assert.Len(t, tags, 7)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "kube_namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "target_kind:deployment")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, "name:test-autoscaler")
	assert.Contains(t, tags, le.IsLeaderLabel+":"+le.JoinLeaderValue)
}

func TestConditionTags(t *testing.T) {
	internal := model.FakePodAutoscalerInternal{
		Namespace: "test-ns",
		Name:      "test-autoscaler",
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name: "test-target",
				Kind: "Deployment",
			},
		},
	}.Build()
	baseTags := baseAutoscalerTags(&internal)
	tags := conditionTags(baseTags, "Active")

	assert.Len(t, tags, 8)
	assert.Contains(t, tags, "namespace:test-ns")
	assert.Contains(t, tags, "kube_namespace:test-ns")
	assert.Contains(t, tags, "target_name:test-target")
	assert.Contains(t, tags, "target_kind:deployment")
	assert.Contains(t, tags, "autoscaler_name:test-autoscaler")
	assert.Contains(t, tags, "name:test-autoscaler")
	assert.Contains(t, tags, "type:Active")
	assert.Contains(t, tags, le.IsLeaderLabel+":"+le.JoinLeaderValue)
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
							Kind: "Deployment",
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
			expectedCount: 6, // horizontal_scaling_received_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_received_replicas" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 5.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "target_name:test-deployment")
						assert.Contains(t, m.Tags, "target_kind:deployment")
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
			expectedCount: 9, // 2 requests + 2 limits + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var requestsCount, limitsCount int
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_scaling_received_requests" {
						requestsCount++
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Contains(t, m.Tags, "source:Autoscaling")
						assert.Contains(t, m.Tags, "kube_container_name:app-container",
							"container name should be in vertical received metrics tags")
					}
					if m.Name == metricPrefix+".vertical_scaling_received_limits" {
						limitsCount++
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Contains(t, m.Tags, "source:Autoscaling")
						assert.Contains(t, m.Tags, "kube_container_name:app-container",
							"container name should be in vertical received metrics tags")
					}
				}
				assert.Equal(t, 2, requestsCount, "expected 2 request metrics (cpu + memory)")
				assert.Equal(t, 2, limitsCount, "expected 2 limit metrics (cpu + memory)")
			},
		},
		{
			name: "extra tags from annotations and UST labels propagated to all metrics",
			setupFunc: func() *model.PodAutoscalerInternal {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
						Annotations: map[string]string{
							"ad.datadoghq.com/tags": `{"team":"autoscaling"}`,
						},
						Labels: map[string]string{
							"tags.datadoghq.com/env": "prod",
						},
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace:  "test-ns",
					Name:       "test-dpa",
					UpstreamCR: crd,
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 5, // horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				for _, m := range metrics {
					assert.Contains(t, m.Tags, "team:autoscaling", "annotation tag should be in metric %s", m.Name)
					assert.Contains(t, m.Tags, "env:prod", "UST label tag should be in metric %s", m.Name)
				}
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
			expectedCount: 7, // 2 conditions + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
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
			expectedCount: 6, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var appliedFound, actionsFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling_applied_replicas" {
						appliedFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 5.0, m.Value)
						assert.Contains(t, m.Tags, "source:Autoscaling")
					}
					if m.Name == metricPrefix+".horizontal_scaling_actions" && slices.Contains(m.Tags, "status:ok") {
						actionsFound = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 4.0, m.Value)
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
			expectedCount: 5, // horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var actionsFound bool
				for _, m := range metrics {
					assert.NotEqual(t, metricPrefix+".horizontal_scaling_applied_replicas", m.Name,
						"horizontal_scaling_applied_replicas should not be emitted with no actions list")
					if m.Name == metricPrefix+".horizontal_scaling_actions" && slices.Contains(m.Tags, "status:error") {
						actionsFound = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 2.0, m.Value)
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
			expectedCount: 6, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
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
			expectedCount: 6, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
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
			expectedCount: 5, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_rollout_triggered" && slices.Contains(m.Tags, "status:ok") {
						found = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 3.0, m.Value)
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
			expectedCount: 5, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".vertical_rollout_triggered" && slices.Contains(m.Tags, "status:error") {
						found = true
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 2.0, m.Value)
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
			expectedCount: 5, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled
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
			name: "local recommender recommended replicas and utilization",
			setupFunc: func() *model.PodAutoscalerInternal {
				utilizationPct := 0.85
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					FallbackScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Replicas:       3,
							Source:         datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
							UtilizationPct: &utilizationPct,
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 7, // local_horizontal_scaling_recommended_replicas + local_horizontal_utilization_pct + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var replicasFound, utilizationFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".local.horizontal_scaling_recommended_replicas" {
						replicasFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 3.0, m.Value)
						assert.Contains(t, m.Tags, "source:Local")
					}
					if m.Name == metricPrefix+".local.horizontal_utilization_pct" {
						utilizationFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.InDelta(t, 0.85, m.Value, 1e-9)
						assert.Contains(t, m.Tags, "source:Local")
					}
				}
				assert.True(t, replicasFound, "local_horizontal_scaling_recommended_replicas metric not found")
				assert.True(t, utilizationFound, "local_horizontal_utilization_pct metric not found")
			},
		},
		{
			name: "local recommender recommended replicas without utilization",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					FallbackScalingValues: model.ScalingValues{
						Horizontal: &model.HorizontalScalingValues{
							Replicas: 2,
							Source:   datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 6, // local_horizontal_scaling_recommended_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var replicasFound bool
				for _, m := range metrics {
					assert.NotEqual(t, metricPrefix+".local.horizontal_utilization_pct", m.Name,
						"local.horizontal_utilization_pct should not be emitted when UtilizationPct is nil")
					if m.Name == metricPrefix+".local.horizontal_scaling_recommended_replicas" {
						replicasFound = true
						assert.Equal(t, 2.0, m.Value)
					}
				}
				assert.True(t, replicasFound, "local_horizontal_scaling_recommended_replicas metric not found")
			},
		},
		{
			name: "horizontal scaling constraints both min and max",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
							MinReplicas: pointer.Ptr(int32(2)),
							MaxReplicas: pointer.Ptr(int32(10)),
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 7, // horizontal_scaling.constraints.{max,min}_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var maxFound, minFound bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".horizontal_scaling.constraints.max_replicas" {
						maxFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 10.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "target_name:test-deployment")
						assert.Contains(t, m.Tags, "autoscaler_name:test-dpa")
					}
					if m.Name == metricPrefix+".horizontal_scaling.constraints.min_replicas" {
						minFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 2.0, m.Value)
					}
				}
				assert.True(t, maxFound, "horizontal_scaling.constraints.max_replicas metric not found")
				assert.True(t, minFound, "horizontal_scaling.constraints.min_replicas metric not found")
			},
		},
		{
			name: "horizontal scaling constraints max only (no min set)",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
							MaxReplicas: pointer.Ptr(int32(20)),
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 6, // horizontal_scaling.constraints.max_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var maxFound bool
				for _, m := range metrics {
					assert.NotEqual(t, metricPrefix+".horizontal_scaling.constraints.min_replicas", m.Name,
						"min_replicas should not be emitted when MinReplicas is nil")
					if m.Name == metricPrefix+".horizontal_scaling.constraints.max_replicas" {
						maxFound = true
						assert.Equal(t, 20.0, m.Value)
					}
				}
				assert.True(t, maxFound, "horizontal_scaling.constraints.max_replicas metric not found")
			},
		},
		{
			name: "vertical scaling container constraints",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
							Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
								{
									Name: "app",
									MinAllowed: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("100m"),
										corev1.ResourceMemory: resource.MustParse("128Mi"),
									},
									MaxAllowed: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2000m"),
										corev1.ResourceMemory: resource.MustParse("1Gi"),
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 9, // 4 container constraints + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var cpuMinFound, memMinFound, cpuMaxFound, memMaxFound bool
				for _, m := range metrics {
					if !slices.Contains(m.Tags, "kube_container_name:app") {
						continue
					}
					switch m.Name {
					case metricPrefix + ".vertical_scaling.constraints.container.cpu.request_min":
						cpuMinFound = true
						assert.Equal(t, 100.0, m.Value, "cpu min should be 100m = 100 millicores")
					case metricPrefix + ".vertical_scaling.constraints.container.memory.request_min":
						memMinFound = true
						assert.Equal(t, float64(128*1024*1024), m.Value, "memory min should be 128Mi in bytes")
					case metricPrefix + ".vertical_scaling.constraints.container.cpu.request_max":
						cpuMaxFound = true
						assert.Equal(t, 2000.0, m.Value, "cpu max should be 2000m = 2000 millicores")
					case metricPrefix + ".vertical_scaling.constraints.container.memory.request_max":
						memMaxFound = true
						assert.Equal(t, float64(1024*1024*1024), m.Value, "memory max should be 1Gi in bytes")
					}
				}
				assert.True(t, cpuMinFound, "cpu.request_min metric not found")
				assert.True(t, memMinFound, "memory.request_min metric not found")
				assert.True(t, cpuMaxFound, "cpu.request_max metric not found")
				assert.True(t, memMaxFound, "memory.request_max metric not found")
			},
		},
		{
			name: "vertical scaling container constraints partial (only cpu min set)",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
							Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
								{
									Name: "sidecar",
									MinAllowed: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse("50m"),
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 6, // 1 constraint metric + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var cpuMinFound bool
				for _, m := range metrics {
					switch m.Name {
					case metricPrefix + ".vertical_scaling.constraints.container.memory.request_min",
						metricPrefix + ".vertical_scaling.constraints.container.cpu.request_max",
						metricPrefix + ".vertical_scaling.constraints.container.memory.request_max":
						t.Errorf("unexpected metric %s: should not be emitted when resource is absent", m.Name)
					case metricPrefix + ".vertical_scaling.constraints.container.cpu.request_min":
						cpuMinFound = true
						assert.Equal(t, 50.0, m.Value)
						assert.Contains(t, m.Tags, "kube_container_name:sidecar")
					}
				}
				assert.True(t, cpuMinFound, "cpu.request_min metric not found")
			},
		},
		{
			name: "vertical scaling container constraints via deprecated Requests field",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
							Containers: []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{
								{
									Name: "app",
									Requests: &datadoghqcommon.DatadogPodAutoscalerContainerResourceConstraints{
										MinAllowed: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
										MaxAllowed: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("2000m"),
											corev1.ResourceMemory: resource.MustParse("1Gi"),
										},
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 9, // 4 container constraints + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var cpuMinFound, memMinFound, cpuMaxFound, memMaxFound bool
				for _, m := range metrics {
					if !slices.Contains(m.Tags, "kube_container_name:app") {
						continue
					}
					switch m.Name {
					case metricPrefix + ".vertical_scaling.constraints.container.cpu.request_min":
						cpuMinFound = true
						assert.Equal(t, 100.0, m.Value)
					case metricPrefix + ".vertical_scaling.constraints.container.memory.request_min":
						memMinFound = true
					case metricPrefix + ".vertical_scaling.constraints.container.cpu.request_max":
						cpuMaxFound = true
						assert.Equal(t, 2000.0, m.Value)
					case metricPrefix + ".vertical_scaling.constraints.container.memory.request_max":
						memMaxFound = true
					}
				}
				assert.True(t, cpuMinFound, "cpu.request_min not found via deprecated Requests field")
				assert.True(t, memMinFound, "memory.request_min not found via deprecated Requests field")
				assert.True(t, cpuMaxFound, "cpu.request_max not found via deprecated Requests field")
				assert.True(t, memMaxFound, "memory.request_max not found via deprecated Requests field")
			},
		},
		{
			name: "horizontal desired replicas from status",
			setupFunc: func() *model.PodAutoscalerInternal {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Spec: datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					Status: datadoghqcommon.DatadogPodAutoscalerStatus{
						Horizontal: &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
							Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
								Replicas: 7,
							},
						},
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace:  "test-ns",
					Name:       "test-dpa",
					UpstreamCR: crd,
				}.Build()
				return &internal
			},
			expectedCount: 6, // status.desired.replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".status.desired.replicas" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 7.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "target_name:test-deployment")
						assert.Contains(t, m.Tags, "autoscaler_name:test-dpa")
					}
				}
				assert.True(t, found, "status.desired.replicas metric not found")
			},
		},
		{
			name: "vertical desired resources from status",
			setupFunc: func() *model.PodAutoscalerInternal {
				crd := &datadoghq.DatadogPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-dpa",
						Namespace: "test-ns",
					},
					Spec: datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					Status: datadoghqcommon.DatadogPodAutoscalerStatus{
						Vertical: &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
							Target: &datadoghqcommon.DatadogPodAutoscalerVerticalTargetStatus{
								DesiredResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
									{
										Name: "app",
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("250m"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("1000m"),
											corev1.ResourceMemory: resource.MustParse("512Mi"),
										},
									},
								},
							},
						},
					},
				}
				internal := model.FakePodAutoscalerInternal{
					Namespace:  "test-ns",
					Name:       "test-dpa",
					UpstreamCR: crd,
				}.Build()
				return &internal
			},
			expectedCount: 9, // 4 vertical desired resources + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var cpuReqFound, memReqFound, cpuLimFound, memLimFound bool
				for _, m := range metrics {
					if !slices.Contains(m.Tags, "kube_container_name:app") {
						continue
					}
					switch m.Name {
					case metricPrefix + ".status.vertical.desired.container.cpu.request":
						cpuReqFound = true
						assert.Equal(t, 250.0, m.Value, "cpu request should be 250m = 250 millicores")
					case metricPrefix + ".status.vertical.desired.container.memory.request":
						memReqFound = true
						assert.Equal(t, float64(256*1024*1024), m.Value, "memory request should be 256Mi in bytes")
					case metricPrefix + ".status.vertical.desired.container.cpu.limit":
						cpuLimFound = true
						assert.Equal(t, 1000.0, m.Value, "cpu limit should be 1000m = 1000 millicores")
					case metricPrefix + ".status.vertical.desired.container.memory.limit":
						memLimFound = true
						assert.Equal(t, float64(512*1024*1024), m.Value, "memory limit should be 512Mi in bytes")
					}
				}
				assert.True(t, cpuReqFound, "status.vertical.desired.container.cpu.request not found")
				assert.True(t, memReqFound, "status.vertical.desired.container.memory.request not found")
				assert.True(t, cpuLimFound, "status.vertical.desired.container.cpu.limit not found")
				assert.True(t, memLimFound, "status.vertical.desired.container.memory.limit not found")
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
			expectedCount: 6, // horizontal_scaling_received_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".local.fallback_enabled" {
						found = true
						assert.Equal(t, 1.0, m.Value, "local fallback should be enabled (1.0)")
					}
				}
				assert.True(t, found, "local.fallback_enabled metric not found")
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

func TestGeneratePodAutoscalerMetrics_NilObject(t *testing.T) {
	metrics := GeneratePodAutoscalerMetrics(nil)
	assert.Nil(t, metrics)
}
