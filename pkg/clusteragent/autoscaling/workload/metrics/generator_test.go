// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package metrics

import (
	"slices"
	"strings"
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

func expectedAdditionalMetricsCount(internal *model.PodAutoscalerInternal) int {
	return expectedApplyModeMetricsCount(internal) + expectedControlledResourcesMetricsCount(internal)
}

func expectedApplyModeMetricsCount(internal *model.PodAutoscalerInternal) int {
	if internal == nil {
		return 0
	}
	count := 0
	if internal.IsHorizontalScalingEnabled() {
		count++
	}
	if internal.IsVerticalScalingEnabled() {
		count++
	}
	return count
}

func expectedControlledResourcesMetricsCount(internal *model.PodAutoscalerInternal) int {
	if internal == nil || internal.Spec() == nil || !internal.IsVerticalScalingEnabled() {
		return 0
	}

	containers := []datadoghqcommon.DatadogPodAutoscalerContainerConstraints{{Name: "*"}}
	if internal.Spec().Constraints != nil && len(internal.Spec().Constraints.Containers) > 0 {
		containers = internal.Spec().Constraints.Containers
	}

	count := 0
	for _, container := range containers {
		if container.Enabled != nil && !*container.Enabled {
			continue
		}
		seenResources := make(map[corev1.ResourceName]struct{})
		for _, resource := range controlledResourcesForMetrics(container.ControlledResources) {
			if _, seen := seenResources[resource]; seen {
				continue
			}
			seenResources[resource] = struct{}{}
			count++
		}
	}
	return count
}

func tagValue(tags []string, key string) string {
	prefix := key + ":"
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimPrefix(tag, prefix)
		}
	}
	return ""
}

