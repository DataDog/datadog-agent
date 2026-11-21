// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package customresources

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	generator "k8s.io/kube-state-metrics/v2/pkg/metric_generator"
)

func TestExtendedPodFactoryEffectiveResourceRequestsMetric(t *testing.T) {
	factory := &extendedPodFactory{}
	generators := factory.MetricFamilyGenerators()
	effectiveRequestsGeneratorIndex := -1
	for i := range generators {
		if generators[i].Name == "kube_pod_container_effective_resource_requests" {
			effectiveRequestsGeneratorIndex = i
			break
		}
	}
	require.NotEqual(t, -1, effectiveRequestsGeneratorIndex)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       types.UID("test-uid"),
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Containers: []corev1.Container{
				{
					Name: "resized",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:             resource.MustParse("500m"),
						corev1.ResourceMemory:          resource.MustParse("1Gi"),
						corev1.ResourceName("example"): resource.MustParse("1"),
					}},
				},
				{
					Name: "partially-reported",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("300m"),
						corev1.ResourceMemory: resource.MustParse("768Mi"),
					}},
				},
				{
					Name: "legacy",
					Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					}},
				},
			},
		},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name: "resized",
					Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					}},
				},
				{
					Name: "partially-reported",
					Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("150m"),
					}},
				},
			},
		},
	}

	family := generators[effectiveRequestsGeneratorIndex].Generate(pod)
	require.Len(t, family.Metrics, 6)

	actual := make(map[string]float64, len(family.Metrics))
	for _, generatedMetric := range family.Metrics {
		assert.Equal(t, []string{"namespace", "pod", "uid", "container", "node", "resource", "unit"}, generatedMetric.LabelKeys)
		assert.Equal(t, []string{"test-namespace", "test-pod", "test-uid"}, generatedMetric.LabelValues[:3])
		assert.Equal(t, "test-node", generatedMetric.LabelValues[4])
		actual[generatedMetric.LabelValues[3]+"/"+generatedMetric.LabelValues[5]] = generatedMetric.Value
	}

	assert.Equal(t, map[string]float64{
		"resized/cpu":               0.2,
		"resized/memory":            512 * 1024 * 1024,
		"partially-reported/cpu":    0.15,
		"partially-reported/memory": 768 * 1024 * 1024,
		"legacy/cpu":                0.1,
		"legacy/memory":             256 * 1024 * 1024,
	}, actual)

	assert.Equal(t, resource.MustParse("500m"), pod.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU])
}

func TestExtendedPodFactoryUnreadyMetric(t *testing.T) {
	factory := &extendedPodFactory{}
	generators := factory.MetricFamilyGenerators()
	unreadyGeneratorIndex := slices.IndexFunc(generators, func(g generator.FamilyGenerator) bool {
		return g.Name == "kube_pod_status_unready"
	})
	require.NotEqual(t, -1, unreadyGeneratorIndex)

	readyCondition := func(status corev1.ConditionStatus) []corev1.PodCondition {
		return []corev1.PodCondition{
			{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			{Type: corev1.PodReady, Status: status},
		}
	}

	tests := []struct {
		name     string
		phase    corev1.PodPhase
		conds    []corev1.PodCondition
		expected bool
	}{
		{name: "ready", phase: corev1.PodRunning, conds: readyCondition(corev1.ConditionTrue)},
		{name: "ready condition false", phase: corev1.PodRunning, conds: readyCondition(corev1.ConditionFalse), expected: true},
		{name: "ready condition unknown", phase: corev1.PodRunning, conds: readyCondition(corev1.ConditionUnknown), expected: true},
		{name: "no ready condition yet", phase: corev1.PodPending, conds: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}}, expected: true},
		{name: "no conditions at all", phase: corev1.PodPending, expected: true},
		{name: "succeeded pod is never ready", phase: corev1.PodSucceeded, conds: readyCondition(corev1.ConditionFalse)},
		{name: "failed pod is never ready", phase: corev1.PodFailed, conds: readyCondition(corev1.ConditionFalse)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "test-namespace",
					UID:       types.UID("test-uid"),
				},
				Status: corev1.PodStatus{Phase: tt.phase, Conditions: tt.conds},
			}

			family := generators[unreadyGeneratorIndex].Generate(pod)
			if !tt.expected {
				assert.Empty(t, family.Metrics)
				return
			}

			require.Len(t, family.Metrics, 1)
			assert.Equal(t, float64(1), family.Metrics[0].Value)
			assert.Equal(t, []string{"namespace", "pod", "uid"}, family.Metrics[0].LabelKeys)
			assert.Equal(t, []string{"test-namespace", "test-pod", "test-uid"}, family.Metrics[0].LabelValues)
		})
	}
}
