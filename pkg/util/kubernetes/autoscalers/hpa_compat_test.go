// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscalers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/dynamicinformer"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/custommetrics"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func newLegacyHPA(apiVersion string, spec, status map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name":      "foo",
			"namespace": "default",
			"uid":       "1234",
			"labels":    map[string]interface{}{"app": "foo"},
		},
		"spec":   spec,
		"status": status,
	}}
}

var (
	scaleTargetRef = map[string]interface{}{"apiVersion": "apps/v1", "kind": "Deployment", "name": "foo"}

	// legacyHPARef is the reference of the external metrics of v2beta2HPA and v2beta1HPA
	legacyHPARef = custommetrics.ObjectReference{Type: "horizontal", Name: "foo", Namespace: "default", UID: "1234"}

	v2beta2HPA = newLegacyHPA("autoscaling/v2beta2",
		map[string]interface{}{
			"scaleTargetRef": scaleTargetRef,
			"minReplicas":    int64(1),
			"maxReplicas":    int64(10),
			"metrics": []interface{}{
				map[string]interface{}{
					"type": "External",
					"external": map[string]interface{}{
						"metric": map[string]interface{}{
							"name":     "requests_per_s",
							"selector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "foo"}},
						},
						"target": map[string]interface{}{"type": "Value", "value": "100"},
					},
				},
				map[string]interface{}{
					"type": "External",
					"external": map[string]interface{}{
						"metric": map[string]interface{}{"name": "queue_length"},
						"target": map[string]interface{}{"type": "AverageValue", "averageValue": "5"},
					},
				},
			},
			"behavior": map[string]interface{}{
				"scaleDown": map[string]interface{}{"stabilizationWindowSeconds": int64(300)},
			},
		},
		map[string]interface{}{
			"currentReplicas": int64(3),
			"desiredReplicas": int64(4),
		},
	)

	v2beta1HPA = newLegacyHPA("autoscaling/v2beta1",
		map[string]interface{}{
			"scaleTargetRef": scaleTargetRef,
			"minReplicas":    int64(1),
			"maxReplicas":    int64(10),
			"metrics": []interface{}{
				map[string]interface{}{
					"type": "External",
					"external": map[string]interface{}{
						"metricName":     "requests_per_s",
						"metricSelector": map[string]interface{}{"matchLabels": map[string]interface{}{"app": "foo"}},
						"targetValue":    "100",
					},
				},
				map[string]interface{}{
					"type": "External",
					"external": map[string]interface{}{
						"metricName":         "queue_length",
						"targetAverageValue": "5",
					},
				},
				map[string]interface{}{
					"type": "Object",
					"object": map[string]interface{}{
						"target":      map[string]interface{}{"apiVersion": "networking.k8s.io/v1", "kind": "Ingress", "name": "main"},
						"metricName":  "requests",
						"targetValue": "2k",
					},
				},
				map[string]interface{}{
					"type": "Pods",
					"pods": map[string]interface{}{
						"metricName":         "packets",
						"selector":           map[string]interface{}{"matchLabels": map[string]interface{}{"verb": "GET"}},
						"targetAverageValue": "1k",
					},
				},
				map[string]interface{}{
					"type": "Resource",
					"resource": map[string]interface{}{
						"name":                     "cpu",
						"targetAverageUtilization": int64(50),
					},
				},
				map[string]interface{}{
					"type": "ContainerResource",
					"containerResource": map[string]interface{}{
						"name":               "memory",
						"container":          "app",
						"targetAverageValue": "100Mi",
					},
				},
			},
		},
		map[string]interface{}{
			"currentReplicas": int64(3),
			"desiredReplicas": int64(4),
			"currentMetrics": []interface{}{
				map[string]interface{}{
					"type": "External",
					"external": map[string]interface{}{
						"metricName":   "requests_per_s",
						"currentValue": "42",
					},
				},
			},
		},
	)
)