func assertControlledResourcesMetrics(t *testing.T, metrics metricsstore.StructuredMetrics, expected map[string]float64) {
	t.Helper()

	actual := map[string]float64{}
	for _, m := range metrics {
		if m.Name != metricPrefix+".vertical_scaling.controlled_resources" {
			continue
		}
		assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
		assert.Empty(t, tagValue(m.Tags, dpaDimensionTagKey))
		container := tagValue(m.Tags, "kube_container_name")
		resourceName := tagValue(m.Tags, resourceNameTagKey)
		actual[container+"/"+resourceName] = m.Value
	}

	assert.Equal(t, expected, actual)
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
			expectedCount: 14, // horizontal_scaling_received_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(patch,eviction,rollout_fallback,pdb_blocked,disruption_throttled,resize_completed)
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
			expectedCount: 17, // 2 requests + 2 limits + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 13, // horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 15, // 2 conditions + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 13, // horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical.rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // horizontal_scaling_applied_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 13, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 13, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 13, // vertical_rollout_triggered(error,ok) + horizontal_scaling_actions(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 15, // local_horizontal_scaling_recommended_replicas + local_horizontal_utilization_pct + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // local_horizontal_scaling_recommended_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			name: "apply mode defaults to apply for enabled dimensions",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; apply_mode count is added by expectedAdditionalMetricsCount
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				dimensions := map[string]bool{}
				for _, m := range metrics {
					if m.Name != metricPrefix+".apply_mode" {
						continue
					}
					assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
					assert.Equal(t, 1.0, m.Value)
					assert.Contains(t, m.Tags, "dpa_mode:apply")
					dimensions[tagValue(m.Tags, "dpa_dimension")] = true
				}
				assert.Equal(t, map[string]bool{
					"horizontal": true,
					"vertical":   true,
				}, dimensions)
			},
		},
		{
			name: "vertical controlled resources default without constraints",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; controlled_resources count is added by expectedAdditionalMetricsCount
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				assertControlledResourcesMetrics(t, metrics, map[string]float64{
					"all/cpu":    1.0,
					"all/memory": 1.0,
				})
			},
		},
		{
			name: "vertical controlled resources default with empty container constraints",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; controlled_resources count is added by expectedAdditionalMetricsCount
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				assertControlledResourcesMetrics(t, metrics, map[string]float64{
					"all/cpu":    1.0,
					"all/memory": 1.0,
				})
			},
		},
		{
			name: "apply mode preview omits disabled vertical dimension",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
							Mode: datadoghq.DatadogPodAutoscalerApplyModePreview,
							Update: &datadoghqcommon.DatadogPodAutoscalerUpdatePolicy{
								Strategy: datadoghqcommon.DatadogPodAutoscalerDisabledUpdateStrategy,
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; apply_mode count is added by expectedAdditionalMetricsCount
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var applyModeMetrics []metricsstore.StructuredMetric
				for _, m := range metrics {
					if m.Name == metricPrefix+".apply_mode" {
						applyModeMetrics = append(applyModeMetrics, m)
					}
				}
				require.Len(t, applyModeMetrics, 1)
				assert.Equal(t, 1.0, applyModeMetrics[0].Value)
				assert.Contains(t, applyModeMetrics[0].Tags, "dpa_mode:preview")
				assert.Contains(t, applyModeMetrics[0].Tags, "dpa_dimension:horizontal")
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
			expectedCount: 15, // horizontal_scaling.constraints.{max,min}_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // horizontal_scaling.constraints.max_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 17, // 4 container constraints + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			name: "vertical controlled resources metrics",
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
									Name:                "app",
									ControlledResources: []corev1.ResourceName{corev1.ResourceCPU},
								},
								{
									Name: "*",
								},
								{
									Name:                "empty",
									ControlledResources: []corev1.ResourceName{},
								},
								{
									Name:                "disabled",
									Enabled:             pointer.Ptr(false),
									ControlledResources: []corev1.ResourceName{corev1.ResourceMemory},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; controlled_resources count is added by expectedAdditionalMetricsCount
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				assertControlledResourcesMetrics(t, metrics, map[string]float64{
					"app/cpu":    1.0,
					"all/cpu":    1.0,
					"all/memory": 1.0,
				})
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
			expectedCount: 14, // 1 constraint metric + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 17, // 4 container constraints + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // status.desired.replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 17, // 4 vertical desired resources + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
			expectedCount: 14, // horizontal_scaling_received_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
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
		{
			name: "in-place vertical scaling action counters",
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
						},
					},
					InPlacePatchSuccessCount:    5,
					InPlacePatchErrorCount:      2,
					InPlaceEvictionSuccessCount: 3,
					InPlaceEvictionErrorCount:   1,
					InPlaceRolloutFallbackCount: 1,
					InPlacePDBBlockedCount:      2,
					InPlaceResizeCompletedCount: 4,
				}.Build()
				return &internal
			},
			expectedCount: 13, // horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				checks := map[string]struct {
					tag   string
					value float64
					found bool
				}{
					"patch_ok":         {tag: "status:ok", value: 5},
					"patch_error":      {tag: "status:error", value: 2},
					"eviction_ok":      {tag: "status:ok", value: 3},
					"eviction_error":   {tag: "status:error", value: 1},
					"rollout_fallback": {value: 1},
					"pdb_blocked":      {value: 2},
					"resize_completed": {value: 4},
				}
				for _, m := range metrics {
					switch m.Name {
					case metricPrefix + ".vertical_inplace.patch":
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Contains(t, m.Tags, "source:Autoscaling")
						if slices.Contains(m.Tags, "status:ok") {
							assert.Equal(t, 5.0, m.Value, "patch ok count")
							c := checks["patch_ok"]
							c.found = true
							checks["patch_ok"] = c
						} else if slices.Contains(m.Tags, "status:error") {
							assert.Equal(t, 2.0, m.Value, "patch error count")
							c := checks["patch_error"]
							c.found = true
							checks["patch_error"] = c
						}
					case metricPrefix + ".vertical_inplace.eviction":
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Contains(t, m.Tags, "source:Autoscaling")
						if slices.Contains(m.Tags, "status:ok") {
							assert.Equal(t, 3.0, m.Value, "eviction ok count")
							c := checks["eviction_ok"]
							c.found = true
							checks["eviction_ok"] = c
						} else if slices.Contains(m.Tags, "status:error") {
							assert.Equal(t, 1.0, m.Value, "eviction error count")
							c := checks["eviction_error"]
							c.found = true
							checks["eviction_error"] = c
						}
					case metricPrefix + ".vertical_inplace.rollout_fallback":
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 1.0, m.Value, "rollout fallback count")
						assert.Contains(t, m.Tags, "source:Autoscaling")
						c := checks["rollout_fallback"]
						c.found = true
						checks["rollout_fallback"] = c
					case metricPrefix + ".vertical_inplace.pdb_blocked":
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 2.0, m.Value, "pdb blocked count")
						assert.Contains(t, m.Tags, "source:Autoscaling")
						c := checks["pdb_blocked"]
						c.found = true
						checks["pdb_blocked"] = c
					case metricPrefix + ".vertical_inplace.resize_completed":
						assert.Equal(t, metricsstore.MetricTypeMonotonicCount, m.Type)
						assert.Equal(t, 4.0, m.Value, "resize completed count")
						assert.Contains(t, m.Tags, "source:Autoscaling")
						c := checks["resize_completed"]
						c.found = true
						checks["resize_completed"] = c
					}
				}
				for key, c := range checks {
					assert.True(t, c.found, "metric not found for key %s", key)
				}
			},
		},
		{
			name: "vertical scaled and evicted replica gauges",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
					},
					ScaledReplicas:  pointer.Ptr(int32(6)),
					EvictedReplicas: pointer.Ptr(int32(2)),
				}.Build()
				return &internal
			},
			expectedCount: 15, // status.vertical.scaled_replicas + status.vertical.evicted_replicas + horizontal_scaling_actions(error,ok) + vertical_rollout_triggered(error,ok) + local.fallback_enabled + vertical_inplace(8)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var scaledFound, evictedFound bool
				for _, m := range metrics {
					switch m.Name {
					case metricPrefix + ".status.vertical.scaled_replicas":
						scaledFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 6.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "autoscaler_name:test-dpa")
					case metricPrefix + ".status.vertical.evicted_replicas":
						evictedFound = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 2.0, m.Value)
						assert.Contains(t, m.Tags, "namespace:test-ns")
						assert.Contains(t, m.Tags, "autoscaler_name:test-dpa")
					}
				}
				assert.True(t, scaledFound, "status.vertical.scaled_replicas metric not found")
				assert.True(t, evictedFound, "status.vertical.evicted_replicas metric not found")
			},
		},
		{
			name: "objective pod resource cpu utilization",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
								PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
									Name: corev1.ResourceCPU,
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
										Utilization: pointer.Ptr(int32(70)),
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 14, // objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".objective.target" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 70.0, m.Value)
						assert.Contains(t, m.Tags, "objective_type:pod_resource")
						assert.Contains(t, m.Tags, "value_type:utilization")
						assert.Contains(t, m.Tags, "resource_name:cpu")
						assert.Contains(t, m.Tags, "objective_index:0")
						for _, tag := range m.Tags {
							assert.False(t, strings.HasPrefix(tag, "kube_container_name:"),
								"pod resource objective should not carry a container tag")
						}
					}
				}
				assert.True(t, found, "objective.target metric not found")
			},
		},
		{
			name: "objective container resource cpu utilization",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
								ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
									Name:      corev1.ResourceCPU,
									Container: "webserver",
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
										Utilization: pointer.Ptr(int32(63)),
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 14, // objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".objective.target" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.Equal(t, 63.0, m.Value)
						assert.Contains(t, m.Tags, "objective_type:container_resource")
						assert.Contains(t, m.Tags, "value_type:utilization")
						assert.Contains(t, m.Tags, "resource_name:cpu")
						assert.Contains(t, m.Tags, "kube_container_name:webserver")
						assert.Contains(t, m.Tags, "objective_index:0")
					}
				}
				assert.True(t, found, "objective.target metric not found")
			},
		},
		{
			name: "objective custom query absolute value",
			setupFunc: func() *model.PodAutoscalerInternal {
				q := resource.MustParse("500M")
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
								CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
										AbsoluteValue: &q,
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 14, // objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".objective.target" {
						found = true
						assert.Equal(t, metricsstore.MetricTypeGauge, m.Type)
						assert.InDelta(t, 5e8, m.Value, 1.0, "500M should be ~5e8 in query-native units")
						assert.Contains(t, m.Tags, "objective_type:custom_query")
						assert.Contains(t, m.Tags, "value_type:absolute_value")
						assert.Contains(t, m.Tags, "objective_index:0")
						for _, tag := range m.Tags {
							assert.False(t, strings.HasPrefix(tag, "resource_name:"),
								"custom query objective should not carry a resource tag")
							assert.False(t, strings.HasPrefix(tag, "kube_container_name:"),
								"custom query objective should not carry a container tag")
						}
					}
				}
				assert.True(t, found, "objective.target metric not found")
			},
		},
		{
			name: "objective container resource cpu absolute value",
			setupFunc: func() *model.PodAutoscalerInternal {
				q := resource.MustParse("500m")
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
								ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
									Name:      corev1.ResourceCPU,
									Container: "app",
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
										AbsoluteValue: &q,
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 14, // objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".objective.target" {
						found = true
						assert.Equal(t, 500.0, m.Value, "cpu absolute 500m should be 500 millicores")
						assert.Contains(t, m.Tags, "objective_type:container_resource")
						assert.Contains(t, m.Tags, "value_type:absolute_value")
						assert.Contains(t, m.Tags, "resource_name:cpu")
						assert.Contains(t, m.Tags, "kube_container_name:app")
						assert.Contains(t, m.Tags, "objective_index:0")
					}
				}
				assert.True(t, found, "objective.target metric not found")
			},
		},
		{
			name: "objective pod resource memory absolute value",
			setupFunc: func() *model.PodAutoscalerInternal {
				q := resource.MustParse("256Mi")
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
								PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
									Name: corev1.ResourceMemory,
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
										AbsoluteValue: &q,
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 14, // objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var found bool
				for _, m := range metrics {
					if m.Name == metricPrefix+".objective.target" {
						found = true
						assert.Equal(t, float64(256*1024*1024), m.Value, "memory absolute 256Mi should be in bytes")
						assert.Contains(t, m.Tags, "objective_type:pod_resource")
						assert.Contains(t, m.Tags, "value_type:absolute_value")
						assert.Contains(t, m.Tags, "resource_name:memory")
						assert.Contains(t, m.Tags, "objective_index:0")
					}
				}
				assert.True(t, found, "objective.target metric not found")
			},
		},
		{
			name: "multiple objectives",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
								PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
									Name: corev1.ResourceCPU,
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
										Utilization: pointer.Ptr(int32(80)),
									},
								},
							},
							{
								Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType,
								ContainerResource: &datadoghqcommon.DatadogPodAutoscalerContainerResourceObjective{
									Name:      corev1.ResourceMemory,
									Container: "sidecar",
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:        datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
										Utilization: pointer.Ptr(int32(55)),
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 15, // 2 objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				var podFound, containerFound bool
				for _, m := range metrics {
					if m.Name != metricPrefix+".objective.target" {
						continue
					}
					if slices.Contains(m.Tags, "objective_type:pod_resource") {
						podFound = true
						assert.Equal(t, 80.0, m.Value)
						assert.Contains(t, m.Tags, "resource_name:cpu")
						assert.Contains(t, m.Tags, "value_type:utilization")
						assert.Contains(t, m.Tags, "objective_index:0")
					}
					if slices.Contains(m.Tags, "objective_type:container_resource") {
						containerFound = true
						assert.Equal(t, 55.0, m.Value)
						assert.Contains(t, m.Tags, "resource_name:memory")
						assert.Contains(t, m.Tags, "kube_container_name:sidecar")
						assert.Contains(t, m.Tags, "objective_index:1")
					}
				}
				assert.True(t, podFound, "pod_resource objective.target not found")
				assert.True(t, containerFound, "container_resource objective.target not found")
			},
		},
		{
			// Multiple custom query objectives share every semantic tag (objective_type,
			// value_type, and no resource/container), so objective_index is the only thing
			// that keeps them as distinct timeseries instead of collapsing into one.
			name: "multiple custom query objectives are disambiguated by index",
			setupFunc: func() *model.PodAutoscalerInternal {
				q0 := resource.MustParse("100M")
				q1 := resource.MustParse("200M")
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
								CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
										AbsoluteValue: &q0,
									},
								},
							},
							{
								Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType,
								CustomQuery: &datadoghqcommon.DatadogPodAutoscalerCustomQueryObjective{
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										Type:          datadoghqcommon.DatadogPodAutoscalerAbsoluteValueObjectiveValueType,
										AbsoluteValue: &q1,
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 15, // 2 objective.target + baseline(13)
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				byIndex := map[string]float64{}
				for _, m := range metrics {
					if m.Name != metricPrefix+".objective.target" {
						continue
					}
					assert.Contains(t, m.Tags, "objective_type:custom_query")
					for _, tag := range m.Tags {
						if strings.HasPrefix(tag, "objective_index:") {
							byIndex[tag] = m.Value
						}
					}
				}
				require.Len(t, byIndex, 2, "each custom query objective should produce a distinct objective_index series")
				assert.InDelta(t, 1e8, byIndex["objective_index:0"], 1.0, "first custom query (100M) at index 0")
				assert.InDelta(t, 2e8, byIndex["objective_index:1"], 1.0, "second custom query (200M) at index 1")
			},
		},
		{
			name: "objective with nil value is not emitted",
			setupFunc: func() *model.PodAutoscalerInternal {
				internal := model.FakePodAutoscalerInternal{
					Namespace: "test-ns",
					Name:      "test-dpa",
					Spec: &datadoghq.DatadogPodAutoscalerSpec{
						TargetRef: v2.CrossVersionObjectReference{
							Name: "test-deployment",
						},
						Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
							{
								Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType,
								PodResource: &datadoghqcommon.DatadogPodAutoscalerPodResourceObjective{
									Name: corev1.ResourceCPU,
									Value: datadoghqcommon.DatadogPodAutoscalerObjectiveValue{
										// Declared Utilization type but pointer left nil.
										Type: datadoghqcommon.DatadogPodAutoscalerUtilizationObjectiveValueType,
									},
								},
							},
						},
					},
				}.Build()
				return &internal
			},
			expectedCount: 13, // baseline only; objective.target not emitted for nil value
			validateMetric: func(t *testing.T, metrics metricsstore.StructuredMetrics) {
				for _, m := range metrics {
					assert.NotEqual(t, metricPrefix+".objective.target", m.Name,
						"objective.target should not be emitted when the value pointer is nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := tt.setupFunc()
			metrics := GeneratePodAutoscalerMetrics(obj)

			require.NotNil(t, metrics)
			assert.Equal(t, tt.expectedCount+expectedAdditionalMetricsCount(obj), len(metrics), "unexpected number of metrics")

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