func quantity(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func TestToHPAv2(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:      "foo",
		Namespace: "default",
		UID:       "1234",
		Labels:    map[string]string{"app": "foo"},
	}
	crossVersionObjectReference := autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "Deployment", Name: "foo"}
	requestsPerSecond := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{
				Name:     "requests_per_s",
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "foo"}},
			},
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.ValueMetricType, Value: quantity("100")},
		},
	}
	queueLength := autoscalingv2.MetricSpec{
		Type: autoscalingv2.ExternalMetricSourceType,
		External: &autoscalingv2.ExternalMetricSource{
			Metric: autoscalingv2.MetricIdentifier{Name: "queue_length"},
			Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: quantity("5")},
		},
	}

	t.Run("autoscaling/v2", func(t *testing.T) {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: objectMeta}
		got, err := ToHPAv2(hpa)
		require.NoError(t, err)
		assert.Same(t, hpa, got)
	})

	t.Run("autoscaling/v2beta2", func(t *testing.T) {
		got, err := ToHPAv2(v2beta2HPA)
		require.NoError(t, err)
		assert.Equal(t, &autoscalingv2.HorizontalPodAutoscaler{
			TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v2beta2", Kind: "HorizontalPodAutoscaler"},
			ObjectMeta: objectMeta,
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: crossVersionObjectReference,
				MinReplicas:    pointer.Ptr[int32](1),
				MaxReplicas:    10,
				Metrics:        []autoscalingv2.MetricSpec{requestsPerSecond, queueLength},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: pointer.Ptr[int32](300)},
				},
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 3, DesiredReplicas: 4},
		}, got)
	})

	t.Run("autoscaling/v2beta1", func(t *testing.T) {
		got, err := ToHPAv2(v2beta1HPA)
		require.NoError(t, err)
		assert.Equal(t, &autoscalingv2.HorizontalPodAutoscaler{
			TypeMeta:   metav1.TypeMeta{APIVersion: "autoscaling/v2beta1", Kind: "HorizontalPodAutoscaler"},
			ObjectMeta: objectMeta,
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: crossVersionObjectReference,
				MinReplicas:    pointer.Ptr[int32](1),
				MaxReplicas:    10,
				Metrics: []autoscalingv2.MetricSpec{
					requestsPerSecond,
					queueLength,
					{
						Type: autoscalingv2.ObjectMetricSourceType,
						Object: &autoscalingv2.ObjectMetricSource{
							DescribedObject: autoscalingv2.CrossVersionObjectReference{APIVersion: "networking.k8s.io/v1", Kind: "Ingress", Name: "main"},
							Metric:          autoscalingv2.MetricIdentifier{Name: "requests"},
							Target:          autoscalingv2.MetricTarget{Type: autoscalingv2.ValueMetricType, Value: quantity("2k")},
						},
					},
					{
						Type: autoscalingv2.PodsMetricSourceType,
						Pods: &autoscalingv2.PodsMetricSource{
							Metric: autoscalingv2.MetricIdentifier{Name: "packets", Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"verb": "GET"}}},
							Target: autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: quantity("1k")},
						},
					},
					{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name:   corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: pointer.Ptr[int32](50)},
						},
					},
					{
						Type: autoscalingv2.ContainerResourceMetricSourceType,
						ContainerResource: &autoscalingv2.ContainerResourceMetricSource{
							Name:      corev1.ResourceMemory,
							Container: "app",
							Target:    autoscalingv2.MetricTarget{Type: autoscalingv2.AverageValueMetricType, AverageValue: quantity("100Mi")},
						},
					},
				},
			},
			// The current metrics are not converted
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 3, DesiredReplicas: 4},
		}, got)
	})

	t.Run("unsupported version", func(t *testing.T) {
		_, err := ToHPAv2(newLegacyHPA("autoscaling/v1", map[string]interface{}{}, map[string]interface{}{}))
		assert.Error(t, err)
	})

	t.Run("not an HPA", func(t *testing.T) {
		_, err := ToHPAv2(&corev1.Pod{})
		assert.Error(t, err)
	})
}

func TestInspectHPALegacyVersions(t *testing.T) {
	expected := []custommetrics.ExternalMetricValue{
		{MetricName: "requests_per_s", Labels: map[string]string{"app": "foo"}, Ref: legacyHPARef},
		{MetricName: "queue_length", Ref: legacyHPARef},
	}

	for _, hpa := range []*unstructured.Unstructured{v2beta2HPA, v2beta1HPA} {
		t.Run(hpa.GetAPIVersion(), func(t *testing.T) {
			assert.ElementsMatch(t, expected, InspectHPA(hpa))
		})
	}
}

func TestHPAInformer(t *testing.T) {
	informerFactory := informers.NewSharedInformerFactory(fake.NewSimpleClientset(), 0)
	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), 0)

	hpaGVR := func(version string) schema.GroupVersionResource {
		return schema.GroupVersionResource{Group: "autoscaling", Version: version, Resource: "horizontalpodautoscalers"}
	}

	informer, err := HPAInformer(hpaGVR("v2"), informerFactory, nil)
	require.NoError(t, err)
	// Informer() wraps the shared typed informer in a new object on every call,
	// so compare the indexer of the underlying shared informer instead.
	assert.Same(t, informerFactory.Autoscaling().V2().HorizontalPodAutoscalers().Informer().GetIndexer(), informer.Informer().GetIndexer())

	for _, version := range []string{"v2beta2", "v2beta1"} {
		informer, err := HPAInformer(hpaGVR(version), informerFactory, dynamicInformerFactory)
		require.NoError(t, err)
		assert.Same(t, dynamicInformerFactory.ForResource(hpaGVR(version)).Informer(), informer.Informer())

		_, err = HPAInformer(hpaGVR(version), informerFactory, nil)
		assert.Error(t, err, "a dynamic informer factory is required for %s", version)
	}
}
